package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type grepArgs struct {
	Pattern string `json:"pattern"`
	Include string `json:"include,omitempty"`
	Path    string `json:"path,omitempty"`
}

func grepTool() *Tool {
	return &Tool{
		Name:        "grep",
		Description: "Search for a regex pattern in file contents. Returns matching files with line numbers. Optionally filter by file pattern (include) and search path.",
		Schema: mkSchema(map[string]any{
			"pattern": strProp("The regex pattern to search for"),
			"include": strProp("Optional file glob pattern to filter by (e.g. '*.go', '*.{ts,tsx}')"),
			"path":    strProp("Optional directory to search in (relative to cwd)"),
		}, []string{"pattern"}),
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var a grepArgs
			if err := json.Unmarshal(args, &a); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("invalid args: %v", err)}
			}

			cmdArgs := []string{"-rn", "--no-heading"}
			if a.Include != "" {
				cmdArgs = append(cmdArgs, "--include", a.Include)
			}
			cmdArgs = append(cmdArgs, a.Pattern)
			if a.Path != "" {
				cmdArgs = append(cmdArgs, a.Path)
			} else {
				cmdArgs = append(cmdArgs, ".")
			}

			cmd := exec.CommandContext(ctx, "rg", cmdArgs...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
					return &ToolResult{Success: true, Data: "no matches found"}
				}
				return &ToolResult{Success: false, Data: fmt.Sprintf("grep error: %v\nstderr: %s", err, strTrim(stderr.String()))}
			}

			result := strings.TrimSpace(stdout.String())
			if result == "" {
				return &ToolResult{Success: true, Data: "no matches found"}
			}

			if len(result) > 20000 {
				result = result[:20000] + "\n... [output truncated at 20000 chars]"
			}

			return &ToolResult{Success: true, Data: result}
		},
	}
}
