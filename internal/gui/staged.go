package gui

import (
	"context"
	"os"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/app/kinopub"
	"github.com/ZioSHik/kinopub-gui/internal/domain"
	"github.com/ZioSHik/kinopub-gui/internal/lib/fsutil"
	"github.com/ZioSHik/kinopub-gui/internal/lib/logx"
	"github.com/ZioSHik/kinopub-gui/internal/services/kinopubapi"
)

// stagedSweepEvery — как часто проверять, не вернулась ли папка загрузки.
//
// Приложение живёт неделями без перезапуска, а сетевой диск пропадает и
// возвращается посреди дня: включился корпоративный VPN, уснул NAS. Привязывать
// перенос к запуску закачки мало — качать может быть уже нечего, и готовый файл
// останется лежать в рабочей папке навсегда.
const stagedSweepEvery = 10 * time.Minute

// stagedShowEvery — как часто обновлять в строках серий «ждёт переноса» и
// процент копирования. Без этого перенос 10-гигабайтной серии на NAS виден
// только в журнале, а строка всё это время уверяет, что файл уже в папке.
const stagedShowEvery = 3 * time.Second

// startStagedSweeper watches for the download folder coming back and moves the
// episodes that waited in the work folder.
//
// Порядок важен: сначала ДЕШЁВАЯ проверка доступности папки загрузки, и только
// если она ответила — обход рабочей папки. Пока диска нет, тик стоит один
// os.Stat и не трогает диск вообще.
func (s *Server) startStagedSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(stagedSweepEvery)
		defer t.Stop()
		show := time.NewTicker(stagedShowEvery)
		defer show.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.sweepStaged(ctx)
			case <-show.C:
				cfg := s.settings.get()
				s.mgr.markStaged(domain.RunConfig{OutputPath: cfg.OutputPath, WorkPath: cfg.WorkPath})
			}
		}
	}()
}

// sweepStaged is one pass: проверить папку загрузки и перенести всё, что её
// дождалось.
func (s *Server) sweepStaged(ctx context.Context) int {
	cfg := s.settings.get()
	if cfg.OutputPath == "" || cfg.WorkPath == "" || cfg.OutputPath == cfg.WorkPath {
		return 0
	}
	// Папка загрузки недоступна — значит переносить некуда, ждём следующего
	// тика. Именно этот случай и есть повод для всей затеи.
	if _, err := os.Stat(cfg.OutputPath); err != nil {
		return 0
	}
	run := domain.RunConfig{OutputPath: cfg.OutputPath, WorkPath: cfg.WorkPath}
	n := kinopub.FlushStaged(ctx, run, fsutil.Move, journalLogger("staged"))
	if n > 0 {
		// Интерфейс должен увидеть переехавшие файлы сразу, а не по обновлению
		// страницы: человек как раз в этот момент и смотрит, доехало ли.
		s.hub.broadcast(Event{Type: "snapshot", Data: s.snapshot()})
	}
	return n
}

// journalLogger пишет только в журнал событий: у сборщика нет карточки задачи,
// а без записи не понять, почему файлы из рабочей папки не доехали.
func journalLogger(job string) domain.Logger {
	return logx.New([]logx.Handler{journalHandler(job)})
}

type journalHandler string

func (h journalHandler) Handle(rec logx.Record) {
	var fields map[string]any
	if len(rec.Fields) > 0 {
		fields = make(map[string]any, len(rec.Fields))
		for _, f := range rec.Fields {
			fields[f.Key] = f.Value
		}
	}
	events.Load().add(string(h), "", LogEntry{
		Time:      rec.Time,
		Level:     logx.LevelString(rec.Level),
		Component: rec.Component,
		Message:   rec.Message,
		Fields:    fields,
	})
}

type stagedState struct {
	disk    string
	percent int
}

// stagedStates says, for every file waiting in the work folder, whether it is
// just waiting or being copied right now — keyed by its place in the download
// folder, как его и помнит список скачанного.
//
// Идущее копирование видно по соседу цели: fsutil.Move пишет в target+".moving".
// Старый .moving от оборванного копирования не должен вечно изображать перенос,
// поэтому считается только свежий.
func stagedStates(run domain.RunConfig) map[string]stagedState {
	out := map[string]stagedState{}
	for target, staged := range kinopub.StagedFiles(run) {
		st := stagedState{disk: diskStaged}
		src, err := os.Stat(staged)
		mv, mvErr := os.Stat(target + ".moving")
		if err == nil && mvErr == nil && time.Since(mv.ModTime()) < 2*time.Minute {
			st.disk = diskMoving
			if src.Size() > 0 {
				st.percent = int(min(mv.Size()*100/src.Size(), 99))
			}
		}
		out[target] = st
	}
	return out
}

// markStaged puts the transfer state on finished episode rows.
func (m *JobManager) markStaged(run domain.RunConfig) {
	staged := stagedStates(run)
	m.mu.RLock()
	jobs := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	m.mu.RUnlock()
	for _, j := range jobs {
		j.mu.Lock()
		id := kinopubapi.ItemIDFromURL(j.url)
		j.mu.Unlock()
		if id == "" {
			continue
		}
		recs := m.index.forSeries(id)
		changed := false
		j.mu.Lock()
		for key, rec := range recs {
			ev, ok := j.episodes[key]
			if !ok || ev.State != epCompleted {
				continue
			}
			st, waiting := staged[rec.Path]
			if !waiting {
				// Доехал: был «переносится» — теперь просто на месте.
				if ev.Disk != diskStaged && ev.Disk != diskMoving {
					continue
				}
				st = stagedState{disk: diskOK}
			}
			if ev.Disk != st.disk || ev.StagePercent != st.percent {
				ev.Disk, ev.StagePercent = st.disk, st.percent
				changed = true
			}
		}
		j.mu.Unlock()
		if changed {
			m.publishNow(j)
		}
	}
}
