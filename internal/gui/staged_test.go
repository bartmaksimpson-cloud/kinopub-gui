package gui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Сборщик отложенного: пока папка загрузки не отвечает — ничего не делает;
// как только ответила — уносит туда готовые файлы из рабочей папки.
func TestSweepStaged(t *testing.T) {
	work := t.TempDir()
	outParent := t.TempDir()
	out := filepath.Join(outParent, "nas") // ещё не существует — «диск отвалился»

	srv := NewServer("test", nil)
	// Настоящий файл настроек пользователя тесты не трогают.
	srv.settings = &settingsStore{cur: Settings{OutputPath: out, WorkPath: work, Container: "mkv"}}

	staged := filepath.Join(work, "Сериал", "Season 01", "S01E01.mkv")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("кино"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Папки загрузки нет — обход рабочей папки даже не начинается.
	if n := srv.sweepStaged(context.Background()); n != 0 {
		t.Fatalf("перенесено %d при недоступной папке загрузки, ожидался ноль", n)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("файл пропал из рабочей папки: %v", err)
	}

	// Диск вернулся.
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if n := srv.sweepStaged(context.Background()); n != 1 {
		t.Fatalf("перенесено %d, ожидался один файл", n)
	}
	if _, err := os.Stat(filepath.Join(out, "Сериал", "Season 01", "S01E01.mkv")); err != nil {
		t.Errorf("файл не доехал до папки загрузки: %v", err)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Error("исходник остался в рабочей папке")
	}
}
