package downloader

import (
	"context"
	"testing"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// Вторая склейка ждёт первую, а не идёт рядом с ней.
func TestMuxGateSerializes(t *testing.T) {
	first, err := acquireMuxGate(context.Background(), nil, domain.EpisodeKey{Season: 1, Episode: 1})
	if err != nil {
		t.Fatalf("первый захват не должен ждать: %v", err)
	}

	got := make(chan struct{})
	go func() {
		release, err := acquireMuxGate(context.Background(), nil, domain.EpisodeKey{Season: 1, Episode: 1})
		if err != nil {
			t.Errorf("второй захват: %v", err)
			return
		}
		release()
		close(got)
	}()

	select {
	case <-got:
		t.Fatal("второй захват прошёл, пока первый держит очередь")
	case <-time.After(50 * time.Millisecond):
	}

	first()

	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("второй захват не прошёл после освобождения")
	}
}

// Отмена эпизода не должна оставлять ждущего в очереди навсегда.
func TestMuxGateHonoursCancel(t *testing.T) {
	release, err := acquireMuxGate(context.Background(), nil, domain.EpisodeKey{Season: 1, Episode: 1})
	if err != nil {
		t.Fatalf("первый захват: %v", err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := acquireMuxGate(ctx, nil, domain.EpisodeKey{Season: 1, Episode: 1}); err == nil {
		t.Fatal("ожидание с отменённым контекстом должно вернуть ошибку")
	}
}
