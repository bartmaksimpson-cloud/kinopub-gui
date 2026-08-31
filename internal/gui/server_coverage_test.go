package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// The Download page sends Seasons/Episodes as text expressions, never explicit
// episode keys. The coverage guard used to treat every such request as "the
// whole title" and 409 it whenever ANY active job existed for the same
// URL+folder — even when the selections were fully disjoint. Overlap must be
// tested against the expression itself.
func TestHandleCreateJob_SeasonExpressionCheckedAgainstCoverage(t *testing.T) {
	const url = "https://kino.watch/item/view/409"

	// The handler checks that ffmpeg exists before queueing; point it at the
	// one executable guaranteed to exist on every platform — this test binary —
	// so the presence check never decides the test's outcome.
	fakeFFmpeg, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	post := func(t *testing.T, s *Server, out, seasons string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(StartRequest{RunRequest: RunRequest{
			URL:        url,
			OutputPath: out,
			Seasons:    seasons,
			FFmpegPath: fakeFFmpeg,
		}})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/api/jobs", bytes.NewReader(body))
		w := httptest.NewRecorder()
		s.handleCreateJob(w, req)
		return w
	}

	t.Run("a disjoint season is a legitimate new run", func(t *testing.T) {
		s := newTestServer(t)
		out := t.TempDir()
		s.mgr.add(activeJob("a", url, out,
			domain.EpisodeKey{Season: 1, Episode: 1}, domain.EpisodeKey{Season: 1, Episode: 2}))
		if w := post(t, s, out, "2"); w.Code != http.StatusAccepted {
			t.Errorf("status = %d (%s), want 202 — season 2 overlaps nothing", w.Code, w.Body.String())
		}
	})

	t.Run("an overlapping expression is refused", func(t *testing.T) {
		s := newTestServer(t)
		out := t.TempDir()
		s.mgr.add(activeJob("a", url, out,
			domain.EpisodeKey{Season: 1, Episode: 1}, domain.EpisodeKey{Season: 1, Episode: 2}))
		if w := post(t, s, out, "1"); w.Code != http.StatusConflict {
			t.Errorf("status = %d, want 409 — season 1 reaches episodes already downloading", w.Code)
		}
	})

	t.Run("an unresolved whole-title job still blocks everything", func(t *testing.T) {
		s := newTestServer(t)
		out := t.TempDir()
		s.mgr.add(activeJob("a", url, out)) // no selection: reach unknown
		if w := post(t, s, out, "2"); w.Code != http.StatusConflict {
			t.Errorf("status = %d, want 409 — the active job may take any episode", w.Code)
		}
	})
}
