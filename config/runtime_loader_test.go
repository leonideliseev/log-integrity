package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lenchik/logmonitor/pkg/appmode"
)

func TestLoadRuntimeForModeAllowsHTTPWithoutAuthToken(t *testing.T) {
	path := writeRuntimeConfig(t, `
api:
  auth_token: ""
security:
  integrity_hmac_key: "1234567890123456"
ssh:
  insecure_ignore_host_key: true
`)

	if _, err := LoadRuntimeForMode(path, appmode.HTTP); err != nil {
		t.Fatalf("expected HTTP config without auth token to load: %v", err)
	}
}

func TestLoadRuntimeForModeAllowsExplicitUnauthenticatedHTTP(t *testing.T) {
	path := writeRuntimeConfig(t, `
api:
  auth_token: ""
  allow_unauthenticated: true
security:
  integrity_hmac_key: "1234567890123456"
ssh:
  insecure_ignore_host_key: true
`)

	if _, err := LoadRuntimeForMode(path, appmode.HTTP); err != nil {
		t.Fatalf("expected explicit unauthenticated config to load: %v", err)
	}
}

func TestLoadRuntimeForModeDefaultsToNotStoringRawContent(t *testing.T) {
	path := writeRuntimeConfig(t, `
api:
  allow_unauthenticated: true
security:
  integrity_hmac_key: "1234567890123456"
ssh:
  insecure_ignore_host_key: true
`)

	cfg, err := LoadRuntimeForMode(path, appmode.HTTP)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Collector.StoreRawContent == nil {
		t.Fatal("expected store_raw_content default to be applied")
	}
	if *cfg.Collector.StoreRawContent {
		t.Fatal("expected raw log content storage to be disabled by default")
	}
}

func writeRuntimeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
