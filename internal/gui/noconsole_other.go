//go:build !windows

package gui

import "os/exec"

// hideConsole is a no-op away from Windows: no other platform invents a
// console window for a child process. See noconsole_windows.go.
func hideConsole(cmd *exec.Cmd) {}
