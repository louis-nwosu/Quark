package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/louis-nwosu/Quark/internal/llm"
)

type Session struct {
	Messages []llm.Message
	System   string
	path     string
}

func NewSession(system string) *Session {
	return &Session{
		System: system,
	}
}

func (s *Session) SessionPath() string {
	if s.path != "" {
		return s.path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "quark", "session.json")
}

func (s *Session) SetSessionPath(path string) {
	s.path = path
}

func (s *Session) AddUserMessage(content string) {
	s.Messages = append(s.Messages, llm.Message{
		Role:    llm.RoleUser,
		Content: content,
	})
}

func (s *Session) AddAssistantMessage(content string) {
	s.Messages = append(s.Messages, llm.Message{
		Role:    llm.RoleAssistant,
		Content: content,
	})
}

func (s *Session) AddToolResult(callID, content string) {
	s.Messages = append(s.Messages, llm.Message{
		Role:       llm.RoleTool,
		ToolCallID: callID,
		Content:    content,
	})
}

func (s *Session) AddAssistantToolCall(tc *llm.ToolCall) {
	s.Messages = append(s.Messages, llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{*tc},
	})
}

func (s *Session) Clear() {
	s.Messages = make([]llm.Message, 0)
}

// ── Token estimation ──────────────────────────────────────────

func EstimateTokens(text string) int {
	return (len(text) + 3) / 4
}

func (s *Session) TotalTokens() int {
	total := 0
	for _, m := range s.Messages {
		total += EstimateTokens(m.Content)
		for _, tc := range m.ToolCalls {
			total += EstimateTokens(tc.Function.Name)
			total += EstimateTokens(string(tc.Function.Arguments))
		}
		total += 5
	}
	return total
}

// ── Context compaction ────────────────────────────────────────

// CompactWithBudget removes or degrades low-priority messages until
// total estimated tokens fall within maxTokens.
// Eviction order:
//  1. Oldest user+assistant text pairs (conversation, no tools)
//  2. Large tool results (>1000 tokens) → degrade to summary
//  3. Oldest tool call chains (assistant + following tool results)
func (s *Session) CompactWithBudget(maxTokens int) {
	if maxTokens <= 0 {
		return
	}
	for s.TotalTokens() > maxTokens {
		removed := false

		// Phase 1: drop oldest user+assistant text pairs
		for i := 0; i < len(s.Messages)-1; i++ {
			if s.Messages[i].Role == llm.RoleUser &&
				s.Messages[i+1].Role == llm.RoleAssistant &&
				len(s.Messages[i+1].ToolCalls) == 0 {
				s.Messages = append(s.Messages[:i], s.Messages[i+2:]...)
				removed = true
				break
			}
		}
		if removed {
			continue
		}

		// Phase 2: degrade large tool results
		for i := 0; i < len(s.Messages); i++ {
			if s.Messages[i].Role == llm.RoleTool && EstimateTokens(s.Messages[i].Content) > 1000 {
				degradeToolResult(&s.Messages[i])
				removed = true
				break
			}
		}
		if removed {
			continue
		}

		// Phase 3: drop oldest tool call chain
		for i := 0; i < len(s.Messages); i++ {
			if s.Messages[i].Role == llm.RoleAssistant && len(s.Messages[i].ToolCalls) > 0 {
				// Find all immediately following tool results
				end := i + 1
				for end < len(s.Messages) && s.Messages[end].Role == llm.RoleTool {
					end++
				}
				s.Messages = append(s.Messages[:i], s.Messages[end:]...)
				removed = true
				break
			}
		}
		if removed {
			continue
		}

		break
	}
}

func degradeToolResult(msg *llm.Message) {
	content := msg.Content
	if EstimateTokens(content) <= 1000 {
		return
	}
	// Determine tool type from content prefix (bash, read, etc.)
	prefix := ""
	body := content
	if len(content) > 100 {
		prefix = content[:100]
		body = content[100:]
	}

	// Take first and last portion
	maxLen := 400
	if len(body) > maxLen {
		head := body[:200]
		tail := body[len(body)-200:]
		msg.Content = prefix + head + "\n... [truncated, " + fmt.Sprintf("%d chars", len(body)) + " total]\n...\n" + tail
	}
}

