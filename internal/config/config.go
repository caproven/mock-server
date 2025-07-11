package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/caproven/mock-server/internal/rest"
)

type Config struct {
	Endpoints []Endpoint `json:"endpoints"`
}

type Endpoint struct {
	Path             string           `yaml:"path"`
	Method           string           `yaml:"method"`
	ResponseStrategy ResponseStrategy `yaml:"response"`
}

type ResponseStrategy struct {
	Static   *Response          `yaml:"static"`
	Weighted []WeightedResponse `yaml:"weighted"`
	Sequence *SequencedResponse `yaml:"sequence"`
}

type WeightedResponse struct {
	Weight   int      `yaml:"weight"`
	Response Response `yaml:"response"`
}

type SequencedResponse struct {
	EndBehavior string                   `yaml:"endBehavior"`
	Responses   []SequencedResponseEntry `yaml:"responses"`
}

type SequencedResponseEntry struct {
	Count    *int     `yaml:"count"`
	Response Response `yaml:"response"`
}

type Response struct {
	StatusCode int               `yaml:"status"`
	Headers    map[string]string `yaml:"headers"`
	Body       ResponseBody      `yaml:"body"`
	Delay      time.Duration     `yaml:"delay"`
}

type ResponseBody struct {
	Literal  string `yaml:"literal"`
	FilePath string `yaml:"filePath"`
}

func (c Config) RestEndpoints() ([]*rest.Endpoint, error) {
	var endpoints []*rest.Endpoint

	for _, endpointCfg := range c.Endpoints {
		strategy := endpointCfg.ResponseStrategy

		var resolver rest.ResponseResolver
		var strategyCount int
		if strategy.Static != nil {
			strategyCount++
			resp, err := strategy.Static.toRest()
			if err != nil {
				return nil, fmt.Errorf("build response for endpoint %q: %w", endpointCfg.Path, err)
			}
			resolver = rest.StaticResponse(resp)
		}
		if strategy.Weighted != nil {
			strategyCount++
			resp, err := convertWeightedToRest(strategy.Weighted)
			if err != nil {
				return nil, fmt.Errorf("build weighted response for endpoint %q: %w", endpointCfg.Path, err)
			}
			resolver = resp
		}
		if strategy.Sequence != nil {
			strategyCount++
			resp, err := convertSequencedToRest(strategy.Sequence)
			if err != nil {
				return nil, fmt.Errorf("build sequenced response for endpoint %q: %w", endpointCfg.Path, err)
			}
			resolver = resp
		}

		if resolver == nil || strategyCount != 1 {
			return nil, fmt.Errorf("endpoint %q must have exactly one response strategy but had %d", endpointCfg.Path, strategyCount)
		}

		endpoint := rest.NewEndpoint(endpointCfg.Path, endpointCfg.Method, resolver)
		endpoints = append(endpoints, endpoint)
	}

	return endpoints, nil
}

func (r Response) toRest() (rest.Response, error) {
	if r.Delay < 0 {
		return rest.Response{}, errors.New("response delay cannot be negative")
	}

	if r.Body.Literal != "" && r.Body.FilePath != "" {
		return rest.Response{}, errors.New("response body cannot use both literal and path")
	}

	respBody := []byte(r.Body.Literal)
	if r.Body.FilePath != "" {
		data, err := os.ReadFile(r.Body.FilePath)
		if err != nil {
			return rest.Response{}, fmt.Errorf("read file %q: %w", r.Body.FilePath, err)
		}
		respBody = data
	}

	resp := rest.Response{
		StatusCode: r.StatusCode,
		Headers:    r.Headers,
		Body:       respBody,
		Delay:      r.Delay,
	}

	return resp, nil
}

func convertWeightedToRest(weighted []WeightedResponse) (*rest.WeightedResponse, error) {
	weightedMap := make(map[*rest.Response]int)

	for _, weightedRespCfg := range weighted {
		resp, err := weightedRespCfg.Response.toRest()
		if err != nil {
			return nil, fmt.Errorf("build weighted response: %w", err)
		}
		weightedMap[&resp] = weightedRespCfg.Weight
	}

	return rest.NewWeightedResponse(weightedMap)
}

func convertSequencedToRest(sequencedResp *SequencedResponse) (*rest.SequencedResponse, error) {
	var sequence []rest.Response

	for _, respEntry := range sequencedResp.Responses {
		count := 1
		if respEntry.Count != nil {
			if *respEntry.Count <= 0 {
				return nil, fmt.Errorf("sequence response count must be >= 1: %d", *respEntry.Count)
			}
			count = *respEntry.Count
		}

		resp, err := respEntry.Response.toRest()
		if err != nil {
			return nil, fmt.Errorf("build sequence response: %w", err)
		}

		for range count {
			sequence = append(sequence, resp)
		}
	}

	endBehavior := rest.SequenceEndBehaviorRepeatLast
	if sequencedResp.EndBehavior != "" {
		endBehavior = rest.SequenceEndBehavior(sequencedResp.EndBehavior)
	}
	return rest.NewSequencedResponse(endBehavior, sequence)
}
