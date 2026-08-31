package gui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

func TestClampPct(t *testing.T) {
	cases := map[int]int{-5: 0, 0: 0, 50: 50, 100: 100, 150: 100}
	for in, want := range cases {
		if got := clampPct(in); got != want {
			t.Errorf("clampPct(%d) = %d, want %d", in, got, want)
		}
	}
}

// newReporterJob builds a JobManager + Job pair suitable for driving an
// eventReporter without any network/engine.
func newReporterJob() (*JobManager, *Job, *eventReporter) {
	m := newJobManager(newHub())
	j := newJob("job-1", "https://kino.watch/item/view/1", domain.RunConfig{})
	m.add(j)
	return m, j, newEventReporter(m, j)
}

func TestReporterStart_SeedsPlanAndPendingRows(t *testing.T) {
	_, j, r := newReporterJob()
	r.Start(domain.SeriesPlan{
		Title:            "My Show",
		PosterURL:        "poster.jpg",
		Total:            10,
		AlreadyCompleted: 2,
		Seasons:          map[int]int{1: 5, 2: 5},
		Planned: []domain.PlannedEpisode{
			{Key: domain.EpisodeKey{Season: 1, Episode: 3}, Title: "Ep3"},
			{Key: domain.EpisodeKey{Season: 1, Episode: 4}},
		},
	})
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.title != "My Show" || j.posterURL != "poster.jpg" {
		t.Errorf("title/poster not seeded: %q / %q", j.title, j.posterURL)
	}
	if j.plan == nil || j.plan.Total != 10 || j.plan.AlreadyCompleted != 2 {
		t.Fatalf("plan wrong: %+v", j.plan)
	}
	if len(j.episodes) != 2 {
		t.Fatalf("expected 2 pending rows, got %d", len(j.episodes))
	}
	ev := j.episodes["S1E3"]
	if ev == nil || ev.State != epPending || ev.Title != "Ep3" {
		t.Errorf("S1E3 row wrong: %+v", ev)
	}
}

func TestReporterStart_ResetsExistingRow(t *testing.T) {
	_, j, r := newReporterJob()
	// Pre-seed a failed row with stale progress (a re-run scenario).
	j.episodes["S1E1"] = &EpisodeView{
		Key: "S1E1", Season: 1, Episode: 1, State: epFailed,
		Percent: 80, Bytes: 1234, SpeedBps: 99, ETASeconds: 5, Error: "old error",
		Tracks: []TrackView{{Label: "Video"}},
	}
	r.Start(domain.SeriesPlan{
		Planned: []domain.PlannedEpisode{{Key: domain.EpisodeKey{Season: 1, Episode: 1}}},
	})
	ev := j.episodes["S1E1"]
	if ev.State != epPending || ev.Percent != 0 || ev.Bytes != 0 || ev.SpeedBps != 0 ||
		ev.ETASeconds != 0 || ev.Error != "" || ev.Tracks != nil {
		t.Errorf("stale row not reset: %+v", ev)
	}
}

func TestReporterEpisodeLifecycle(t *testing.T) {
	_, j, r := newReporterJob()
	key := domain.EpisodeKey{Season: 1, Episode: 1}

	r.EpisodeStarted(key)
	if j.episodes["S1E1"].State != epRunning {
		t.Fatalf("after start: %q", j.episodes["S1E1"].State)
	}

	r.EpisodeCompleted(key)
	ev := j.episodes["S1E1"]
	if ev.State != epCompleted || ev.Percent != 100 || ev.SpeedBps != 0 {
		t.Errorf("after complete: %+v", ev)
	}
}

func TestReporterEpisodeStarted_ResetsCompletedPercent(t *testing.T) {
	_, j, r := newReporterJob()
	key := domain.EpisodeKey{Season: 1, Episode: 1}
	j.episodes["S1E1"] = &EpisodeView{Key: "S1E1", Season: 1, Episode: 1, Percent: 100, State: epCompleted}
	r.EpisodeStarted(key)
	if j.episodes["S1E1"].Percent != 0 {
		t.Errorf("a re-started episode at 100%% should reset to 0, got %d", j.episodes["S1E1"].Percent)
	}
}