// ── Conversation summarization ────────────────────────────────

// MessagesForSummary extracts the oldest messages (excluding system prompt)
// that are candidates for LLM-based summarization. Returns the messages
// and the count of messages it wants to replace.
// It preserves complete tool call chains and stops before recent exchanges.
func (s *Session) MessagesForSummary() ([]llm.Message, int) {
	total := s.TotalTokens()
	if total == 0 {
		return nil, 0
	}

	// Summarize up to ~30% of total tokens, but never the last 3 user turns
	targetTokens := total * 30 / 100
	if targetTokens < 200 {
		return nil, 0
	}

	start := 0
	if len(s.Messages) > 0 && s.Messages[0].Role == llm.RoleSystem {
		start = 1
	}

	var collected []llm.Message
	collectedTokens := 0
	// If we have fewer than 2 user turns, there's nothing worth summarizing
	userCount := 0
	for i := start; i < len(s.Messages); i++ {
		if s.Messages[i].Role == llm.RoleUser {
			userCount++
		}
	}
	if userCount < 3 {
		return nil, 0
	}

	// Walk from start, collecting until we hit target or approach the last 2 user turns
	userTurnsSeen := 0
	userTurnsTotal := 0
	for i := len(s.Messages) - 1; i >= start; i-- {
		if s.Messages[i].Role == llm.RoleUser {
			userTurnsTotal++
		}
	}

	for i := start; i < len(s.Messages); i++ {
		if collectedTokens >= targetTokens {
			break
		}
		if s.Messages[i].Role == llm.RoleUser {
			userTurnsSeen++
			// Keep at least 2 user turns intact (leave room for current turn too)
			if userTurnsSeen > userTurnsTotal-2 {
				break
			}
		}

		tokens := EstimateTokens(s.Messages[i].Content)
		for _, tc := range s.Messages[i].ToolCalls {
			tokens += EstimateTokens(tc.Function.Name)
			tokens += EstimateTokens(string(tc.Function.Arguments))
		}
		collected = append(collected, s.Messages[i])
		collectedTokens += tokens + 5
	}

	if len(collected) < 2 {
		return nil, 0
	}

	// Verify we're not breaking a tool chain mid-way
	if len(collected) > 0 {
		last := collected[len(collected)-1]
		if last.Role == llm.RoleAssistant && len(last.ToolCalls) > 0 {
			// We'd break the chain — don't include this message
			collected = collected[:len(collected)-1]
		}
	}

	return collected, len(collected)
}

func (s *Session) ReplaceWithSummary(summary string, count int) {
	if count <= 0 {
		return
	}

	start := 0
	if len(s.Messages) > 0 && s.Messages[0].Role == llm.RoleSystem {
		start = 1
	}
	if start+count > len(s.Messages) {
		count = len(s.Messages) - start
	}
	if count <= 0 {
		return
	}

	summaryMsg := llm.Message{
		Role:    llm.RoleSystem,
		Content: "[Summary of previous conversation]\n" + summary,
	}

	newMessages := make([]llm.Message, start)
	copy(newMessages, s.Messages[:start])
	newMessages = append(newMessages, summaryMsg)
	newMessages = append(newMessages, s.Messages[start+count:]...)
	s.Messages = newMessages
}

// ── Older API kept for compatibility ──────────────────────────

func (s *Session) Compact(threshold int) {
	if threshold > 0 {
		s.CompactWithBudget(threshold)
	}
}

func (s *Session) AllMessages() []llm.Message {
	if s.System == "" {
		return s.Messages
	}
	return append([]llm.Message{{
		Role:    llm.RoleSystem,
		Content: s.System,
	}}, s.Messages...)
}

func (s *Session) Save() error {
	path := s.SessionPath()
	if path == "" {
		return fmt.Errorf("no session path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	data, err := json.MarshalIndent(s.Messages, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func (s *Session) Load() error {
	path := s.SessionPath()
	if path == "" {
		return fmt.Errorf("no session path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read: %w", err)
	}
	var msgs []llm.Message
	if err := json.Unmarshal(data, &msgs); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	s.Messages = msgs
	return nil
}
