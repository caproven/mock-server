package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/lmittmann/tint"

	"github.com/caproven/mock-server/internal/rest"
)

type Config struct {
	Endpoints []EndpointConfig `json:"endpoints"`
}

type EndpointConfig struct {
	Path     string         `yaml:"path"`
	Method   string         `yaml:"method"`
	Response ResponseConfig `yaml:"response"`
}

type ResponseConfig struct {
	StatusCode int                `yaml:"status"`
	Headers    map[string]string  `yaml:"headers"`
	Body       ResponseBodyConfig `yaml:"body"`
}

type ResponseBodyConfig struct {
	Literal  string `yaml:"literal"`
	FilePath string `yaml:"filePath"`
}

func main() {
	slog.SetDefault(slog.New(tint.NewHandler(os.Stdout, &tint.Options{
		AddSource: true,
	})))

	configFilePath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	config, err := readConfig(*configFilePath)
	if err != nil {
		slog.Error("failed to read config", "err", err)
		os.Exit(1)
	}

	endpoints, err := endpointsFromConfig(config)
	if err != nil {
		slog.Error("failed to parse endpoints from config", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	rest.RegisterHandlers(mux, endpoints)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	slog.Info("starting server", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func readConfig(filePath string) (Config, error) {
	configFile, err := os.Open(filePath)
	if err != nil {
		return Config{}, fmt.Errorf("open config file: %w", err)
	}
	defer func(file io.Closer) {
		if err := file.Close(); err != nil {
			slog.Error("failed to close config file", "err", err)
		}
	}(configFile)

	var config Config
	if err := yaml.NewDecoder(configFile).Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode config file: %w", err)
	}

	return config, nil
}

func endpointsFromConfig(config Config) ([]rest.Endpoint, error) {
	var endpoints []rest.Endpoint
	for _, endpoint := range config.Endpoints {
		resp := endpoint.Response
		if resp.Body.Literal != "" && resp.Body.FilePath != "" {
			return nil, fmt.Errorf("response body for path %q cannot use both literal and path", endpoint.Path)
		}

		respBody := []byte(resp.Body.Literal)
		if resp.Body.FilePath != "" {
			data, err := os.ReadFile(resp.Body.FilePath)
			if err != nil {
				return nil, fmt.Errorf("read file %q: %w", resp.Body.FilePath, err)
			}
			respBody = data
		}

		endpoints = append(endpoints, rest.Endpoint{
			Path:   endpoint.Path,
			Method: endpoint.Method,
			Response: rest.Response{
				StatusCode: resp.StatusCode,
				Headers:    resp.Headers,
				Body:       respBody,
			},
		})
	}

	return endpoints, nil
}
