package gui

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestListDirDrivesSentinel checks the wiring added for cross-drive
// navigation: listDir(drivesSentinel) must dispatch to listDrives(), and the
// sentinel must appear as a parent on Windows and only there (drive switching
// is a Windows-only concept — see system_windows.go / system_notwindows.go).
func TestListDirDrivesSentinel(t *testing.T) {
	_, err := listDir(drivesSentinel)
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("listDir(drivesSentinel) on windows: %v", err)
		}
	} else if err == nil {
		t.Fatal("listDir(drivesSentinel) on non-windows: expected an error, got nil")
	}

	// On Windows "/" resolves to the root of the current drive, which is
	// exactly where the "up" button has to offer the drive list — that is the
	// feature. Everywhere else "/" is the top of the only tree there is, and
	// the sentinel must never appear.
	root, err := listDir("/")
	if err != nil {
		t.Fatalf("listDir(/): %v", err)
	}
	if runtime.GOOS == "windows" {
		if root.Parent != drivesSentinel {
			t.Fatalf("listDir(/).Parent = %q at a drive root, want the drive list", root.Parent)
		}
	} else if root.Parent == drivesSentinel {
		t.Fatalf("listDir(/).Parent resolved to drivesSentinel on %s", runtime.GOOS)
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

// Папка может прекрасно открываться и не принимать запись — типичная сетевая
// шара, смонтированная только для чтения. Проверять надо записью, а не наличием.
func TestCheckDirWritable(t *testing.T) {
	dir := t.TempDir()
	if err := checkDirWritable(dir); err != nil {
		t.Errorf("обычная папка признана недоступной: %v", err)
	}

	// Несуществующую создаём — пользователь вправе указать путь заранее.
	fresh := filepath.Join(dir, "новая", "вложенная")
	if err := checkDirWritable(fresh); err != nil {
		t.Errorf("не удалось создать папку: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("папка не создана: %v", err)
	}

	// Пустой путь — не «текущий каталог», а ошибка: иначе файлы уехали бы
	// неизвестно куда.
	if err := checkDirWritable("   "); err == nil {
		t.Error("пустой путь принят")
	}

	// После проверки не должно оставаться следов.
	entries, _ := os.ReadDir(fresh)
	if len(entries) != 0 {
		t.Errorf("проверка оставила мусор: %v", entries)
	}
}

// Ярлыки нужны, чтобы до сетевой папки не пришлось идти кликами от корня.
func TestFSPlaces_HasHome(t *testing.T) {
	places := fsPlaces()
	if len(places) == 0 {
		t.Fatal("ярлыков нет вообще")
	}
	home, _ := os.UserHomeDir()
	found := false
	for _, p := range places {
		if p.Path == home {
			found = true
		}
		if p.Name == "" || p.Path == "" {
			t.Errorf("пустой ярлык: %+v", p)
		}
	}
	if home != "" && !found {
		t.Error("домашней папки нет среди ярлыков")
	}
}
