package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/macbookpro/quark/internal/llm"
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

// Compact removes the oldest conversational turns when messages exceed the threshold.
// A turn is a user message followed by an assistant text response (no tool calls).
// Turns with tool calls are preserved to keep execution context intact.
func (s *Session) Compact(threshold int) {
	if threshold <= 0 {
		return
	}
	for len(s.Messages) > threshold {
		removed := false
		for i := 0; i < len(s.Messages)-1; i++ {
			if s.Messages[i].Role == llm.RoleUser &&
				s.Messages[i+1].Role == llm.RoleAssistant &&
				len(s.Messages[i+1].ToolCalls) == 0 {
				// found a compactible turn: user + assistant text
				s.Messages = append(s.Messages[:i], s.Messages[i+2:]...)
				removed = true
				break
			}
		}
		if !removed {
			break
		}
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
