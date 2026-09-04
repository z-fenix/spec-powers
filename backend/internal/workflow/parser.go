package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// TaskSpec is one sub-issue entry parsed from the tasks artifact.
type TaskSpec struct {
	Title       string
	Description string
	Stage       int
}

// tasksBlock is the JSON payload embedded in the tasks artifact's fenced
// json block.
type tasksBlock struct {
	Tasks []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Stage       int    `json:"stage"`
	} `json:"tasks"`
}

// ParseTasks extracts and validates the fenced json task list from a tasks
// artifact. Stage defaults to 1 when omitted; titles must be non-blank and
// stages non-negative.
func ParseTasks(content string) ([]TaskSpec, error) {
	block, ok := extractJSONFence(content)
	if !ok {
		return nil, errors.New("tasks artifact has no ```json block")
	}
	var parsed tasksBlock
	if err := json.Unmarshal([]byte(block), &parsed); err != nil {
		return nil, fmt.Errorf("tasks artifact json block is invalid: %w", err)
	}
	if len(parsed.Tasks) == 0 {
		return nil, errors.New("tasks artifact json block contains no tasks")
	}
	specs := make([]TaskSpec, 0, len(parsed.Tasks))
	for i, tk := range parsed.Tasks {
		if strings.TrimSpace(tk.Title) == "" {
			return nil, fmt.Errorf("task %d has a blank title", i+1)
		}
		if tk.Stage < 0 {
			return nil, fmt.Errorf("task %d has a negative stage", i+1)
		}
		stage := tk.Stage
		if stage == 0 {
			stage = 1
		}
		specs = append(specs, TaskSpec{
			Title:       strings.TrimSpace(tk.Title),
			Description: tk.Description,
			Stage:       stage,
		})
	}
	return specs, nil
}

// extractJSONFence returns the body of the first ```json fenced block.
func extractJSONFence(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			continue
		}
		lang := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), "```"))
		if !strings.EqualFold(lang, "json") {
			continue
		}
		var body []string
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "```") {
				return strings.Join(body, "\n"), true
			}
			body = append(body, lines[j])
		}
		return "", false // json fence never closed
	}
	return "", false
}
