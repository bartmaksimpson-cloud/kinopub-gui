package gui

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/app/kinopub"
	"github.com/ZioSHik/kinopub-gui/internal/domain"
	"github.com/ZioSHik/kinopub-gui/internal/lib/fsutil"
	"github.com/ZioSHik/kinopub-gui/internal/lib/logx"
)

// stagedSweepEvery — как часто проверять, не вернулась ли папка загрузки.
//
// Приложение живёт неделями без перезапуска, а сетевой диск пропадает и
// возвращается посреди дня: включился корпоративный VPN, уснул NAS. Привязывать
// перенос к запуску закачки мало — качать может быть уже нечего, и готовый файл
// останется лежать в рабочей папке навсегда.
const stagedSweepEvery = 10 * time.Minute

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
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.sweepStaged(ctx)
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
	n := kinopub.FlushStaged(ctx, run, fsutil.Move, quietLogger())
	if n > 0 {
		// Интерфейс должен увидеть переехавшие файлы сразу, а не по обновлению
		// страницы: человек как раз в этот момент и смотрит, доехало ли.
		s.hub.broadcast(Event{Type: "snapshot", Data: s.snapshot()})
	}
	return n
}

// quietLogger пишет в никуда: у сборщика нет карточки задачи, в которую можно
// было бы показать строку лога, а копить их в памяти у процесса, живущего
// неделями, — верный способ съесть её незаметно.
func quietLogger() domain.Logger {
	return logx.New([]logx.Handler{logx.NewFileHandler(io.Discard, logx.NewCoordinator(io.Discard))})
}
