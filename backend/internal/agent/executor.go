package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/issue"
	"specpowers/backend/internal/llm"
	"specpowers/backend/internal/skill"
	"specpowers/backend/internal/store"
)

// Run log kinds.
const (
	logLLMRequest  = "llm_request"
	logLLMResponse = "llm_response"
	logToolCall    = "tool_call"
	logToolResult  = "tool_result"
	logError       = "error"
)

// defaultMaxTurns bounds the tool loop so a confused model cannot loop
// forever; one turn is one LLM completion.
const defaultMaxTurns = 12

// Executor runs an agent on an issue: an LLM tool loop over the plain
// completion client. The model replies with one JSON object per turn —
// either a tool call or the final message (which becomes a comment).
type Executor struct {
	issues   store.IssueStore
	comments store.CommentStore
	metadata store.IssueMetadataStore
	projects store.ProjectStore
	client   llm.Client
	skills   *skill.Registry
	workDir  string
	checkout func(ctx context.Context, pointer, destDir string) error
	logs     logAppender
	// mentionHook fires after the agent posts a comment so comments can
	// hand work to other agents via @-mentions.
	mentionHook func(ctx context.Context, issueID, authorID, content string) error
	// flow drives the classic workflow (change per issue, versioned
	// artifacts, guard-checked phase advance). Nil keeps runs flow-less.
	flow     FlowDriver
	MaxTurns int
}

type ExecutorDeps struct {
	Issues   store.IssueStore
	Comments store.CommentStore
	Metadata store.IssueMetadataStore
	Projects store.ProjectStore
	Client   llm.Client
	Skills   *skill.Registry
	WorkDir  string
	Checkout func(ctx context.Context, pointer, destDir string) error
	Logs     logAppender
	// MentionHook fires after the agent posts a comment (see mention.go).
	MentionHook func(ctx context.Context, issueID, authorID, content string) error
	// Flow drives the classic workflow (see flow.go).
	Flow     FlowDriver
	MaxTurns int
}

func NewExecutor(deps ExecutorDeps) *Executor {
	maxTurns := deps.MaxTurns
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns
	}
	return &Executor{
		issues:      deps.Issues,
		comments:    deps.Comments,
		metadata:    deps.Metadata,
		projects:    deps.Projects,
		client:      deps.Client,
		skills:      deps.Skills,
		workDir:     deps.WorkDir,
		checkout:    deps.Checkout,
		logs:        deps.Logs,
		mentionHook: deps.MentionHook,
		flow:        deps.Flow,
		MaxTurns:    maxTurns,
	}
}

var _ RunExecutor = (*Executor)(nil)

// Execute drives the tool loop for one run.
func (e *Executor) Execute(ctx context.Context, run *domain.Run, agent *domain.Agent) error {
	if e.client == nil {
		return fmt.Errorf("LLM client is not configured (set SP_LLM_API_KEY/SP_LLM_MODEL)")
	}
	iss, err := e.issues.GetIssue(ctx, run.IssueID)
	if err != nil {
		return fmt.Errorf("load issue: %w", err)
	}
	metadata, _ := e.metadata.ListIssueMetadata(ctx, run.IssueID)
	comments, _ := e.comments.ListComments(ctx, run.IssueID)

	// Classic workflow: every agent run is anchored to the issue's change;
	// the phase's skill steers the loop and the flow tools advance it.
	var change *domain.Change
	var phaseSkill *skill.Skill
	if e.flow != nil {
		change, err = e.flow.EnsureChange(ctx, agent.ID, run.IssueID)
		if err != nil {
			return fmt.Errorf("ensure change: %w", err)
		}
		phaseSkill, err = e.flow.PhaseSkill(ctx, agent.ID, change)
		if err != nil {
			// A change past the flow (archived/failed) has no next skill;
			// the run then just answers without flow tooling guidance.
			phaseSkill = nil
		}
	}

	system := e.systemPrompt(agent, change, phaseSkill)
	task := e.taskPrompt(iss, metadata, comments)
	transcript := []string{task}

	for turn := 1; turn <= e.MaxTurns; turn++ {
		user := strings.Join(transcript, "\n\n")
		e.appendLog(ctx, run.ID, logLLMRequest, "system:\n"+system+"\n\nuser:\n"+user)
		reply, err := e.client.Complete(ctx, system, user)
		if err != nil {
			e.appendLog(ctx, run.ID, logError, err.Error())
			return fmt.Errorf("llm turn %d: %w", turn, err)
		}
		e.appendLog(ctx, run.ID, logLLMResponse, reply)

		action, err := parseAction(reply)
		if err != nil {
			e.appendLog(ctx, run.ID, logError, "unparseable reply: "+err.Error())
			transcript = append(transcript, "Your last reply was not a valid action JSON ("+err.Error()+"). Reply again with exactly one JSON object.")
			continue
		}
		if action.Tool == "" {
			if strings.TrimSpace(action.Message) == "" {
				e.appendLog(ctx, run.ID, logError, "final reply has empty message")
				return fmt.Errorf("final reply has empty message")
			}
			if _, err := e.comments.CreateComment(ctx, &domain.IssueComment{
				IssueID:  run.IssueID,
				AuthorID: agent.ID,
				Content:  action.Message,
			}); err != nil {
				return fmt.Errorf("post final comment: %w", err)
			}
			e.fireMentionHook(ctx, run.IssueID, agent.ID, action.Message)
			return nil
		}
		e.appendLog(ctx, run.ID, logToolCall, action.Tool+" "+mustJSON(action.Args))
		result := e.runTool(ctx, run, agent, iss, change, action)
		e.appendLog(ctx, run.ID, logToolResult, result)
		transcript = append(transcript,
			"You called tool "+action.Tool+".\nTOOL_RESULT:\n"+result+"\n\nContinue: reply with exactly one JSON object.")
	}
	return fmt.Errorf("max turns (%d) exhausted without a final answer", e.MaxTurns)
}

