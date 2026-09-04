package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"specpowers/backend/internal/agent"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/llm"
	"specpowers/backend/internal/skill"
)

// AgentRuntimeOptions configures the local runtime loop.
type AgentRuntimeOptions struct {
	Credential AgentCredential
	// Poll is the idle claim interval; used when Once is false.
	Poll time.Duration
	// Once executes at most one claim cycle (one claim, possibly a run).
	Once    bool
	WorkDir string
	// LLM is the completion client the tool loop uses. The production
	// command builds it from SP_LLM_*; tests inject a fake.
	LLM llm.Client
	// Skills defaults to the embedded registry.
	Skills *skill.Registry
	Stdout io.Writer
}

// RunAgentRuntime polls the server for runs assigned to the registered
// agent and executes them locally with the agent executor: comments,
// status changes and run logs are written back through the runtime API.
// With Once it returns after the first claim cycle; otherwise it loops
// until ctx is cancelled.
func RunAgentRuntime(ctx context.Context, opt AgentRuntimeOptions) error {
	if opt.LLM == nil {
		return fmt.Errorf("no LLM client configured for the local runtime")
	}
	poll := opt.Poll
	if poll <= 0 {
		poll = 3 * time.Second
	}
	skills := opt.Skills
	if skills == nil {
		reg, err := skill.DefaultRegistry()
		if err != nil {
			return fmt.Errorf("skill registry: %w", err)
		}
		skills = reg
	}

	c := New(opt.Credential.Server, opt.Credential.Token)
	stores := newRemoteStores(c)
	flow := &remoteFlow{c: c}
	exec := agent.NewExecutor(agent.ExecutorDeps{
		Issues:   stores,
		Comments: stores,
		Metadata: stores,
		Projects: stores,
		Client:   opt.LLM,
		Skills:   skills,
		WorkDir:  opt.WorkDir,
		Logs:     stores,
		Flow:     flow,
	})
	identity := &domain.Agent{ID: opt.Credential.AgentID, Name: opt.Credential.AgentName}

	for {
		run, err := c.ClaimRun()
		if err != nil {
			return fmt.Errorf("claim run: %w", err)
		}
		if run == nil {
			if opt.Once {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(poll):
			}
			continue
		}

		domainRun := &domain.Run{
			ID: run.ID, AgentID: run.AgentID, IssueID: run.IssueID,
			Trigger: run.Trigger, Status: run.Status, Error: run.Error,
		}
		if opt.Stdout != nil {
			fmt.Fprintf(opt.Stdout, "run %s: executing issue %s (trigger %s)\n", run.ID, run.IssueID, run.Trigger)
		}
		err = exec.Execute(ctx, domainRun, identity)
		status, errMsg := "done", ""
		if err != nil {
			status, errMsg = "failed", err.Error()
		}
		if err := c.FinishRun(run.ID, status, errMsg); err != nil {
			return fmt.Errorf("finish run %s: %w", run.ID, err)
		}
		if opt.Stdout != nil {
			fmt.Fprintf(opt.Stdout, "run %s: %s\n", run.ID, status)
		}
		if opt.Once {
			return nil
		}
	}
}

// LoadLLMFromEnv builds the local LLM client from SP_LLM_* variables,
// mirroring the server configuration.
func LoadLLMFromEnv(getenv func(string) string) (llm.Client, error) {
	apiKey, model := getenv("SP_LLM_API_KEY"), getenv("SP_LLM_MODEL")
	if apiKey == "" || model == "" {
		return nil, fmt.Errorf("LLM is not configured: set SP_LLM_API_KEY and SP_LLM_MODEL (and optionally SP_LLM_BASE_URL)")
	}
	return llm.NewOpenAIClient(getenv("SP_LLM_BASE_URL"), apiKey, model), nil
}

// cmdAgentRun runs the foreground local runtime: it polls the server for
// this agent's runs and executes them in the current directory.
func cmdAgentRun(args []string, stdout, stderr io.Writer) int {
	e := &env{stdout: stdout, stderr: stderr}
	fs := flag.NewFlagSet("agent run", flag.ContinueOnError)
	e.resolveFlags(fs)
	name := fs.String("name", "", "registered agent name (default the only registered agent)")
	once := fs.Bool("once", false, "execute at most one claim cycle, then exit")
	poll := fs.Duration("poll", 3*time.Second, "idle polling interval")
	workdir := fs.String("workdir", "", "directory for repo checkouts (default .specpower/agent-work)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cred, err := resolveAgentCredential(*name)
	if err != nil {
		return e.fail(1, fmt.Errorf("%w (run `sp agent register` first)", err))
	}
	llmClient, err := LoadLLMFromEnv(os.Getenv)
	if err != nil {
		return e.fail(1, err)
	}
	if *workdir == "" {
		*workdir = ".specpower/agent-work"
	}

	fmt.Fprintf(stdout, "agent %s (%s) polling %s every %s (Ctrl-C to stop)\n",
		cred.AgentName, cred.AgentID, cred.Server, *poll)
	err = RunAgentRuntime(context.Background(), AgentRuntimeOptions{
		Credential: cred,
		Poll:       *poll,
		Once:       *once,
		WorkDir:    *workdir,
		LLM:        llmClient,
		Stdout:     stdout,
	})
	if errors.Is(err, context.Canceled) {
		return 0
	}
	if err != nil {
		return e.fail(1, err)
	}
	return 0
}
