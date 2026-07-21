package reporter

import (
	"context"
)

type JSONReporter struct {
	sink EventSink
}

func NewJSONReporter(sink EventSink) *JSONReporter {
	return &JSONReporter{
		sink: sink,
	}
}

func (r *JSONReporter) publish(
	ctx context.Context,
	event Event,
) {
	_ = r.sink.Publish(ctx, event)
}

func (r *JSONReporter) OnThinking(ctx context.Context) {
	r.publish(ctx, Event{
		Type: EventThinking,
	})
}

func (r *JSONReporter) OnToolCall(
	ctx context.Context,
	toolName string,
	args string,
) {
	r.publish(ctx, Event{
		Type:     EventToolCall,
		ToolName: toolName,
		Content:  args,
	})
}

func (r *JSONReporter) OnToolResult(
	ctx context.Context,
	toolName string,
	result string,
	isError bool,
) {
	r.publish(ctx, Event{
		Type:     EventToolResult,
		ToolName: toolName,
		Result:   result,
		IsError:  isError,
	})
}

func (r *JSONReporter) OnMessage(
	ctx context.Context,
	content string,
) {
	r.publish(ctx, Event{
		Type:    EventTextCompleted,
		Content: content,
	})
}

func (r *JSONReporter) OnTextDelta(
	ctx context.Context,
	delta string,
) {
	r.publish(ctx, Event{
		Type:    EventTextDelta,
		Content: delta,
	})
}

func (r *JSONReporter) OnTextComplete(ctx context.Context) {
	r.publish(ctx, Event{
		Type: EventTextCompleted,
	})
}
