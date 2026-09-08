package downloader

import (
	"context"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// Склейка идёт по одной за раз на всё приложение.
//
// На этой стадии ffmpeg читает десятки гигабайт собранных сегментов с рабочего
// диска и пишет столько же в выходную папку. Две склейки одновременно ничего не
// ускоряют: упирается всё в диск, а на сетевой шаре головка вдобавок мечется
// между двумя файлами, и обе идут медленнее любой одиночной. Скачивание при
// этом не останавливается — ждёт только вторая склейка.
//
// ponytail: семафор на пакет, а не поле Downloader — у каждой задачи свой
// Downloader (см. gui.buildEngineDeps), а ограничен тут один общий диск. Если
// когда-нибудь выходных дисков станет несколько, ключевать по выходной папке.
var muxGate = make(chan struct{}, 1)

// acquireMuxGate waits for the single assembly slot and returns the release.
//
// Ожидание объявляется в карточке: без этого серия висела бы на «скачивание,
// 100%» и выглядела зависшей — ровно та беда, ради которой стадии и заведены.
func acquireMuxGate(ctx context.Context, sink domain.ProgressSink, key domain.EpisodeKey) (func(), error) {
	release := func() { <-muxGate }
	select {
	case muxGate <- struct{}{}:
		return release, nil
	default:
	}
	if stager, ok := sink.(domain.EpisodeStageSink); ok {
		stager.EpisodeStage(key, domain.EpisodeStage{Phase: "mux", Format: "в очереди"})
	}
	select {
	case muxGate <- struct{}{}:
		return release, nil
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}
}
