package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/macbookpro/quark/internal/llm"
)

type ToolResult struct {
	Success bool   `json:"success"`
	Data    string `json:"data"`
}

type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Execute     func(ctx context.Context, args json.RawMessage) *ToolResult
}

type Registry struct {
	tools map[string]*Tool
}

func NewRegistry() *Registry {
	r := &Registry{tools: make(map[string]*Tool)}
	r.registerBuiltins()
	return r
}

func (r *Registry) Register(t *Tool) {
	r.tools[t.Name] = t
}

func (r *Registry) Get(name string) *Tool {
	return r.tools[name]
}

func (r *Registry) Schemas() []llm.ToolSchema {
	out := make([]llm.ToolSchema, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, llm.ToolSchema{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Schema,
		})
	}
	return out
}

func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) *ToolResult {
	t := r.tools[name]
	if t == nil {
		return &ToolResult{
			Success: false,
			Data:    fmt.Sprintf("unknown tool: %s", name),
		}
	}
	args = NormalizeArgs(args)
	return t.Execute(ctx, args)
}

// NormalizeArgs converts tool call arguments to a JSON object.
// Some providers return arguments as a JSON string (e.g. "{\"key\":\"val\"}")
// instead of a JSON object (e.g. {"key":"val"}). This handles both.
func NormalizeArgs(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.RawMessage(s)
	}
	return raw
}

func (r *Registry) registerBuiltins() {
	r.Register(readTool())
	r.Register(writeTool())
	r.Register(editTool())
	r.Register(bashTool())
	r.Register(grepTool())
	r.Register(globTool())
	r.Register(gitStatusTool())
	r.Register(gitDiffTool())
	r.Register(gitCommitTool())
}

func mkSchema(props map[string]any, required []string) json.RawMessage {
	s := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	data, _ := json.Marshal(s)
	return data
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func strTrim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 2000 {
		s = s[:2000] + "... [truncated]"
	}
	return s
}