func TestReporterEpisodeFailed_PreservesDeferred(t *testing.T) {
	_, j, r := newReporterJob()
	key := domain.EpisodeKey{Season: 1, Episode: 1}
	// Mark deferred first; a subsequent generic Failed must NOT clobber it.
	r.EpisodeDeferred(key, errors.New("transient"), 2)
	if j.episodes["S1E1"].State != epDeferred || j.episodes["S1E1"].Attempts != 2 {
		t.Fatalf("deferred not recorded: %+v", j.episodes["S1E1"])
	}
	r.EpisodeFailed(key, errors.New("boom"))
	if j.episodes["S1E1"].State != epDeferred {
		t.Errorf("Failed clobbered deferred state: %q", j.episodes["S1E1"].State)
	}
	if j.episodes["S1E1"].Error != "boom" {
		t.Errorf("error should update to boom, got %q", j.episodes["S1E1"].Error)
	}
}

func TestReporterEpisodeFailed_SetsFailed(t *testing.T) {
	_, j, r := newReporterJob()
	key := domain.EpisodeKey{Season: 1, Episode: 1}
	r.EpisodeStarted(key)
	r.EpisodeFailed(key, errors.New("kaboom"))
	ev := j.episodes["S1E1"]
	if ev.State != epFailed || ev.Error != "kaboom" {
		t.Errorf("after fail: %+v", ev)
	}
}

// A stop the user asked for must not surface as an internal Go error chain:
// whatever the engine was doing when the context died, the row reads "canceled".
func TestReporterFoldsCancellationIntoCanceled(t *testing.T) {
	wrapped := fmt.Errorf("audio track 0: %w", fmt.Errorf("segment 10 failed: %w", context.Canceled))

	t.Run("failed", func(t *testing.T) {
		_, j, r := newReporterJob()
		key := domain.EpisodeKey{Season: 1, Episode: 1}
		r.EpisodeStarted(key)
		r.EpisodeFailed(key, wrapped)
		if got := j.episodes["S1E1"].Error; got != errCanceled {
			t.Errorf("Error = %q, want %q", got, errCanceled)
		}
	})

	t.Run("deferred", func(t *testing.T) {
		_, j, r := newReporterJob()
		key := domain.EpisodeKey{Season: 1, Episode: 1}
		r.EpisodeStarted(key)
		r.EpisodeDeferred(key, wrapped, 1)
		if got := j.episodes["S1E1"].Error; got != errCanceled {
			t.Errorf("Error = %q, want %q", got, errCanceled)
		}
	})

	t.Run("real errors are untouched", func(t *testing.T) {
		_, j, r := newReporterJob()
		key := domain.EpisodeKey{Season: 1, Episode: 1}
		r.EpisodeStarted(key)
		r.EpisodeFailed(key, errors.New("segment 10 failed: 403 Forbidden"))
		if got := j.episodes["S1E1"].Error; got != "segment 10 failed: 403 Forbidden" {
			t.Errorf("Error = %q, want the original message", got)
		}
	})
}

func TestReporterByteProgress(t *testing.T) {
	_, j, r := newReporterJob()
	key := domain.EpisodeKey{Season: 1, Episode: 1}
	r.ByteProgress(key, 25, 100)
	ev := j.episodes["S1E1"]
	if ev.Bytes != 25 || ev.Total != 100 || ev.Percent != 25 {
		t.Errorf("byte progress: %+v", ev)
	}
	// Zero total → percent untouched (stays 25), no divide-by-zero.
	r.ByteProgress(key, 30, 0)
	if j.episodes["S1E1"].Percent != 25 {
		t.Errorf("zero total should leave percent, got %d", j.episodes["S1E1"].Percent)
	}
}

func TestReporterSegmentProgress(t *testing.T) {
	_, j, r := newReporterJob()
	key := domain.EpisodeKey{Season: 1, Episode: 1}
	r.SegmentProgress(key, 3, 10, 300, 1000)
	ev := j.episodes["S1E1"]
	if ev.SegDone != 3 || ev.SegTotal != 10 || ev.Percent != 30 || ev.Bytes != 300 || ev.Total != 1000 {
		t.Errorf("segment progress: %+v", ev)
	}
}

func TestReporterTrackProgress_NonHLS(t *testing.T) {
	_, j, r := newReporterJob()
	key := domain.EpisodeKey{Season: 1, Episode: 1}
	// No HLS tracks → percent tracks the single-stream percent and only increases.
	r.TrackProgress(key, domain.TrackRef{}, 40)
	if j.episodes["S1E1"].Percent != 40 || j.episodes["S1E1"].State != epRunning {
		t.Errorf("track progress: %+v", j.episodes["S1E1"])
	}
	r.TrackProgress(key, domain.TrackRef{}, 30) // lower → ignored
	if j.episodes["S1E1"].Percent != 40 {
		t.Errorf("percent must not decrease, got %d", j.episodes["S1E1"].Percent)
	}
}

