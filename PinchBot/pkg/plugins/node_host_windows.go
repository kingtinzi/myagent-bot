//go:build windows

package plugins

import (
	"os/exec"
	"syscall"
)

// setNodeHostNoConsoleWindow avoids a visible black CMD window when spawning node.exe
// from a GUI parent (e.g. launcher-chat.exe with -H windowsgui).
func setNodeHostNoConsoleWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		HideWindow:    true,
	}
}
