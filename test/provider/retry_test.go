package provider_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Sun668/go-tiny-claw/internal/provider"
	"github.com/Sun668/go-tiny-claw/internal/schema"
)

type stubProvider struct {
	err   error
	fails int
	calls int
}

func (p *stubProvider) Generate(
	_ context.Context,
	_ []schema.Message,
	_ []schema.ToolDefinition,
) (*schema.Message, error) {
	p.calls++
	if p.calls <= p.fails {
		return nil, p.err
	}
	return &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "ok",
	}, nil
}

func newRetry(next provider.LLMProvider) *provider.RetryingProvider {
	retry := provider.NewRetryingProvider(next)
	retry.Backoff = 0
	return retry
}

func TestRetryingProviderRetriesRetryableGenerate(t *testing.T) {
	stub := &stubProvider{
		err:   &provider.HTTPError{StatusCode: 429},
		fails: 2,
	}

	message, err := newRetry(stub).Generate(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("可重试错误在限额内应成功: %v", err)
	}
	if message == nil || message.Content != "ok" {
		t.Fatalf("应返回成功消息，实际: %+v", message)
	}
	if stub.calls != 3 {
		t.Fatalf("应调用 3 次，实际: %d", stub.calls)
	}
}

func TestRetryingProviderDoesNotRetryFatalGenerate(t *testing.T) {
	stub := &stubProvider{
		err:   &provider.HTTPError{StatusCode: 401},
		fails: 3,
	}

	_, err := newRetry(stub).Generate(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("不可重试错误应失败")
	}
	if stub.calls != 1 {
		t.Fatalf("不可重试错误只应调用 1 次，实际: %d", stub.calls)
	}
}

func TestRetryingProviderStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stub := &stubProvider{
		err:   &provider.HTTPError{StatusCode: 500},
		fails: 3,
	}

	first := true
	wrapped := &cancelAfterFirst{
		stub:   stub,
		cancel: cancel,
		first:  &first,
	}

	_, err := newRetry(wrapped).Generate(ctx, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消后应返回 context.Canceled，实际: %v", err)
	}
	if wrapped.stub.calls != 1 {
		t.Fatalf("取消后不应继续重试，实际调用: %d", wrapped.stub.calls)
	}
}

type cancelAfterFirst struct {
	stub   *stubProvider
	cancel context.CancelFunc
	first  *bool
}

func (p *cancelAfterFirst) Generate(
	ctx context.Context,
	messages []schema.Message,
	tools []schema.ToolDefinition,
) (*schema.Message, error) {
	message, err := p.stub.Generate(ctx, messages, tools)
	if *p.first {
		*p.first = false
		p.cancel()
	}
	return message, err
}

type streamCall struct {
	startErr error
	events   []provider.StreamEvent
}

type stubStreamProvider struct {
	calls []streamCall
	used  int
}

func (p *stubStreamProvider) Generate(
	context.Context,
	[]schema.Message,
	[]schema.ToolDefinition,
) (*schema.Message, error) {
	return nil, errors.New("不应走同步 Generate")
}

func (p *stubStreamProvider) GenerateStream(
	_ context.Context,
	_ []schema.Message,
	_ []schema.ToolDefinition,
) (<-chan provider.StreamEvent, error) {
	if p.used >= len(p.calls) {
		return nil, errors.New("没有更多流式调用")
	}
	call := p.calls[p.used]
	p.used++
	if call.startErr != nil {
		return nil, call.startErr
	}

	events := make(chan provider.StreamEvent, len(call.events))
	for _, event := range call.events {
		events <- event
	}
	close(events)
	return events, nil
}

func collectStream(t *testing.T, events <-chan provider.StreamEvent) []provider.StreamEvent {
	t.Helper()
	var collected []provider.StreamEvent
	for event := range events {
		collected = append(collected, event)
	}
	return collected
}