func TestReporterHLSProgress(t *testing.T) {
	_, j, r := newReporterJob()
	key := domain.EpisodeKey{Season: 1, Episode: 1}
	r.HLSProgress(key, []domain.TrackProgressInfo{
		{Label: "Video", DoneSegments: 5, TotalSegments: 10, DownloadedBytes: 500, ApproxTotalBytes: 1000},
		{Label: "Audio", DoneSegments: 0, TotalSegments: 0}, // zero total → 0%
	})
	ev := j.episodes["S1E1"]
	if len(ev.Tracks) != 2 {
		t.Fatalf("want 2 tracks, got %d", len(ev.Tracks))
	}
	if ev.Tracks[0].Percent != 50 || ev.Tracks[0].Label != "Video" {
		t.Errorf("video track: %+v", ev.Tracks[0])
	}
	if ev.Tracks[1].Percent != 0 {
		t.Errorf("zero-total audio track should be 0%%, got %d", ev.Tracks[1].Percent)
	}
}

func TestReporterUpdateSpeed(t *testing.T) {
	_, _, r := newReporterJob()
	ev := &EpisodeView{}
	// First call only records the baseline (no speed yet).
	r.updateSpeed(ev, 0, 1000)
	if ev.SpeedBps != 0 {
		t.Errorf("first sample should not set speed, got %v", ev.SpeedBps)
	}
	// Simulate elapsed time by backdating lastTime, then a second sample.
	ev.lastTime = time.Now().Add(-1 * time.Second)
	ev.lastBytes = 0
	r.updateSpeed(ev, 500, 1000) // 500 bytes in ~1s
	if ev.SpeedBps <= 0 {
		t.Errorf("speed should be positive after a second sample, got %v", ev.SpeedBps)
	}
	if ev.ETASeconds <= 0 {
		t.Errorf("ETA should be positive while bytes remain, got %d", ev.ETASeconds)
	}
}

func TestReporterUpdateSpeed_NegativeInstClampedToZero(t *testing.T) {
	_, _, r := newReporterJob()
	ev := &EpisodeView{}
	ev.lastTime = time.Now().Add(-1 * time.Second)
	ev.lastBytes = 1000 // downloaded goes DOWN (counter reset) → inst negative → clamp 0
	r.updateSpeed(ev, 500, 2000)
	if ev.SpeedBps != 0 {
		t.Errorf("negative instantaneous speed should clamp to 0, got %v", ev.SpeedBps)
	}
}

// Seeding the plan must not disturb an episode the user is holding. A run
// started to release ONE episode of a paused job re-pauses the rest, so
// resetting them here would show them queued while the engine holds them — and
// throw away the progress they are holding on to.
func TestReporterStart_KeepsHeldEpisodesHeld(t *testing.T) {
	_, j, r := newReporterJob()
	j.episodes["S1E1"] = &EpisodeView{Key: "S1E1", Season: 1, Episode: 1, State: epPaused, Percent: 42, Bytes: 4200}
	j.episodes["S1E2"] = &EpisodeView{Key: "S1E2", Season: 1, Episode: 2, State: epFailed, Percent: 7, Error: "boom"}

	r.Start(domain.SeriesPlan{
		Title: "Show",
		Total: 2,
		Planned: []domain.PlannedEpisode{
			{Key: domain.EpisodeKey{Season: 1, Episode: 1}},
			{Key: domain.EpisodeKey{Season: 1, Episode: 2}},
		},
	})

	held := j.episodes["S1E1"]
	if held.State != epPaused {
		t.Errorf("held episode = %q, want it left paused", held.State)
	}
	if held.Percent != 42 || held.Bytes != 4200 {
		t.Errorf("held episode lost its progress: %+v", held)
	}
	// Everything else is a fresh attempt and does get reset.
	fresh := j.episodes["S1E2"]
	if fresh.State != epPending || fresh.Percent != 0 || fresh.Error != "" {
		t.Errorf("planned episode = %+v, want it reset to pending", fresh)
	}
}
