package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedCrash writes one crash.log entry containing a secret, so every test here
// exercises the redaction on the way out as well.
func seedCrash(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "kinopub"), 0o700); err != nil {
		t.Fatal(err)
	}
	entry := "=== 2026-09-04T12:00:00Z | job run | boom\n" +
		"GET https://api.kino.watch/v1/items?access_token=abcdef0123456789 failed\n" +
		"internal/gui/jobs.go:1412\n"
	if err := os.WriteFile(filepath.Join(dir, "kinopub", "crash.log"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestReportBodyPrefersJobDetail covers reporting an ordinary failure: a job
// error never reaches crash.log because nothing panicked, so the UI hands it
// over directly — and it must be redacted just as hard, since a wrapped URL
// error carries the kino.watch token in its query string.
func TestReportBodyPrefersJobDetail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedCrash(t, dir)

	detail := "segment 66 failed: GET https://api.kino.watch/v1/x?access_token=live_secret_value: unexpected EOF"
	body := reportBody(detail)

	if strings.Contains(body, "live_secret_value") {
		t.Fatalf("the token survived redaction:\n%s", body)
	}
	if !strings.Contains(body, "segment 66 failed") {
		t.Fatalf("the job error was dropped:\n%s", body)
	}
	if strings.Contains(body, "job run") {
		t.Fatalf("crash.log was used even though a detail was supplied:\n%s", body)
	}

	if got := reportTitle(body, detail); !strings.HasPrefix(got, "download failed: ") {
		t.Fatalf("title = %q, a job failure must not be labelled a crash", got)
	}
	crash := reportBody("")
	if got := reportTitle(crash, ""); !strings.HasPrefix(got, "crash: ") {
		t.Fatalf("title = %q, a recovered panic is a crash", got)
	}
}
