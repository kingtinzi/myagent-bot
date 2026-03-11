//go:build !windows

package main

import "os/exec"

func setNoWindow(cmd *exec.Cmd) {
	// No-op on non-Windows; console visibility is not an issue there.
}
