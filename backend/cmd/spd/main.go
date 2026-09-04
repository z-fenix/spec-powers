package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"specpowers/backend/internal/config"
	"specpowers/backend/internal/server"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.LLMAPIKey == "" || cfg.LLMModel == "" {
		log.Printf("warning: SP_LLM_API_KEY/SP_LLM_MODEL not set; AI splitting is disabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := server.Build(ctx, cfg, server.Options{})
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer s.Close()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
		close(done)
	}()

	log.Printf("spd listening on %s (env=%s)", cfg.Addr, cfg.Env)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
	<-done
}
