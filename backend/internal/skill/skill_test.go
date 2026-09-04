package skill

import (
	"strings"
	"testing"
	"testing/fstest"
)

// ---- fakes ----

func mapFS(files map[string]string) fstest.MapFS {
	fs := fstest.MapFS{}
	for name, content := range files {
		fs[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fs
}

const sampleMD = `---
key: demo-skill
name: Demo Skill
description: a demo skill for tests
order: 2
---

# Demo

Step one.
`

// ---- Registry ----

func TestRegistryParsesFrontmatterAndBody(t *testing.T) {
	reg, err := NewRegistry(mapFS(map[string]string{"skills/demo.md": sampleMD}))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	s, ok := reg.Get("demo-skill")
	if !ok {
		t.Fatal("Get(demo-skill) not found")
	}
	if s.Key != "demo-skill" {
		t.Errorf("Key = %q, want demo-skill", s.Key)
	}
	if s.Name != "Demo Skill" {
		t.Errorf("Name = %q, want Demo Skill", s.Name)
	}
	if s.Description != "a demo skill for tests" {
		t.Errorf("Description = %q, want a demo skill for tests", s.Description)
	}
	if s.Order != 2 {
		t.Errorf("Order = %d, want 2", s.Order)
	}
	if !strings.Contains(s.Instructions, "# Demo") || !strings.Contains(s.Instructions, "Step one.") {
		t.Errorf("Instructions = %q, want the markdown body", s.Instructions)
	}
	if strings.Contains(s.Instructions, "key: demo-skill") {
		t.Errorf("Instructions must not contain the frontmatter, got %q", s.Instructions)
	}
}

func TestRegistryGetUnknownKey(t *testing.T) {
	reg, err := NewRegistry(mapFS(map[string]string{"skills/demo.md": sampleMD}))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if _, ok := reg.Get("nope"); ok {
		t.Error("Get(nope) should not be found")
	}
}

func TestRegistryListsInFlowOrder(t *testing.T) {
	reg, err := NewRegistry(mapFS(map[string]string{
		"skills/b.md": strings.Replace(sampleMD, "demo-skill", "beta", 1),
		"skills/a.md": strings.Replace(sampleMD, "demo-skill", "alpha", 1),
	}))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	// both share order 2; stable order keeps the flow deterministic
	list := reg.List()
	if len(list) != 2 {
		t.Fatalf("List() length = %d, want 2", len(list))
	}
	for _, s := range list {
		if s.Key != "beta" && s.Key != "alpha" {
			t.Errorf("unexpected key %q", s.Key)
		}
	}
}

func TestRegistryRejectsSkillWithoutKey(t *testing.T) {
	bad := strings.Replace(sampleMD, "key: demo-skill\n", "", 1)
	if _, err := NewRegistry(mapFS(map[string]string{"skills/demo.md": bad})); err == nil {
		t.Error("NewRegistry should reject a skill without a key")
	}
}

// ---- default registry (embedded skills) ----

func TestDefaultRegistryHasCoreFlow(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	want := []string{KeyBrainstorm, KeyWritePlan, KeySubagentDrivenDevelopment}
	list := reg.List()
	if len(list) != len(want) {
		t.Fatalf("List() length = %d, want %d", len(list), len(want))
	}
	for i, key := range want {
		if list[i].Key != key {
			t.Errorf("List()[%d].Key = %q, want %q", i, list[i].Key, key)
		}
		if list[i].Name == "" || list[i].Description == "" {
			t.Errorf("skill %q missing name/description", key)
		}
		if len(list[i].Instructions) < 50 {
			t.Errorf("skill %q instructions too short: %d bytes", key, len(list[i].Instructions))
		}
	}
	// flow order must be strictly increasing
	for i := 1; i < len(list); i++ {
		if list[i].Order <= list[i-1].Order {
			t.Errorf("order not increasing: %q(%d) before %q(%d)", list[i-1].Key, list[i-1].Order, list[i].Key, list[i].Order)
		}
	}
}

func TestDefaultRegistryGet(t *testing.T) {
	reg, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	if _, ok := reg.Get(KeyWritePlan); !ok {
		t.Errorf("Get(%q) not found", KeyWritePlan)
	}
}

// ---- flow selection ----

func TestNextForChange(t *testing.T) {
	cases := []struct {
		phase, status, want string
	}{
		{KindProposal, "active", KeyBrainstorm},
		{KindSpecs, "active", KeyWritePlan},
		{KindDesign, "active", KeyWritePlan},
		{KindTasks, "active", KeySubagentDrivenDevelopment},
		{KindTasks, "archived", ""},
		{KindTasks, "failed", ""},
		{"mystery", "active", ""},
	}
	for _, tc := range cases {
		if got := NextForChange(tc.phase, tc.status); got != tc.want {
			t.Errorf("NextForChange(%q, %q) = %q, want %q", tc.phase, tc.status, got, tc.want)
		}
	}
}
