package httpapp

import (
	"testing"

	"github.com/lenchik/logmonitor/config"
)

func TestEffectiveAPIAuthTokenReturnsConfiguredToken(t *testing.T) {
	cfg := &config.Config{}
	cfg.API.AuthToken = "secret"

	if got := effectiveAPIAuthToken(cfg); got != "secret" {
		t.Fatalf("expected configured API token, got %q", got)
	}
}

func TestEffectiveAPIAuthTokenDisablesAuthWhenExplicitlyAllowed(t *testing.T) {
	cfg := &config.Config{}
	cfg.API.AuthToken = "secret"
	cfg.API.AllowUnauthenticated = true

	if got := effectiveAPIAuthToken(cfg); got != "" {
		t.Fatalf("expected empty token when unauthenticated API is allowed, got %q", got)
	}
}
