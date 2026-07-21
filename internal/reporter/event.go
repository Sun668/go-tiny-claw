package reporter

import "context"

type EventType string

const (
	EventThinking        EventType = "thinking"
	EventTextDelta       EventType = "text_delta"
	EventTextCompleted   EventType = "text_completed"
	EventToolCall        EventType = "tool_call"
	EventToolResult      EventType = "tool_result"
	EventTaskCompleted   EventType = "task_completed"
	EventTaskCanceled    EventType = "task_canceled"
	EventTaskFailed      EventType = "task_failed"
	EventError           EventType = "error"
	EventPong            EventType = "pong"
	EventApprovalRequest EventType = "approval_request"
)

type Event struct {
	Type      EventType `json:"type"`
	Content   string    `json:"content,omitempty"`
	ToolName  string    `json:"tool_name,omitempty"`
	Result    string    `json:"result,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
	Decision  string    `json:"decision,omitempty"`
	Risk      string    `json:"risk,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	IsError   bool      `json:"is_error,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type EventSink interface {
	Publish(ctx context.Context, event Event) error
}
