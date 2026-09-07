package gui

import (
	"github.com/ZioSHik/kinopub-gui/internal/domain"
	"os"
	"path/filepath"
	"testing"
)

// Три исхода проверки различаются намеренно: отключённый диск ждут, удалённый
// файл перекачивают. Раньше оба выглядели как «не скачано», и приложение молча
// начинало качать сериал, который целиком лежал на выключенном NAS.
func TestCheckDisk(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "S01E01.mkv")
	if err := os.WriteFile(present, []byte("кино"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := checkDisk(present); got != diskOK {
		t.Errorf("файл на месте: %q, ожидалось %q", got, diskOK)
	}

	// Папка есть, файла нет — его удалили.
	if got := checkDisk(filepath.Join(dir, "S01E02.mkv")); got != diskMissing {
		t.Errorf("удалённый файл: %q, ожидалось %q", got, diskMissing)
	}

	// Нет и папки — так выглядит отключённый сетевой диск.
	if got := checkDisk(filepath.Join(dir, "нет-такой-папки", "S01E03.mkv")); got != diskOffline {
		t.Errorf("недоступная папка: %q, ожидалось %q", got, diskOffline)
	}

	if got := checkDisk(""); got != diskMissing {
		t.Errorf("пустой путь: %q", got)
	}
}

// Список переживает перезапуск приложения: в этом весь смысл — файл состояния
// лежит рядом с фильмом и исчезает вместе с сетевым диском.
func TestDownloadIndexRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ix := &downloadIndex{path: filepath.Join(dir, "downloads.json"), recs: map[string]DownloadRec{}}
	ix.remember("8739", "Во все тяжкие", completedInfoFor(1, 2, "Z:\\во\\S01E02.mkv", 15<<30))

	again := &downloadIndex{path: ix.path, recs: map[string]DownloadRec{}}
	again.load()
	recs := again.forSeries("8739")
	if len(recs) != 1 {
		t.Fatalf("после перезапуска записей %d, ожидалась одна", len(recs))
	}
	rec := recs["S1E2"]
	if rec.Path == "" || rec.Bytes != 15<<30 || rec.Season != 1 || rec.Episode != 2 {
		t.Errorf("запись потеряла данные: %+v", rec)
	}
	if len(again.forSeries("другой")) != 0 {
		t.Error("чужой сериал не должен находиться")
	}
}

func completedInfoFor(season, episode int, path string, size int64) domain.CompletedInfo {
	return domain.CompletedInfo{
		Key:   domain.EpisodeKey{Season: season, Episode: episode},
		Path:  path,
		Bytes: size,
	}
}
