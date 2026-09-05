package gui

import (
	"context"
	"errors"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// errText renders an engine error for the UI. A download stopped by the user
// (job or episode cancel, pause) surfaces as a wrapped context.Canceled —
// "audio track 0: segment 10 failed: context canceled" — which reads like a
// crash for something the user just asked for. Collapse the whole family to the
// same plain "canceled" the cancel paths already write, so the card shows an
// outcome instead of an internal error chain.
func errText(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return errCanceled
	}
	return err.Error()
}

// eventReporter implements domain.ProgressReporter plus every optional progress
// sink the engine probes for (ByteProgressSink, SegmentProgressSink,
// HLSProgressSink) and the optional EpisodeDeferred hook. It folds each update
// into its Job and asks the manager to broadcast.
type eventReporter struct {
	mgr *JobManager
	job *Job
}

func newEventReporter(mgr *JobManager, job *Job) *eventReporter {
	return &eventReporter{mgr: mgr, job: job}
}

// Compile-time checks that we satisfy every interface the engine may assert.
var (
	_ domain.ProgressReporter    = (*eventReporter)(nil)
	_ domain.ByteProgressSink    = (*eventReporter)(nil)
	_ domain.SegmentProgressSink = (*eventReporter)(nil)
	_ domain.HLSProgressSink     = (*eventReporter)(nil)
)

func (r *eventReporter) Start(plan domain.SeriesPlan) {
	r.job.mu.Lock()
	if plan.Title != "" && r.job.title == "" {
		r.job.title = plan.Title
	}
	// The folder the engine actually resolved. Recorded separately from the title
	// because a card started from a search result keeps that view's short title
	// while the folder is named after the full one — deriving the path from
	// r.job.title would point at a directory that does not exist.
	if plan.Dir != "" {
		r.job.seriesDir = plan.Dir
	}
	// Surface the poster the engine scraped, so jobs started without a Preview
	// (which would otherwise seed it) still show cover art.
	if plan.PosterURL != "" && r.job.posterURL == "" {
		r.job.posterURL = plan.PosterURL
	}
	seasons := make(map[int]int, len(plan.Seasons))
	for k, v := range plan.Seasons {
		seasons[k] = v
	}
	r.job.plan = &PlanView{
		Title:            plan.Title,
		Total:            plan.Total,
		AlreadyCompleted: plan.AlreadyCompleted,
		Seasons:          seasons,
	}
	// Seed a pending row per planned episode so not-yet-started episodes are
	// visible immediately and can be reordered ("download next") before they run.
	// On a re-run (retry/resume) a row may already exist in a failed/paused state
	// — reset every planned (i.e. not-yet-completed) episode to pending and clear
	// stale progress so the card reflects the fresh attempt. Completed episodes
	// are never in plan.Planned, so they keep their state.
	for _, pe := range plan.Planned {
		key := epKey(pe.Key)
		if pe.Title != "" {
			r.job.titles[key] = pe.Title
		}
		ev, ok := r.job.episodes[key]
		if !ok {
			ev = &EpisodeView{Key: key, Season: pe.Key.Season, Episode: pe.Key.Episode, Title: pe.Title}
			r.job.episodes[key] = ev
		}
		// An episode the user is holding stays held. This run may have been
		// started to release ONE episode of a paused job, re-pausing the rest —
		// resetting them here would show them queued while the engine holds them,
		// and lose the progress they are holding on to. A whole-job resume clears
		// its paused rows before the run (resetEpisodesForRerunLocked), so only
		// deliberate holds survive this.
		if ok && ev.State == epPaused {
			continue
		}
		ev.State = epPending
		ev.Percent = 0
		ev.Bytes = 0
		ev.SpeedBps = 0
		ev.ETASeconds = 0
		ev.Error = ""
		ev.Tracks = nil
	}
	r.job.mu.Unlock()
	r.mgr.publishNow(r.job)
}

func (r *eventReporter) EpisodeStarted(key domain.EpisodeKey) {
	r.job.mu.Lock()
	ev := r.job.ensureEpisode(key)
	ev.State = epRunning
	if ev.Percent >= 100 {
		ev.Percent = 0
	}
	ev.Error = ""
	ev.lastTime = time.Time{}
	ev.lastBytes = 0
	r.job.mu.Unlock()
	r.mgr.publishNow(r.job)
}

