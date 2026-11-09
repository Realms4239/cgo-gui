//go:build !windows

package vm

import (
	"context"
	"os/exec"
)

// bgCmd — hors Windows : passthrough (voir kit.bgCmd).
func bgCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// bgCmdCtx — idem avec annulation.
func bgCmdCtx(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