// ---- run logs ----

type logAppender interface {
	AppendRunLog(ctx context.Context, l *domain.RunLog) (*domain.RunLog, error)
}

func (e *Executor) appendLog(ctx context.Context, runID, kind, content string) {
	if e.logs == nil {
		return
	}
	_, _ = e.logs.AppendRunLog(ctx, &domain.RunLog{RunID: runID, Kind: kind, Content: content})
}

// fireMentionHook notifies the mention trigger about an agent-posted
// comment; failures are logged and non-fatal.
func (e *Executor) fireMentionHook(ctx context.Context, issueID, authorID, content string) {
	if e.mentionHook == nil {
		return
	}
	if err := e.mentionHook(ctx, issueID, authorID, content); err != nil {
		e.appendLog(ctx, "", logError, "mention hook: "+err.Error())
	}
}

// ---- actions ----

type action struct {
	Final   bool
	Message string
	Tool    string
	Args    map[string]any
}

func parseAction(reply string) (*action, error) {
	raw := strings.TrimSpace(reply)
	// Tolerate fenced replies: ```json ... ```
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	if start := strings.Index(raw, "{"); start > 0 {
		if end := strings.LastIndex(raw, "}"); end > start {
			raw = raw[start : end+1]
		}
	}
	var parsed struct {
		Action  string         `json:"action"`
		Message string         `json:"message"`
		Tool    string         `json:"tool"`
		Args    map[string]any `json:"args"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	switch parsed.Action {
	case "final":
		return &action{Final: true, Message: parsed.Message}, nil
	case "tool":
		if parsed.Tool == "" {
			return nil, fmt.Errorf("tool action without tool name")
		}
		return &action{Tool: parsed.Tool, Args: parsed.Args}, nil
	default:
		return nil, fmt.Errorf("unknown action %q", parsed.Action)
	}
}

// ---- tools ----

func (e *Executor) runTool(ctx context.Context, run *domain.Run, agent *domain.Agent, iss *domain.Issue, change *domain.Change, a *action) string {
	switch a.Tool {
	case "read_issue":
		metadata, _ := e.metadata.ListIssueMetadata(ctx, iss.ID)
		comments, _ := e.comments.ListComments(ctx, iss.ID)
		return mustJSON(map[string]any{"issue": iss, "comments": comments, "metadata": metadata})
	case "checkout_repo":
		return e.toolCheckout(ctx, run, iss)
	case "get_flow":
		if change == nil {
			return mustJSON(map[string]any{"error": "no classic flow configured"})
		}
		out := map[string]any{"change": change}
		if e.flow != nil {
			if sk, err := e.flow.PhaseSkill(ctx, agent.ID, change); err == nil {
				out["next_skill"] = sk.Key
			}
		}
		return mustJSON(out)
	case "write_artifact":
		kind, _ := a.Args["kind"].(string)
		content, _ := a.Args["content"].(string)
		art, err := e.flow.WriteArtifact(ctx, agent.ID, change, kind, content)
		if err != nil {
			return mustJSON(map[string]any{"error": err.Error()})
		}
		return mustJSON(map[string]any{"artifact": art})
	case "advance_phase":
		updated, err := e.flow.AdvancePhase(ctx, agent.ID, change)
		if err != nil {
			return mustJSON(map[string]any{"error": err.Error()})
		}
		*change = *updated
		return mustJSON(map[string]any{"change": change})
	case "submit_verify":
		content, _ := a.Args["content"].(string)
		art, err := e.flow.SubmitVerify(ctx, agent.ID, change, content)
		if err != nil {
			return mustJSON(map[string]any{"error": err.Error()})
		}
		return mustJSON(map[string]any{"artifact": art})
	case "archive":
		updated, err := e.flow.Archive(ctx, agent.ID, change)
		if err != nil {
			return mustJSON(map[string]any{"error": err.Error()})
		}
		*change = *updated
		return mustJSON(map[string]any{"change": change})
	case "post_comment":
		content, _ := a.Args["content"].(string)
		if strings.TrimSpace(content) == "" {
			return mustJSON(map[string]any{"error": "content is required"})
		}
		parentID, _ := a.Args["parent_comment_id"].(string)
		c, err := e.comments.CreateComment(ctx, &domain.IssueComment{
			IssueID:  iss.ID,
			ParentID: parentID,
			AuthorID: agent.ID,
			Content:  content,
		})
		if err != nil {
			return mustJSON(map[string]any{"error": err.Error()})
		}
		e.fireMentionHook(ctx, iss.ID, agent.ID, content)
		return mustJSON(map[string]any{"comment_id": c.ID})
	case "set_status":
		status, _ := a.Args["status"].(string)
		if !issue.CanTransition(iss.Status, status) {
			return mustJSON(map[string]any{"error": "illegal status transition " + iss.Status + " -> " + status})
		}
		// Classic-flow gate: while the issue's change is active, the issue
		// cannot leave the working statuses — finish the flow (verify,
		// archive) first. Refusals are logged so the run record shows them.
		if e.flow != nil && change != nil && change.Status == "active" &&
			(status == issue.StatusInReview || status == issue.StatusDone) {
			e.appendLog(ctx, run.ID, logError,
				"classic flow gate: change "+change.ID+" is still active ("+change.Phase+
					"); complete the workflow (write_artifact, advance_phase, submit_verify, archive) before moving the issue to "+status)
			return mustJSON(map[string]any{
				"error": "gate: the classic flow for this issue is still active (phase " + change.Phase +
					"); call archive after the flow completes, then set_status",
			})
		}
		updated := *iss
		updated.Status = status
		if _, err := e.issues.UpdateIssue(ctx, &updated); err != nil {
			return mustJSON(map[string]any{"error": err.Error()})
		}
		*iss = updated
		return mustJSON(map[string]any{"status": status})
	default:
		return mustJSON(map[string]any{"error": "unknown tool " + a.Tool})
	}
}

// toolCheckout materializes the project's resources on disk: local
// directories are reported as-is, github repos are cloned into the run's
// work directory.
func (e *Executor) toolCheckout(ctx context.Context, run *domain.Run, iss *domain.Issue) string {
	resources, err := e.projects.ListProjectResources(ctx, iss.ProjectID)
	if err != nil {
		return mustJSON(map[string]any{"error": err.Error()})
	}
	type checkedOut struct {
		Type  string `json:"type"`
		Label string `json:"label"`
		Path  string `json:"path"`
		Error string `json:"error,omitempty"`
	}
	results := []checkedOut{}
	for _, r := range resources {
		switch r.Type {
		case "local_directory":
			results = append(results, checkedOut{Type: r.Type, Label: r.Label, Path: r.Pointer})
		case "github_repo":
			dest := filepath.Join(e.workDir, run.ID, filepath.Base(r.Pointer))
			var errMsg string
			clone := e.checkout
			if clone == nil {
				clone = gitClone
			}
			if err := clone(ctx, r.Pointer, dest); err != nil {
				errMsg = err.Error()
			}
			results = append(results, checkedOut{Type: r.Type, Label: r.Label, Path: dest, Error: errMsg})
		default:
			results = append(results, checkedOut{Type: r.Type, Label: r.Label, Error: "unknown resource type"})
		}
	}
	return mustJSON(map[string]any{"resources": results})
}

func gitClone(ctx context.Context, pointer, destDir string) error {
	url := "https://github.com/" + pointer + ".git"
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, destDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone %s: %v: %s", url, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---- prompts ----

func (e *Executor) systemPrompt(agent *domain.Agent, change *domain.Change, phaseSkill *skill.Skill) string {
	var b strings.Builder
	b.WriteString("You are " + agent.Name + ", an autonomous coding agent working on issues.\n")
	if agent.Description != "" {
		b.WriteString("\nAbout you: " + agent.Description + "\n")
	}
	if e.skills != nil && len(agent.Skills) > 0 {
		b.WriteString("\nYour skills (follow their instructions while working):\n")
		for _, key := range agent.Skills {
			if s, ok := e.skills.Get(key); ok {
				b.WriteString("\n## Skill: " + s.Name + " (" + s.Key + ")\n" + s.Instructions + "\n")
			}
		}
	}
	if change != nil {
		b.WriteString("\n## Classic workflow (mandatory)\n")
		b.WriteString("This issue runs the classic flow (proposal → specs → design → tasks) as change " + change.ID + ".\n")
		b.WriteString("Current phase: " + change.Phase + " (change status: " + change.Status + ").\n")
		if phaseSkill != nil {
			b.WriteString("\nFollow this phase skill:\n\n## Skill: " + phaseSkill.Name + " (" + phaseSkill.Key + ")\n" + phaseSkill.Instructions + "\n")
		}
		b.WriteString(`
Produce each phase's document with write_artifact, then advance_phase (the guard refuses skipped phases, missing artifacts or failed verifications). Verify with submit_verify when required, and archive the change only when the flow completes. You cannot move the issue to in_review/done while the change is active — the gate refuses it.
`)
	}
	b.WriteString(`
You work in turns. Every reply MUST be exactly one JSON object, no other text:

To use a tool:
{"action":"tool","tool":"<name>","args":{...}}

Available tools:
- read_issue: read the issue, its comments and metadata. args: {}
- checkout_repo: materialize the project's repositories on disk and get their paths. args: {}
- post_comment: post a comment on the issue as yourself. args: {"content":"...", "parent_comment_id":"(optional)"}
- set_status: change the issue status (todo, in_progress, in_review, done, blocked, cancelled). args: {"status":"..."}

Workflow tools (when the issue runs the classic flow):
- get_flow: current change, phase and next skill. args: {}
- write_artifact: store a versioned phase artifact (proposal, specs, design, tasks). args: {"kind":"...","content":"..."}
- advance_phase: advance to the next phase; the guard refuses illegal advances. args: {}
- submit_verify: submit a verify report (YAML result: pass|fail). args: {"content":"..."}
- archive: archive the change after the flow completes. args: {}

To finish:
{"action":"final","message":"<your final answer, posted as a comment>"}

Work step by step; post progress comments as you go; finish with a final message summarizing the outcome.
`)
	return b.String()
}

func (e *Executor) taskPrompt(iss *domain.Issue, metadata []domain.IssueMetadata, comments []domain.IssueComment) string {
	var b strings.Builder
	b.WriteString("You are assigned to issue " + iss.ID + ".\n\n")
	b.WriteString("Title: " + iss.Title + "\n")
	if iss.Description != "" {
		b.WriteString("\nDescription:\n" + iss.Description + "\n")
	}
	b.WriteString("\nStatus: " + iss.Status + "\n")
	if len(comments) > 0 {
		b.WriteString("\nComments:\n")
		for _, c := range comments {
			b.WriteString("- [" + c.AuthorID + "] " + c.Content + "\n")
		}
	}
	if len(metadata) > 0 {
		b.WriteString("\nMetadata:\n")
		for _, m := range metadata {
			b.WriteString("- " + m.Key + " = " + m.Value + "\n")
		}
	}
	b.WriteString("\nBegin. Reply with exactly one JSON object.")
	return b.String()
}

// ---- helpers ----

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("{\"error\": %q}", err.Error())
	}
	return string(b)
}
