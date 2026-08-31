package statestore

import (
	"sync"
	"testing"
)

// The shared lock still has to be a lock: overlapping holders for one path must
// be serialized, or two jobs' read-modify-write cycles drop each other's
// completions — the whole reason the table exists.
func TestLockPathSerializesSamePath(t *testing.T) {
	const goroutines = 50

	var (
		wg      sync.WaitGroup
		counter int  // guarded solely by the path lock; -race catches any overlap
		nested  bool // set if a second holder is ever inside the critical section
		inside  bool
	)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := lockPath("/series/state.json")
			defer unlock()
			if inside {
				nested = true
			}
			inside = true
			counter++
			inside = false
		}()
	}
	wg.Wait()

	if nested {
		t.Error("two goroutines held the same path lock at once")
	}
	if counter != goroutines {
		t.Errorf("counter = %d, want %d", counter, goroutines)
	}
}

// Different paths must not block each other: a lock held on one series' state
// file cannot stall a write to another's.
func TestLockPathIndependentPaths(t *testing.T) {
	unlockA := lockPath("/a/state.json")
	defer unlockA()

	done := make(chan struct{})
	go func() {
		unlockB := lockPath("/b/state.json")
		unlockB()
		close(done)
	}()
	<-done // hangs (and fails by test timeout) if the paths shared a lock
}

// The table must shrink back to empty. It used to keep an entry per state-file
// path for the life of the process, so every series ever downloaded left one
// behind.
func TestLockPathReleasesEntries(t *testing.T) {
	pathLocks.mu.Lock()
	before := len(pathLocks.m)
	pathLocks.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// A mix of contended and unique paths, as concurrent jobs produce.
			unlock := lockPath(string(rune('a'+n%7)) + "/state.json")
			unlock()
		}(i)
	}
	wg.Wait()

	pathLocks.mu.Lock()
	after := len(pathLocks.m)
	pathLocks.mu.Unlock()
	if after != before {
		t.Errorf("lock table grew from %d to %d entries; released locks are not dropped", before, after)
	}
}
