//go:build windows

package gui

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideConsole keeps a console child from opening a window of its own.
//
// This binary is linked with -H windowsgui and so has no console. When it
// spawns one that does — ffmpeg, ffprobe, tar — Windows allocates a fresh
// console for it, which appears as an empty command-prompt window over the UI.
// Closing that window sends CTRL_CLOSE_EVENT to the whole process group, so a
// user tidying it away kills the running ffmpeg: it finishes muxing, then dies
// with 0xC000013A (STATUS_CONTROL_C_EXIT) and the download is reported failed
// even though the file was complete.
//
// CREATE_NO_WINDOW is the flag that actually prevents the console being
// created; HideWindow only hides a window that was made anyway.
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}
