// Package cli implements the sp command line tool: a REST client for the
// spd server plus a local workspace cache under .specpower/. All workflow
// commands (open / guard / handoff / state record-check / verify / archive)
// talk to the server so artifacts land in the database.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is a minimal REST client for the /api/v1 surface.
type Client struct {
	BaseURL string // e.g. http://localhost:8080
	Token   string
	HTTP    *http.Client
}

// New returns a client for the server at baseURL, authenticated with token.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{},
	}
}

// APIError is a server error in the standard envelope
// {"error":{"code":"...","message":"..."}}.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NotFound reports whether the error is the server's 404 envelope.
func NotFound(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.Status == http.StatusNotFound
}

func (c *Client) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+"/api/v1"+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &env) == nil && env.Error.Code != "" {
			return &APIError{Status: resp.StatusCode, Code: env.Error.Code, Message: env.Error.Message}
		}
		return &APIError{Status: resp.StatusCode, Code: "http_error", Message: strings.TrimSpace(string(raw))}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

// ---- types mirrored from the server DTOs ----

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type LoginResult struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type Agent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Skills      []string `json:"skills"`
	Runtime     string   `json:"runtime"`
	CreatedBy   string   `json:"created_by"`
}

type AgentRegisterResult struct {
	Agent Agent  `json:"agent"`
	Token string `json:"token"`
}

// RunRow mirrors a run row for the runtime claim/finish endpoints.
type RunRow struct {
	ID         string  `json:"id"`
	AgentID    string  `json:"agent_id"`
	IssueID    string  `json:"issue_id"`
	Trigger    string  `json:"trigger"`
	Status     string  `json:"status"`
	Error      string  `json:"error"`
	CreatedAt  string  `json:"created_at"`
	StartedAt  *string `json:"started_at,omitempty"`
	FinishedAt *string `json:"finished_at,omitempty"`
}

// RunContext is the issue-scoped payload behind the executor's read tools:
// the issue itself, every comment (including other agents'), the metadata
// bag and the project resources.
type RunContext struct {
	Issue     RuntimeIssue      `json:"issue"`
	Comments  []RuntimeComment  `json:"comments"`
	Metadata  []RuntimeMetadata `json:"metadata"`
	Resources []RuntimeResource `json:"resources"`
}

