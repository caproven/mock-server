package rest

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
)

type ResponseResolver interface {
	NextResponse() Response
	// TODO consider adding "StrategyName" func or similar so we can include in logs when registering
}

type StaticResponse Response

func (r StaticResponse) NextResponse() Response {
	return Response(r)
}

type WeightedResponse struct {
	responses   []Response
	weights     []int
	weightTotal int
}

func NewWeightedResponse(weightedResponses map[*Response]int) (*WeightedResponse, error) {
	if len(weightedResponses) == 0 {
		return nil, errors.New("no weighted responses")
	}

	var weightTotal int
	var responses []Response
	var weights []int

	for resp, weight := range weightedResponses {
		if weight <= 0 {
			return nil, errors.New("weight must be >= 1")
		}
		weightTotal += weight
		weights = append(weights, weightTotal)
		responses = append(responses, *resp)
	}

	return &WeightedResponse{
		responses:   responses,
		weights:     weights,
		weightTotal: weightTotal,
	}, nil
}

func (w *WeightedResponse) NextResponse() Response {
	val := rand.N(w.weightTotal)

	for i, weight := range w.weights {
		if val < weight {
			return w.responses[i]
		}
	}
	// TODO how to test this? This shouldn't be able to fail but loop may end

	slog.Warn("didn't find a weighted response")
	return Response{}
}

type Endpoint struct {
	Path             string
	Method           string
	responseResolver ResponseResolver
}

func NewEndpoint(path, method string, respResolver ResponseResolver) *Endpoint {
	return &Endpoint{
		Path:             path,
		Method:           method,
		responseResolver: respResolver,
	}
}

// Response yields the next response that should be returned when the endpoint is hit.
func (p *Endpoint) Response() Response {
	return p.responseResolver.NextResponse()
}

type Response struct {
	Headers    map[string]string
	Body       []byte
	StatusCode int
}

type httpMux interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// RegisterHandlers registers endpoint handlers to the given HTTP mux.
func RegisterHandlers(mux httpMux, endpoints []*Endpoint) {
	for _, endpoint := range endpoints {
		slog.Info("registering endpoint", "method", endpoint.Method, "path", endpoint.Path)
		pattern := endpoint.Path
		if endpoint.Method != "" {
			pattern = fmt.Sprintf("%s %s", endpoint.Method, pattern)
		}
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			slog.Info("handling request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("addr", r.RemoteAddr),
			)

			resp := endpoint.Response()

			w.WriteHeader(resp.StatusCode)
			for header, val := range resp.Headers {
				w.Header().Set(header, val)
			}
			if _, err := w.Write(resp.Body); err != nil {
				slog.Warn("failed to write response", "err", err)
				return
			}
		})
	}
}
