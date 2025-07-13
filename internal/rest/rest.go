package rest

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"
)

type ResponseResolver interface {
	NextResponse() Response
	// TODO consider adding "StrategyName" func or similar so we can include in logs when registering
}

type StaticResponse Response

func (r StaticResponse) NextResponse() Response {
	return Response(r)
}

type numberGenerator interface {
	// N returns an integer in the half-open interval [0, n).
	N(n int) int
}

type rng struct{}

func (r rng) N(n int) int {
	return rand.N(n)
}

type WeightedResponse struct {
	numGenerator numberGenerator
	responses    []Response
	weights      []int
	weightTotal  int
}

type WeightedResponseEntry struct {
	Response Response
	Weight   int
}

// NewWeightedResponse builds a weighted response strategy from the given responses.
// If numGenerator is nil, a random source is used.
func NewWeightedResponse(entries []WeightedResponseEntry, numGenerator numberGenerator) (*WeightedResponse, error) {
	// entries imo makes more sense as a map, but switched to a slice so internal ordering is deterministic
	if len(entries) == 0 {
		return nil, errors.New("no weighted responses")
	}

	if numGenerator == nil {
		numGenerator = rng{}
	}

	var weightTotal int
	var responses []Response
	var weights []int

	for _, entry := range entries {
		if entry.Weight <= 0 {
			return nil, errors.New("weight must be >= 1")
		}
		weightTotal += entry.Weight
		weights = append(weights, weightTotal)
		responses = append(responses, entry.Response)
	}

	return &WeightedResponse{
		numGenerator: numGenerator,
		responses:    responses,
		weights:      weights,
		weightTotal:  weightTotal,
	}, nil
}

func (w *WeightedResponse) NextResponse() Response {
	val := w.numGenerator.N(w.weightTotal)

	for i, weight := range w.weights {
		if val < weight {
			return w.responses[i]
		}
	}

	// At present, it doesn't make sense for NextResponse implementations to fail as there
	// should always be a response to return. Opting to panic when invariant is broken,
	// but may switch to returning error in the future.
	panic("number generator should always return a valid weight")
}

type SequenceBehavior string

const (
	SequenceBehaviorLoop       SequenceBehavior = "loop"
	SequenceBehaviorRepeatLast SequenceBehavior = "repeatLast"
)

type SequencedResponse struct {
	endBehavior SequenceBehavior
	sequence    []Response

	idx int
	mu  sync.Mutex
}

func NewSequencedResponse(endBehavior SequenceBehavior, sequence []Response) (*SequencedResponse, error) {
	switch endBehavior {
	case SequenceBehaviorLoop, SequenceBehaviorRepeatLast:
	default:
		return nil, fmt.Errorf("unknown sequence end behavior %q", endBehavior)
	}
	if len(sequence) == 0 {
		return nil, errors.New("no sequence responses")
	}

	sequencedResp := &SequencedResponse{
		endBehavior: endBehavior,
		sequence:    sequence,
	}
	return sequencedResp, nil
}

func (s *SequencedResponse) NextResponse() Response {
	s.mu.Lock()
	defer s.mu.Unlock()

	resp := s.sequence[s.idx]
	if s.idx < len(s.sequence)-1 { // have remaining sequence
		s.idx++
	} else if s.idx >= len(s.sequence)-1 && s.endBehavior == SequenceBehaviorLoop {
		s.idx++
		s.idx %= len(s.sequence)
	}

	return resp
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

type ResponseOption func(*Response) error

type Response struct {
	headers    map[string]string
	body       []byte
	statusCode int
	delay      time.Duration
}

func WithResponseHeaders(headers map[string]string) ResponseOption {
	return func(r *Response) error {
		r.headers = headers
		return nil
	}
}

func WithResponseBody(body []byte) ResponseOption {
	return func(r *Response) error {
		r.body = body
		return nil
	}
}

func WithResponseStatus(statusCode int) ResponseOption {
	return func(r *Response) error {
		if statusCode < 100 || statusCode > 599 {
			return fmt.Errorf("invalid status code: %d", statusCode)
		}
		r.statusCode = statusCode
		return nil
	}
}

func WithResponseDelay(delay time.Duration) ResponseOption {
	return func(r *Response) error {
		if delay < 0 {
			return errors.New("delay cannot be negative")
		}
		r.delay = delay
		return nil
	}
}

func NewResponse(opts ...ResponseOption) (Response, error) {
	var resp Response

	for _, opt := range opts {
		if err := opt(&resp); err != nil {
			return Response{}, fmt.Errorf("apply response option: %w", err)
		}
	}

	if resp.statusCode == 0 {
		resp.statusCode = http.StatusOK
	}

	return resp, nil
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

			if resp.delay != 0 {
				time.Sleep(resp.delay)
			}

			for header, val := range resp.headers {
				w.Header().Set(header, val)
			}
			w.WriteHeader(resp.statusCode)
			if _, err := w.Write(resp.body); err != nil {
				slog.Warn("failed to write response", "err", err)
				return
			}
		})
	}
}
