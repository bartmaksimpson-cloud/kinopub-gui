package gui

import (
	"net/http"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/lib/credstore"
)

// TestUpdaterSendsTokenOnlyWhenStored pins the authorization: the repository
// is public, so updates must work with no token at all, and a stored one is
// sent only to lift the rate limit. Sending an empty bearer would turn a
// working update into an HTTP 401.
func TestUpdaterSendsTokenOnlyWhenStored(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	u := newUpdateChecker("v1.0.0")

	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}

	u.authorize(req)
	if h := req.Header.Get("Authorization"); h != "" {
		t.Fatalf("no token stored, yet an Authorization header was set: %q", h)
	}

	if err := credstore.Update(func(c *credstore.Credentials) { c.GitHubToken = "github_pat_example" }); err != nil {
		t.Fatalf("store token: %v", err)
	}

	u.authorize(req)
	if h := req.Header.Get("Authorization"); h != "Bearer github_pat_example" {
		t.Fatalf("Authorization = %q, want the stored token as a bearer", h)
	}
}
