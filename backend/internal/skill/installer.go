package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cliUsageDoc is appended to every exported SKILL.md so an agent that
// installed the skill can operate the classic flow through the sp CLI.
const cliUsageDoc = `## sp CLI 使用说明

本技能配合 spec-powers 的 classic 流程使用，通过 sp CLI 操作当前 change：

- ` + "`sp open --issue <ID> [--manual]`" + `：为 issue 创建（或绑定）change，并绑定本地状态。
- ` + "`sp guard [--change ID]`" + `：查看当前阶段与门禁状态（phase_legal / can_advance / can_archive）。
- ` + "`sp artifact write <KIND> --file <PATH>`" + `：写入 proposal / specs / design / tasks 产物。
- ` + "`sp handoff [--change ID]`" + `：通过门禁后推进到下一阶段。
- ` + "`sp state record-check build --command CMD --exit-code N`" + `：记录构建检查。
- ` + "`sp state record-check verify --command CMD --exit-code N`" + `：记录验证检查并提交 verify 报告。
- ` + "`sp verify [--change ID] --file report.yaml`" + `：提交 YAML 验证报告（` + "`result: pass|fail`" + ` + ` + "`summary:`" + `）。
- ` + "`sp archive [--change ID]`" + `：归档 change，唤醒父 issue 验收。

严格遵守 TDD：先写测试并确认失败，再写实现，直到测试全部通过。`

// ExportSkill renders a skill as Claude Code skill content: SKILL.md
// frontmatter (name/description) followed by the instructions and the sp
// CLI usage appendix.
func ExportSkill(s *Skill) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + s.Key + "\n")
	b.WriteString("description: " + s.Description + "\n")
	b.WriteString("---\n\n")
	b.WriteString(s.Instructions)
	b.WriteString("\n\n")
	b.WriteString(cliUsageDoc)
	b.WriteString("\n")
	return b.String()
}

// ErrSkillExists reports that a skill directory already exists and the
// install was run without force.
var ErrSkillExists = errors.New("skill already installed")

// InstallSkills writes each skill as dir/<key>/SKILL.md. An existing target
// is refused unless force is set. Each written file is read back to verify
// the install.
func InstallSkills(dir string, skills []*Skill, force bool) ([]string, error) {
	var installed []string
	for _, s := range skills {
		target := filepath.Join(dir, s.Key)
		if _, err := os.Stat(target); err == nil {
			if !force {
				return installed, fmt.Errorf("%w: %s (pass --force to overwrite)", ErrSkillExists, s.Key)
			}
		}
		if err := os.MkdirAll(target, 0o755); err != nil {
			return installed, fmt.Errorf("create %s: %w", target, err)
		}
		content := ExportSkill(s)
		path := filepath.Join(target, "SKILL.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return installed, fmt.Errorf("write %s: %w", path, err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return installed, fmt.Errorf("verify %s: %w", path, err)
		}
		if string(b) != content {
			return installed, fmt.Errorf("verify %s: content mismatch after write", path)
		}
		installed = append(installed, s.Key)
	}
	return installed, nil
}

// ListInstalled scans dir for skill directories (a directory containing
// SKILL.md) and returns their keys sorted.
func ListInstalled(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var keys []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); err != nil {
			continue
		}
		keys = append(keys, e.Name())
	}
	sort.Strings(keys)
	return keys, nil
}

// UninstallSkill removes dir/<key>. It refuses when the directory does not
// exist or contains no SKILL.md, so non-skill directories are never touched.
func UninstallSkill(dir, key string) error {
	target := filepath.Join(dir, key)
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("skill %s is not installed: %w", key, err)
	}
	if _, err := os.Stat(filepath.Join(target, "SKILL.md")); err != nil {
		return fmt.Errorf("%s has no SKILL.md: refusing to remove a non-skill directory", target)
	}
	return os.RemoveAll(target)
}
