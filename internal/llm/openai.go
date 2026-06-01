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

type OpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content,omitempty"`
	Reasoning  string           `json:"reasoning,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openAIToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function openAIToolFunction  `json:"function"`
}

type openAIToolFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type openAITool struct {
	Type     string          `json:"type"`
	Function json.RawMessage `json:"function"`
}

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Tools    []openAITool    `json:"tools,omitempty"`
	Stream   *bool           `json:"stream,omitempty"`
}

func boolPtr(b bool) *bool { return &b }

type openAIChoice struct {
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type openAIChatResponse struct {
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

func (p *OpenAIProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	body := openAIChatRequest{
		Model:    p.model,
		Messages: p.toMessages(req.Messages),
		Tools:    p.toTools(req.Tools),
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	if strings.Contains(p.baseURL, "openrouter") {
		httpReq.Header.Set("X-Title", "Quark")
		httpReq.Header.Set("HTTP-Referer", "https://github.com/louis-nwosu/Quark")
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai read: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai %d: %s", resp.StatusCode, string(raw))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(raw, &chatResp); err != nil {
		return nil, fmt.Errorf("openai decode: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("openai: no choices returned")
	}

	choice := chatResp.Choices[0]
	res := &ChatResponse{
		Usage: Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
		},
	}

	if choice.Message.Content != nil {
		res.Text = *choice.Message.Content
	} else if choice.Message.Reasoning != "" {
		res.Text = choice.Message.Reasoning
	}

	if len(choice.Message.ToolCalls) > 0 {
		tc := choice.Message.ToolCalls[0]
		args := tc.Function.Arguments
		// Some providers return args as a JSON string — unwrap it
		if len(args) > 0 && args[0] == '"' {
			var s string
			if err := json.Unmarshal(args, &s); err == nil {
				args = json.RawMessage(s)
			}
		}
		res.ToolCall = &ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		}
	}

	return res, nil
}

func (p *OpenAIProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 10)

	body := openAIChatRequest{
		Model:    p.model,
		Messages: p.toMessages(req.Messages),
		Tools:    p.toTools(req.Tools),
		Stream:   boolPtr(true),
	}
	bodyBytes, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		close(ch)
		return ch, fmt.Errorf("openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	if strings.Contains(p.baseURL, "openrouter") {
		httpReq.Header.Set("X-Title", "Quark")
		httpReq.Header.Set("HTTP-Referer", "https://github.com/louis-nwosu/Quark")
	}

	go func() {
		defer close(ch)

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			ch <- StreamEvent{Type: StreamError, Error: fmt.Errorf("openai http: %w", err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			raw, _ := io.ReadAll(resp.Body)
			ch <- StreamEvent{Type: StreamError, Error: fmt.Errorf("openai %d: %s", resp.StatusCode, string(raw))}
			return
		}

		// Accumulate tool call deltas by index
		type accTool struct {
			ID        string
			Type      string
			Name      strings.Builder
			Arguments strings.Builder
		}
		toolAcc := make(map[int]*accTool)
		buf := make([]byte, 1024)

		// Decode body until we hit the tool call info at the end
		var finishReason string

		dec := NewSSEDecoder(resp.Body)
		for {
			data, err := dec.Decode()
			if err == io.EOF {
				break
			}
			if err != nil {
				ch <- StreamEvent{Type: StreamError, Error: fmt.Errorf("openai sse: %w", err)}
				return
			}

			if string(data) == "[DONE]" {
				break
			}

			var delta struct {
				Choices []struct {
					Delta struct {
						Content          *string            `json:"content"`
						Reasoning        *string            `json:"reasoning"`
						ReasoningContent *string            `json:"reasoning_content"`
						ToolCalls        []struct {
							Index    int     `json:"index"`
							ID       *string `json:"id"`
							Type     *string `json:"type"`
							Function *struct {
								Name      *string `json:"name"`
								Arguments *string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal(data, &delta); err != nil {
				_ = err // skip malformed lines
				continue
			}

			if len(delta.Choices) == 0 {
				continue
			}
			choice := delta.Choices[0]

			if choice.FinishReason != nil {
				finishReason = *choice.FinishReason
			}

			// reasoning chunk (skip empty)
			if choice.Delta.Reasoning != nil && *choice.Delta.Reasoning != "" {
				ch <- StreamEvent{Type: StreamReasoning, Text: *choice.Delta.Reasoning}
			}
			if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
				ch <- StreamEvent{Type: StreamReasoning, Text: *choice.Delta.ReasoningContent}
			}

			// text chunk (skip empty)
			if choice.Delta.Content != nil && *choice.Delta.Content != "" {
				ch <- StreamEvent{Type: StreamChunk, Text: *choice.Delta.Content}
			}

			// tool call deltas
			for _, tc := range choice.Delta.ToolCalls {
				at, ok := toolAcc[tc.Index]
				if !ok {
					at = &accTool{}
					toolAcc[tc.Index] = at
				}
				if tc.ID != nil {
					at.ID = *tc.ID
				}
				if tc.Type != nil {
					at.Type = *tc.Type
				}
				if tc.Function != nil {
					if tc.Function.Name != nil {
						at.Name.WriteString(*tc.Function.Name)
					}
					if tc.Function.Arguments != nil {
						at.Arguments.WriteString(*tc.Function.Arguments)
					}
				}
			}
			_ = buf
		}

		// If we accumulated tool calls, emit them
		if finishReason == "tool_calls" && len(toolAcc) > 0 {
			// Send all accumulated text first (if any)
			for _, at := range toolAcc {
				args := json.RawMessage(at.Arguments.String())
				// unwrap JSON strings
				if len(args) > 0 && args[0] == '"' {
					var s string
					if err := json.Unmarshal(args, &s); err == nil {
						args = json.RawMessage(s)
					}
				}
				tc := &ToolCall{
					ID:   at.ID,
					Type: at.Type,
					Function: ToolCallFunction{
						Name:      at.Name.String(),
						Arguments: args,
					},
				}
				ch <- StreamEvent{Type: StreamToolCall, ToolCall: tc}
			}
		}

		ch <- StreamEvent{Type: StreamDone}
	}()

	return ch, nil
}

func (p *OpenAIProvider) toMessages(msgs []Message) []openAIMessage {
	out := make([]openAIMessage, 0, len(msgs))
	for _, m := range msgs {
		om := openAIMessage{
			Role:       string(m.Role),
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		if m.Content != "" {
			c := m.Content
			om.Content = &c
		}
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, openAIToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: openAIToolFunction{
					Name:      tc.Function.Name,
					Arguments: stringifyArgs(tc.Function.Arguments),
				},
			})
		}
		out = append(out, om)
	}
	return out
}

// stringifyArgs ensures tool call arguments are encoded as a JSON string,
// which is what the OpenAI API expects in request messages.
func stringifyArgs(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	if raw[0] == '"' {
		return raw
	}
	if raw[0] == '{' || raw[0] == '[' {
		s, _ := json.Marshal(string(raw))
		return json.RawMessage(s)
	}
	return raw
}

func (p *OpenAIProvider) toTools(tools []ToolSchema) []openAITool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openAITool, 0, len(tools))
	for _, t := range tools {
		fn := map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  json.RawMessage(t.InputSchema),
		}
		fnData, _ := json.Marshal(fn)
		out = append(out, openAITool{
			Type:     "function",
			Function: fnData,
		})
	}
	return out
}

// SSEDecoder parses SSE (Server-Sent Events) from a reader.
// Supports both OpenAI format (data: {...}) and Anthropic format (event: <type>\ndata: {...}).
type SSEDecoder struct {
	r       io.Reader
	dataBuf []byte
	event   string
}

func NewSSEDecoder(r io.Reader) *SSEDecoder {
	return &SSEDecoder{r: r}
}

// Decode reads the next SSE data payload (OpenAI format).
func (d *SSEDecoder) Decode() ([]byte, error) {
	for {
		line, err := d.readLine()
		if err != nil {
			d.dataBuf = nil
			return nil, err
		}

		if len(line) == 0 {
			// empty line = end of event
			if d.dataBuf != nil {
				data := d.dataBuf
				d.dataBuf = nil
				return data, nil
			}
			continue
		}

		if len(line) >= 6 && string(line[:6]) == "data: " {
			d.dataBuf = append(d.dataBuf, line[6:]...)
		}
	}
}

// DecodeEvent reads the next SSE event, returning (eventType, data).
// Handles the Anthropic format: event: <name>\ndata: {...}\n\n
func (d *SSEDecoder) DecodeEvent() (event string, data []byte, err error) {
	d.event = ""
	d.dataBuf = nil

	for {
		line, err := d.readLine()
		if err != nil {
			return d.event, d.dataBuf, err
		}

		if len(line) == 0 {
			// end of event
			return d.event, d.dataBuf, nil
		}

		if len(line) >= 7 && string(line[:7]) == "event: " {
			d.event = string(line[7:])
		} else if len(line) >= 6 && string(line[:6]) == "data: " {
			d.dataBuf = append(d.dataBuf, line[6:]...)
		}
	}
}

func (d *SSEDecoder) readLine() ([]byte, error) {
	line := make([]byte, 0, 256)
	for {
		b := make([]byte, 1)
		_, err := d.r.Read(b)
		if err != nil {
			if len(line) > 0 {
				return line, nil
			}
			return nil, err
		}
		if b[0] == '\n' {
			return line, nil
		}
		if b[0] == '\r' {
			continue
		}
		line = append(line, b[0])
	}
}

func (p *OpenAIProvider) modelFromID(id string) string {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		// provider/model format, just take model
		return parts[1]
	}
	return id
}
