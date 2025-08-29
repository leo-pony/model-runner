//go:build !darwin && !windows

package sandbox

import (
	"context"
	"os/exec"
)

// LlamaCppTemplate is the sandbox template to use for llama.cpp processes.
const LlamaCppTemplate = ``

// CommandContext creates a sandboxed version of an os/exec.Cmd. On Linux, we
// don't currently support sandboxing since we already run inside containers, so
// this function is a direct passthrough.
func CommandContext(ctx context.Context, template, name string, args ...string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, name, args...), nil
}
