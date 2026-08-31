package gui

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
	"github.com/ZioSHik/kinopub-gui/internal/lib/logx"
	"github.com/ZioSHik/kinopub-gui/internal/services/statestore"
)

// persistLibraryMetadata runs on a background backfill goroutine while the
// engine may be recording completions into the same state file. Both writers
// must share the statestore path lock — an unlocked read-modify-write in the
// backfill lands last with a stale Completed map and silently erases the
// record the engine just wrote.
func TestPersistLibraryMetadata_DoesNotLoseConcurrentCompletions(t *testing.T) {
	dir := t.TempDir()
	seriesDir := filepath.Join(dir, "Show")
	store := statestore.New(dir, logx.New(nil))
	store.SetSeriesDir(seriesDir)
	ctx := context.Background()
	const series = domain.SeriesID("409")

	// Seed the state file with a metadata block missing genres/type — the shape
	// the backfill exists to repair.
	if err := store.SetMetadata(ctx, series, domain.SeriesMetadata{Title: "Show", InputURL: "https://kino.watch/item/view/409"}); err != nil {
		t.Fatal(err)
	}

	stateFile := filepath.Join(seriesDir, stateFileName)
	const episodes = 40
	var wg sync.WaitGroup
	for i := 1; i <= episodes; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_ = store.MarkCompleted(ctx, domain.CompletedInfo{
				Key: domain.EpisodeKey{Series: series, Season: 1, Episode: i},
			})
		}(i)
		go func() {
			defer wg.Done()
			_ = persistLibraryMetadata(stateFile, []string{"Drama"}, "serial")
		}()
	}
	wg.Wait()

	state, err := store.Load(ctx, series)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Completed) != episodes {
		t.Errorf("completed records = %d, want %d — the metadata backfill erased concurrent completions",
			len(state.Completed), episodes)
	}
	if state.Metadata == nil || len(state.Metadata.Genres) == 0 || state.Metadata.Type == "" {
		t.Errorf("metadata = %+v, want genres and type filled in", state.Metadata)
	}
}
