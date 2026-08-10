package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/Sun668/go-tiny-claw/internal/approval"
	"github.com/Sun668/go-tiny-claw/internal/observability"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type BaseTool interface {
	Name() string
	Definition() schema.ToolDefinition
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}
type MiddlewareFunc func(ctx context.Context, call schema.ToolCall) (allowed bool, rejectReason string)

type Registry interface {
	Register(tool BaseTool)
	Use(middleware MiddlewareFunc)
	GetAvailableTools() []schema.ToolDefinition
	Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult
	GetRiskLevel(name string) approval.RiskLevel
}

type registryImpl struct {
	tools       map[string]BaseTool
	middlewares []MiddlewareFunc
}

func NewRegistry() Registry {
	return &registryImpl{
		tools:       make(map[string]BaseTool),
		middlewares: make([]MiddlewareFunc, 0),
	}
}

func (r *registryImpl) Use(middleware MiddlewareFunc) {
	r.middlewares = append(r.middlewares, middleware)
}

func (r *registryImpl) Register(tool BaseTool) {
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		log.Printf("[Warning] 工具 '%s' 已经被注册，将被覆盖。\n", name)
	}
	r.tools[name] = tool
	log.Printf("[Registry] 成功挂载工具: %s\n", name)
}

func (r *registryImpl) GetAvailableTools() []schema.ToolDefinition {
	var defs []schema.ToolDefinition
	for _, tool := range r.tools {
		defs = append(defs, tool.Definition())
	}
	return defs
}

func (r *registryImpl) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	ctx, span := observability.StartSpan(ctx, "Tool.Execute")
	span.AddAttribute("tool_name", call.Name)
	span.AddAttribute("arguments", string(call.Arguments))
	defer span.EndSpan() // 结束工具执行跨度

	tool, exists := r.tools[call.Name]
	if !exists {
		errMsg := fmt.Sprintf("Error: 系统中不存在名为 '%s' 的工具。", call.Name)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true,
		}
	}

	for _, middleware := range r.middlewares {
		allowed, rejectReason := middleware(ctx, call)
		if !allowed {
			log.Printf("[Registry] ⚠️ 工具 %s 被 Middleware 拦截: %s\n", call.Name, rejectReason)
			return schema.ToolResult{
				ToolCallID: call.ID,
				Output:     rejectReason,
				IsError:    true,
			}
		}
	}

	output, err := tool.Execute(ctx, call.Arguments)

	if err != nil {
		// 父 ctx 取消 / Run 超时：保留中断语义，Engine 会检查 ctx 后中止，不把半截结果当成功 Observation。
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return schema.ToolResult{
				ToolCallID: call.ID,
				Output:     "工具执行已中断",
				IsError:    true,
			}
		}

		// 工具级失败（含 ErrToolTimeout）：优先使用工具返回的文案写入 Observation。
		msg := output
		if msg == "" {
			msg = fmt.Sprintf("执行工具 %s 失败: %v", call.Name, err)
		}

		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     msg,
			IsError:    true,
		}
	}

	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     output,
		IsError:    false,
	}
}

func (r *registryImpl) GetRiskLevel(name string) approval.RiskLevel {
	tool, exists := r.tools[name]
	if !exists {
		return approval.RiskDangerous
	}

	riskedTool, ok := tool.(RiskedTool)
	if !ok {
		return approval.RiskDangerous
	}

	return riskedTool.RiskLevel()
}
