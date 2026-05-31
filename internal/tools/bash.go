package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type bashArgs struct {
	Command string `json:"command"`
}

func bashTool() *Tool {
	return &Tool{
		Name:        "bash",
		Description: "Execute a shell command in the current working directory. Use this to run build commands, tests, install packages, or any other terminal operation. The command runs with a 30-second timeout.",
		Schema: mkSchema(map[string]any{
			"command": strProp("The shell command to execute"),
		}, []string{"command"}),
		Execute: func(ctx context.Context, args json.RawMessage) *ToolResult {
			var a bashArgs
			if err := json.Unmarshal(args, &a); err != nil {
				return &ToolResult{Success: false, Data: fmt.Sprintf("invalid args: %v", err)}
			}

			cmd := exec.CommandContext(ctx, "sh", "-c", a.Command)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					return &ToolResult{Success: false, Data: fmt.Sprintf("error executing command: %v", err)}
				}
			}

			out := stdout.String()
			errOut := stderr.String()

			var b strings.Builder
			b.WriteString(fmt.Sprintf("exit code: %d\n", exitCode))
			if out != "" {
				b.WriteString(fmt.Sprintf("stdout:\n%s\n", strTrim(out)))
			}
			if errOut != "" {
				b.WriteString(fmt.Sprintf("stderr:\n%s\n", strTrim(errOut)))
			}

			return &ToolResult{Success: exitCode == 0, Data: b.String()}
		},
	}
}
