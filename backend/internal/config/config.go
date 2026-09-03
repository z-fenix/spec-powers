package config

import (
	"errors"
	"fmt"
	"os"
)

type Config struct {
	Addr        string
	DatabaseURL string
	JWTSecret   string
	Env         string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:        getenv("SP_ADDR"),
		DatabaseURL: getenv("SP_DATABASE_URL"),
		JWTSecret:   getenv("SP_JWT_SECRET"),
		Env:         getenv("SP_ENV"),
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.Env == "" {
		cfg.Env = "dev"
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "postgres://specpowers:specpowers@localhost:5432/specpowers?sslmode=disable"
	}
	if cfg.JWTSecret == "" {
		if cfg.Env == "production" {
			return Config{}, errors.New("SP_JWT_SECRET is required when SP_ENV=production")
		}
		cfg.JWTSecret = "dev-insecure-secret"
	}
	if cfg.Env == "dev" && cfg.JWTSecret == "dev-insecure-secret" {
		fmt.Fprintln(os.Stderr, "warning: using default dev JWT secret; set SP_JWT_SECRET for anything real")
	}
	return cfg, nil
}
