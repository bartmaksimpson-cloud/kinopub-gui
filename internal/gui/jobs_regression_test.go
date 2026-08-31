package gui

// Regression tests for a batch of concurrency/consistency fixes:
//   - the engine's cancel ack resurrecting a deleted episode row,
//   - JobView snapshots sharing the live plan map with SSE marshaling,
//   - remove() racing a concurrent resume (purge deleting files under a
//     freshly revived engine),
//   - dedupeQueue deleting expression-scoped cards it should trim.

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// The engine acknowledges every per-episode cancel with a generic
// EpisodeFailed("canceled"). That ack must not re-create the row cancelEpisode
// just deleted — the resurrected failed row with a Retry button is exactly what
// the cancel exists to remove.
func TestCancelEpisode_EngineAckDoesNotResurrectTheRow(t *testing.T) {
	m := newJobManager(newHub())
	j := runningJobWithEpisodes("j", epRunning, epPending)
	j.plan = &PlanView{Total: 2, Seasons: map[int]int{1: 2}}
	m.add(j)

	key := domain.EpisodeKey{Season: 1, Episode: 2}
	if !m.cancelEpisode("j", key) {
		t.Fatal("a queued episode should be cancelable")
	}
	newEventReporter(m, j).EpisodeFailed(key, errors.New("canceled"))

	j.mu.Lock()
	defer j.mu.Unlock()
	if _, still := j.episodes["S1E2"]; still {
		t.Error("the engine's cancel ack resurrected the deleted row")
	}
	if j.plan.Total != 1 || j.plan.Seasons[1] != 1 {
		t.Errorf("plan = %d total / %d in season, want 1/1", j.plan.Total, j.plan.Seasons[1])
	}
}

// Canceling the engine's last runnable episode leaves only user-held ones; the
// ack must trigger the same auto-pause a completion or failure would, or the
// run sits "running" doing nothing while holding a download slot.
func TestCancelEpisode_AckPausesJobWhenOnlyHeldEpisodesRemain(t *testing.T) {
	m := newJobManager(newHub())
	j := runningJobWithEpisodes("j", epPaused, epPending)
	stopped := false
	j.cancel = func() { stopped = true }
	m.add(j)

	key := domain.EpisodeKey{Season: 1, Episode: 2}
	if !m.cancelEpisode("j", key) {
		t.Fatal("a queued episode should be cancelable")
	}
	newEventReporter(m, j).EpisodeFailed(key, errors.New("canceled"))

	if !j.paused.Load() || !stopped {
		t.Error("canceling the last runnable episode left the run holding its slot")
	}
}

// When the download finished in the same instant the user canceled it, success
// wins: the file is on disk, so the row comes back — and the plan counts the
// cancel subtracted come back with it.
func TestCancelEpisode_CompletionWinsAndRestoresThePlan(t *testing.T) {
	m := newJobManager(newHub())
	j := runningJobWithEpisodes("j", epRunning, epRunning)
	j.plan = &PlanView{Total: 2, Seasons: map[int]int{1: 2}}
	m.add(j)

	key := domain.EpisodeKey{Season: 1, Episode: 2}
	if !m.cancelEpisode("j", key) {
		t.Fatal("a downloading episode should be cancelable")
	}
	newEventReporter(m, j).EpisodeCompleted(key)

	j.mu.Lock()
	defer j.mu.Unlock()
	ev := j.episodes["S1E2"]
	if ev == nil || ev.State != epCompleted {
		t.Fatalf("episode row = %+v, want it back as completed", ev)
	}
	if j.plan.Total != 2 || j.plan.Seasons[1] != 2 {
		t.Errorf("plan = %d total / %d in season, want the counts restored to 2/2", j.plan.Total, j.plan.Seasons[1])
	}
}

// JobView snapshots are json.Marshal-ed by SSE and handler goroutines with no
// lock held, so the plan they carry must be a copy — cancelEpisode mutates the
// live plan (including its Seasons map) in place under j.mu.
func TestSnapshot_DoesNotShareThePlanWithTheLiveJob(t *testing.T) {
	j := runningJobWithEpisodes("j", epPending)
	j.plan = &PlanView{Total: 5, Seasons: map[int]int{1: 5}}

	view := j.snapshot()

	j.mu.Lock()
	j.plan.Total = 4
	j.plan.Seasons[1] = 4
	j.mu.Unlock()

	if view.Plan.Total != 5 || view.Plan.Seasons[1] != 5 {
		t.Errorf("snapshot plan mutated after the fact (%d/%d) — it shares state with the live job",
			view.Plan.Total, view.Plan.Seasons[1])
	}
}

