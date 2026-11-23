package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv          string
	HTTPAddr        string
	DatabaseURL     string
	CSRFSecret      string
	CSRFHMACSecret  string
	UploadDir       string
	UploadMaxBytes  int64
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

func (c *Config) Validate() error {
	if c.HTTPAddr == "" {
		return errors.New("HTTP_ADDR cannot be empty")
	}
	validEnvs := map[string]bool{
		"development": true,
		"dev":         true,
		"production":  true,
		"prod":        true,
		"testing":     true,
		"test":        true,
	}
	if !validEnvs[c.AppEnv] {
		return errors.New("invalid APP_ENV: " + c.AppEnv)
	}
	return nil
}

func Load() (*Config, error) {
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}

	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	dbURL := os.Getenv("DATABASE_URL")

	csrfSecret := os.Getenv("CSRF_HMAC_SECRET")
	if csrfSecret == "" {
		csrfSecret = os.Getenv("CSRF_SECRET")
	}
	if csrfSecret == "" {
		csrfSecret = "super-secret-key-for-dev-only"
	}

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "./uploads"
	}

	maxBytesStr := os.Getenv("UPLOAD_MAX_BYTES")
	maxBytes, err := strconv.ParseInt(maxBytesStr, 10, 64)
	if err != nil || maxBytes <= 0 {
		maxBytes = 5242880 // Default 5 MiB (SSOT §5.5)
	}

	cfg := &Config{
		AppEnv:          appEnv,
		HTTPAddr:        httpAddr,
		DatabaseURL:     dbURL,
		CSRFSecret:      csrfSecret,
		CSRFHMACSecret:  csrfSecret,
		UploadDir:       uploadDir,
		UploadMaxBytes:  maxBytes,
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    15 * time.Second,
		IdleTimeout:     60 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}
