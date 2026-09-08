package gui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
	"github.com/ZioSHik/kinopub-gui/internal/lib/fsutil"
)

// Скачанное записывается ДВАЖДЫ: рядом с файлом (файл состояния, который видит
// библиотека и другая машина) и здесь, в собственной папке приложения.
//
// Первая запись живёт вместе с файлами: сетевой диск отвалился — и приложение
// разом забывает про всё, что на нём лежит. Именно это и произошло: пропала
// шара, скан вернул пусто, и движок пошёл качать сериал заново. Локальный
// список — память приложения о собственной работе, и её не должно уносить
// вместе с диском.
//
// Он не подменяет файл состояния: тот остаётся источником правды о файлах,
// потому что переживает переустановку и переезд на другую машину. Локальный
// список отвечает на другой вопрос — «я это скачивал?» — и позволяет отличить
// «файла нет» от «папки сейчас нет».

// DownloadRec is one finished download as the app remembers it.
type DownloadRec struct {
	SeriesID    string    `json:"seriesId"`
	SeriesTitle string    `json:"seriesTitle,omitempty"`
	Key         string    `json:"key"` // "S1E2"
	Season      int       `json:"season"`
	Episode     int       `json:"episode"`
	Title       string    `json:"title,omitempty"`
	Path        string    `json:"path"`
	Bytes       int64     `json:"bytes"`
	CompletedAt time.Time `json:"completedAt"`
}

// DiskState is what a check of one record found.
const (
	diskOK      = "ok"      // файл на месте
	diskOffline = "offline" // папка недоступна — диск отключён, сеть пропала
	diskMissing = "missing" // папка доступна, а файла в ней нет
)

type downloadIndex struct {
	mu   sync.Mutex
	path string
	recs map[string]DownloadRec // ключ: seriesID + "/" + Key
}

func newDownloadIndex() *downloadIndex {
	ix := &downloadIndex{recs: map[string]DownloadRec{}}
	if dir, err := configDir(); err == nil {
		ix.path = filepath.Join(dir, "downloads.json")
		ix.load()
	}
	return ix
}

func (ix *downloadIndex) load() {
	data, err := os.ReadFile(ix.path)
	if err != nil {
		return
	}
	var list []DownloadRec
	if json.Unmarshal(data, &list) != nil {
		return
	}
	for _, r := range list {
		ix.recs[r.SeriesID+"/"+r.Key] = r
	}
}

// saveLocked writes the whole list. Записей тут тысячи в худшем случае — это
// десятки килобайт, и переписывать их целиком дешевле, чем поддерживать
// дозапись, у которой свои способы порваться на середине.
func (ix *downloadIndex) saveLocked() {
	if ix.path == "" {
		return
	}
	list := make([]DownloadRec, 0, len(ix.recs))
	for _, r := range ix.recs {
		list = append(list, r)
	}
	sort.Slice(list, func(a, b int) bool {
		if list[a].SeriesID != list[b].SeriesID {
			return list[a].SeriesID < list[b].SeriesID
		}
		if list[a].Season != list[b].Season {
			return list[a].Season < list[b].Season
		}
		return list[a].Episode < list[b].Episode
	})
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(ix.path), 0o755)
	_ = fsutil.AtomicWrite(ix.path, data, 0o600)
}

// remember records a finished download.
func (ix *downloadIndex) remember(seriesID, seriesTitle string, info domain.CompletedInfo) {
	if seriesID == "" || info.Path == "" {
		return
	}
	key := epKey(info.Key)
	ix.mu.Lock()
	ix.recs[seriesID+"/"+key] = DownloadRec{
		SeriesID:    seriesID,
		SeriesTitle: seriesTitle,
		Key:         key,
		Season:      info.Key.Season,
		Episode:     info.Key.Episode,
		Title:       info.Title,
		Path:        info.Path,
		Bytes:       info.Bytes,
		CompletedAt: time.Now(),
	}
	ix.saveLocked()
	ix.mu.Unlock()
}

