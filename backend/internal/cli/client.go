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
