// Package server assembles the spd server from configuration: connection
// pool, migrations, stores, handlers and the optional AI splitter. It is
// shared by cmd/spd and the sp CLI integration tests, which build a real
// server against a test database.
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"specpowers/backend/internal/agent"
	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/collab"
	"specpowers/backend/internal/config"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/llm"
	"specpowers/backend/internal/notification"
	"specpowers/backend/internal/pr"
	"specpowers/backend/internal/project"
	"specpowers/backend/internal/property"
	"specpowers/backend/internal/skill"
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
	// stopWorker cancels the agent runtime queue loop; nil when no worker
	// was started.
	stopWorker context.CancelFunc
}

// Close releases the owned pool and stops the agent runtime worker; an
// injected pool is left alone.
func (s *Server) Close() {
	if s.stopWorker != nil {
		s.stopWorker()
	}
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
	properties := postgres.NewPropertyStore(pool)
	changes := postgres.NewChangeStore(pool)
	artifacts := postgres.NewArtifactStore(pool)
	taskMappings := postgres.NewTaskMappingStore(pool)
	agents := postgres.NewAgentStore(pool)
	runs := postgres.NewRunStore(pool)
	runLogs := postgres.NewRunLogStore(pool)
	issueEvents := postgres.NewIssueEventStore(pool)
	pullRequests := postgres.NewPullRequestStore(pool)

	tokens := auth.NewTokenService(cfg.JWTSecret, 24*time.Hour)
	notificationStore := postgres.NewNotificationStore(pool)
	notificationSvc := notification.NewService(notificationStore)
	authHandler := auth.NewHandler(auth.NewService(users, workspaces, members, tokens))
	issueService := issue.NewService(issues, projects, users).WithEventStore(issueEvents)
	// Mention auto-claim: comments mentioning an agent enqueue its run.
	mentionTrigger := agent.NewMentionTrigger(agents, runs)
	collabSvc := collab.NewService(issues, projects, comments, attachments, metadata, cfg.AttachmentDir).
		WithNotifier(notificationSvc).
		WithUserDirectory(users).
		WithCommentObserver(func(ctx context.Context, c *domain.IssueComment) {
			if err := mentionTrigger.OnComment(ctx, c.IssueID, c.AuthorID, c.Content); err != nil {
				log.Printf("agent mention trigger: %v", err)
			}
		})
	// Issue ↔ PR association: keys ("SP-44") in PR titles/bodies/branches
	// link PRs to issues; close intents apply on merge.
	prService := pr.NewService(pullRequests, issues, projects).WithEventStore(issueEvents)
	prHandler := pr.NewHandler(prService, tokens)
	issueHandler := issue.NewHandler(issueService, tokens).WithCollab(
		collab.NewHandler(collabSvc, tokens).Routes(),
	).WithPullRequests(prHandler.IssueRoutes()).WithProperties(
		property.NewHandler(property.NewService(properties, projects, issues), tokens).ValueRoutes(),
	)
	propertyHandler := property.NewHandler(property.NewService(properties, projects, issues), tokens)
	projectHandler := project.NewHandler(
		project.NewService(projects, users, members, workspaces),
		tokens,
		issueHandler.Routes(),
	).WithPullRequests(prHandler.Routes()).WithProperties(propertyHandler.DefinitionRoutes())
	workflowService := workflow.NewService(changes, artifacts, taskMappings, issues, projects)
	workflowService = workflowService.WithWaker(issues)
	runTrigger := agent.NewTrigger(agents, runs)
	workflowService = workflowService.WithWakeupHook(runTrigger)
	workflowService = workflowService.WithCreator(issueService)
	skillRegistry, err := skill.DefaultRegistry()
	if err != nil {
		if ownsPool {
			pool.Close()
		}
		return nil, fmt.Errorf("skill registry: %w", err)
	}
	workflowService = workflowService.WithSkills(skillRegistry)
	// Agent identities act on changes of their assigned issues without
	// project membership (their runs are system-driven).
	workflowService = workflowService.WithAgentAccess(agent.StoreAgentAccess{Agents: agents})

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
	skillHandler := skill.NewHandler(skillRegistry, tokens)
	notificationHandler := notification.NewHandler(notificationSvc, tokens)

	// Agent runtime: definitions, run queue, LLM tool-loop executor and the
	// issue-service trigger that enqueues runs on assignment / status change.
	agentSvc := agent.NewService(agents, users, skillRegistry)
	executor := agent.NewExecutor(agent.ExecutorDeps{
		Issues:      issues,
		Comments:    comments,
		Metadata:    metadata,
		Projects:    projects,
		Client:      llmClient,
		Skills:      skillRegistry,
		WorkDir:     cfg.AgentWorkDir,
		Logs:        runLogs,
		Usage:       runs,
		Flow:        agent.NewWorkflowFlow(workflowService),
		MentionHook: mentionTrigger.OnComment,
	})
	queue := agent.NewQueue(runs, runLogs, agents, executor).WithNotifier(notificationSvc, issues)
	issueService = issueService.WithRunTrigger(runTrigger.WithNotifier(notificationSvc))
	// Long-lived credentials for locally registered agent runtimes (sp agent
	// register). Revocation is deleting the agent.
	runtimeTokens := auth.NewTokenService(cfg.JWTSecret, agent.RuntimeTokenTTL)
	agentHandler := agent.NewHandler(agentSvc, queue, runs, runLogs, issues, tokens, runtimeTokens)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	go queue.Loop(workerCtx)
	// Due-date notifications: periodic scan writing "due" notices for human
	// assignees when deadlines approach or pass; shares the worker lifetime.
	dueScanner := notification.NewDueScanner(issues, agents, notificationStore, notificationSvc)
	go dueScanner.Loop(workerCtx, time.Minute)
	runtimeHandler := agent.NewRuntimeHandler(agent.RuntimeHandlerDeps{
		Agents:      agents,
		Runs:        runs,
		Logs:        runLogs,
		Issues:      issues,
		Comments:    comments,
		Metadata:    metadata,
		Projects:    projects,
		Tokens:      tokens,
		MentionHook: mentionTrigger.OnComment,
	})

	return &Server{
		Handler: httpapi.NewRouter(httpapi.Deps{
			Auth:    authHandler.Routes(),
			Project: projectHandler.Routes(),
			Changes: workflowHandler.Routes(),
			Skills:  skillHandler.Routes(),
			Agents:  agentHandler.AgentRoutes(),
			Runs:    agentHandler.RunRoutes(),
			Notifs:  notificationHandler.Routes(),
			Runtime: runtimeHandler.Routes(),
			Static:  staticFromConfig(cfg),
		}),
		pool:       pool,
		ownsPool:   ownsPool,
		stopWorker: stopWorker,
	}, nil
}

// staticFromConfig returns the SPA file handler when SP_STATIC_DIR points at
// a built frontend; an empty dir disables static serving (API-only mode).
func staticFromConfig(cfg config.Config) http.Handler {
	if cfg.StaticDir == "" {
		return nil
	}
	return httpapi.SPAHandler(cfg.StaticDir)
}
