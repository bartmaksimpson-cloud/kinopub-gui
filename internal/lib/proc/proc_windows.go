//go:build windows

package proc

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hide keeps Windows from creating a console for the child.
//
// CREATE_NO_WINDOW — именно тот флаг, который не даёт консоли появиться.
// HideWindow прячет уже созданное окно, то есть лечит симптом позже и хуже.
func hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
