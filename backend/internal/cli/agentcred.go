package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// homeDir resolves the user's home directory; overridden in tests.
var homeDir = os.UserHomeDir

// spDir is the agent credential directory under the user's home.
const spDir = ".sp"

// AgentCredential is a locally registered agent's runtime credential:
// the bearer token its runtime presents when claiming and executing runs.
type AgentCredential struct {
	Server    string `json:"server"`
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	Token     string `json:"token"`
	SavedAt   string `json:"saved_at,omitempty"`
}

func agentCredDir() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, spDir, "agents"), nil
}

func agentCredPath(name string) (string, error) {
	dir, err := agentCredDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

// SaveAgentCredential writes the credential to ~/.sp/agents/<name>.json
// with owner-only permissions (it contains a bearer token).
func SaveAgentCredential(name string, cred AgentCredential) error {
	path, err := agentCredPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if cred.SavedAt == "" {
		cred.SavedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// LoadAgentCredential reads one stored credential.
func LoadAgentCredential(name string) (AgentCredential, error) {
	var cred AgentCredential
	path, err := agentCredPath(name)
	if err != nil {
		return cred, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cred, fmt.Errorf("no registered agent %q: run `sp agent register --name NAME`", name)
		}
		return cred, err
	}
	if err := json.Unmarshal(b, &cred); err != nil {
		return cred, fmt.Errorf("parse %s: %w", path, err)
	}
	return cred, nil
}

// ListAgentCredentialNames returns the names of the locally registered
// agents, sorted.
func ListAgentCredentialNames() ([]string, error) {
	dir, err := agentCredDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-len(".json")])
		}
	}
	sort.Strings(names)
	return names, nil
}

// DeleteAgentCredential removes one stored credential; removing a missing
// one is an error so `sp agent deregister` cannot silently no-op.
func DeleteAgentCredential(name string) error {
	path, err := agentCredPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no registered agent %q: run `sp agent register --name NAME`", name)
		}
		return err
	}
	return nil
}

// resolveAgentCredential picks the credential to run with: an explicit name
// wins; otherwise exactly one stored credential is used unambiguous.
func resolveAgentCredential(name string) (AgentCredential, error) {
	if name != "" {
		return LoadAgentCredential(name)
	}
	names, err := ListAgentCredentialNames()
	if err != nil {
		return AgentCredential{}, err
	}
	switch len(names) {
	case 1:
		return LoadAgentCredential(names[0])
	case 0:
		return AgentCredential{}, fmt.Errorf("no registered agent on this machine: run `sp agent register`")
	default:
		return AgentCredential{}, fmt.Errorf("multiple registered agents (%s): pass --name", strings.Join(names, ", "))
	}
}
