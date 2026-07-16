// internal/provider/interface.go
package provider

import (
	"context"

	"github.com/Sun668/go-tiny-claw/internal/schema"
)

// LLMProvider defines the unified interface for communicating with large models
type LLMProvider interface {
	// Generate receives the current context history and available tools list, returns the model response
	Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error)
}

type StreamEventType string

const (
	StreamTextDelta StreamEventType = "text_delta"
	StreamCompleted StreamEventType = "completed"
	StreamError     StreamEventType = "error"
)

type StreamEvent struct {
	Type    StreamEventType
	Text    string
	Message *schema.Message
	Usage   *schema.Usage
	Err     error
}

type StreamingProvider interface {
	LLMProvider
	GenerateStream(ctx context.Context, messages []schema.Message, tools []schema.ToolDefinition) (<-chan StreamEvent, error)
}
