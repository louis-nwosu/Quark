package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type editArgs struct {
	FilePath string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func editTool() *Tool {
	return &Tool{
		Name:        "edit",
		Description: "Make a targeted edit to a file by finding a specific block of text (old_string) and replacing it with new text (new_string). Uses exact string matching. The file path should be relative to the current working directory.",
		Schema: mkSchema(map[string]any{
			"file_path":  strProp("The path to the file to edit (relative to cwd)"),
			"old_string": strProp("The exact text to find and replace"),
			"new_string": strProp("The text to replace it with"),
		}, []string{"file_path", "old_string", "new_string"}),
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var a editArgs
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

			content := string(data)

			idx := strings.Index(content, a.OldString)
			if idx == -1 {
				return &ToolResult{Success: false, Data: "old_string not found in file"}
			}

			newContent := strings.Replace(content, a.OldString, a.NewString, 1)

			if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("error writing file: %v", err)}
			}

			rel, _ := filepath.Rel(os.Getenv("PWD"), path)
			if rel == "" {
				rel = path
			}

			return &ToolResult{Success: true, Data: fmt.Sprintf("edited %s: replaced %d chars with %d chars", rel, len(a.OldString), len(a.NewString))}
		},
	}
}
