package sandbox

import (
	"context"
	"os/exec"
)

// LlamaCppTemplate is the sandbox template to use for llama.cpp processes.
const LlamaCppTemplate = ``

// CommandContext creates a sandboxed version of an os/exec.Cmd. On Windows, we
// use the wsb exec command to wrap the process.
func CommandContext(ctx context.Context, template, name string, args ...string) (*exec.Cmd, error) {
	// TODO: Implement.
	return exec.CommandContext(ctx, name, args...), nil
}
