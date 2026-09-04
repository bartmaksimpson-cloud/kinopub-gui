package gui

import (
	"net/http"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/lib/credstore"
)

// TestUpdaterUsesTokenRepoAndHeader pins the two halves of updating from a
// private repo: without a token nothing changes for ordinary installs (public
// repo, no Authorization), and with one the checker switches repositories and
// authenticates. Getting either half backwards means silently asking the wrong
// GitHub repo for releases.
func TestUpdaterUsesTokenRepoAndHeader(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	u := newUpdateChecker("v1.0.0")

	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/x", nil)
	if err != nil {
		t.Fatal(err)
	}

	if got := u.repo(); got != updateRepo {
		t.Fatalf("without a token the repo is %q, want the public %q", got, updateRepo)
	}
	u.authorize(req)
	if h := req.Header.Get("Authorization"); h != "" {
		t.Fatalf("no token stored, yet an Authorization header was set: %q", h)
	}

	if err := credstore.Update(func(c *credstore.Credentials) { c.GitHubToken = "github_pat_example" }); err != nil {
		t.Fatalf("store token: %v", err)
	}

	if got := u.repo(); got != issueRepo {
		t.Fatalf("with a token the repo is %q, want the private %q", got, issueRepo)
	}
	u.authorize(req)
	if h := req.Header.Get("Authorization"); h != "Bearer github_pat_example" {
		t.Fatalf("Authorization = %q, want the stored token as a bearer", h)
	}
}
