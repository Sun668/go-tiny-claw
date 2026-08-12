package engine_test

import (
	"testing"

	"github.com/Sun668/go-tiny-claw/internal/engine"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

func TestEnsureToolObservationsKeepsCompletedAndFillsRest(t *testing.T) {
	toolCalls := []schema.ToolCall{
		{ID: "call-1", Name: "bash"},
		{ID: "call-2", Name: "write_file"},
		{ID: "call-3", Name: "bash"},
	}
	observationMsgs := []schema.Message{
		{
			Role:       schema.RoleUser,
			Content:    "已完成",
			ToolCallID: "call-1",
		},
		{},
		{},
	}

	engine.EnsureToolObservations(toolCalls, observationMsgs, "工具调用已取消")

	if observationMsgs[0].Content != "已完成" || observationMsgs[0].ToolCallID != "call-1" {
		t.Fatalf("已完成的 Observation 不应被覆盖: %+v", observationMsgs[0])
	}
	if observationMsgs[1].ToolCallID != "call-2" || observationMsgs[1].Content != "工具调用已取消" {
		t.Fatalf("第二个 Observation 应补取消: %+v", observationMsgs[1])
	}
	if observationMsgs[2].ToolCallID != "call-3" || observationMsgs[2].Content != "工具调用已取消" {
		t.Fatalf("第三个 Observation 应补取消: %+v", observationMsgs[2])
	}
}
