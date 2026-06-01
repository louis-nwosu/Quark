package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/louis-nwosu/Quark/internal/llm"
	"github.com/louis-nwosu/Quark/internal/tools"
)

type EventType int

const (
	EventText      EventType = iota
	EventTextChunk
	EventReasoning
	EventToolStart
	EventToolResult
	EventToolError
	EventError
	EventDone
)

type ToolEvent struct {
	Name      string
	Arguments map[string]any
	Input     string
}

type ToolResultEvent struct {
	Name    string
	Success bool
	Data    string
	Summary string
}

type Event struct {
	Type       EventType
	Text       string
	Tool       *ToolEvent
	ToolResult *ToolResultEvent
	Error      error
}

type Agent struct {
	provider         llm.Provider
	tools            *tools.Registry
	session          *Session
	CompactThreshold int
	ContextWindow    int
	BudgetRatio      float64
}

func New(provider llm.Provider, reg *tools.Registry) *Agent {
	defaultSystem := `You are Quark, a lightweight coding assistant.

You help users write, edit, and debug code. You have access to tools that let you:

- read: Read file contents
- write: Create or overwrite files
- edit: Make targeted string replacement edits
- bash: Execute shell commands
- grep: Search file contents with regex
- glob: Find files matching patterns
- git_status: Show working tree status
- git_diff: Show git diff output
- git_commit: Create git commits

When editing files, prefer using the edit tool over write to make minimal changes.
Always show the user relevant code when discussing changes.
Use bash to run tests, builds, and verify your changes.
Be concise and precise in your responses.`

	return &Agent{
		provider:      provider,
		tools:         reg,
		session:       NewSession(defaultSystem),
		ContextWindow: 128000,
		BudgetRatio:   0.8,
	}
}

func (a *Agent) Clear() {
	a.session.Clear()
	a.session.Save()
}

func (a *Agent) MessageCount() int {
	return len(a.session.Messages)
}

func (a *Agent) HasSession() bool {
	return a.MessageCount() > 0
}

func (a *Agent) SaveSession() error {
	return a.session.Save()
}

func (a *Agent) LoadSession() error {
	return a.session.Load()
}

func (a *Agent) Run(ctx context.Context, msg string, events chan<- Event) {
	defer close(events)
	defer a.session.Save()

	budget := a.CompactThreshold
	if budget <= 0 {
		budget = int(float64(a.ContextWindow) * a.BudgetRatio)
	}

	// Step 1: token-aware compaction (drop/degrade low-priority messages)
	a.session.CompactWithBudget(budget)

	// Step 2: LLM-based summarization if still over 70% of budget
	if a.session.TotalTokens() > int(float64(budget)*0.7) {
		a.summarizeOldTurns(ctx)
	}

	a.session.AddUserMessage(msg)

	for i := 0; i < 25; i++ {
		req := &llm.ChatRequest{
			Model:    "",
			Messages: a.session.AllMessages(),
			Tools:    a.tools.Schemas(),
		}

		stream, err := a.provider.ChatStream(ctx, req)
		if err != nil {
			events <- Event{Type: EventError, Error: fmt.Errorf("llm error: %w", err)}
			return
		}

		var textBuf strings.Builder
		var toolCall *llm.ToolCall

		for se := range stream {
			switch se.Type {
			case llm.StreamChunk:
				textBuf.WriteString(se.Text)
				events <- Event{Type: EventTextChunk, Text: se.Text}
			case llm.StreamReasoning:
				events <- Event{Type: EventReasoning, Text: se.Text}
			case llm.StreamToolCall:
				if toolCall == nil {
					toolCall = se.ToolCall
				}
			case llm.StreamError:
				events <- Event{Type: EventError, Error: se.Error}
				return
			case llm.StreamDone:
				// stream complete
			}
		}

		if toolCall != nil {
			tc := toolCall
			a.session.AddAssistantToolCall(tc)

			var args map[string]any
			if err := json.Unmarshal(tc.Function.Arguments, &args); err != nil {
				args = map[string]any{"raw": string(tc.Function.Arguments)}
			}

			events <- Event{
				Type: EventToolStart,
				Tool: &ToolEvent{
					Name:      tc.Function.Name,
					Arguments: args,
					Input:     string(tc.Function.Arguments),
				},
			}

			result := a.tools.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
			a.session.AddToolResult(tc.ID, result.Data)

			summary := summarizeToolResult(tc.Function.Name, result)

			if result.Success {
				events <- Event{
					Type: EventToolResult,
					ToolResult: &ToolResultEvent{
						Name:    tc.Function.Name,
						Success: true,
						Data:    result.Data,
						Summary: summary,
					},
				}
			} else {
				events <- Event{
					Type: EventToolError,
					ToolResult: &ToolResultEvent{
						Name:    tc.Function.Name,
						Success: false,
						Data:    result.Data,
						Summary: summary,
					},
				}
			}

			continue
		}

		if textBuf.Len() > 0 {
			fullText := textBuf.String()
			a.session.AddAssistantMessage(fullText)
			events <- Event{Type: EventText, Text: fullText}
		}

		events <- Event{Type: EventDone}
		return
	}

	events <- Event{Type: EventError, Error: fmt.Errorf("exceeded maximum tool call rounds (25)")}
}

func (a *Agent) summarizeOldTurns(ctx context.Context) {
	msgs, count := a.session.MessagesForSummary()
	if count < 2 {
		return
	}

	// Format the messages as a conversation text
	var b strings.Builder
	b.WriteString("Summarize this coding conversation segment. Include: decisions made, files modified, commands run, user preferences, and current state. Keep the summary concise.\n\n")
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleUser:
			b.WriteString("User: ")
			b.WriteString(m.Content)
			b.WriteString("\n")
		case llm.RoleAssistant:
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					b.WriteString("Assistant (tool: ")
					b.WriteString(tc.Function.Name)
					b.WriteString("):\n")
				}
			} else if m.Content != "" {
				b.WriteString("Assistant: ")
				b.WriteString(m.Content)
				b.WriteString("\n")
			}
		case llm.RoleTool:
			content := m.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			b.WriteString("Tool result: ")
			b.WriteString(content)
			b.WriteString("\n")
		}
	}

	summaryReq := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: b.String()},
		},
	}

	resp, err := a.provider.Chat(ctx, summaryReq)
	if err != nil || resp == nil || resp.Text == "" {
		return
	}

	a.session.ReplaceWithSummary(resp.Text, count)
}

func summarizeToolResult(name string, r *tools.ToolResult) string {
	if !r.Success {
		return truncate(r.Data, 120)
	}
	switch name {
	case "read":
		lines := countLines(r.Data)
		return fmt.Sprintf("%d lines", lines)
	case "write":
		return truncate(r.Data, 80)
	case "edit":
		return truncate(r.Data, 80)
	case "bash":
		lines := countLines(r.Data)
		return fmt.Sprintf("exit 0, %d lines", lines)
	case "grep", "glob":
		n := countLines(r.Data)
		if n <= 1 {
			return "no matches"
		}
		return fmt.Sprintf("%d matches", n-1)
	case "git_status", "git_diff", "git_commit":
		return truncate(r.Data, 80)
	}
	return truncate(r.Data, 80)
}

func countLines(s string) int {
	n := 0
	for _, c := range s {
		if c == '\n' {
			n++
		}
	}
	return n
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
