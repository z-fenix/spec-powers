package workflow

import (
	"strings"
	"testing"
)

const validTasksMarkdown = `# Tasks

Some prose explaining the plan.

` + "```json" + `
{
  "tasks": [
    {"title": "基础框架", "description": "搭建服务骨架", "stage": 1},
    {"title": "数据库迁移", "description": "建表脚本", "stage": 1},
    {"title": "看板页面", "description": "前端看板", "stage": 2}
  ]
}
` + "```" + `

Notes below.
`

func TestParseTasksValid(t *testing.T) {
	tasks, err := ParseTasks(validTasksMarkdown)
	if err != nil {
		t.Fatalf("ParseTasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("len = %d, want 3", len(tasks))
	}
	if tasks[0].Title != "基础框架" || tasks[0].Description != "搭建服务骨架" || tasks[0].Stage != 1 {
		t.Errorf("tasks[0] = %+v", tasks[0])
	}
	if tasks[2].Title != "看板页面" || tasks[2].Stage != 2 {
		t.Errorf("tasks[2] = %+v", tasks[2])
	}
}

func TestParseTasksStageDefaultsToOne(t *testing.T) {
	md := "```json\n{\"tasks\":[{\"title\":\"a\"}]}\n```"
	tasks, err := ParseTasks(md)
	if err != nil {
		t.Fatalf("ParseTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Stage != 1 {
		t.Errorf("tasks = %+v, want one task with stage 1", tasks)
	}
}

func TestParseTasksNoFence(t *testing.T) {
	if _, err := ParseTasks("# Tasks\n\nno block here"); err == nil {
		t.Fatal("expected error when no json fence present")
	}
}

func TestParseTasksInvalidJSON(t *testing.T) {
	md := "```json\n{\"tasks\": [ not json\n```"
	if _, err := ParseTasks(md); err == nil {
		t.Fatal("expected error on invalid json inside fence")
	}
}

func TestParseTasksEmptyList(t *testing.T) {
	md := "```json\n{\"tasks\": []}\n```"
	if _, err := ParseTasks(md); err == nil {
		t.Fatal("expected error on empty tasks list")
	}
}

func TestParseTasksBlankTitle(t *testing.T) {
	md := "```json\n{\"tasks\":[{\"title\":\"  \"}]}\n```"
	if _, err := ParseTasks(md); err == nil {
		t.Fatal("expected error on blank title")
	}
}

func TestParseTasksNegativeStage(t *testing.T) {
	md := "```json\n{\"tasks\":[{\"title\":\"a\",\"stage\":-1}]}\n```"
	if _, err := ParseTasks(md); err == nil {
		t.Fatal("expected error on negative stage")
	}
}

func TestParseTasksIgnoresNonJSONFences(t *testing.T) {
	md := "```go\nfmt.Println(\"json\")\n```"
	if _, err := ParseTasks(md); err == nil {
		t.Fatal("expected error when only non-json fences present")
	}
}

func TestParseTasksDescriptionOptional(t *testing.T) {
	md := "```json\n{\"tasks\":[{\"title\":\"a\",\"stage\":3}]}\n```"
	tasks, err := ParseTasks(md)
	if err != nil {
		t.Fatalf("ParseTasks: %v", err)
	}
	if tasks[0].Description != "" {
		t.Errorf("description = %q, want empty", tasks[0].Description)
	}
	if tasks[0].Stage != 3 {
		t.Errorf("stage = %d, want 3", tasks[0].Stage)
	}
}

func TestParseTasksCaseInsensitiveFenceTag(t *testing.T) {
	md := "```JSON\n{\"tasks\":[{\"title\":\"a\"}]}\n```"
	if _, err := ParseTasks(strings.ToLower(md)); err != nil {
		t.Fatalf("ParseTasks: %v", err)
	}
}