type RuntimeIssue struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ParentID    string `json:"parent_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	AssigneeID  string `json:"assignee_id"`
}

type RuntimeComment struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	AuthorID string `json:"author_id"`
	Content  string `json:"content"`
}

type RuntimeMetadata struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

type RuntimeResource struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Label   string `json:"label"`
	Pointer string `json:"pointer"`
}

type Change struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	IssueID   string `json:"issue_id"`
	Phase     string `json:"phase"`
	Status    string `json:"status"`
}

type TaskMapping struct {
	ID       string `json:"id"`
	IssueID  string `json:"issue_id"`
	Title    string `json:"title"`
	Stage    int    `json:"stage"`
	Position int    `json:"position"`
}

type GuardReport struct {
	ChangeID     string   `json:"change_id"`
	Phase        string   `json:"phase"`
	NextPhase    string   `json:"next_phase"`
	PhaseLegal   bool     `json:"phase_legal"`
	HandoffFresh bool     `json:"handoff_fresh"`
	VerifyPassed bool     `json:"verify_passed"`
	CanAdvance   bool     `json:"can_advance"`
	CanArchive   bool     `json:"can_archive"`
	Reasons      []string `json:"reasons"`
}

type Handoff struct {
	ID        string `json:"id"`
	FromPhase string `json:"from_phase"`
	ToPhase   string `json:"to_phase"`
}

type Skill struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Order        int    `json:"order"`
	Instructions string `json:"instructions"`
}

type Artifact struct {
	ID       string `json:"id"`
	ChangeID string `json:"change_id"`
	Kind     string `json:"kind"`
	Version  int    `json:"version"`
}

// ---- endpoint methods ----

func (c *Client) Register(email, password, displayName string) (LoginResult, error) {
	var res LoginResult
	err := c.do(http.MethodPost, "/auth/register", map[string]string{
		"email": email, "password": password, "display_name": displayName,
	}, &res)
	return res, err
}

func (c *Client) Login(email, password string) (LoginResult, error) {
	var res LoginResult
	err := c.do(http.MethodPost, "/auth/login", map[string]string{
		"email": email, "password": password,
	}, &res)
	return res, err
}

// RegisterAgent creates a local-runtime agent and returns its runtime
// credential token.
func (c *Client) RegisterAgent(name, description string, skills []string) (AgentRegisterResult, error) {
	var res AgentRegisterResult
	err := c.do(http.MethodPost, "/agents/register", map[string]any{
		"name": name, "description": description, "skills": skills,
	}, &res)
	return res, err
}

// DeleteAgent removes the agent record (revokes its runtime credential).
func (c *Client) DeleteAgent(id string) error {
	return c.do(http.MethodDelete, "/agents/"+id, nil, nil)
}

// ---- agent runtime endpoints ----

// ClaimRun claims the oldest queued run of the credential's agent. A nil
// run (JSON null) means the queue is empty, not an error.
func (c *Client) ClaimRun() (*RunRow, error) {
	var res struct {
		Run *RunRow `json:"run"`
	}
	if err := c.do(http.MethodPost, "/runtime/claim", nil, &res); err != nil {
		return nil, err
	}
	return res.Run, nil
}

// FinishRun reports a run's terminal state.
func (c *Client) FinishRun(runID, status, errMsg string) error {
	return c.do(http.MethodPost, "/runtime/runs/"+runID+"/finish", map[string]string{
		"status": status, "error": errMsg,
	}, nil)
}

// AppendRunLog streams one run log entry to the server.
func (c *Client) AppendRunLog(runID, kind, content string) error {
	return c.do(http.MethodPost, "/runtime/runs/"+runID+"/log", map[string]string{
		"kind": kind, "content": content,
	}, nil)
}

// GetRunContext reads the issue, its comments, metadata and resources.
func (c *Client) GetRunContext(issueID string) (*RunContext, error) {
	var res RunContext
	if err := c.do(http.MethodGet, "/runtime/issues/"+issueID, nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// PostIssueComment comments on the issue as the credential's agent.
func (c *Client) PostIssueComment(issueID, content, parentCommentID string) (string, error) {
	var res struct {
		CommentID string `json:"comment_id"`
	}
	err := c.do(http.MethodPost, "/runtime/issues/"+issueID+"/comments", map[string]string{
		"content": content, "parent_comment_id": parentCommentID,
	}, &res)
	return res.CommentID, err
}

// SetIssueStatus moves the issue's status (server validates the transition).
func (c *Client) SetIssueStatus(issueID, status string) error {
	return c.do(http.MethodPost, "/runtime/issues/"+issueID+"/status", map[string]string{
		"status": status,
	}, nil)
}

// GetChangeByIssue returns the change running for an issue, or an *APIError
// with status 404 when none exists.
func (c *Client) GetChangeByIssue(issueID string) (*Change, error) {
	var res struct {
		Change Change `json:"change"`
	}
	err := c.do(http.MethodGet, "/changes?issue_id="+issueID, nil, &res)
	if err != nil {
		return nil, err
	}
	return &res.Change, nil
}

// CreateChange opens a change for an issue: the server runs the AI classic
// split and returns the change plus generated task mappings.
func (c *Client) CreateChange(issueID string) (*Change, []TaskMapping, error) {
	var res struct {
		Change Change       `json:"change"`
		Tasks  []TaskMapping `json:"tasks"`
	}
	err := c.do(http.MethodPost, "/changes", map[string]string{"issue_id": issueID}, &res)
	if err != nil {
		return nil, nil, err
	}
	return &res.Change, res.Tasks, nil
}

// CreateChangeManual opens a bare proposal-phase change without the AI
// split, for the agent-driven skill flow.
func (c *Client) CreateChangeManual(issueID string) (*Change, error) {
	var res struct {
		Change Change `json:"change"`
	}
	err := c.do(http.MethodPost, "/changes", map[string]any{"issue_id": issueID, "manual": true}, &res)
	if err != nil {
		return nil, err
	}
	return &res.Change, nil
}

// ListSkills returns the known skills in flow order.
func (c *Client) ListSkills() ([]Skill, error) {
	var res struct {
		Skills []Skill `json:"skills"`
	}
	err := c.do(http.MethodGet, "/skills", nil, &res)
	if err != nil {
		return nil, err
	}
	return res.Skills, nil
}

// GetSkill returns one skill including its instructions.
func (c *Client) GetSkill(key string) (*Skill, error) {
	var res struct {
		Skill Skill `json:"skill"`
	}
	err := c.do(http.MethodGet, "/skills/"+key, nil, &res)
	if err != nil {
		return nil, err
	}
	return &res.Skill, nil
}

// NextSkill resolves the skill the agent should load next for the change.
func (c *Client) NextSkill(changeID string) (*Skill, error) {
	var res struct {
		Skill Skill `json:"skill"`
	}
	err := c.do(http.MethodGet, "/changes/"+changeID+"/skills/next", nil, &res)
	if err != nil {
		return nil, err
	}
	return &res.Skill, nil
}

// WriteArtifact stores a new version of one artifact kind for the change.
// runID records the producing run for log tracing; empty for human writes.
func (c *Client) WriteArtifact(changeID, kind, content, runID string) (*Artifact, error) {
	var res struct {
		Artifact Artifact `json:"artifact"`
	}
	err := c.do(http.MethodPost, "/changes/"+changeID+"/artifacts/"+kind, map[string]string{
		"content": content,
		"run_id":  runID,
	}, &res)
	if err != nil {
		return nil, err
	}
	return &res.Artifact, nil
}

func (c *Client) GetChange(changeID string) (*Change, error) {
	var res struct {
		Change Change `json:"change"`
	}
	err := c.do(http.MethodGet, "/changes/"+changeID, nil, &res)
	if err != nil {
		return nil, err
	}
	return &res.Change, nil
}

func (c *Client) ListTasks(changeID string) ([]TaskMapping, error) {
	var res struct {
		Tasks []TaskMapping `json:"tasks"`
	}
	err := c.do(http.MethodGet, "/changes/"+changeID+"/tasks", nil, &res)
	if err != nil {
		return nil, err
	}
	return res.Tasks, nil
}

func (c *Client) GetGuard(changeID string) (*GuardReport, error) {
	var res struct {
		Guard GuardReport `json:"guard"`
	}
	err := c.do(http.MethodGet, "/changes/"+changeID+"/guard", nil, &res)
	if err != nil {
		return nil, err
	}
	return &res.Guard, nil
}

func (c *Client) AdvanceGuard(changeID string) (*Change, *Handoff, error) {
	var res struct {
		Change  Change  `json:"change"`
		Handoff Handoff `json:"handoff"`
	}
	err := c.do(http.MethodPost, "/changes/"+changeID+"/guard", nil, &res)
	if err != nil {
		return nil, nil, err
	}
	return &res.Change, &res.Handoff, nil
}

// SubmitVerify stores a verify report (YAML) and returns the parsed result
// plus whether it passed.
func (c *Client) SubmitVerify(changeID, content string) (string, bool, error) {
	var res struct {
		Result string `json:"result"`
		Passed bool   `json:"passed"`
	}
	err := c.do(http.MethodPost, "/changes/"+changeID+"/verify", map[string]string{
		"content": content,
	}, &res)
	if err != nil {
		return "", false, err
	}
	return res.Result, res.Passed, nil
}

// SubmitVerifyReport stores a verify report and returns the created verify
// artifact (used by the runtime flow driver). runID records the producing
// run; empty for human writes.
func (c *Client) SubmitVerifyReport(changeID, content, runID string) (*Artifact, bool, error) {
	var res struct {
		Artifact Artifact `json:"artifact"`
		Passed   bool     `json:"passed"`
	}
	err := c.do(http.MethodPost, "/changes/"+changeID+"/verify", map[string]string{
		"content": content,
		"run_id":  runID,
	}, &res)
	if err != nil {
		return nil, false, err
	}
	return &res.Artifact, res.Passed, nil
}

func (c *Client) Archive(changeID string) (*Change, error) {
	var res struct {
		Change Change `json:"change"`
	}
	err := c.do(http.MethodPost, "/changes/"+changeID+"/archive", nil, &res)
	if err != nil {
		return nil, err
	}
	return &res.Change, nil
}
