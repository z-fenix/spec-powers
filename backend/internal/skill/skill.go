// Package skill defines agent skills: instruction packages the agent
// runtime loads to drive the superpowers-style flow (brainstorm →
// write-plan → subagent-driven-development). Skills are markdown files with
// a small frontmatter block, embedded in the binary and served over the
// API; the flow order and the mapping from a change's phase to the next
// skill live here too.
package skill

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// Flow skill keys.
const (
	KeyBrainstorm                = "brainstorm"
	KeyWritePlan                 = "write-plan"
	KeySubagentDrivenDevelopment = "subagent-driven-development"
)

// Classic flow phase names mirrored from the workflow domain; kept here so
// the skill package stays dependency-free (workflow imports skill).
const (
	KindProposal = "proposal"
	KindSpecs    = "specs"
	KindDesign   = "design"
	KindTasks    = "tasks"
)

// Skill is one loadable instruction package.
type Skill struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Order        int    `json:"order"`
	Instructions string `json:"instructions"`
}

// Registry holds the known skills indexed by key.
type Registry struct {
	byKey   map[string]*Skill
	ordered []*Skill
}

//go:embed skills/*.md
var defaultFS embed.FS

// DefaultRegistry loads the skills embedded in the binary.
func DefaultRegistry() (*Registry, error) {
	return NewRegistry(defaultFS)
}

// NewRegistry loads every *.md file under skills/ in fsys. Files without a
// key in the frontmatter are rejected so a broken skill fails at startup.
func NewRegistry(fsys fs.FS) (*Registry, error) {
	entries, err := fs.Glob(fsys, "skills/*.md")
	if err != nil {
		return nil, fmt.Errorf("glob skills: %w", err)
	}
	reg := &Registry{byKey: map[string]*Skill{}}
	for _, name := range entries {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		s, err := parse(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if _, dup := reg.byKey[s.Key]; dup {
			return nil, fmt.Errorf("%s: duplicate skill key %q", name, s.Key)
		}
		reg.byKey[s.Key] = s
		reg.ordered = append(reg.ordered, s)
	}
	sort.SliceStable(reg.ordered, func(i, j int) bool {
		if reg.ordered[i].Order != reg.ordered[j].Order {
			return reg.ordered[i].Order < reg.ordered[j].Order
		}
		return reg.ordered[i].Key < reg.ordered[j].Key
	})
	return reg, nil
}

// List returns the skills in flow order.
func (r *Registry) List() []*Skill {
	out := make([]*Skill, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// Get returns the skill with the given key.
func (r *Registry) Get(key string) (*Skill, bool) {
	s, ok := r.byKey[key]
	return s, ok
}

// Parse parses skill frontmatter + body from raw content. Used by the
// registry for embedded skills and by remote import for downloaded ones.
func Parse(content string) (*Skill, error) {
	return parse(content)
}

// parse splits a skill file into frontmatter fields and the markdown body.
// The frontmatter is a leading --- delimited block of `key: value` lines.
func parse(content string) (*Skill, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing frontmatter")
	}
	end := -1
	fields := map[string]string{}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
		line := lines[i]
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("bad frontmatter line %q", line)
		}
		fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if end == -1 {
		return nil, fmt.Errorf("unterminated frontmatter")
	}
	s := &Skill{
		Key:          fields["key"],
		Name:         fields["name"],
		Description:  fields["description"],
		Instructions: strings.TrimSpace(strings.Join(lines[end+1:], "\n")),
	}
	if s.Key == "" {
		return nil, fmt.Errorf("frontmatter has no key")
	}
	if fields["order"] != "" {
		n, err := strconv.Atoi(fields["order"])
		if err != nil {
			return nil, fmt.Errorf("bad order %q", fields["order"])
		}
		s.Order = n
	}
	return s, nil
}

// NextForChange maps a change's phase and status to the key of the skill
// that should run next. A change that is not active (archived, failed) has
// no next skill, and neither does an unknown phase.
func NextForChange(phase, status string) string {
	if status != "active" {
		return ""
	}
	switch phase {
	case KindProposal:
		return KeyBrainstorm
	case KindSpecs, KindDesign:
		return KeyWritePlan
	case KindTasks:
		return KeySubagentDrivenDevelopment
	default:
		return ""
	}
}
