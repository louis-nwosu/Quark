package llm

import (
	"context"
	"encoding/json"

	"github.com/macbookpro/quark/internal/config"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSystem    Role = "system"
)

type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function ToolCallFunction  `json:"function"`
}

type ToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ChatRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSchema
}

type ChatResponse struct {
	Text     string
	ToolCall *ToolCall
	Usage    Usage
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type StreamEventType int

const (
	StreamChunk  StreamEventType = iota
	StreamToolCall
	StreamDone
	StreamError
)

type StreamEvent struct {
	Type     StreamEventType
	Text     string
	ToolCall *ToolCall
	Error    error
}

type Provider interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
}

func NewProvider(cfg *config.Config) (Provider, error) {
	pc, ok := cfg.Providers[cfg.DefaultProvider]
	if !ok {
		return nil, &ErrNoProvider{Name: cfg.DefaultProvider}
	}
	if pc.APIKey == "" {
		return nil, &ErrNoAPIKey{Provider: cfg.DefaultProvider}
	}

	switch cfg.DefaultProvider {
	case "openai", "openrouter":
		return &OpenAIProvider{
			apiKey:  pc.APIKey,
			model:   pc.Model,
			baseURL: pc.BaseURL,
		}, nil
	case "anthropic":
		return &AnthropicProvider{
			apiKey:  pc.APIKey,
			model:   pc.Model,
			baseURL: pc.BaseURL,
		}, nil
	default:
		return nil, &ErrUnknownProvider{Name: cfg.DefaultProvider}
	}
}

type ErrNoProvider struct{ Name string }
func (e *ErrNoProvider) Error() string { return "no config for provider: " + e.Name }

type ErrNoAPIKey struct{ Provider string }
func (e *ErrNoAPIKey) Error() string { return "no API key for provider: " + e.Provider }

type ErrUnknownProvider struct{ Name string }
func (e *ErrUnknownProvider) Error() string { return "unknown provider: " + e.Name }
