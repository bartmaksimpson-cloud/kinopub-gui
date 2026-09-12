package gui

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Журнал событий на диске.
//
// Лог задачи живёт в памяти, обрезан до последних maxJobLogs строк и пропадает
// при перезапуске. Разбирать по нему, что случилось вчера, невозможно: ошибка
// склейки «Боя товара» исчезла раньше, чем до неё дошли руки. Поэтому каждая
// строка лога каждой задачи дописывается ещё и сюда.
//
// ponytail: JSON Lines + один архивный файл, а не SQLite — хватает для «найти
// ошибки задачи за неделю» полным чтением файла. Индексы понадобятся, если
// журнал вырастет за сотни мегабайт или захочется сложных выборок.

const (
	eventsFileName  = "events.jsonl"
	eventsMaxBytes  = 64 << 20 // потом файл уезжает в events.1.jsonl, старый архив стирается
	eventsQueueSize = 8192
)

// EventRec is one line of the on-disk journal.
type EventRec struct {
	LogEntry
	Job   string `json:"job"`
	Title string `json:"title,omitempty"`
}

type eventLog struct {
	path string
	ch   chan EventRec
}

// events is the process-wide journal; nil (no config dir) disables it. Атомарный
// указатель, потому что серверов в тестах много и они поднимаются параллельно
// с уже идущими задачами.
var events atomic.Pointer[eventLog]

// initEventLog opens the journal once per process.
func initEventLog() {
	if events.Load() == nil {
		events.CompareAndSwap(nil, openEventLog())
	}
}

func openEventLog() *eventLog {
	dir, err := configDir()
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	l := &eventLog{path: filepath.Join(dir, eventsFileName), ch: make(chan EventRec, eventsQueueSize)}
	go l.writeLoop()
	return l
}

// add queues a record. Никогда не ждёт: вызывается под замком карточки, и
// медленный диск не должен подвешивать интерфейс. Переполнение очереди — это
// тысячи строк в секунду, и потерять часть из них лучше, чем встать.
func (l *eventLog) add(job, title string, e LogEntry) {
	if l == nil {
		return
	}
	select {
	case l.ch <- EventRec{LogEntry: e, Job: job, Title: title}:
	default:
	}
}

func (l *eventLog) writeLoop() {
	for rec := range l.ch {
		f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			continue
		}
		w := bufio.NewWriter(f)
		enc := json.NewEncoder(w)
		_ = enc.Encode(rec)
		// Забираем всё, что успело накопиться, одним открытием файла.
		for more := true; more; {
			select {
			case r, ok := <-l.ch:
				if !ok {
					more = false
					break
				}
				_ = enc.Encode(r)
			default:
				more = false
			}
		}
		_ = w.Flush()
		info, statErr := f.Stat()
		_ = f.Close()
		if statErr == nil && info.Size() > eventsMaxBytes {
			_ = os.Rename(l.path, archivePath(l.path))
		}
	}
}

func archivePath(path string) string {
	return strings.TrimSuffix(path, ".jsonl") + ".1.jsonl"
}

// query reads the journal (архив, затем текущий файл) and returns the newest
// `limit` records that match, oldest first.
func (l *eventLog) query(job, level, text string, since time.Time, limit int) []EventRec {
	var out []EventRec
	if l == nil {
		return out
	}
	levels := map[string]bool{}
	for _, lv := range strings.Split(strings.ToUpper(level), ",") {
		if lv != "" {
			levels[lv] = true
		}
	}
	text = strings.ToLower(text)
	for _, p := range []string{archivePath(l.path), l.path} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
		for sc.Scan() {
			line := sc.Bytes()
			if text != "" && !strings.Contains(strings.ToLower(string(line)), text) {
				continue
			}
			var r EventRec
			if json.Unmarshal(line, &r) != nil {
				continue
			}
			if job != "" && r.Job != job {
				continue
			}
			if len(levels) > 0 && !levels[r.Level] {
				continue
			}
			if !since.IsZero() && r.Time.Before(since) {
				continue
			}
			out = append(out, r)
			if len(out) > 2*limit {
				out = append(out[:0], out[len(out)-limit:]...)
			}
		}
		_ = f.Close()
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// handleEventJournal: GET /api/journal?job=job-14&level=ERROR,WARN&q=склейка&since=2026-09-12T00:00:00Z&limit=500
func (s *Server) handleEventJournal(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 20000 {
		limit = 1000
	}
	var since time.Time
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "since: нужен RFC3339, например 2026-09-12T00:00:00Z")
			return
		}
		since = t
	}
	writeJSON(w, http.StatusOK, events.Load().query(q.Get("job"), q.Get("level"), q.Get("q"), since, limit))
}

// jobOutcomeEntry is the line that closes a run. Причина провала задачи живёт в
// errMsg карточки и в лог не попадала — в журнале без неё не понять, чем
// кончилось.
func jobOutcomeEntry(at time.Time, status, errMsg string) LogEntry {
	e := LogEntry{Time: at, Level: "INFO", Component: "job", Message: "задача: " + status}
	if status == statusFailed {
		e.Level = "ERROR"
	}
	if errMsg != "" {
		e.Fields = map[string]any{"error": errMsg}
	}
	return e
}
