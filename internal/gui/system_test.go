package gui

import (
	"runtime"
	"testing"
)

// TestListDirDrivesSentinel checks the wiring added for cross-drive
// navigation: listDir(drivesSentinel) must dispatch to listDrives(), and a
// non-Windows root ("/") must never itself resolve to the sentinel (drive
// switching is a Windows-only concept — see system_windows.go /
// system_notwindows.go).
func TestListDirDrivesSentinel(t *testing.T) {
	_, err := listDir(drivesSentinel)
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("listDir(drivesSentinel) on windows: %v", err)
		}
	} else if err == nil {
		t.Fatal("listDir(drivesSentinel) on non-windows: expected an error, got nil")
	}

	root, err := listDir("/")
	if err != nil {
		t.Fatalf("listDir(/): %v", err)
	}
	if root.Parent == drivesSentinel {
		t.Fatalf("listDir(/).Parent resolved to drivesSentinel on %s — should only happen on windows", runtime.GOOS)
	}
}
