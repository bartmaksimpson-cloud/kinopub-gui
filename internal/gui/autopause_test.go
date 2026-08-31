package gui

import (
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// runningJobWithEpisodes builds a live job whose episodes start in the given
// states, keyed S1E1, S1E2, …
func runningJobWithEpisodes(id string, states ...string) *Job {
	j := newJob(id, "u", domain.RunConfig{})
	j.status = statusRunning
	for i, st := range states {
		key := epKey(domain.EpisodeKey{Season: 1, Episode: i + 1})
		j.episodes[key] = &EpisodeView{Key: key, Season: 1, Episode: i + 1, State: st}
	}
	return j
}

func TestNothingLeftButPaused(t *testing.T) {
	cases := []struct {
		name   string
		states []string
		want   bool
	}{
		{"one paused, one still queued", []string{epPaused, epPending}, false},
		{"one paused, one downloading", []string{epPaused, epRunning}, false},
		{"one paused, one parked for retry", []string{epPaused, epDeferred}, false},
		{"paused alongside a finished one", []string{epPaused, epCompleted}, true},
		{"paused alongside a failed one", []string{epPaused, epFailed}, true},
		{"every episode paused", []string{epPaused, epPaused}, true},
		// Nothing is held, so there is nothing waiting on the user — the run is
		// simply over, and that is not a pause.
		{"all finished", []string{epCompleted, epCompleted}, false},
		{"no episodes at all", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := runningJobWithEpisodes("j", tc.states...)
			if got := nothingLeftButPausedLocked(j); got != tc.want {
				t.Errorf("nothingLeftButPausedLocked = %v, want %v", got, tc.want)
			}
		})
	}
}

// Pausing episodes one by one must pause the JOB as soon as the engine has
// nothing left — otherwise the card reads "downloading" while nothing moves and
// the run keeps one of the active-download slots to itself.
func TestPauseEpisode_LastOneHeldPausesTheJob(t *testing.T) {
	m := newJobManager(newHub())
	j := runningJobWithEpisodes("j", epRunning, epPending)
	stopped := false
	j.cancel = func() { stopped = true }
	m.add(j)

	if !m.pauseEpisode("j", domain.EpisodeKey{Season: 1, Episode: 1}) {
		t.Fatal("pausing a downloading episode should be accepted")
	}
	if j.paused.Load() || stopped {
		t.Fatal("one episode still has work to do — the job must keep running")
	}

	if !m.pauseEpisode("j", domain.EpisodeKey{Season: 1, Episode: 2}) {
		t.Fatal("pausing a queued episode should be accepted")
	}
	if !j.paused.Load() {
		t.Error("with every episode held the job itself should be paused")
	}
	if !stopped {
		t.Error("the run should be stopped, not left polling for work")
	}
}

// The other way the engine runs dry: the last unpaused episode finishes.
func TestEpisodeCompleted_PausesJobWhenOnlyHeldEpisodesRemain(t *testing.T) {
	m := newJobManager(newHub())
	j := runningJobWithEpisodes("j", epPaused, epRunning)
	stopped := false
	j.cancel = func() { stopped = true }
	m.add(j)

	newEventReporter(m, j).EpisodeCompleted(domain.EpisodeKey{Season: 1, Episode: 2})

	if !j.paused.Load() || !stopped {
		t.Error("the last active episode finished with a held sibling — the job should pause")
	}
}

func TestEpisodeCompleted_KeepsRunningWhileWorkRemains(t *testing.T) {
	m := newJobManager(newHub())
	j := runningJobWithEpisodes("j", epPending, epRunning)
	stopped := false
	j.cancel = func() { stopped = true }
	m.add(j)

	newEventReporter(m, j).EpisodeCompleted(domain.EpisodeKey{Season: 1, Episode: 2})

	if j.paused.Load() || stopped {
		t.Error("an episode is still queued — the run must carry on")
	}
}

// Once the job is paused there is no engine to hear a per-episode resume, so a
// fresh run is started. It must cover the WHOLE job with the other episodes
// re-held: a run scoped to the one episode left the rest outside it, and their
// own Resume then reached an engine that had never heard of them — three
// episodes released, one downloading.
func TestResumeEpisode_OnPausedJobRunsWholeJobAndReHoldsSiblings(t *testing.T) {
	m := newJobManager(newHub())
	// Occupy the only slot so the rerun stays queued and never reaches the real
	// download path.
	m.maxActive = 1
	m.running = 1

	j := runningJobWithEpisodes("j", epPaused, epPaused, epPaused)
	j.status = statusPaused
	j.paused.Store(true)
	m.add(j)

	if !m.resumeEpisode("j", domain.EpisodeKey{Season: 1, Episode: 2}) {
		t.Fatal("resuming a held episode of a paused job should be accepted")
	}

	j.mu.Lock()
	status, scope := j.status, j.retryOnly
	states := map[string]string{}
	for k, ev := range j.episodes {
		states[k] = ev.State
	}
	j.mu.Unlock()

	if status != statusQueued {
		t.Errorf("status = %q, want the job queued for a fresh run", status)
	}
	// The whole job runs: scoping it would leave the held siblings out of reach.
	if len(scope) != 0 {
		t.Errorf("retryOnly = %v, want the run unscoped", scope)
	}
	if states["S1E2"] != epPending {
		t.Errorf("resumed episode = %q, want it queued again", states["S1E2"])
	}
	if states["S1E1"] != epPaused || states["S1E3"] != epPaused {
		t.Errorf("siblings = %q/%q, want both left held", states["S1E1"], states["S1E3"])
	}
	if j.paused.Load() {
		t.Error("the job is running again, not paused")
	}

	// The hold list dies with the run that owned it, so this run gets its own
	// pause requests for the siblings.
	held := map[int]bool{}
	for {
		select {
		case k := <-j.pauseEp:
			held[k.Episode] = true
			continue
		default:
		}
		break
	}
	if !held[1] || !held[3] || held[2] {
		t.Errorf("queued pauses = %v, want exactly the two held siblings", held)
	}
}

// Canceling an episode drops it from the card entirely: the user removed it from
// the run, so leaving a failed row with a Retry button is dead weight.
func TestCancelEpisode_RemovesTheRowAndShrinksThePlan(t *testing.T) {
	m := newJobManager(newHub())
	j := runningJobWithEpisodes("j", epRunning, epPending)
	j.plan = &PlanView{Total: 2, Seasons: map[int]int{1: 2}}
	m.add(j)

	if !m.cancelEpisode("j", domain.EpisodeKey{Season: 1, Episode: 2}) {
		t.Fatal("a queued episode should be cancelable")
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if _, still := j.episodes["S1E2"]; still {
		t.Error("the canceled episode should be gone from the card")
	}
	if _, ok := j.episodes["S1E1"]; !ok {
		t.Error("its sibling must be untouched")
	}
	if j.plan.Total != 1 {
		t.Errorf("plan total = %d, want 1 — the card should count what is left", j.plan.Total)
	}
	if j.plan.Seasons[1] != 1 {
		t.Errorf("season count = %d, want 1", j.plan.Seasons[1])
	}
}

func TestCancelEpisode_LeavesPlanAloneWhenAbsent(t *testing.T) {
	m := newJobManager(newHub())
	j := runningJobWithEpisodes("j", epRunning)
	m.add(j) // no plan attached yet — the run has not reported one

	if !m.cancelEpisode("j", domain.EpisodeKey{Season: 1, Episode: 1}) {
		t.Fatal("a downloading episode should be cancelable")
	}
	if len(j.episodes) != 0 {
		t.Errorf("episodes = %v, want the canceled one gone", j.episodes)
	}
}
