package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AnthropicProvider struct {
	apiKey  string
	model   string
	baseURL string
}

type anthropicContent struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Input   any    `json:"input,omitempty"`
	ToolID  string `json:"tool_use_id,omitempty"`
	Content any    `json:"content,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicRequest struct {
	Model       string               `json:"model"`
	MaxTokens   int                  `json:"max_tokens"`
	System      string               `json:"system,omitempty"`
	Messages    []anthropicMessage   `json:"messages"`
	Tools       []anthropicToolSchema `json:"tools,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResponse struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
	Usage   anthropicUsage    `json:"usage"`
	StopReason string         `json:"stop_reason"`
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 10)

	body := p.buildRequest(req)

	go func() {
		defer close(ch)

		resp, err := p.postRequest(ctx, body)
		if err != nil {
			ch <- StreamEvent{Type: StreamError, Error: fmt.Errorf("anthropic http: %w", err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			raw, _ := io.ReadAll(resp.Body)
			ch <- StreamEvent{Type: StreamError, Error: fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(raw))}
			return
		}

		var toolID, toolName string
		var toolArgs strings.Builder

		dec := NewSSEDecoder(resp.Body)
		for {
			event, data, err := dec.DecodeEvent()
			if err == io.EOF {
				break
			}
			if err != nil {
				ch <- StreamEvent{Type: StreamError, Error: fmt.Errorf("anthropic sse: %w", err)}
				return
			}

			switch event {
			case "content_block_start":
				var cbs struct {
					Index int `json:"index"`
					ContentBlock struct {
						Type string `json:"type"`
						Text string `json:"text"`
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"content_block"`
				}
				json.Unmarshal(data, &cbs)
				if cbs.ContentBlock.Type == "text" {
					if cbs.ContentBlock.Text != "" {
						ch <- StreamEvent{Type: StreamChunk, Text: cbs.ContentBlock.Text}
					}
				} else if cbs.ContentBlock.Type == "tool_use" {
					toolID = cbs.ContentBlock.ID
					toolName = cbs.ContentBlock.Name
					toolArgs.Reset()
				}

			case "content_block_delta":
				var cbd struct {
					Index int `json:"index"`
					Delta struct {
						Type        string `json:"type"`
						Text        string `json:"text"`
						PartialJSON string `json:"partial_json"`
					} `json:"delta"`
				}
				json.Unmarshal(data, &cbd)
				if cbd.Delta.Type == "text_delta" {
					ch <- StreamEvent{Type: StreamChunk, Text: cbd.Delta.Text}
				} else if cbd.Delta.Type == "input_json_delta" {
					toolArgs.WriteString(cbd.Delta.PartialJSON)
				}

			case "content_block_stop":
				if toolID != "" && toolName != "" {
					args := json.RawMessage(toolArgs.String())
					if len(args) > 0 && args[0] == '"' {
						var s string
						if err := json.Unmarshal(args, &s); err == nil {
							args = json.RawMessage(s)
						}
					}
					ch <- StreamEvent{Type: StreamToolCall, ToolCall: &ToolCall{
						ID:   toolID,
						Type: "tool_use",
						Function: ToolCallFunction{
							Name:      toolName,
							Arguments: args,
						},
					}}
					toolID = ""
					toolName = ""
				}

			case "message_stop":
				ch <- StreamEvent{Type: StreamDone}
			}
		}
	}()

	return ch, nil
}

func (p *AnthropicProvider) buildMessages(msgs []Message) (string, []anthropicMessage) {
	var systemPrompt string
	var out []anthropicMessage
	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			systemPrompt += m.Content + "\n"
		case RoleUser:
			out = append(out, anthropicMessage{
				Role:    "user",
				Content: []anthropicContent{{Type: "text", Text: m.Content}},
			})
		case RoleAssistant:
			var content []anthropicContent
			if m.Content != "" {
				content = append(content, anthropicContent{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				var v any
				json.Unmarshal(tc.Function.Arguments, &v)
				content = append(content, anthropicContent{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: v,
				})
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: content})
		case RoleTool:
			out = append(out, anthropicMessage{
				Role: "user",
				Content: []anthropicContent{{
					Type:    "tool_result",
					ToolID:  m.ToolCallID,
					Content: m.Content,
				}},
			})
		}
	}
	if len(out) == 0 {
		out = []anthropicMessage{{Role: "user", Content: []anthropicContent{{Type: "text", Text: ""}}}}
	}
	if systemPrompt == "" {
		systemPrompt = "You are a helpful coding assistant. You can use tools to read, write, and search files, run commands, and interact with git."
	}
	return systemPrompt, out
}

func (p *AnthropicProvider) buildRequest(req *ChatRequest) anthropicRequest {
	system, msgs := p.buildMessages(req.Messages)
	tools := make([]anthropicToolSchema, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, anthropicToolSchema{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return anthropicRequest{
		Model:     p.model,
		MaxTokens: 4096,
		System:    system,
		Messages:  msgs,
		Tools:     tools,
	}
}

func (p *AnthropicProvider) postRequest(ctx context.Context, body anthropicRequest) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic marshal: %w", err)
	}
	baseURL := p.baseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	return http.DefaultClient.Do(httpReq)
}

func (p *AnthropicProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	body := p.buildRequest(req)
	resp, err := p.postRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic read: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(raw))
	}

	var anthResp anthropicResponse
	if err := json.Unmarshal(raw, &anthResp); err != nil {
		return nil, fmt.Errorf("anthropic decode: %w", err)
	}

	res := &ChatResponse{
		Usage: Usage{
			InputTokens:  anthResp.Usage.InputTokens,
			OutputTokens: anthResp.Usage.OutputTokens,
		},
	}

	for _, c := range anthResp.Content {
		switch c.Type {
		case "text":
			res.Text += c.Text
		case "tool_use":
			inputData, _ := json.Marshal(c.Input)
			res.ToolCall = &ToolCall{
				ID:   c.ID,
				Type: "tool_use",
				Function: ToolCallFunction{
					Name:      c.Name,
					Arguments: inputData,
				},
			}
		}
	}

	return res, nil
}
