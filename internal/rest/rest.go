package rest

import (
	"fmt"
	"log/slog"
	"net/http"
)

type Endpoint struct {
	Path     string
	Method   string
	Response Response
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
func RegisterHandlers(mux httpMux, endpoints []Endpoint) {
	for _, endpoint := range endpoints {
		slog.Info("registering endpoint", "method", endpoint.Method, "path", endpoint.Path)
		pattern := fmt.Sprintf("%s %s", endpoint.Method, endpoint.Path)
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			slog.Info("handling request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("addr", r.RemoteAddr),
			)
			w.WriteHeader(endpoint.Response.StatusCode)
			for header, val := range endpoint.Response.Headers {
				w.Header().Set(header, val)
			}
			if _, err := w.Write(endpoint.Response.Body); err != nil {
				slog.Error("failed to write response", "err", err)
				return
			}
		})
	}
}
