package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(func(string) string { return "" })
	if err != nil {
		t.Fatalf("Load with empty env: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr default = %q, want :8080", cfg.Addr)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env default = %q, want dev", cfg.Env)
	}
	if cfg.DatabaseURL == "" {
		t.Error("DatabaseURL should have a dev default")
	}
	if cfg.JWTSecret == "" {
		t.Error("JWTSecret should have a dev default")
	}
}

func TestLoadOverrides(t *testing.T) {
	env := map[string]string{
		"SP_ADDR":         ":9000",
		"SP_ENV":          "test",
		"SP_DATABASE_URL": "postgres://u:p@h:5432/db",
		"SP_JWT_SECRET":   "s3cret-value",
	}
	cfg, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":9000" || cfg.Env != "test" || cfg.DatabaseURL != "postgres://u:p@h:5432/db" || cfg.JWTSecret != "s3cret-value" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadRejectsProductionWithoutSecret(t *testing.T) {
	env := map[string]string{"SP_ENV": "production"}
	_, err := Load(func(k string) string { return env[k] })
	if err == nil {
		t.Fatal("expected error when production lacks SP_JWT_SECRET")
	}
	if !strings.Contains(err.Error(), "SP_JWT_SECRET") {
		t.Errorf("error should mention SP_JWT_SECRET, got %v", err)
	}
}
