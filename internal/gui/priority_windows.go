//go:build windows

package gui

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// lowerPriority runs a child below everything the person is doing at the
// keyboard. Перекодирование занимает все ядра часами, и без этого машина в
// разгар дня становится неотзывчивой: курсор дёргается, браузер думает.
// Приоритет НИЖЕ обычного не отнимает у ffmpeg процессорное время, пока оно
// никому не нужно, — он лишь уступает его первому, кому оно понадобится.
func lowerPriority(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.BELOW_NORMAL_PRIORITY_CLASS
}

// applyLowPriority is the post-start half, which Windows does not need: the
// class is set at creation. See priority_other.go.
func applyLowPriority(pid int) {}
