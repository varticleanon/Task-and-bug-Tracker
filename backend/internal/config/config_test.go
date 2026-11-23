package config

import (
	"os"
	"testing"
)

func TestConfigLoadAndValidate(t *testing.T) {
	os.Setenv("APP_ENV", "development")
	os.Setenv("HTTP_ADDR", ":8080")
	defer func() {
		os.Unsetenv("APP_ENV")
		os.Unsetenv("HTTP_ADDR")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error loading config, got %v", err)
	}

	if cfg.AppEnv != "development" {
		t.Errorf("expected APP_ENV development, got %s", cfg.AppEnv)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Errorf("expected HTTP_ADDR :8080, got %s", cfg.HTTPAddr)
	}
}

func TestConfigValidationInvalidEnv(t *testing.T) {
	cfg := &Config{
		AppEnv:         "invalid_env",
		HTTPAddr:       ":8080",
		UploadMaxBytes: 100,
	}

	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid APP_ENV, got nil")
	}
}
