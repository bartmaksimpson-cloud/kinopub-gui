package gui

import (
	"github.com/ZioSHik/kinopub-gui/internal/domain"
	"github.com/ZioSHik/kinopub-gui/internal/services/kinopubapi"
)

// refreshExisting marks the episodes this job already has on disk.
//
// Пока запуск идёт, о таких сериях сообщает движок — он их пропускает и говорит
// об этом. Но карточка живёт и без запуска: остановленная задача показывала
// «на паузе, 0 Б» и кнопку «Продолжить» у серий, которые давно лежат в папке
// загрузки. Данные для правды есть на диске — там же, где их берёт библиотека.
//
// Скан идёт ВНЕ замка карточки: он ходит по сетевой папке и может отвечать
// секундами, а держать в это время карточку заблокированной — значит подвесить
// весь интерфейс.
func (m *JobManager) refreshExisting(j *Job) {
	j.mu.Lock()
	url, out := j.url, j.outputPath
	j.mu.Unlock()
	if url == "" || out == "" {
		return
	}
	id := kinopubapi.ItemIDFromURL(url)
	if id == "" {
		return
	}

	type onDisk struct {
		bytes int64
	}
	found := map[string]onDisk{}
	for _, series := range scanLibrary([]string{out}).Series {
		if !seriesMatchesItem(series, id) {
			continue
		}
		for _, ep := range series.Episodes {
			if ep.Exists {
				found[epKey(domain.EpisodeKey{Season: ep.Season, Episode: ep.Episode})] = onDisk{bytes: ep.Bytes}
			}
		}
	}
	if len(found) == 0 {
		return
	}

	changed := false
	j.mu.Lock()
	for key, disk := range found {
		ev, ok := j.episodes[key]
		if !ok || ev.State == epRunning {
			continue // идущую серию не трогаем: у неё своя, живая правда
		}
		if ev.State == epCompleted && ev.Existing {
			continue
		}
		ev.State = epCompleted
		ev.Existing = true
		ev.Percent = 100
		ev.Error = ""
		ev.SpeedBps, ev.ETASeconds = 0, 0
		ev.Stage, ev.StageFormat, ev.StageEncoder, ev.StageThreads = "", "", "", 0
		ev.StagePercent, ev.StageETASeconds = 0, 0
		if disk.bytes > 0 {
			ev.Bytes, ev.Total, ev.TotalApprox = disk.bytes, disk.bytes, false
		}
		changed = true
	}
	j.mu.Unlock()
	if changed {
		m.publishNow(j)
	}
}
