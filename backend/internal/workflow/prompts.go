package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// PromptData feeds the phase prompt templates.
type PromptData struct {
	IssueTitle       string
	IssueDescription string
	ProjectContext   string
	// Artifacts holds the already-generated artifacts by kind; later phases
	// build on the earlier ones.
	Artifacts map[string]string
}

const defaultProposalTemplate = `You are a senior product engineer writing a feature proposal.

Write the PROPOSAL artifact for the issue below, as GitHub-flavored Markdown.

Issue title: {{.IssueTitle}}
Issue description:
{{.IssueDescription}}

Project context:
{{.ProjectContext}}

Cover: motivation, goal, scope (in and out), and acceptance criteria.
Write in the same language as the issue description.`

const defaultSpecsTemplate = `You are a senior product engineer writing a specification.

Based on the proposal below, write the SPECS artifact as GitHub-flavored Markdown.

Project context:
{{.ProjectContext}}

Proposal:
{{index .Artifacts "proposal"}}

Specify behavior precisely: inputs, outputs, rules, and error cases.
Write in the same language as the issue description.`

const defaultDesignTemplate = `You are a senior software architect writing a technical design.

Based on the proposal and specs below, write the DESIGN artifact as GitHub-flavored Markdown.

Project context:
{{.ProjectContext}}

Proposal:
{{index .Artifacts "proposal"}}

Specs:
{{index .Artifacts "specs"}}

Cover architecture, data model, API surface, and the key trade-offs.
Write in the same language as the issue description.`

const defaultTasksTemplate = `You are a tech lead breaking work into staged sub-issues.

Based on the artifacts below, write the TASKS artifact.

Project context:
{{.ProjectContext}}

Proposal:
{{index .Artifacts "proposal"}}

Specs:
{{index .Artifacts "specs"}}

Design:
{{index .Artifacts "design"}}

Requirements:
- Brief Markdown intro is fine, then one fenced block that MUST start with ` + "```json" + `
- The block contains {"tasks": [{"title": "...", "description": "...", "stage": 1}]}
- "stage" is a 1-based execution stage; tasks in the same stage may run in parallel, later stages depend on earlier ones
- "title" is a short imperative sub-issue title; "description" tells the implementer what to do
- Tasks must cover the whole design; keep each independently implementable

Write titles and descriptions in the same language as the issue description.`

func defaultTemplates() map[string]*template.Template {
	tpl := map[string]string{
		KindProposal: defaultProposalTemplate,
		KindSpecs:    defaultSpecsTemplate,
		KindDesign:   defaultDesignTemplate,
		KindTasks:    defaultTasksTemplate,
	}
	out := make(map[string]*template.Template, len(tpl))
	for kind, text := range tpl {
		out[kind] = template.Must(template.New(kind).Parse(text))
	}
	return out
}

// LoadTemplates returns the prompt templates: embedded defaults, with any
// <kind>.md file in dir overriding its kind. An empty dir keeps all defaults;
// a non-empty dir that cannot be read is an error.
func LoadTemplates(dir string) (map[string]*template.Template, error) {
	tpl := defaultTemplates()
	if dir == "" {
		return tpl, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read prompt dir: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		kind := strings.TrimSuffix(name, ".md")
		if !IsValidKind(kind) {
			continue
		}
		text, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read prompt template %s: %w", name, err)
		}
		parsed, err := template.New(kind).Parse(string(text))
		if err != nil {
			return nil, fmt.Errorf("parse prompt template %s: %w", name, err)
		}
		tpl[kind] = parsed
	}
	return tpl, nil
}

// RenderPrompt executes the kind's template against data.
func RenderPrompt(tpl map[string]*template.Template, kind string, data PromptData) (string, error) {
	t, ok := tpl[kind]
	if !ok {
		return "", fmt.Errorf("unknown artifact kind: %s", kind)
	}
	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("render %s prompt: %w", kind, err)
	}
	return sb.String(), nil
}

// ComposePrompt renders the default template for kind.
func ComposePrompt(kind string, data PromptData) (string, error) {
	return RenderPrompt(defaultTemplates(), kind, data)
}
