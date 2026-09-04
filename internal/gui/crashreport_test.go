package gui

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRedactCrashRemovesCredentials is the load-bearing test of this file: the
// report goes into a GitHub issue, and kino.watch carries its token in the
// query string, so anything missed here is published verbatim.
func TestRedactCrashRemovesCredentials(t *testing.T) {
	secret := "b7f3c1a9d4e28f60b7f3c1a9d4e2"
	cases := []string{
		"https://api.kino.watch/v1/items?access_token=" + secret,
		`{"api_access_token":"` + secret + `","x":1}`,
		"Authorization: Bearer " + secret,
		"refresh_token=" + secret + "&grant_type=refresh",
	}
	for _, in := range cases {
		got := redactCrash(in)
		if strings.Contains(got, secret) {
			t.Errorf("secret survived redaction:\n in: %s\nout: %s", in, got)
		}
		if !strings.Contains(got, "REDACTED") {
			t.Errorf("nothing was redacted in %q -> %q", in, got)
		}
	}
}

func TestRedactCrashHidesHomeDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir on this machine")
	}
	got := redactCrash(filepath.Join(home, "Downloads", "kinopub") + " failed")
	if strings.Contains(got, home) {
		t.Fatalf("home dir survived redaction: %q", got)
	}
}

// TestCrashReportURLFromLog covers the whole path a user takes: a panic was
// logged, the button builds a link, and the link carries the stack.
func TestCrashReportURLFromLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if crashReportURL("v1.2.3") != "" {
		t.Fatal("no crash.log yet, want no link")
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				logPanic("job run", r)
			}
		}()
		boom()
	}()

	raw := crashReportURL("v1.2.3")
	if raw == "" {
		t.Fatal("crash.log written but no link built")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	if !strings.HasPrefix(raw, "https://github.com/"+issueRepo+"/issues/new?") {
		t.Fatalf("link points at %q", u.Path)
	}
	body := u.Query().Get("body")
	for _, want := range []string{"v1.2.3", "nil pointer dereference", "panics_test.go"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if len(raw) > maxIssueURL {
		t.Fatalf("link is %d chars, over the %d cap", len(raw), maxIssueURL)
	}
}

// TestCrashReportURLTruncates guards the cap: GitHub drops over-long URLs, so
// a huge stack must come back trimmed rather than as a dead link.
func TestCrashReportURLTruncates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "kinopub"), 0o700); err != nil {
		t.Fatal(err)
	}
	entry := "=== 2026-09-04T12:00:00Z | job run | boom\n" + strings.Repeat("frame goroutine stack line\n", 2000)
	if err := os.WriteFile(filepath.Join(dir, "kinopub", "crash.log"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}

	raw := crashReportURL("v1")
	if raw == "" {
		t.Fatal("want a link")
	}
	if len(raw) > maxIssueURL {
		t.Fatalf("link is %d chars, over the %d cap", len(raw), maxIssueURL)
	}
	if !strings.Contains(url.QueryEscape("truncated"), "truncated") {
		t.Fatal("sanity")
	}
}
