package engine

import "github.com/Sun668/go-tiny-claw/internal/schema"

// EnsureToolObservations 为尚未写入的 ToolCall 补齐 Observation。
// 用于取消/中断路径：Assistant 消息若已带 ToolCalls 入库，必须成对补全，避免下一轮上下文断裂。
func EnsureToolObservations(
	toolCalls []schema.ToolCall,
	observationMsgs []schema.Message,
	content string,
) {
	for i, toolCall := range toolCalls {
		if i >= len(observationMsgs) {
			return
		}
		if observationMsgs[i].ToolCallID != "" {
			continue
		}
		observationMsgs[i] = schema.Message{
			Role:       schema.RoleUser,
			Content:    content,
			ToolCallID: toolCall.ID,
		}
	}
}
