package gui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// writeBrokenSeries lays out a series folder that gives the doctor exactly two
// things to fix: a completed record whose file is gone, and an orphan .tmp with
// no finished file beside it (what an interrupted download leaves behind).
func writeBrokenSeries(t *testing.T, dir string) (stateFile, tmpFile string) {
	t.Helper()
	state := domain.DownloadState{
		Series: "42",
		Completed: map[string]domain.CompletedRec{
			"S1E1": {Season: 1, Episode: 1, Path: "Season 01/S01E01.mkv", Bytes: 1000, CompletedAt: time.Now()},
		},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	stateFile = filepath.Join(dir, stateFileName)
	if err := os.WriteFile(stateFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	tmpFile = filepath.Join(dir, "S01E02.mkv.tmp")
	if err := os.WriteFile(tmpFile, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	return stateFile, tmpFile
}

func completedCount(t *testing.T, stateFile string) int {
	t.Helper()
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	var state domain.DownloadState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return len(state.Completed)
}

// A live download owns the .tmp files in its output folder — they are what a
// resume continues from — so the doctor must report but change nothing.
func TestRunDoctorRefusesRepairWhileDownloadIsLive(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Some Series")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile, tmpFile := writeBrokenSeries(t, dir)

	view, err := runDoctor(context.Background(),
		DoctorRequest{OutputDir: root, Fix: true, CleanTmp: true},
		[]string{root}, // a job is downloading into this very folder
	)
	if err != nil {
		t.Fatalf("runDoctor: %v", err)
	}

	if !view.RepairBlocked {
		t.Error("repairBlocked should be set so the UI can explain the refusal")
	}
	if view.Fixed {
		t.Error("fixed should be false — nothing was repaired")
	}
	if !view.HasIssues {
		t.Error("the check itself must still run and report what's wrong")
	}
	if n := completedCount(t, stateFile); n != 1 {
		t.Errorf("state entry was dropped despite the guard: %d entries left, want 1", n)
	}
	if _, err := os.Stat(tmpFile); err != nil {
		t.Errorf("orphan .tmp was deleted despite the guard: %v", err)
	}
}

// The guard is scoped to the folder: a download into an unrelated root must not
// freeze maintenance everywhere else.
func TestRunDoctorRepairsWhenLiveJobIsElsewhere(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Some Series")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile, tmpFile := writeBrokenSeries(t, dir)

	view, err := runDoctor(context.Background(),
		DoctorRequest{OutputDir: root, Fix: true, CleanTmp: true},
		[]string{t.TempDir()}, // a job, but downloading somewhere else entirely
	)
	if err != nil {
		t.Fatalf("runDoctor: %v", err)
	}

	if view.RepairBlocked {
		t.Error("an unrelated folder must not block repair")
	}
	if !view.Fixed {
		t.Error("fixed should be true")
	}
	if n := completedCount(t, stateFile); n != 0 {
		t.Errorf("broken state entry should be dropped, %d left", n)
	}
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("orphan .tmp should be deleted, stat err = %v", err)
	}
}

// A plain check asks for nothing destructive, so there is nothing to refuse —
// the banner must not appear just because a download is running.
func TestRunDoctorReadOnlyCheckIsNeverBlocked(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Some Series")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile, _ := writeBrokenSeries(t, dir)

	view, err := runDoctor(context.Background(), DoctorRequest{OutputDir: root}, []string{root})
	if err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
	if view.RepairBlocked {
		t.Error("a read-only check should not report a blocked repair")
	}
	if !view.HasIssues {
		t.Error("issues should still be reported")
	}
	if n := completedCount(t, stateFile); n != 1 {
		t.Errorf("a read-only check must not touch the state file: %d entries, want 1", n)
	}
}

func TestPathsOverlap(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "series", "Season 01")
	other := t.TempDir()

	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"same folder", root, root, true},
		{"child inside parent", sub, root, true},
		{"parent contains child", root, sub, true},
		{"unrelated roots", root, other, false},
		{"trailing separator", root + string(filepath.Separator), root, true},
		{"empty is never an overlap", "", root, false},
		// A sibling whose name merely starts with the same characters is not
		// inside it — the check must compare path elements, not string prefixes.
		{"prefix-lookalike sibling", root + "-backup", root, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathsOverlap(tc.a, tc.b); got != tc.want {
				t.Errorf("pathsOverlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestLiveOutputPathsExcludesFinishedJobs(t *testing.T) {
	m := newJobManager(newHub())

	for _, tc := range []struct {
		id, status, out string
	}{
		{"queued", statusQueued, "/dl/a"},
		{"running", statusRunning, "/dl/a"}, // same folder — deduped
		{"paused", statusPaused, "/dl/b"},
		{"failed", statusFailed, "/dl/c"}, // retryable: its partials still matter
		{"done", statusCompleted, "/dl/done"},
		{"canceled", statusCanceled, "/dl/canceled"},
	} {
		j := newJob(tc.id, "u", domain.RunConfig{})
		j.status = tc.status
		j.outputPath = tc.out
		m.add(j)
	}

	got := map[string]bool{}
	for _, p := range m.liveOutputPaths() {
		if got[p] {
			t.Errorf("duplicate path %q", p)
		}
		got[p] = true
	}
	want := []string{"/dl/a", "/dl/b", "/dl/c"}
	if len(got) != len(want) {
		t.Fatalf("liveOutputPaths() = %v, want %v", got, want)
	}
	for _, p := range want {
		if !got[p] {
			t.Errorf("missing live path %q", p)
		}
	}
}
