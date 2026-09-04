package gui

import (
	"bytes"
	"encoding/json"
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

// TestListDirEmptyDirMarshalsAsList guards the black-screen bug: an empty
// folder used to leave FSListing.Dirs nil, which encoding/json writes as
// "dirs": null, and DirPicker.tsx reads listing.dirs.length with no guard —
// so the TypeError took the entire UI down.
func TestListDirEmptyDirMarshalsAsList(t *testing.T) {
	dir := t.TempDir()

	listing, err := listDir(dir)
	if err != nil {
		t.Fatalf("listDir(%q): %v", dir, err)
	}
	if listing.Dirs == nil {
		t.Fatal("listDir on an empty folder returned nil Dirs")
	}

	encoded, err := json.Marshal(listing)
	if err != nil {
		t.Fatalf("marshal listing: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"dirs":[]`)) {
		t.Fatalf("empty folder must encode dirs as [], got %s", encoded)
	}
}
