package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/caproven/mock-server/internal/config"
	"github.com/caproven/mock-server/internal/rest"
	"github.com/goccy/go-yaml"
	"github.com/lmittmann/tint"
)

func main() {
	slog.SetDefault(slog.New(tint.NewHandler(os.Stdout, &tint.Options{
		AddSource: true,
	})))

	configFilePath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := readConfig(*configFilePath)
	if err != nil {
		slog.Error("failed to read config", "err", err)
		os.Exit(1)
	}

	endpoints, err := cfg.RestEndpoints()
	if err != nil {
		slog.Error("failed to build rest endpoints", "err", err)
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

func readConfig(filePath string) (config.Config, error) {
	configFile, err := os.Open(filePath)
	if err != nil {
		return config.Config{}, fmt.Errorf("open config file: %w", err)
	}
	defer func(file io.Closer) {
		if err := file.Close(); err != nil {
			slog.Error("failed to close config file", "err", err)
		}
	}(configFile)

	var cfg config.Config
	if err := yaml.NewDecoder(configFile).Decode(&cfg); err != nil {
		return config.Config{}, fmt.Errorf("decode config file: %w", err)
	}

	return cfg, nil
}
