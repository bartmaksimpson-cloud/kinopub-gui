//go:build windows

package gui

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

// TestHideConsoleSetsCreateNoWindow runs only on the windows CI job, which is
// the only place it means anything. Losing this flag brings back an empty
// console window over the UI — and, because closing it signals the whole
// process group, a finished ffmpeg dying with 0xC000013A.
func TestHideConsoleSetsCreateNoWindow(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "echo")
	hideConsole(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("hideConsole left SysProcAttr nil")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x, CREATE_NO_WINDOW (%#x) not set",
			cmd.SysProcAttr.CreationFlags, windows.CREATE_NO_WINDOW)
	}
}

// TestHideConsoleKeepsExistingAttrs guards the merge: open_windows.go sets
// SysProcAttr.CmdLine to control quoting, and hiding the console must not
// wipe that out if the two are ever combined.
func TestHideConsoleKeepsExistingAttrs(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /c "echo hi"`}
	hideConsole(cmd)

	if cmd.SysProcAttr.CmdLine != `cmd.exe /c "echo hi"` {
		t.Fatalf("CmdLine was clobbered: %q", cmd.SysProcAttr.CmdLine)
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("CREATE_NO_WINDOW not set alongside an existing CmdLine")
	}
}
