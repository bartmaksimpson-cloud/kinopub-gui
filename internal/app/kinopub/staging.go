package kinopub

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// Сетевой диск исчезает буднично: у человека включился корпоративный VPN,
// заворачивающий весь трафик, — и папка Z: перестала существовать посреди
// сериала. Раньше это означало «серия провалена навсегда», и запуск успевал
// пометить так все оставшиеся, потому что причина у всех одна и мгновенная.
//
// Причина при этом временная, и файл девать есть куда: рабочая папка на месте.
// Поэтому серия собирается в неё, а в папку загрузки уезжает, как только та
// вернётся.

// unavailableOutputMarkers — тексты, которыми операционные системы сообщают
// «раздела сейчас нет», а не «вам сюда нельзя». Отказ в правах чинится
// настройками, отсутствие сети — временем, и путать их нельзя.
var unavailableOutputMarkers = []string{
	"network path was not found", // Windows 53: сетевой путь не найден
	"network name is no longer",  // Windows 64: имя больше не доступно
	"device is not ready",        // Windows 21: диск отвалился
	"no such host",               // имя NAS не разрешается
	"host is down",               //
	"no route to host",           //
	"connection refused",         // сервер SMB не отвечает
	"operation timed out",        //
	"i/o timeout",                //
	"transport endpoint is not connected",
	"stale file handle", // монтирование пережило разрыв и стало негодным
}

// outputUnavailable reports whether an error means the destination is
// temporarily gone rather than forbidden.
func outputUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, m := range unavailableOutputMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// stagedPathFor is where an episode waits out the outage: тот же путь, но в
// рабочей папке, которая повторяет структуру папки загрузки. Пустая строка —
// складывать некуда (рабочая папка не задана или совпадает с целевой).
func stagedPathFor(cfg domain.RunConfig, outPath string) string {
	if cfg.WorkPath == "" || cfg.WorkPath == cfg.OutputPath {
		return ""
	}
	staged := domain.WorkPathFor(cfg.WorkPath, cfg.OutputPath, outPath)
	if staged == outPath {
		return ""
	}
	return staged
}

// outputPathFor is the reverse mapping: по файлу в рабочей папке — его место в
// папке загрузки. Пустая строка, если файл лежит вне зеркала.
func outputPathFor(cfg domain.RunConfig, staged string) string {
	if cfg.WorkPath == "" || cfg.OutputPath == "" {
		return ""
	}
	rel, err := filepath.Rel(cfg.WorkPath, staged)
	if err != nil || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.Join(cfg.OutputPath, rel)
}

// isMediaFile отделяет готовые файлы от рабочего мусора: недокачанные сегменты,
// незавершённые склейки и промежуточные потоки переносить нечего.
func isMediaFile(name string) bool {
	lower := strings.ToLower(name)
	for _, bad := range []string{".tmp", ".part", ".ts", ".hls-tmp"} {
		if strings.HasSuffix(lower, bad) {
			return false
		}
	}
	// init.mp4 — заголовок fMP4-дорожки, а не фильм. По имени он неотличим от
	// готового файла, и первая версия этой проверки его переносила: на NAS
	// приезжала папка с одним init.mp4 внутри вместо серии.
	if lower == "init.mp4" {
		return false
	}
	return strings.HasSuffix(lower, ".mkv") || strings.HasSuffix(lower, ".mp4")
}

// isTempDir reports whether a directory holds work in progress rather than
// results. Внутрь таких папок ходить незачем: там сегменты, а не серии.
func isTempDir(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".hls-tmp") || strings.HasSuffix(lower, ".tmp")
}

// FlushStaged moves everything that waited out an outage into the download
// folder. Вызывается в начале запуска: к этому моменту диск обычно уже вернулся,
// а человек ничего для этого не делал.
//
// Ошибки не останавливают запуск: не переехало — значит подождёт следующего
// раза, файл при этом никуда не девается.
func FlushStaged(ctx context.Context, cfg domain.RunConfig, move func(from, to string) error, log domain.Logger) int {
	if cfg.WorkPath == "" || cfg.OutputPath == "" || cfg.WorkPath == cfg.OutputPath {
		return 0
	}
	moved := 0
	_ = filepath.Walk(cfg.WorkPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || ctx.Err() != nil {
			return nil
		}
		// В папку с сегментами не заходим вовсе: там тысячи файлов, среди них
		// init.mp4, и по имени он выглядит как готовое кино. Проверять
		// СТРУКТУРУ надёжнее, чем расширение: имя врёт, расположение — нет.
		if info.IsDir() {
			if isTempDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isMediaFile(info.Name()) {
			return nil
		}
		target := outputPathFor(cfg, path)
		if target == "" {
			return nil
		}
		if _, err := os.Stat(target); err == nil {
			return nil // на месте уже есть файл — трогать чужое не будем
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil // папка загрузки всё ещё недоступна — подождём
		}
		if err := move(path, target); err != nil {
			log.Warn("не удалось перенести отложенный файл",
				domain.F("from", path), domain.F("to", target), domain.F("error", err.Error()))
			return nil
		}
		moved++
		log.Info("перенёс файл, дождавшийся папки загрузки",
			domain.F("to", target))
		return nil
	})
	return moved
}

// outputReachable reports whether the download folder can be looked at at all.
//
// Папки может не быть просто потому, что её ещё не создали, — это нормально.
// Ненормально, когда нет самого диска: Z:\ отключён, \\nas\share не отвечает.
// Тогда os.Stat отвечает «путь не найден», ровно как для несозданной папки,
// поэтому смотрим на корень тома.
func outputReachable(dir string) bool {
	if dir == "" {
		return true
	}
	_, err := os.Stat(dir)
	if err == nil {
		return true
	}
	if outputUnavailable(err) {
		return false
	}
	if vol := filepath.VolumeName(dir); vol != "" {
		if _, err := os.Stat(vol + string(filepath.Separator)); err != nil {
			return false
		}
	}
	return true
}

// outputPollInterval — как часто проверять, не вернулась ли папка загрузки.
var outputPollInterval = 15 * time.Second

// waitForOutput blocks until the download folder is reachable or ctx ends.
//
// Без этого запуск на отключённом диске не мог прочитать файл состояния рядом с
// сериалом, получал пустое «ничего не скачано» и через рабочую папку начинал
// качать заново уже скачанные серии (так и было: 0 из 62 при 13 готовых).
// Решать, что качать, не видя папки загрузки, нельзя — поэтому ждём её.
func waitForOutput(ctx context.Context, dir string, log domain.Logger) error {
	if outputReachable(dir) {
		return nil
	}
	log.Warn("папка загрузки недоступна — жду её, чтобы не качать заново уже скачанное",
		domain.F("output", dir))
	t := time.NewTicker(outputPollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if outputReachable(dir) {
				log.Info("папка загрузки вернулась", domain.F("output", dir))
				return nil
			}
		}
	}
}
