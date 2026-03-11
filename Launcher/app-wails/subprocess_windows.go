//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// setNoWindow makes the child process not show a console window (no black box).
func setNoWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		HideWindow:    true,
	}
}
