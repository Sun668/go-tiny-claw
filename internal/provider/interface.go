package provider

import (
	"context"

	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type LLMProvider interface {
	Generate(ctx context.Context, messsages []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error)
}
