//go:build !windows

package plugins

import "os/exec"

func setNodeHostNoConsoleWindow(cmd *exec.Cmd) {}
