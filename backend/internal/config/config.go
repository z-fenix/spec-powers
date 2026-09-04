package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Addr          string
	DatabaseURL   string
	JWTSecret     string
	Env           string
	AttachmentDir string
	LLMBaseURL    string
	LLMAPIKey     string
	LLMModel      string
	LLMPromptDir  string
	LLMMaxRetries int
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:          getenv("SP_ADDR"),
		DatabaseURL:   getenv("SP_DATABASE_URL"),
		JWTSecret:     getenv("SP_JWT_SECRET"),
		Env:           getenv("SP_ENV"),
		AttachmentDir: getenv("SP_ATTACHMENT_DIR"),
		LLMBaseURL:    getenv("SP_LLM_BASE_URL"),
		LLMAPIKey:     getenv("SP_LLM_API_KEY"),
		LLMModel:      getenv("SP_LLM_MODEL"),
		LLMPromptDir:  getenv("SP_LLM_PROMPT_DIR"),
	}
	if cfg.LLMBaseURL == "" {
		cfg.LLMBaseURL = "https://api.openai.com/v1"
	}
	cfg.LLMMaxRetries = 2
	if v := getenv("SP_LLM_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.LLMMaxRetries = n
		}
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.Env == "" {
		cfg.Env = "dev"
	}
	if cfg.AttachmentDir == "" {
		cfg.AttachmentDir = "data/attachments"
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