// forSeries returns what the app downloaded for one series id.
func (ix *downloadIndex) forSeries(seriesID string) map[string]DownloadRec {
	out := map[string]DownloadRec{}
	if seriesID == "" {
		return out
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	for _, r := range ix.recs {
		if r.SeriesID == seriesID {
			out[r.Key] = r
		}
	}
	return out
}

func (ix *downloadIndex) all() []DownloadRec {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	out := make([]DownloadRec, 0, len(ix.recs))
	for _, r := range ix.recs {
		out = append(out, r)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Path < out[b].Path })
	return out
}

// checkDisk says what became of a remembered file.
//
// Различать «папки нет» и «файла нет» важнее, чем кажется: первое — временно и
// лечится включением диска, второе — окончательно и означает перекачивание.
// Раньше оба случая выглядели одинаково: «не скачано».
func checkDisk(path string) string {
	if path == "" {
		return diskMissing
	}
	if _, err := os.Stat(path); err == nil {
		return diskOK
	}
	// Файла нет. Вопрос в том, есть ли папка: недоступная шара отвечает
	// ошибкой на всё подряд, и принимать это за пропавший файл нельзя.
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); err != nil {
		return diskOffline
	}
	return diskMissing
}

// indexingStateStore records every completion in the app's own list on the way
// through. Обёртка, а не новая реализация: файл состояния рядом с фильмом
// остаётся источником правды, локальный список — памятью приложения о своей
// работе, и портить первое ради второго незачем.
type indexingStateStore struct {
	domain.StateStore
	ix          *downloadIndex
	seriesID    string
	seriesTitle string
}

func (s indexingStateStore) MarkCompleted(ctx context.Context, info domain.CompletedInfo) error {
	// Записываем в свой список независимо от того, легла ли запись рядом с
	// фильмом: файл состояния живёт на сетевой папке, и падает эта запись как
	// раз тогда, когда папка отвалилась — то есть ровно в тот момент, ради
	// которого локальный список и заводился.
	err := s.StateStore.MarkCompleted(ctx, info)
	s.ix.remember(s.seriesID, s.seriesTitle, info)
	return err
}

// SetSeriesDir forwards the optional probe the engine makes on the state store.
//
// Обёртка встраивает интерфейс domain.StateStore, в котором этого метода нет,
// поэтому наружу он не пробрасывался, и проверка типа в движке молча не
// срабатывала. Файл состояния читался из корня выходной папки вместо папки
// сериала — там пусто, и полностью скачанный сериал планировался заново.
func (s indexingStateStore) SetSeriesDir(dir string) {
	if ss, ok := s.StateStore.(interface{ SetSeriesDir(string) }); ok {
		ss.SetSeriesDir(dir)
	}
}

// Load merges the app's own list into the state read from disk.
//
// Движок решает, что качать, по файлу состояния рядом с фильмами. Файл лежит
// на сетевой папке и исчезает вместе с ней — и сериал, целиком скачанный на
// выключенный NAS, планируется заново. Список приложения переживает это и
// подсказывает: вот эти серии скачаны.
//
// Подсказка принимается ТОЛЬКО если файл проверен на месте. Пропавший файл
// (папка доступна, файла нет) в список готовых не попадает — его действительно
// надо качать. Недоступная папка тоже: пока её нет, класть туда нечего, и
// запуск всё равно упрётся в неё раньше.
func (s indexingStateStore) Load(ctx context.Context, series domain.SeriesID) (domain.DownloadState, error) {
	st, err := s.StateStore.Load(ctx, series)
	if err != nil {
		return st, err
	}
	if s.seriesID == "" {
		return st, nil
	}
	if st.Completed == nil {
		st.Completed = map[string]domain.CompletedRec{}
	}
	for key, rec := range s.ix.forSeries(s.seriesID) {
		if _, known := st.Completed[key]; known {
			continue
		}
		if checkDisk(rec.Path) != diskOK {
			continue
		}
		st.Completed[key] = domain.CompletedRec{
			Season:      rec.Season,
			Episode:     rec.Episode,
			Path:        rec.Path,
			Bytes:       rec.Bytes,
			CompletedAt: rec.CompletedAt,
			Title:       rec.Title,
		}
	}
	return st, nil
}
