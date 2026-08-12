package context_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
)

func writeSkill(t *testing.T, workDir, dirName, content string) {
	t.Helper()
	skillDir := filepath.Join(workDir, ".claw", "skills", dirName)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("写入 SKILL.md 失败: %v", err)
	}
}

func TestSkillCatalogOmitsBody(t *testing.T) {
	workDir := t.TempDir()
	writeSkill(t, workDir, "commit", `---
name: commit
description: >-
  按仓库规范创建 git commit。
---
# 提交规范
永远不要把正文预加载进系统提示。
`)

	loader := ctxpkg.NewSkillLoader(workDir)
	catalog := loader.CatalogPrompt()

	if !strings.Contains(catalog, "**commit**") {
		t.Fatalf("目录应包含技能名，实际: %s", catalog)
	}
	if !strings.Contains(catalog, "按仓库规范创建 git commit。") {
		t.Fatalf("目录应包含触发条件，实际: %s", catalog)
	}
	if strings.Contains(catalog, "永远不要把正文预加载进系统提示") {
		t.Fatal("目录不能包含技能正文")
	}
	if !strings.Contains(catalog, "read_skill") {
		t.Fatal("目录应提示使用 read_skill 按需加载")
	}
}

func TestSkillLoadReturnsBodyAndCompanions(t *testing.T) {
	workDir := t.TempDir()
	writeSkill(t, workDir, "review", `---
name: review
description: 按团队规范审查代码
---
先看 diff，再给结论。
`)
	if err := os.WriteFile(
		filepath.Join(workDir, ".claw", "skills", "review", "reference.md"),
		[]byte("# 参考"),
		0644,
	); err != nil {
		t.Fatalf("写入附件失败: %v", err)
	}

	loader := ctxpkg.NewSkillLoader(workDir)
	skill, companions, err := loader.Load("review")
	if err != nil {
		t.Fatalf("加载技能失败: %v", err)
	}
	if !strings.Contains(skill.Body, "先看 diff，再给结论。") {
		t.Fatalf("应返回技能正文，实际: %s", skill.Body)
	}
	if len(companions) != 1 || companions[0] != ".claw/skills/review/reference.md" {
		t.Fatalf("附件列表错误: %v", companions)
	}
}

func TestSkillLoadMissingName(t *testing.T) {
	loader := ctxpkg.NewSkillLoader(t.TempDir())
	_, _, err := loader.Load("missing")
	if err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("期望未找到技能的中文错误，实际: %v", err)
	}
}
