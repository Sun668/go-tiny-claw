package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type BaseTool interface {
	Name() string

	Definition() schema.ToolDefinition

	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// internal/tools/registry.go (续)

// Registry 定义了工具的注册与分发接口
type Registry interface {
	// Register 挂载一个新的工具到系统中
	Register(tool BaseTool)

	// GetAvailableTools 返回当前系统挂载的所有工具的 Schema，供 Main Loop 交给 Provider
	GetAvailableTools() []schema.ToolDefinition

	// Execute 实际路由并执行模型请求的工具调用
	Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult
}

// registryImpl 是 Registry 接口的默认实现
type registryImpl struct {
	// 使用 map 以工具的 Name 作为 Key 进行快速 O(1) 路由查找
	tools map[string]BaseTool
}

func NewRegistry() Registry {
	return &registryImpl{
		tools: make(map[string]BaseTool),
	}
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
	// 1. 路由查找：如果在注册表中找不到该工具，这是模型产生了幻觉，直接向模型抛出错误
	tool, exists := r.tools[call.Name]
	if !exists {
		errMsg := fmt.Sprintf("Error: 系统中不存在名为 '%s' 的工具。", call.Name)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true, // 标记为错误，模型看到后会尝试纠正
		}
	}

	// 2. 执行工具逻辑：将原始的 JSON 字节流直接丢给具体工具
	output, err := tool.Execute(ctx, call.Arguments)

	// 3. 封装结果：将执行结果或底层物理错误封装后返回给 Main Loop
	if err != nil {
		errMsg := fmt.Sprintf("Error executing %s: %v", call.Name, err)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true,
		}
	}

	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     output,
		IsError:    false,
	}
}
