package gui

import (
	"context"
	"errors"
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

// Список приложения должен подсказывать движку, что серия уже скачана, — но
// только про файлы, проверенные на месте. Иначе пропавший файл никогда бы не
// перекачался, а это хуже лишней перекачки.
func TestIndexingStateStore_LoadSkipsOnlyMissing(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "S01E01.mkv")
	if err := os.WriteFile(present, []byte("кино"), 0o644); err != nil {
		t.Fatal(err)
	}

	ix := &downloadIndex{path: filepath.Join(dir, "downloads.json"), recs: map[string]DownloadRec{}}
	ix.remember("8739", "Во все тяжкие", completedInfoFor(1, 1, present, 4))
	ix.remember("8739", "Во все тяжкие", completedInfoFor(1, 2, filepath.Join(dir, "S01E02.mkv"), 4)) // файла нет
	ix.remember("8739", "Во все тяжкие", completedInfoFor(1, 3, filepath.Join(dir, "нет-папки", "S01E03.mkv"), 4))

	store := indexingStateStore{StateStore: emptyStateStore{}, ix: ix, seriesID: "8739"}
	st, err := store.Load(context.Background(), domain.SeriesID("8739"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := st.Completed["S1E1"]; !ok {
		t.Error("проверенный файл не попал в готовые — серия будет скачана заново")
	}
	if _, ok := st.Completed["S1E2"]; ok {
		t.Error("пропавший файл попал в готовые — его никогда не перекачают")
	}
	if _, ok := st.Completed["S1E3"]; !ok {
		t.Error("файл на отключённом диске не попал в готовые — его скачают заново в рабочую папку")
	}
}

// emptyStateStore — файл состояния, которого нет: ровно то, что видит движок,
// когда сетевая папка отвалилась.
type emptyStateStore struct{}

func (emptyStateStore) Load(context.Context, domain.SeriesID) (domain.DownloadState, error) {
	return domain.DownloadState{}, nil
}
func (emptyStateStore) MarkCompleted(context.Context, domain.CompletedInfo) error { return nil }
func (emptyStateStore) SetMetadata(context.Context, domain.SeriesID, domain.SeriesMetadata) error {
	return nil
}
func (emptyStateStore) IsCompleted(domain.DownloadState, domain.EpisodeKey) bool { return false }

// Обёртка не должна скрывать необязательный интерфейс состояния: движок ищет
// SetSeriesDir проверкой типа, и без проброса читал файл состояния из корня
// выходной папки, а не из папки сериала.
type seriesDirStore struct {
	emptyStateStore
	dir string
}

func (s *seriesDirStore) SetSeriesDir(dir string) { s.dir = dir }

func TestIndexingStateStore_ForwardsSetSeriesDir(t *testing.T) {
	inner := &seriesDirStore{}
	var store domain.StateStore = indexingStateStore{StateStore: inner, ix: &downloadIndex{recs: map[string]DownloadRec{}}, seriesID: "8739"}

	ss, ok := store.(interface{ SetSeriesDir(string) })
	if !ok {
		t.Fatal("indexingStateStore не отдаёт SetSeriesDir — движок не найдёт папку сериала")
	}
	ss.SetSeriesDir(`Z:\Сериал`)
	if inner.dir != `Z:\Сериал` {
		t.Fatalf("папка сериала не дошла до хранилища: %q", inner.dir)
	}
}

// Скачанное запоминается, даже если запись рядом с фильмом не удалась: сетевая
// папка отваливается именно тогда, когда локальный список и нужен.
func TestIndexingStateStore_RemembersWhenDiskWriteFails(t *testing.T) {
	ix := &downloadIndex{recs: map[string]DownloadRec{}}
	store := indexingStateStore{StateStore: failingStateStore{}, ix: ix, seriesID: "8739"}

	err := store.MarkCompleted(context.Background(), domain.CompletedInfo{
		Key:  domain.EpisodeKey{Season: 2, Episode: 2},
		Path: `Z:\Сериал\Season 02\S02E02.mkv`,
	})
	if err == nil {
		t.Fatal("ошибка записи файла состояния должна возвращаться наверх")
	}
	if _, ok := ix.forSeries("8739")["S2E2"]; !ok {
		t.Fatal("серия не попала в список приложения")
	}
}

type failingStateStore struct{ emptyStateStore }

func (failingStateStore) MarkCompleted(context.Context, domain.CompletedInfo) error {
	return errors.New("the network path was not found")
}

// Серия, собранная в рабочей папке на время отключения диска, запоминается по
// месту в папке загрузки: сборщик перенесёт её туда, и запись на рабочую папку
// указывала бы на пустоту — серию скачали бы в третий раз.
func TestIndexingStateStore_RemembersStagedByOutputPath(t *testing.T) {
	ix := &downloadIndex{recs: map[string]DownloadRec{}}
	work, out := filepath.FromSlash("/work"), filepath.FromSlash("/nas")
	store := indexingStateStore{StateStore: emptyStateStore{}, ix: ix, seriesID: "8739",
		run: domain.RunConfig{WorkPath: work, OutputPath: out}}

	_ = store.MarkCompleted(context.Background(), domain.CompletedInfo{
		Key:  domain.EpisodeKey{Season: 1, Episode: 1},
		Path: filepath.Join(work, "Сериал", "Season 01", "S01E01.mkv"),
	})
	want := filepath.Join(out, "Сериал", "Season 01", "S01E01.mkv")
	if got := ix.forSeries("8739")["S1E1"].Path; got != want {
		t.Fatalf("запомнен путь %q, ожидался %q", got, want)
	}
}
