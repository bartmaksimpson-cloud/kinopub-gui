//go:build !windows

package gui

import (
	"os/exec"

	"golang.org/x/sys/unix"
)

// lowerPriority is a no-op before the process exists: unix sets the niceness on
// a live pid, not on the exec attributes. See applyLowPriority.
func lowerPriority(cmd *exec.Cmd) {}

// applyLowPriority nices the child down so hours of encoding do not make the
// machine unusable for whoever is sitting at it. Errors are ignored on purpose:
// lowering priority may be refused (a container, an odd policy), and that is
// never a reason to fail a download.
func applyLowPriority(pid int) {
	_ = unix.Setpriority(unix.PRIO_PROCESS, pid, 10)
}
