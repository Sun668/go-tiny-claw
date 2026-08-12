package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/tools"
)

func TestReadSkillReturnsGuideAndCompanions(t *testing.T) {
	workDir := t.TempDir()
	skillDir := filepath.Join(workDir, ".claw", "skills", "commit")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("创建技能目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: commit
description: 创建符合规范的提交
---
用 HEREDOC 写 commit message。
`), 0644); err != nil {
		t.Fatalf("写入 SKILL.md 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "examples.md"), []byte("example"), 0644); err != nil {
		t.Fatalf("写入附件失败: %v", err)
	}

	tool := tools.NewReadSkillTool(workDir)
	output, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"commit"}`))
	if err != nil {
		t.Fatalf("读取技能失败: %v", err)
	}
	if !strings.Contains(output, "用 HEREDOC 写 commit message。") {
		t.Fatalf("应包含执行指南，实际: %s", output)
	}
	if !strings.Contains(output, ".claw/skills/commit/examples.md") {
		t.Fatalf("应列出附件路径，实际: %s", output)
	}
}

func TestReadSkillMissingReturnsChineseError(t *testing.T) {
	tool := tools.NewReadSkillTool(t.TempDir())
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"nope"}`))
	if err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("期望未找到技能的中文错误，实际: %v", err)
	}
}
