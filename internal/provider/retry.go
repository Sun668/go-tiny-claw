package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/schema"
)

const (
	defaultRetryAttempts = 3
	defaultRetryBackoff  = 200 * time.Millisecond
)

type RetryingProvider struct {
	next        LLMProvider
	MaxAttempts int
	Backoff     time.Duration
}

func NewRetryingProvider(next LLMProvider) *RetryingProvider {
	return &RetryingProvider{
		next:        next,
		MaxAttempts: defaultRetryAttempts,
		Backoff:     defaultRetryBackoff,
	}
}

func (p *RetryingProvider) Generate(
	ctx context.Context,
	messages []schema.Message,
	availableTools []schema.ToolDefinition,
) (*schema.Message, error) {
	var last error
	attempts := p.attempts()

	for i := 1; i <= attempts; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		message, err := p.next.Generate(ctx, messages, availableTools)
		if err == nil {
			return message, nil
		}

		last = err
		if ClassifyError(err) != ErrorKindRetryable || i == attempts {
			return nil, err
		}
		if err := p.wait(ctx); err != nil {
			return nil, err
		}
	}

	return nil, last
}

func (p *RetryingProvider) GenerateStream(
	ctx context.Context,
	messages []schema.Message,
	tools []schema.ToolDefinition,
) (<-chan StreamEvent, error) {
	streamer, ok := p.next.(StreamingProvider)
	if !ok {
		message, err := p.Generate(ctx, messages, tools)
		if err != nil {
			return nil, err
		}
		events := make(chan StreamEvent, 1)
		events <- StreamEvent{
			Type:    StreamCompleted,
			Message: message,
			Usage:   message.Usage,
		}
		close(events)
		return events, nil
	}

	events := make(chan StreamEvent, 16)
	go func() {
		defer close(events)
		p.replayStream(ctx, streamer, messages, tools, events)
	}()
	return events, nil
}

func (p *RetryingProvider) replayStream(
	ctx context.Context,
	streamer StreamingProvider,
	messages []schema.Message,
	tools []schema.ToolDefinition,
	out chan<- StreamEvent,
) {
	attempts := p.attempts()

	for i := 1; i <= attempts; i++ {
		if err := ctx.Err(); err != nil {
			sendStreamEvent(ctx, out, StreamEvent{Type: StreamError, Err: err})
			return
		}

		inner, err := streamer.GenerateStream(ctx, messages, tools)
		if err != nil {
			if ClassifyError(err) == ErrorKindRetryable && i < attempts {
				if waitErr := p.wait(ctx); waitErr != nil {
					sendStreamEvent(ctx, out, StreamEvent{Type: StreamError, Err: waitErr})
					return
				}
				continue
			}
			sendStreamEvent(ctx, out, StreamEvent{Type: StreamError, Err: err})
			return
		}

		forwarded := false
		retry := false

		for event := range inner {
			if event.Type == StreamError {
				if !forwarded && ClassifyError(event.Err) == ErrorKindRetryable && i < attempts {
					retry = true
					break
				}
				sendStreamEvent(ctx, out, event)
				return
			}

			if event.Type == StreamTextDelta || event.Type == StreamCompleted {
				forwarded = true
			}
			if !sendStreamEvent(ctx, out, event) {
				return
			}
			if event.Type == StreamCompleted {
				return
			}
		}

		if retry {
			if waitErr := p.wait(ctx); waitErr != nil {
				sendStreamEvent(ctx, out, StreamEvent{Type: StreamError, Err: waitErr})
				return
			}
			continue
		}

		if forwarded {
			sendStreamEvent(ctx, out, StreamEvent{
				Type: StreamError,
				Err:  fmt.Errorf("流式响应未返回最终消息"),
			})
			return
		}

		if i < attempts {
			if waitErr := p.wait(ctx); waitErr != nil {
				sendStreamEvent(ctx, out, StreamEvent{Type: StreamError, Err: waitErr})
				return
			}
			continue
		}

		sendStreamEvent(ctx, out, StreamEvent{
			Type: StreamError,
			Err:  fmt.Errorf("流式响应未返回最终消息"),
		})
		return
	}
}

func (p *RetryingProvider) attempts() int {
	if p.MaxAttempts > 0 {
		return p.MaxAttempts
	}
	return defaultRetryAttempts
}

func (p *RetryingProvider) wait(ctx context.Context) error {
	if p.Backoff <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(p.Backoff)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var _ LLMProvider = (*RetryingProvider)(nil)
var _ StreamingProvider = (*RetryingProvider)(nil)
