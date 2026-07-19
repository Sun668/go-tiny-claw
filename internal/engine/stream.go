package engine

import (
	"context"
	"fmt"

	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/reporter"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

func (e *AgentEngine) generate(
	ctx context.Context,
	messages []schema.Message,
	tools []schema.ToolDefinition,
	rep reporter.Reporter,
	emitText bool,
) (*schema.Message, bool, error) {
	streamProvider, ok := e.provider.(provider.StreamingProvider)
	if !ok {
		message, err := e.provider.Generate(ctx, messages, tools)
		return message, false, err
	}

	events, err := streamProvider.GenerateStream(ctx, messages, tools)
	if err != nil {
		return nil, false, err
	}

	streamReporter, canStream := rep.(reporter.StreamReporter)
	emittedText := false
	var finalMessage *schema.Message

	for {
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()

		case event, ok := <-events:
			if !ok {
				if finalMessage == nil {
					return nil, false, fmt.Errorf("流式响应未返回最终消息")
				}
				return finalMessage, emittedText, nil
			}

			switch event.Type {
			case provider.StreamTextDelta:
				if emitText && canStream {
					streamReporter.OnTextDelta(ctx, event.Text)
					emittedText = true
				}

			case provider.StreamCompleted:
				finalMessage = event.Message
				if emitText && canStream {
					streamReporter.OnTextComplete(ctx)
				}

			case provider.StreamError:
				if event.Err == nil {
					return nil, false, fmt.Errorf("流式响应失败")
				}
				return nil, false, event.Err
			}
		}
	}
}
