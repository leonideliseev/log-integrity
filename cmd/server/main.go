// Package main starts the log monitoring server.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lenchik/logmonitor/config"
	"github.com/lenchik/logmonitor/internal/app"
)

// main loads config, builds the application and runs it until shutdown signal arrives.
func main() {
	configPath := flag.String("config", "config.yaml", "Path to YAML config")
	flag.Parse()

	cfg, err := config.LoadRuntime(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("create app: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		log.Fatalf("run app: %v", err)
	}
}