// A resume that fetched the *Job just before remove() deleted it from the map
// must not revive the card: the engine it would start writes the very files a
// purge is deleting, and the revived job is unreachable from the UI (a ghost).
func TestRemove_AStaleResumeCannotReviveTheCard(t *testing.T) {
	m := newJobManager(newHub())
	j := runningJobWithEpisodes("j", epPaused)
	j.status = statusPaused
	m.add(j)

	if ok, _ := m.remove("j", false); !ok {
		t.Fatal("a paused card should be removable")
	}
	// Reproduce the race window: the resume path already holds the pointer and
	// only now takes j.mu. Re-adding the entry makes the id resolvable again the
	// way the stale pointer was.
	m.add(j)
	if m.resumeJob("j") {
		t.Error("a removed card was revived by a stale resume")
	}
	if m.rerunJob("j") {
		t.Error("a removed card was revived by a stale rerun")
	}
	if m.rerunJobEpisode("j", domain.EpisodeKey{Season: 1, Episode: 1}) {
		t.Error("a removed card was revived by a stale per-episode retry")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.status != statusPaused {
		t.Errorf("status = %q, want the removed card left untouched", j.status)
	}
}

// The same item arrives under many spellings — a bare id, a trailing slash,
// http vs https, query noise. The duplicate guard must match them all, or a
// second engine lands on the same series folder and state file.
func TestDuplicateGuard_MatchesAcrossURLSpellings(t *testing.T) {
	const out = "/downloads"
	m := newJobManager(newHub())
	m.add(activeJob("a", "https://kino.watch/item/view/409", out))
	for _, url := range []string{
		"409",
		"https://kino.watch/item/view/409/",
		"http://kino.watch/item/view/409?x=1",
	} {
		cfg := domain.RunConfig{InputURL: url, OutputPath: out}
		if _, ok := m.findActiveDuplicate(cfg); !ok {
			t.Errorf("URL %q escaped the duplicate guard", url)
		}
		if _, whole, _, found := m.queueCoverage(cfg); !found || !whole {
			t.Errorf("URL %q: coverage found=%v whole=%v, want true/true", url, found, whole)
		}
	}
}

// Queues written by older versions carry that version's user tuning
// (Concurrency up to 16, inter-request delays, chunked opt-outs). A restored
// card must come back clamped to the current fixed tuning, or a single Resume
// floods the CDN with the very parallelism those knobs were removed to prevent.
func TestRestoreJob_NormalizesLegacyTuning(t *testing.T) {
	j := restoreJob(persistedJob{
		ID:     "job-1",
		URL:    "https://kino.pub/item/view/409",
		Status: statusRunning,
		Cfg: domain.RunConfig{
			InputURL:       "https://kino.pub/item/view/409",
			MaxConcurrency: 16,
			MaxRetries:     99,
			MinIntervalMS:  5000,
			NoChunked:      true,
		},
	})
	if j.cfg.MaxConcurrency != episodeConcurrency {
		t.Errorf("MaxConcurrency = %d, want the fixed %d", j.cfg.MaxConcurrency, episodeConcurrency)
	}
	if j.cfg.MaxRetries != episodeRetries {
		t.Errorf("MaxRetries = %d, want the fixed %d", j.cfg.MaxRetries, episodeRetries)
	}
	if j.cfg.MinIntervalMS != 0 || j.cfg.NoChunked {
		t.Errorf("MinIntervalMS = %d, NoChunked = %v — legacy throttling knobs must not survive restore",
			j.cfg.MinIntervalMS, j.cfg.NoChunked)
	}
}

// A restored card whose scope came from a season/episode expression has
// resolved rows but no SelectedEpisodes. Its reach must be measured by the
// rows: with nothing overlapping it survives whole, and with a partial overlap
// it is trimmed — not silently deleted with the episodes only it owns.
func TestDedupeQueue_ExpressionScopedCardIsTrimmedNotDropped(t *testing.T) {
	const url = "https://kino.watch/item/view/409"
	const out = "/downloads"
	rows := func(j *Job, season, from, to int, state string) {
		for i := from; i <= to; i++ {
			key := fmt.Sprintf("S%dE%d", season, i)
			j.episodes[key] = &EpisodeView{Key: key, Season: season, Episode: i, State: state}
		}
	}

	t.Run("disjoint seasons both survive untouched", func(t *testing.T) {
		m := newJobManager(newHub())
		a := queuedAt("s1", url, out, time.Hour)
		rows(a, 1, 1, 3, epPaused)
		b := queuedAt("s2", url, out, time.Minute)
		rows(b, 2, 1, 3, epPaused)
		m.add(a)
		m.add(b)
		if n := m.dedupeQueue(); n != 0 {
			t.Fatalf("dropped %d cards, want 0 — the cards cover different seasons", n)
		}
		j, _ := m.get("s2")
		j.mu.Lock()
		defer j.mu.Unlock()
		if len(j.cfg.SelectedEpisodes) != 0 {
			t.Errorf("selection = %v, want the expression scope left untouched", j.cfg.SelectedEpisodes)
		}
		if len(j.episodes) != 3 {
			t.Errorf("episodes = %d rows, want all 3 kept", len(j.episodes))
		}
	})

	t.Run("partial overlap is trimmed to the unclaimed rows", func(t *testing.T) {
		m := newJobManager(newHub())
		older := queuedAt("older", url, out, time.Hour, domain.EpisodeKey{Season: 1, Episode: 1})
		full := queuedAt("full", url, out, time.Minute)
		rows(full, 1, 1, 3, epPaused)
		m.add(older)
		m.add(full)
		if n := m.dedupeQueue(); n != 0 {
			t.Fatalf("dropped %d cards, want 0 — episodes 2-3 are this card's alone", n)
		}
		j, _ := m.get("full")
		j.mu.Lock()
		defer j.mu.Unlock()
		if len(j.cfg.SelectedEpisodes) != 2 {
			t.Errorf("selection = %v, want exactly episodes 2-3", j.cfg.SelectedEpisodes)
		}
		for _, k := range j.cfg.SelectedEpisodes {
			if k.Episode == 1 {
				t.Errorf("selection kept %v, which the older card already claims", k)
			}
		}
		if j.episodes["S1E1"] != nil {
			t.Error("the overlapping row should be handed back to the older card")
		}
		if j.episodes["S1E2"] == nil || j.episodes["S1E3"] == nil {
			t.Error("rows this card still owns must survive the trim")
		}
	})
}
