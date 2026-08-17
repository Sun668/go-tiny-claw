package provider

import (
	"context"
	"errors"
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
	MaxBackoff  time.Duration
	CallTimeout time.Duration
}

func NewRetryingProvider(next LLMProvider) *RetryingProvider {
	return &RetryingProvider{
		next:        next,
		MaxAttempts: defaultRetryAttempts,
		Backoff:     defaultRetryBackoff,
		MaxBackoff:  2 * time.Second,
		CallTimeout: 60 * time.Second,
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

		attemptCtx, cancelAttempt := p.attemptContext(ctx)
		message, err := p.next.Generate(attemptCtx, messages, availableTools)
		cancelAttempt()
		if err == nil {
			return message, nil
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("模型调用超时: %w", err)
		}

		last = err
		if !p.shouldRetry(err) || i == attempts {
			return nil, err
		}
		if err := p.wait(ctx, i); err != nil {
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

		attemptCtx, cancelAttempt := p.attemptContext(ctx)
		inner, err := streamer.GenerateStream(attemptCtx, messages, tools)
		if err != nil {
			cancelAttempt()
			if ctx.Err() != nil {
				sendStreamEvent(ctx, out, StreamEvent{Type: StreamError, Err: ctx.Err()})
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				err = fmt.Errorf("模型调用超时: %w", err)
			}
			if p.shouldRetry(err) && i < attempts {
				if waitErr := p.wait(ctx, i); waitErr != nil {
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
		done := false

		func() {
			defer cancelAttempt()
			for event := range inner {
				if event.Type == StreamError {
					if ctx.Err() != nil {
						sendStreamEvent(ctx, out, StreamEvent{Type: StreamError, Err: ctx.Err()})
						done = true
						return
					}
					if errors.Is(event.Err, context.DeadlineExceeded) {
						event.Err = fmt.Errorf("模型调用超时: %w", event.Err)
					}
					if !forwarded && p.shouldRetry(event.Err) && i < attempts {
						retry = true
						return
					}
					sendStreamEvent(ctx, out, event)
					done = true
					return
				}

				if event.Type == StreamTextDelta || event.Type == StreamCompleted {
					forwarded = true
				}
				if !sendStreamEvent(ctx, out, event) {
					done = true
					return
				}
				if event.Type == StreamCompleted {
					done = true
					return
				}
			}
		}()

		if done {
			return
		}

		if retry {
			if waitErr := p.wait(ctx, i); waitErr != nil {
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
			if waitErr := p.wait(ctx, i); waitErr != nil {
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

func (p *RetryingProvider) maxBackoff() time.Duration {
	if p.MaxBackoff > 0 {
		return p.MaxBackoff
	}
	return 2 * time.Second
}

func (p *RetryingProvider) backoffDelay(failedAttempt int) time.Duration {
	if p.Backoff <= 0 || failedAttempt <= 0 {
		return 0
	}
	delay := p.Backoff
	for i := 1; i < failedAttempt; i++ {
		if delay > p.maxBackoff()/2 {
			return p.maxBackoff()
		}
		delay *= 2
	}
	if delay > p.maxBackoff() {
		return p.maxBackoff()
	}
	return delay
}

func (p *RetryingProvider) shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return ClassifyError(err) == ErrorKindRetryable
}

func (p *RetryingProvider) attemptContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if p.CallTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, p.CallTimeout)
}

func (p *RetryingProvider) wait(ctx context.Context, failedAttempt int) error {
	delay := p.backoffDelay(failedAttempt)
	if delay <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(delay)
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
