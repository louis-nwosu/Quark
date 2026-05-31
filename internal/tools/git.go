package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func gitStatusTool() *Tool {
	return &Tool{
		Name:        "git_status",
		Description: "Show the working tree status (equivalent to 'git status'). Returns a summary of changes including staged, unstaged, and untracked files.",
		Schema: mkSchema(map[string]any{}, nil),
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			return runGit(ctx, "status")
		},
	}
}

func gitDiffTool() *Tool {
	return &Tool{
		Name:        "git_diff",
		Description: "Show git diff output: unstaged changes (default) or staged changes with --cached. Returns the diff output showing line-by-line changes.",
		Schema: mkSchema(map[string]any{
			"cached": map[string]any{"type": "boolean", "description": "Show staged changes instead of unstaged"},
		}, nil),
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var a struct {
				Cached bool `json:"cached"`
			}
			json.Unmarshal(args, &a)

			if a.Cached {
				return runGit(ctx, "diff", "--cached")
			}
			return runGit(ctx, "diff")
		},
	}
}

type gitCommitArgs struct {
	Message string `json:"message"`
}

func gitCommitTool() *Tool {
	return &Tool{
		Name:        "git_commit",
		Description: "Create a git commit with the given commit message. All staged changes will be committed. Returns the commit hash and summary.",
		Schema: mkSchema(map[string]any{
			"message": strProp("The commit message"),
		}, []string{"message"}),
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var a gitCommitArgs
			if err := json.Unmarshal(args, &a); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("invalid args: %v", err)}
			}

			cmd := exec.CommandContext(ctx, "git", "commit", "-m", a.Message)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			if err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("git commit failed:\nstderr: %s", strTrim(stderr.String()))}
			}

			out := strings.TrimSpace(stdout.String())
			return &ToolResult{Success: true, Data: out}
		},
	}
}

func runGit(ctx context.Context, args ...string) *ToolResult {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return &ToolResult{Success: false, Data: fmt.Sprintf("git error: %v\nstderr: %s", err, strTrim(stderr.String()))}
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		out = "(no output)"
	}

	return &ToolResult{Success: true, Data: out}
}
