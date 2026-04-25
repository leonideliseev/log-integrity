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
	httpapp "github.com/lenchik/logmonitor/internal/app/http"
	"github.com/lenchik/logmonitor/pkg/appmode"
)

// main loads config, builds the application and runs it until shutdown signal arrives.
func main() {
	configPath := flag.String("config", "config.yaml", "Path to YAML config")
	flag.Parse()

	if _, err := appmode.Require(appmode.HTTP); err != nil {
		log.Fatalf("startup mode: %v", err)
	}

	cfg, err := config.LoadRuntimeForMode(*configPath, appmode.HTTP)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	application, err := httpapp.New(cfg)
	if err != nil {
		log.Fatalf("create app: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		log.Fatalf("run app: %v", err)
	}
}
