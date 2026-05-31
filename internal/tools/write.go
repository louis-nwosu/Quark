package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type writeArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func writeTool() *Tool {
	return &Tool{
		Name:        "write",
		Description: "Create a new file or overwrite an existing file with the given content. The file path should be relative to the current working directory. Creates parent directories if they don't exist.",
		Schema: mkSchema(map[string]any{
			"file_path": strProp("The path to the file to write (relative to cwd)"),
			"content":   strProp("The content to write to the file"),
		}, []string{"file_path", "content"}),
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var a writeArgs
			if err := json.Unmarshal(args, &a); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("invalid args: %v", err)}
			}

			path := a.FilePath
			if !filepath.IsAbs(path) {
				cwd, _ := os.Getwd()
				path = filepath.Join(cwd, path)
			}

			dir := filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("error creating directory: %v", err)}
			}

			if err := os.WriteFile(path, []byte(a.Content), 0644); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("error writing file: %v", err)}
			}

			rel, _ := filepath.Rel(os.Getenv("PWD"), path)
			if rel == "" {
				rel = path
			}

			return &ToolResult{Success: true, Data: fmt.Sprintf("wrote %d bytes to %s", len(a.Content), rel)}
		},
	}
}
