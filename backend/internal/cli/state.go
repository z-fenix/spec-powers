package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// specDir is the local workspace runtime directory, relative to cwd.
const specDir = ".specpower"

// State is the workspace cache binding this directory to a change, plus the
// recorded command checks.
type State struct {
	ProjectID string  `json:"project_id,omitempty"`
	IssueID   string  `json:"issue_id,omitempty"`
	ChangeID  string  `json:"change_id,omitempty"`
	Phase     string  `json:"phase,omitempty"`
	Status    string  `json:"status,omitempty"`
	Checks    []Check `json:"checks,omitempty"`
	UpdatedAt string  `json:"updated_at,omitempty"`
}

// Check is one recorded command execution (sp state record-check).
type Check struct {
	Scope      string `json:"scope"` // build | verify
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	Cwd        string `json:"cwd,omitempty"`
	RecordedAt string `json:"recorded_at"`
}

// Session caches the authentication token and the server it belongs to.
type Session struct {
	Server string `json:"server"`
	Token  string `json:"token,omitempty"`
	Email  string `json:"email,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

func loadJSON(name string, out any) error {
	b, err := os.ReadFile(filepath.Join(specDir, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func saveJSON(name string, v any) error {
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(specDir, name), append(b, '\n'), 0o644)
}

// LoadState reads .specpower/state.json; a missing file yields the zero
// State with no error.
func LoadState() (State, error) {
	var s State
	err := loadJSON("state.json", &s)
	return s, err
}

// SaveState writes .specpower/state.json.
func SaveState(s State) error {
	return saveJSON("state.json", &s)
}

// LoadSession reads .specpower/session.json; a missing file yields the zero
// Session with no error.
func LoadSession() (Session, error) {
	var s Session
	err := loadJSON("session.json", &s)
	return s, err
}

// SaveSession writes .specpower/session.json.
func SaveSession(s Session) error {
	return saveJSON("session.json", &s)
}
