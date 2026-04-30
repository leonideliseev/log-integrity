package httpapp

import (
	"testing"

	"github.com/lenchik/logmonitor/config"
)

func TestEffectiveAPIAuthTokenAlwaysDisablesAuth(t *testing.T) {
	cfg := &config.Config{}
	cfg.API.AuthToken = "secret"

	if got := effectiveAPIAuthToken(cfg); got != "" {
		t.Fatalf("expected auth to be disabled, got token %q", got)
	}
}

func TestEffectiveAPIAuthTokenIgnoresAllowUnauthenticatedFlag(t *testing.T) {
	cfg := &config.Config{}
	cfg.API.AuthToken = "secret"
	cfg.API.AllowUnauthenticated = true

	if got := effectiveAPIAuthToken(cfg); got != "" {
		t.Fatalf("expected auth to stay disabled, got token %q", got)
	}
}
