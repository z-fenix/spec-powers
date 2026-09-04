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

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/collab"
	"specpowers/backend/internal/config"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/project"
	"specpowers/backend/internal/store/postgres"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, postgres.NewMigrationDB(pool), postgres.MigrationsFS); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	users := postgres.NewUserStore(pool)
	workspaces := postgres.NewWorkspaceStore(pool)
	members := postgres.NewMemberStore(pool)
	projects := postgres.NewProjectStore(pool)
	issues := postgres.NewIssueStore(pool)
	comments := postgres.NewCommentStore(pool)
	attachments := postgres.NewAttachmentStore(pool)
	metadata := postgres.NewIssueMetadataStore(pool)

	tokens := auth.NewTokenService(cfg.JWTSecret, 24*time.Hour)
	authHandler := auth.NewHandler(auth.NewService(users, workspaces, members, tokens))
	issueHandler := issue.NewHandler(issue.NewService(issues, projects, users), tokens).WithCollab(
		collab.NewHandler(
			collab.NewService(issues, projects, comments, attachments, metadata, cfg.AttachmentDir),
			tokens,
		).Routes(),
	)
	projectHandler := project.NewHandler(
		project.NewService(projects, users, members, workspaces),
		tokens,
		issueHandler.Routes(),
	)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.NewRouter(httpapi.Deps{Auth: authHandler.Routes(), Project: projectHandler.Routes()}),
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
