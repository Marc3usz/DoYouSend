// Package config loads runtime configuration from the environment.
//
// Every value comes from the environment (see .env.example); nothing is hardcoded and
// no secret is ever committed.
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppEnv      string
	APIPort     string
	DatabaseURL string
	RedisURL    string
	// DryRun keeps the system from reaching real e-mail/SMS providers. It defaults to true
	// and may only be turned off with the project supervisor's approval.
	DryRun bool
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:      env("APP_ENV", "development"),
		APIPort:     env("API_PORT", "8080"),
		DatabaseURL: env("DATABASE_URL", ""),
		RedisURL:    env("REDIS_URL", ""),
	}

	dryRun, err := strconv.ParseBool(env("DRY_RUN", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("parse DRY_RUN: %w", err)
	}
	cfg.DryRun = dryRun

	return cfg, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
