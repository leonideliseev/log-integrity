// Package cliapp wires the standalone CLI runtime without HTTP dependencies.
package cliapp

import (
	"context"
	"fmt"

	"github.com/lenchik/logmonitor/config"
	generalapp "github.com/lenchik/logmonitor/internal/app/general"
	"github.com/lenchik/logmonitor/internal/runtimeinfo"
	"github.com/lenchik/logmonitor/pkg/appmode"
)

// LoadConfig loads CLI runtime configuration with mode-aware validation.
func LoadConfig(path string) (*config.Config, error) {
	cfg, err := config.LoadRuntimeForMode(path, appmode.CLI)
	if err != nil {
		return nil, fmt.Errorf("cli app: load config: %w", err)
	}
	return cfg, nil
}

// NewRuntime builds the standalone CLI runtime from the provided configuration.
func NewRuntime(cfg *config.Config) (*generalapp.Runtime, error) {
	runtime, err := generalapp.NewRuntime(cfg)
	if err != nil {
		return nil, fmt.Errorf("cli app: build runtime: %w", err)
	}
	return runtime, nil
}

// NewRuntimeFromPath loads config and builds the standalone CLI runtime in one step.
func NewRuntimeFromPath(path string) (*generalapp.Runtime, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	return NewRuntime(cfg)
}

// LoadSnapshotFromPath loads config and builds a non-mutating runtime snapshot for diagnostic commands.
func LoadSnapshotFromPath(path string) (runtimeinfo.Snapshot, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return runtimeinfo.Snapshot{}, err
	}
	return generalapp.BuildRuntimeSnapshot(cfg), nil
}

// ProbeReadinessFromPath loads config and performs lightweight readiness checks for diagnostic commands.
func ProbeReadinessFromPath(ctx context.Context, path string) (runtimeinfo.Readiness, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return runtimeinfo.Readiness{}, err
	}
	return generalapp.ProbeReadiness(ctx, cfg), nil
}
