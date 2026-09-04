package gui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/lib/credstore"
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

// TestPostCrashIssueSendsRedactedAndDedupes covers the whole automatic path:
// the right endpoint, the token as a bearer, a body with the secret already
// stripped, and a second identical crash refusing to file a duplicate — a
// panic in a loop can recur every tick.
func TestPostCrashIssueSendsRedactedAndDedupes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedCrash(t, dir)

	if err := credstore.Update(func(c *credstore.Credentials) { c.GitHubToken = "tok_secret" }); err != nil {
		t.Fatal(err)
	}

	var calls int
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"html_url": "https://github.com/x/y/issues/1"})
	}))
	defer srv.Close()

	old := githubAPI
	githubAPI = srv.URL
	defer func() { githubAPI = old }()

	url, err := postCrashIssue(context.Background(), "v1.2.3", "")
	if err != nil {
		t.Fatalf("postCrashIssue: %v", err)
	}
	if url != "https://github.com/x/y/issues/1" {
		t.Fatalf("returned %q, want the issue URL from the API", url)
	}
	if want := "/repos/" + issueRepo + "/issues"; gotPath != want {
		t.Fatalf("posted to %q, want %q", gotPath, want)
	}
	if gotAuth != "Bearer tok_secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if strings.Contains(gotBody, "abcdef0123456789") {
		t.Fatalf("the kino.watch token was published verbatim:\n%s", gotBody)
	}
	for _, want := range []string{"REDACTED", "v1.2.3", "jobs.go:1412"} {
		if !strings.Contains(gotBody, want) {
			t.Fatalf("body missing %q:\n%s", want, gotBody)
		}
	}

	if _, err := postCrashIssue(context.Background(), "v1.2.3", ""); err != errAlreadyReported {
		t.Fatalf("second call returned %v, want errAlreadyReported", err)
	}
	if calls != 1 {
		t.Fatalf("GitHub was called %d times for one crash", calls)
	}
}

// TestPostCrashIssueNeedsToken keeps the manual path honest: with no token the
// send must refuse rather than silently do nothing.
func TestPostCrashIssueNeedsToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedCrash(t, dir)

	if _, err := postCrashIssue(context.Background(), "v1", ""); err == nil {
		t.Fatal("want an error when no token is configured")
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