func TestRetryingProviderRetriesStreamErrorBeforeDelta(t *testing.T) {
	stub := &stubStreamProvider{
		calls: []streamCall{
			{events: []provider.StreamEvent{{
				Type: provider.StreamError,
				Err:  &provider.HTTPError{StatusCode: 429},
			}}},
			{events: []provider.StreamEvent{
				{Type: provider.StreamTextDelta, Text: "ok"},
				{Type: provider.StreamCompleted, Message: &schema.Message{
					Role:    schema.RoleAssistant,
					Content: "ok",
				}},
			}},
		},
	}

	events, err := newRetry(stub).GenerateStream(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("生成流失败: %v", err)
	}

	collected := collectStream(t, events)
	if len(collected) != 2 {
		t.Fatalf("重试成功后应只看到成功事件，实际: %+v", collected)
	}
	if collected[0].Type != provider.StreamTextDelta || collected[0].Text != "ok" {
		t.Fatalf("第一条应为成功 delta，实际: %+v", collected[0])
	}
	if collected[1].Type != provider.StreamCompleted {
		t.Fatalf("第二条应为 completed，实际: %+v", collected[1])
	}
	if stub.used != 2 {
		t.Fatalf("应重试一次，实际调用: %d", stub.used)
	}
}

func TestRetryingProviderDoesNotRetryStreamAfterDelta(t *testing.T) {
	stub := &stubStreamProvider{
		calls: []streamCall{
			{events: []provider.StreamEvent{
				{Type: provider.StreamTextDelta, Text: "hello"},
				{Type: provider.StreamError, Err: &provider.HTTPError{StatusCode: 429}},
			}},
			{events: []provider.StreamEvent{
				{Type: provider.StreamTextDelta, Text: "不应出现"},
			}},
		},
	}

	events, err := newRetry(stub).GenerateStream(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("生成流失败: %v", err)
	}

	collected := collectStream(t, events)
	if len(collected) != 2 {
		t.Fatalf("已吐字后不应重试，事件: %+v", collected)
	}
	if collected[0].Type != provider.StreamTextDelta || collected[0].Text != "hello" {
		t.Fatalf("应先转发已吐出的字，实际: %+v", collected[0])
	}
	if collected[1].Type != provider.StreamError {
		t.Fatalf("已吐字后的失败应原样转发，实际: %+v", collected[1])
	}
	if stub.used != 1 {
		t.Fatalf("已吐字后不应再次调用，实际: %d", stub.used)
	}
}

func TestRetryingProviderDoesNotRetryFatalStream(t *testing.T) {
	stub := &stubStreamProvider{
		calls: []streamCall{
			{startErr: &provider.HTTPError{StatusCode: 401}},
			{events: []provider.StreamEvent{{
				Type: provider.StreamTextDelta,
				Text: "不应出现",
			}}},
		},
	}

	events, err := newRetry(stub).GenerateStream(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("生成流失败: %v", err)
	}

	collected := collectStream(t, events)
	if len(collected) != 1 || collected[0].Type != provider.StreamError {
		t.Fatalf("不可重试的启动失败应变成 StreamError，实际: %+v", collected)
	}
	if stub.used != 1 {
		t.Fatalf("不可重试错误只应调用 1 次，实际: %d", stub.used)
	}
}

type blockingProvider struct {
	calls int
}

func (p *blockingProvider) Generate(
	ctx context.Context,
	_ []schema.Message,
	_ []schema.ToolDefinition,
) (*schema.Message, error) {
	p.calls++
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestRetryingProviderRetriesAttemptTimeout(t *testing.T) {
	stub := &blockingProvider{}
	retry := newRetry(stub)
	retry.MaxAttempts = 2
	retry.CallTimeout = 20 * time.Millisecond

	_, err := retry.Generate(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("单次调用超时用尽后应失败")
	}
	if !strings.Contains(err.Error(), "模型调用超时") {
		t.Fatalf("错误信息应说明模型调用超时，实际: %v", err)
	}
	if stub.calls != 2 {
		t.Fatalf("超时应重试，实际调用: %d", stub.calls)
	}
}

func TestRetryingProviderDoesNotRetryParentCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stub := &blockingProvider{}
	retry := newRetry(stub)
	retry.CallTimeout = time.Second

	_, err := retry.Generate(ctx, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("父 ctx 取消应返回 Canceled，实际: %v", err)
	}
	if stub.calls != 0 {
		t.Fatalf("父 ctx 已取消不应开打，实际调用: %d", stub.calls)
	}
}