func (r *eventReporter) TrackProgress(key domain.EpisodeKey, track domain.TrackRef, percent int) {
	r.job.mu.Lock()
	ev := r.job.ensureEpisode(key)
	if ev.State == epPending {
		ev.State = epRunning
	}
	// For non-HLS (single-stream) downloads the overall percent tracks the
	// reported track percent. HLS overrides Percent via SegmentProgress.
	if len(ev.Tracks) == 0 && percent > ev.Percent {
		ev.Percent = clampPct(percent)
	}
	r.job.mu.Unlock()
	r.mgr.publish(r.job)
}

func (r *eventReporter) EpisodeCompleted(key domain.EpisodeKey) {
	r.job.mu.Lock()
	// Стадия относится к работе, которой больше нет: иначе готовый эпизод
	// остаётся с надписью «перекодирование».
	if ev, ok := r.job.episodes[epKey(key)]; ok {
		ev.Stage, ev.StageFormat, ev.StageEncoder, ev.StageThreads = "", "", "", 0
		ev.StagePercent, ev.StageETASeconds = 0, 0
	}
	// Success beat a simultaneous per-episode cancel: the file landed on disk,
	// so the row cancelEpisode deleted comes back (via ensureEpisode below) —
	// and the plan counts the cancel subtracted come back with it, keeping
	// "N of M episodes" consistent with the rows on the card.
	if r.job.canceledEps[epKey(key)] {
		delete(r.job.canceledEps, epKey(key))
		if r.job.plan != nil {
			r.job.plan.Total++
			if r.job.plan.Seasons != nil {
				r.job.plan.Seasons[key.Season]++
			}
		}
	}
	ev := r.job.ensureEpisode(key)
	ev.State = epCompleted
	ev.Percent = 100
	ev.SpeedBps = 0
	ev.ETASeconds = 0
	ev.Error = ""
	r.job.mu.Unlock()
	r.mgr.publishNow(r.job)
	// This may have been the last episode the engine had to work on, with every
	// other one held by a per-episode pause. The run would otherwise stay alive
	// polling for work that only a user can release.
	r.mgr.autoPauseIfAllHeld(r.job)
}

func (r *eventReporter) EpisodeFailed(key domain.EpisodeKey, err error) {
	r.job.mu.Lock()
	// The engine acknowledges a per-episode cancel through this generic hook.
	// cancelEpisode already removed the row (and shrank the plan), so falling
	// through to ensureEpisode would re-create it as a failed row with a Retry —
	// the exact lingering card entry the cancel exists to remove.
	if r.job.canceledEps[epKey(key)] {
		delete(r.job.canceledEps, epKey(key))
		r.job.mu.Unlock()
		r.mgr.publishNow(r.job)
		// The canceled episode may have been the engine's last runnable one, with
		// every sibling held by a per-episode pause — same check as a failure.
		r.mgr.autoPauseIfAllHeld(r.job)
		return
	}
	ev := r.job.ensureEpisode(key)
	// Don't clobber a deferred state with a generic failure; the deferred hook
	// fires separately. Mark failed only if not already parked for retry.
	if ev.State != epDeferred {
		ev.State = epFailed
	}
	ev.SpeedBps = 0
	ev.ETASeconds = 0
	if err != nil {
		ev.Error = errText(err)
	}
	r.job.mu.Unlock()
	r.mgr.publishNow(r.job)
	// A failure ends this episode too: if only paused ones are left, the run has
	// nothing to do and must not sit there holding a download slot.
	r.mgr.autoPauseIfAllHeld(r.job)
}

// EpisodeDeferred is the optional hook the engine calls when an episode is
// parked for a later retry after a transient failure.
func (r *eventReporter) EpisodeDeferred(key domain.EpisodeKey, err error, attempts int) {
	r.job.mu.Lock()
	ev := r.job.ensureEpisode(key)
	ev.State = epDeferred
	ev.Attempts = attempts
	ev.SpeedBps = 0
	ev.ETASeconds = 0
	if err != nil {
		ev.Error = errText(err)
	}
	r.job.mu.Unlock()
	r.mgr.publishNow(r.job)
}

func (r *eventReporter) Stop() {}

// ByteProgress reports bytes downloaded out of total for progressive downloads.
func (r *eventReporter) ByteProgress(key domain.EpisodeKey, downloaded, total int64) {
	r.job.mu.Lock()
	ev := r.job.ensureEpisode(key)
	ev.Bytes = downloaded
	ev.Total = total
	ev.TotalApprox = false // progressive download reports the real Content-Length
	if total > 0 {
		ev.Percent = clampPct(int(downloaded * 100 / total))
	}
	r.updateSpeed(ev, downloaded, total)
	r.job.mu.Unlock()
	r.mgr.publish(r.job)
}

