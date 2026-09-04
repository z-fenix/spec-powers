// Package server assembles the spd server from configuration: connection
// pool, migrations, stores, handlers and the optional AI splitter. It is
// shared by cmd/spd and the sp CLI integration tests, which build a real
// server against a test database.
package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/collab"
	"specpowers/backend/internal/config"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/llm"
	"specpowers/backend/internal/project"
	"specpowers/backend/internal/store/postgres"
	"specpowers/backend/internal/workflow"
)

// Options carries injectable dependencies for tests. Zero values keep the
// production wiring.
type Options struct {
	// Pool reuses an existing connection pool; the server does not close it.
	// When nil, a pool is created from cfg.DatabaseURL and owned by Server.
	Pool *pgxpool.Pool
	// LLM overrides the configured OpenAI client and enables the splitter
	// regardless of SP_LLM_* keys. When nil, the splitter is wired from
	// config (disabled when the keys are unset).
	LLM llm.Client
}

// Server is an assembled spd server: an HTTP handler plus its pool.
type Server struct {
	Handler  http.Handler
	pool     *pgxpool.Pool
	ownsPool bool
}

// Close releases the owned pool; an injected pool is left alone.
func (s *Server) Close() {
	if s.ownsPool && s.pool != nil {
		s.pool.Close()
	}
}

// Build wires the full server. It always runs migrations.
func Build(ctx context.Context, cfg config.Config, opt Options) (*Server, error) {
	var (
		pool     *pgxpool.Pool
		ownsPool bool
		err      error
	)
	if opt.Pool != nil {
		pool = opt.Pool
	} else {
		pool, err = postgres.New(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("database: %w", err)
		}
		ownsPool = true
	}
	if err := postgres.Migrate(ctx, postgres.NewMigrationDB(pool), postgres.MigrationsFS); err != nil {
		if ownsPool {
			pool.Close()
		}
		return nil, fmt.Errorf("migrate: %w", err)
	}

	users := postgres.NewUserStore(pool)
	workspaces := postgres.NewWorkspaceStore(pool)
	members := postgres.NewMemberStore(pool)
	projects := postgres.NewProjectStore(pool)
	issues := postgres.NewIssueStore(pool)
	comments := postgres.NewCommentStore(pool)
	attachments := postgres.NewAttachmentStore(pool)
	metadata := postgres.NewIssueMetadataStore(pool)
	changes := postgres.NewChangeStore(pool)
	artifacts := postgres.NewArtifactStore(pool)
	taskMappings := postgres.NewTaskMappingStore(pool)

	tokens := auth.NewTokenService(cfg.JWTSecret, 24*time.Hour)
	authHandler := auth.NewHandler(auth.NewService(users, workspaces, members, tokens))
	issueService := issue.NewService(issues, projects, users)
	issueHandler := issue.NewHandler(issueService, tokens).WithCollab(
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
	workflowService := workflow.NewService(changes, artifacts, taskMappings, issues, projects)
	workflowService = workflowService.WithWaker(issues)

	llmClient := opt.LLM
	if llmClient == nil && cfg.LLMAPIKey != "" && cfg.LLMModel != "" {
		llmClient = llm.NewOpenAIClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	}
	if llmClient != nil {
		promptTemplates, err := workflow.LoadTemplates(cfg.LLMPromptDir)
		if err != nil {
			if ownsPool {
				pool.Close()
			}
			return nil, fmt.Errorf("prompt templates: %w", err)
		}
		splitter := workflow.NewSplitter(workflow.SplitterDeps{
			Client:     llmClient,
			Changes:    changes,
			Artifacts:  artifacts,
			Mappings:   taskMappings,
			Issues:     issues,
			Creator:    issueService,
			Contexts:   projects,
			Templates:  promptTemplates,
			MaxRetries: cfg.LLMMaxRetries,
		})
		workflowService = workflowService.WithSplitter(splitter)
	}
	workflowHandler := workflow.NewHandler(workflowService, tokens)

	return &Server{
		Handler: httpapi.NewRouter(httpapi.Deps{
			Auth:    authHandler.Routes(),
			Project: projectHandler.Routes(),
			Changes: workflowHandler.Routes(),
		}),
		pool:     pool,
		ownsPool: ownsPool,
	}, nil
}
