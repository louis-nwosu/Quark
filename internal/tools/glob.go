package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func globTool() *Tool {
	return &Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Uses the `find` command under the hood. Returns a list of matching file paths relative to the search directory.",
		Schema: mkSchema(map[string]any{
			"pattern": strProp("The glob pattern to match (e.g. '**/*.go', 'src/**/*.ts')"),
			"path":    strProp("Optional directory to search in (relative to cwd, defaults to '.')"),
		}, []string{"pattern"}),
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var a globArgs
			if err := json.Unmarshal(args, &a); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("invalid args: %v", err)}
			}

			searchPath := a.Path
			if searchPath == "" {
				searchPath = "."
			}

			cmdArgs := []string{"-c", fmt.Sprintf("find %s -path '%s' 2>/dev/null | head -200", searchPath, a.Pattern)}

			cmd := exec.CommandContext(ctx, "sh", cmdArgs...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("glob error: %v", err)}
			}

			result := strings.TrimSpace(stdout.String())
			if result == "" {
				return &ToolResult{Success: true, Data: "no files matched the pattern"}
			}

			count := len(strings.Split(result, "\n"))
			result += fmt.Sprintf("\n(%d files matched)", count)

			return &ToolResult{Success: true, Data: result}
		},
	}
}