// SegmentProgress reports HLS segment-level progress with an approximate total.
func (r *eventReporter) SegmentProgress(key domain.EpisodeKey, doneSegments, totalSegments int, downloadedBytes, approxTotalBytes int64) {
	r.job.mu.Lock()
	ev := r.job.ensureEpisode(key)
	ev.SegDone = doneSegments
	ev.SegTotal = totalSegments
	ev.Bytes = downloadedBytes
	ev.Total = approxTotalBytes
	ev.TotalApprox = true // HLS total is estimated from average segment size
	if totalSegments > 0 {
		ev.Percent = clampPct(doneSegments * 100 / totalSegments)
	}
	r.updateSpeed(ev, downloadedBytes, approxTotalBytes)
	r.job.mu.Unlock()
	r.mgr.publish(r.job)
}

// HLSProgress reports the full per-track breakdown for nested progress bars.
func (r *eventReporter) HLSProgress(key domain.EpisodeKey, tracks []domain.TrackProgressInfo) {
	r.job.mu.Lock()
	ev := r.job.ensureEpisode(key)
	views := make([]TrackView, 0, len(tracks))
	for _, t := range tracks {
		pct := 0
		if t.TotalSegments > 0 {
			pct = clampPct(t.DoneSegments * 100 / t.TotalSegments)
		}
		views = append(views, TrackView{
			Label:       t.Label,
			Percent:     pct,
			Done:        t.DoneSegments,
			Total:       t.TotalSegments,
			Bytes:       t.DownloadedBytes,
			ApproxTotal: t.ApproxTotalBytes,
		})
	}
	ev.Tracks = views
	r.job.mu.Unlock()
	r.mgr.publish(r.job)
}

// EpisodeStage records what the engine is doing to this episode right now.
func (r *eventReporter) EpisodeStage(key domain.EpisodeKey, stage domain.EpisodeStage) {
	r.job.mu.Lock()
	ev := r.job.ensureEpisode(key)
	if ev.Stage != stage.Phase {
		// Новая стадия — новая точка отсчёта, иначе остаток считался бы по
		// скорости предыдущей.
		ev.stageStart = time.Now()
		ev.stageFirst = stage.Done
		ev.StagePercent, ev.StageETASeconds = 0, 0
		// Скорость и остаток скачивания относятся только к скачиванию.
		if stage.Phase != "download" {
			ev.SpeedBps, ev.ETASeconds = 0, 0
		}
	}
	ev.Stage = stage.Phase
	ev.StageFormat = stage.Format
	ev.StageEncoder = stage.Encoder
	ev.StageThreads = stage.Threads
	if stage.Total > 0 && stage.Done >= 0 {
		ev.StagePercent = clampPct(int(stage.Done * 100 / stage.Total))
		// Остаток по фактической скорости ЭТОЙ стадии, от момента её начала.
		if elapsed := time.Since(ev.stageStart).Seconds(); elapsed > 2 {
			if progressed := stage.Done - ev.stageFirst; progressed > 0 {
				rate := float64(progressed) / elapsed
				if left := float64(stage.Total - stage.Done); left > 0 && rate > 0 {
					ev.StageETASeconds = int(left / rate)
				} else {
					ev.StageETASeconds = 0
				}
			}
		}
	}
	r.job.mu.Unlock()
	r.mgr.publish(r.job)
}

// updateSpeed maintains a smoothed download speed and ETA. Caller holds j.mu.
func (r *eventReporter) updateSpeed(ev *EpisodeView, downloaded, total int64) {
	now := time.Now()
	if !ev.lastTime.IsZero() {
		dt := now.Sub(ev.lastTime).Seconds()
		if dt >= 0.25 {
			inst := float64(downloaded-ev.lastBytes) / dt
			if inst < 0 {
				inst = 0
			}
			if ev.SpeedBps == 0 {
				ev.SpeedBps = inst
			} else {
				// Exponential moving average to smooth jitter.
				ev.SpeedBps = ev.SpeedBps*0.7 + inst*0.3
			}
			if ev.SpeedBps > 0 && total > downloaded {
				ev.ETASeconds = int(float64(total-downloaded) / ev.SpeedBps)
			}
			ev.lastBytes = downloaded
			ev.lastTime = now
		}
	} else {
		ev.lastBytes = downloaded
		ev.lastTime = now
	}
}

func clampPct(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}
