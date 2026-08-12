package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type ReadSkillTool struct {
	loader *ctxpkg.SkillLoader
}

func NewReadSkillTool(workDir string) *ReadSkillTool {
	return &ReadSkillTool{
		loader: ctxpkg.NewSkillLoader(workDir),
	}
}

func (t *ReadSkillTool) Name() string {
	return "read_skill"
}

func (t *ReadSkillTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "读取指定 Agent Skill 的完整执行指南。当用户任务匹配某个技能的触发条件时，必须先调用本工具，再按指南执行。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "技能名称，与系统提示中列出的技能名一致",
				},
			},
			"required": []string{"name"},
		},
	}
}

type readSkillArgs struct {
	Name string `json:"name"`
}

func (t *ReadSkillTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input readSkillArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	skill, companions, err := t.loader.Load(input.Name)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# 技能: %s\n", skill.Name))
	b.WriteString(fmt.Sprintf("触发条件: %s\n", skill.Description))
	b.WriteString(fmt.Sprintf("路径: %s\n\n", skill.RelPath))
	b.WriteString("## 执行指南\n")
	b.WriteString(skill.Body)

	if len(companions) > 0 {
		b.WriteString("\n\n## 同目录附件\n需要时使用 read_file 读取：\n")
		for _, path := range companions {
			b.WriteString(fmt.Sprintf("- %s\n", path))
		}
	}

	return b.String(), nil
}

func (t *ReadSkillTool) RiskLevel() approval.RiskLevel {
	return approval.RiskSafe
}
