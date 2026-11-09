//go:build windows

package vm

import (
	"context"
	"os/exec"

	"golang.org/x/sys/windows"
)

// bgCmd — vmrun/VBoxManage SANS flash console (voir kit.bgCmd : même
// raison). Sorties toujours capturées par les appelants.
func bgCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

// bgCmdCtx — idem avec annulation (GuestIP borné).
func bgCmdCtx(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}
