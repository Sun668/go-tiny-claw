package engine_test

import (
	"context"
	"strings"
	"testing"

	ctxpkg "github.com/Sun668/go-tiny-claw/internal/context"
	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/Sun668/go-tiny-claw/internal/tools"
)

func TestRunSubStopsWhenSpawnLimitExceeded(t *testing.T) {
	p := &subUsageProvider{
		messages: []*schema.Message{{
			Role:    schema.RoleAssistant,
			Content: "found it",
		}},
	}
	eng := newStreamTestEngine(t, p)
	eng.MaxSubagents = 1

	if _, err := eng.RunSub(context.Background(), "first", tools.NewRegistry(), nil); err != nil {
		t.Fatalf("第一次派出应成功: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("第一次派出应打 1 次模型，实际: %d", p.calls)
	}

	_, err := eng.RunSub(context.Background(), "second", tools.NewRegistry(), nil)
	if err == nil {
		t.Fatal("超过派出次数应失败")
	}
	if !strings.Contains(err.Error(), "子智能体的数量上限") {
		t.Fatalf("错误信息应说明派出次数，实际: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("超限后不应再打模型，实际: %d", p.calls)
	}
}

func TestRunResetsSubagentSpawnCount(t *testing.T) {
	p := &subUsageProvider{
		messages: []*schema.Message{{
			Role:    schema.RoleAssistant,
			Content: "found it",
		}},
	}
	eng := newStreamTestEngine(t, p)
	eng.MaxSubagents = 1

	if _, err := eng.RunSub(context.Background(), "before", tools.NewRegistry(), nil); err != nil {
		t.Fatalf("第一次派出应成功: %v", err)
	}

	session := ctxpkg.NewSession("sub-limit-1", t.TempDir())
	session.Append(schema.Message{Role: schema.RoleUser, Content: "hi"})
	if err := eng.Run(context.Background(), session, nil); err != nil {
		t.Fatalf("Run 应清零派出计数: %v", err)
	}

	if _, err := eng.RunSub(context.Background(), "after", tools.NewRegistry(), nil); err != nil {
		t.Fatalf("新的 Run 清零后应能再派: %v", err)
	}
}
