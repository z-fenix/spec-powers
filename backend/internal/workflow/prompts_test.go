package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func samplePromptData() PromptData {
	return PromptData{
		IssueTitle:       "发布拆分器",
		IssueDescription: "按 classic 流程拆分",
		ProjectContext:   "项目背景：Go + React",
		Artifacts: map[string]string{
			KindProposal: "# Proposal\n做拆分器",
		},
	}
}

func TestComposePromptIncludesIssueAndContext(t *testing.T) {
	out, err := ComposePrompt(KindProposal, samplePromptData())
	if err != nil {
		t.Fatalf("ComposePrompt: %v", err)
	}
	if !strings.Contains(out, "发布拆分器") {
		t.Error("prompt should include the issue title")
	}
	if !strings.Contains(out, "按 classic 流程拆分") {
		t.Error("prompt should include the issue description")
	}
	if !strings.Contains(out, "Go + React") {
		t.Error("prompt should include the project context")
	}
}

func TestComposePromptIncludesPriorArtifacts(t *testing.T) {
	out, err := ComposePrompt(KindDesign, samplePromptData())
	if err != nil {
		t.Fatalf("ComposePrompt: %v", err)
	}
	if !strings.Contains(out, "做拆分器") {
		t.Error("design prompt should include the proposal artifact")
	}
}

func TestComposePromptTasksAsksForJSONBlock(t *testing.T) {
	out, err := ComposePrompt(KindTasks, samplePromptData())
	if err != nil {
		t.Fatalf("ComposePrompt: %v", err)
	}
	if !strings.Contains(out, "```json") {
		t.Error("tasks prompt should require a ```json block")
	}
}

func TestComposePromptEachPhaseHasTemplate(t *testing.T) {
	for _, kind := range []string{KindProposal, KindSpecs, KindDesign, KindTasks} {
		out, err := ComposePrompt(kind, samplePromptData())
		if err != nil {
			t.Fatalf("ComposePrompt(%s): %v", kind, err)
		}
		if strings.TrimSpace(out) == "" {
			t.Errorf("ComposePrompt(%s) produced empty prompt", kind)
		}
	}
}

func TestComposePromptUnknownKind(t *testing.T) {
	if _, err := ComposePrompt("nope", samplePromptData()); err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestComposePromptMissingPriorArtifactSection(t *testing.T) {
	data := samplePromptData()
	data.Artifacts = nil
	out, err := ComposePrompt(KindDesign, data)
	if err != nil {
		t.Fatalf("ComposePrompt: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("prompt should still compose without prior artifacts")
	}
}

func TestLoadTemplatesOverride(t *testing.T) {
	dir := t.TempDir()
	custom := "CUSTOM-TEMPLATE {{.IssueTitle}}"
	if err := os.WriteFile(filepath.Join(dir, "proposal.md"), []byte(custom), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	tpl, err := LoadTemplates(dir)
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	out, err := RenderPrompt(tpl, KindProposal, samplePromptData())
	if err != nil {
		t.Fatalf("RenderPrompt: %v", err)
	}
	if !strings.Contains(out, "CUSTOM-TEMPLATE 发布拆分器") {
		t.Errorf("override template not used, got %q", out)
	}
	// other kinds keep defaults
	specsOut, err := RenderPrompt(tpl, KindSpecs, samplePromptData())
	if err != nil {
		t.Fatalf("RenderPrompt(specs): %v", err)
	}
	if !strings.Contains(specsOut, "做拆分器") {
		t.Errorf("default specs template should still work, got %q", specsOut)
	}
}

func TestLoadTemplatesMissingDirIsError(t *testing.T) {
	if _, err := LoadTemplates(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected error for missing prompt dir")
	}
}

func TestComposePromptUsesDefaultsWhenNoOverride(t *testing.T) {
	tpl, err := LoadTemplates("")
	if err != nil {
		t.Fatalf("LoadTemplates(\"\"): %v", err)
	}
	out, err := RenderPrompt(tpl, KindProposal, samplePromptData())
	if err != nil {
		t.Fatalf("RenderPrompt: %v", err)
	}
	if !strings.Contains(out, "发布拆分器") {
		t.Error("default proposal template should include issue title")
	}
}
