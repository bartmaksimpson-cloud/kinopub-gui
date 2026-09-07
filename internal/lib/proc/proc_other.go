//go:build !windows

package proc

import "os/exec"

// hide is a no-op away from Windows: no other platform invents a console window
// for a child process.
func hide(cmd *exec.Cmd) {}
