package gui

import (
	"path/filepath"
	"testing"
	"time"
)

// Журнал переживает перезапуск и отбирает строки по задаче, уровню и тексту.
func TestEventJournalQuery(t *testing.T) {
	l := &eventLog{path: filepath.Join(t.TempDir(), eventsFileName), ch: make(chan EventRec, 16)}
	t0 := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	l.add("job-14", "Во все тяжкие", LogEntry{Time: t0, Level: "INFO", Message: "muxing"})
	l.add("job-14", "Во все тяжкие", LogEntry{Time: t0.Add(time.Minute), Level: "ERROR", Message: "склейка упала"})
	l.add("job-17", "Мир Дикого Запада", LogEntry{Time: t0.Add(2 * time.Minute), Level: "ERROR", Message: "no such host"})
	close(l.ch)
	l.writeLoop()

	if got := l.query("job-14", "error,warn", "", time.Time{}, 100); len(got) != 1 || got[0].Message != "склейка упала" {
		t.Fatalf("по задаче и уровню: %+v", got)
	}
	if got := l.query("", "", "HOST", time.Time{}, 100); len(got) != 1 || got[0].Job != "job-17" {
		t.Fatalf("по тексту: %+v", got)
	}
	if got := l.query("", "", "", t0.Add(30*time.Second), 1); len(got) != 1 || got[0].Job != "job-17" {
		t.Fatalf("since+limit должны отдать самую свежую: %+v", got)
	}
}
