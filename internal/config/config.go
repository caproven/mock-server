package config

import (
	"errors"
	"fmt"
	"os"

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
}

type WeightedResponse struct {
	Weight   int      `yaml:"weight"`
	Response Response `yaml:"response"`
}

type Response struct {
	StatusCode int               `yaml:"status"`
	Headers    map[string]string `yaml:"headers"`
	Body       ResponseBody      `yaml:"body"`
}

type ResponseBody struct {
	Literal  string `yaml:"literal"`
	FilePath string `yaml:"filePath"`
}

func (c Config) RestEndpoints() ([]*rest.Endpoint, error) {
	var endpoints []*rest.Endpoint

	for _, endpointCfg := range c.Endpoints {
		strategy := endpointCfg.ResponseStrategy
		if strategy.Static != nil && strategy.Weighted != nil {
			return nil, fmt.Errorf("multiple response strategies for endpoint %q", endpointCfg.Path)
		}

		var resolver rest.ResponseResolver
		if strategy.Static != nil {
			resp, err := strategy.Static.toRest()
			if err != nil {
				return nil, fmt.Errorf("build response for endpoint %q: %w", endpointCfg.Path, err)
			}
			resolver = rest.StaticResponse(resp)
		} else if strategy.Weighted != nil {
			resp, err := convertWeightedToRest(strategy.Weighted)
			if err != nil {
				return nil, fmt.Errorf("build weighted response for endpoint %q: %w", endpointCfg.Path, err)
			}
			resolver = resp
		}

		if resolver == nil {
			return nil, fmt.Errorf("no response strategy for endpoint %q", endpointCfg.Path)
		}

		endpoint := rest.NewEndpoint(endpointCfg.Path, endpointCfg.Method, resolver)
		endpoints = append(endpoints, endpoint)
	}

	return endpoints, nil
}

func (r Response) toRest() (rest.Response, error) {
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
