package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type readArgs struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func readTool() *Tool {
	return &Tool{
		Name:        "read",
		Description: "Read the contents of a file. Provide file_path (required), and optionally offset (line number to start from, 1-indexed) and limit (max lines to read). The file path should be relative to the current working directory.",
		Schema: mkSchema(map[string]any{
			"file_path": strProp("The path to the file to read (relative to cwd)"),
			"offset":    intProp("Line number to start from (1-indexed, optional)"),
			"limit":     intProp("Maximum number of lines to read (optional)"),
		}, []string{"file_path"}),
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var a readArgs
			if err := json.Unmarshal(args, &a); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("invalid args: %v", err)}
			}

			path := a.FilePath
			if !filepath.IsAbs(path) {
				cwd, _ := os.Getwd()
				path = filepath.Join(cwd, path)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("error reading file: %v", err)}
			}

			lines := splitLines(string(data))

			start := 0
			if a.Offset > 0 {
				start = a.Offset - 1
			}
			if start >= len(lines) {
				return &ToolResult{Success: false, Data: "offset exceeds file length"}
			}

			end := len(lines)
			if a.Limit > 0 && start+a.Limit < end {
				end = start + a.Limit
			}

			out := lines[start:end]
			result := ""
			for i, line := range out {
				result += fmt.Sprintf("%d: %s\n", start+i+1, line)
			}

			if len(result) > 50000 {
				result = result[:50000] + "\n... [output truncated at 50000 chars]"
			}

			return &ToolResult{Success: true, Data: result}
		},
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
