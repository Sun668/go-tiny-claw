package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Sun668/go-tiny-claw/internal/schema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

type OpenAIProvider struct {
	client openai.Client
	model  string
}

func NewOpenAICompatibleProvider(model string) *OpenAIProvider {
	apiKey := os.Getenv("ZHIPU_API_KEY")
	if apiKey == "" {
		panic("请设置 ZHIPU_API_KEY 环境变量")
	}
	baseURL := os.Getenv("ZHIPU_BASE_URL")
	return &OpenAIProvider{
		client: openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseURL)),
		model:  model,
	}
}

func WrapOpenAIError(prefix string, err error) error {
	if err == nil {
		return nil
	}

	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		err = &HTTPError{
			StatusCode: apiErr.StatusCode,
			Err:        err,
		}
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func (p *OpenAIProvider) buildParams(msgs []schema.Message, availableTools []schema.ToolDefinition) (openai.ChatCompletionNewParams, error) {
	var openaiMsgs []openai.ChatCompletionMessageParamUnion

	for _, msg := range msgs {
		switch msg.Role {
		case schema.RoleSystem:
			openaiMsgs = append(openaiMsgs, openai.SystemMessage(msg.Content))

		case schema.RoleUser:
			if msg.ToolCallID != "" {
				openaiMsgs = append(openaiMsgs, openai.ToolMessage(msg.Content, msg.ToolCallID))
			} else {
				openaiMsgs = append(openaiMsgs, openai.UserMessage(msg.Content))
			}
		case schema.RoleAssistant:
			astParam := openai.ChatCompletionAssistantMessageParam{}

			// 即使是空字符串 ""，也要发给智谱，否则会触发 1214 错误码
			astParam.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: openai.String(msg.Content),
			}

			if len(msg.ToolCalls) > 0 {
				var toolCalls []openai.ChatCompletionMessageToolCallUnionParam
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID:   tc.ID,
							Type: "function",
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: string(tc.Arguments),
							},
						},
					})
				}
				astParam.ToolCalls = toolCalls
			}

			openaiMsgs = append(openaiMsgs, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &astParam,
			})
		}
	}

	// v3 新 API：ChatCompletionToolUnionParam + ChatCompletionFunctionTool()
	var openaiTools []openai.ChatCompletionToolUnionParam
	for _, toolDef := range availableTools {
		var params shared.FunctionParameters
		if m, ok := toolDef.InputSchema.(map[string]interface{}); ok {
			params = shared.FunctionParameters(m)
		} else {
			b, _ := json.Marshal(toolDef.InputSchema)
			_ = json.Unmarshal(b, &params)
		}

		openaiTools = append(openaiTools, openai.ChatCompletionFunctionTool(
			shared.FunctionDefinitionParam{
				Name:        toolDef.Name,
				Description: openai.String(toolDef.Description),
				Parameters:  params,
			},
		))
	}

	params := openai.ChatCompletionNewParams{
		Model:    p.model,
		Messages: openaiMsgs,
	}
	if len(openaiTools) > 0 {
		params.Tools = openaiTools
	}
	return params, nil
}

func (p *OpenAIProvider) Generate(ctx context.Context, msgs []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	params, err := p.buildParams(msgs, availableTools)
	if err != nil {
		return nil, fmt.Errorf("构建 OpenAI 参数失败: %w", err)
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, WrapOpenAIError("OpenAI/Zhipu API 请求失败", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("API 返回了空的 Choices")
	}

	choice := resp.Choices[0].Message
	resultMsg := &schema.Message{
		Role:    schema.RoleAssistant,
		Content: choice.Content,
	}

	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		resultMsg.Usage = &schema.Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
		}
	}

	for _, tc := range choice.ToolCalls {
		if tc.Type == "function" {
			resultMsg.ToolCalls = append(resultMsg.ToolCalls, schema.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: []byte(tc.Function.Arguments),
			})
		}
	}

	return resultMsg, nil
}

func (p *OpenAIProvider) GenerateStream(
	ctx context.Context,
	msgs []schema.Message,
	availableTools []schema.ToolDefinition,
) (<-chan StreamEvent, error) {
	params, err := p.buildParams(msgs, availableTools)
	if err != nil {
		return nil, err
	}

	params.StreamOptions.IncludeUsage = openai.Bool(true)

	events := make(chan StreamEvent, 16)

	go func() {
		defer close(events)

		stream := p.client.Chat.Completions.NewStreaming(ctx, params)
		defer stream.Close()

		var content strings.Builder
		toolCalls := make([]*toolCallAccumulator, 0)
		var usage *schema.Usage

		for stream.Next() {
			chunk := stream.Current()

			if chunk.Usage.PromptTokens > 0 ||
				chunk.Usage.CompletionTokens > 0 {
				usage = &schema.Usage{
					PromptTokens:     int(chunk.Usage.PromptTokens),
					CompletionTokens: int(chunk.Usage.CompletionTokens),
				}
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta

			if delta.Content != "" {
				content.WriteString(delta.Content)

				if !sendStreamEvent(ctx, events, StreamEvent{
					Type: StreamTextDelta,
					Text: delta.Content,
				}) {
					return
				}
			}

			for _, deltaToolCall := range delta.ToolCalls {
				index := int(deltaToolCall.Index)

				for len(toolCalls) <= index {
					toolCalls = append(toolCalls, nil)
				}

				if toolCalls[index] == nil {
					toolCalls[index] = &toolCallAccumulator{}
				}

				accumulator := toolCalls[index]

				if deltaToolCall.ID != "" {
					accumulator.id = deltaToolCall.ID
				}

				if deltaToolCall.Function.Name != "" {
					accumulator.name = deltaToolCall.Function.Name
				}

				if deltaToolCall.Function.Arguments != "" {
					accumulator.arguments.WriteString(
						deltaToolCall.Function.Arguments,
					)
				}
			}
		}

		if err := stream.Err(); err != nil {
			if !sendStreamEvent(ctx, events, StreamEvent{
				Type: StreamError,
				Err:  WrapOpenAIError("流式响应失败", err),
			}) {
				return
			}
			return
		}

		finalMessage := &schema.Message{
			Role:    schema.RoleAssistant,
			Content: content.String(),
			Usage:   usage,
		}

		for _, accumulator := range toolCalls {
			if accumulator == nil {
				continue
			}

			arguments := accumulator.arguments.String()
			if arguments == "" {
				arguments = "{}"
			}

			if !json.Valid([]byte(arguments)) {
				sendStreamEvent(ctx, events, StreamEvent{
					Type: StreamError,
					Err: fmt.Errorf(
						"工具 %s 返回非法 JSON 参数: %s",
						accumulator.name,
						arguments,
					),
				})
				return
			}

			finalMessage.ToolCalls = append(
				finalMessage.ToolCalls,
				schema.ToolCall{
					ID:        accumulator.id,
					Name:      accumulator.name,
					Arguments: json.RawMessage(arguments),
				},
			)
		}

		if !sendStreamEvent(ctx, events, StreamEvent{
			Type:    StreamCompleted,
			Message: finalMessage,
			Usage:   usage,
		}) {
			return
		}
	}()

	return events, nil
}

type toolCallAccumulator struct {
	id        string
	name      string
	arguments strings.Builder
}

func sendStreamEvent(
	ctx context.Context,
	events chan<- StreamEvent,
	event StreamEvent,
) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
