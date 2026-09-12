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
	url, out, status := j.url, j.outputPath, j.status
	j.mu.Unlock()
	if url == "" || out == "" {
		return
	}
	// Пока запуск идёт, правду о сериях знает движок: он их и качает, и
	// пропускает. Скан диска в это время только спорил бы с ним, причём с
	// задержкой на обход сетевой папки.
	if status == statusRunning || status == statusResolving {
		return
	}
	id := kinopubapi.ItemIDFromURL(url)
	if id == "" {
		return
	}

	type onDisk struct {
		bytes int64
		state string
	}
	found := map[string]onDisk{}

	// Сначала собственный список: он знает, ЧТО приложение скачивало, даже
	// когда папку сейчас не видно. Библиотека такого сказать не может — она
	// читает файлы состояния, а те лежат вместе с фильмами и пропадают вместе
	// с диском.
	for key, rec := range m.index.forSeries(id) {
		found[key] = onDisk{bytes: rec.Bytes, state: checkDisk(rec.Path)}
	}

	// Дополняем сканом папки: серии, скачанные до появления списка или другой
	// машиной, в нём не значатся, а на диске лежат.
	for _, series := range scanLibrary([]string{out}).Series {
		if !seriesMatchesItem(series, id) {
			continue
		}
		for _, ep := range series.Episodes {
			key := epKey(domain.EpisodeKey{Season: ep.Season, Episode: ep.Episode})
			if _, known := found[key]; known || !ep.Exists {
				continue
			}
			found[key] = onDisk{bytes: ep.Bytes, state: diskOK}
			// И запоминаем: иначе, стоит диску отключиться, движок не узнает об
			// этой серии ни из файла состояния, ни из списка — и скачает её
			// заново, хотя карточка только что показывала «скачано».
			m.index.remember(id, series.Title, domain.CompletedInfo{
				Key:   domain.EpisodeKey{Season: ep.Season, Episode: ep.Episode},
				Path:  ep.Path,
				Bytes: ep.Bytes,
				Title: ep.Title,
			})
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
		if ev.State == epCompleted && ev.Disk == disk.state {
			continue
		}
		// Файла нет, а папка на месте — это не «скачано»: значит его удалили, и
		// притворяться, что серия готова, значит прятать работу, которую придётся
		// сделать заново.
		if disk.state == diskMissing {
			if ev.State == epCompleted {
				ev.State = epFailed
				ev.Disk = diskMissing
				changed = true
			}
			continue
		}
		ev.State = epCompleted
		ev.Disk = disk.state
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

// refreshAllExisting re-checks every job's episodes against the disk.
func (m *JobManager) refreshAllExisting() {
	m.mu.RLock()
	jobs := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	m.mu.RUnlock()
	for _, j := range jobs {
		m.refreshExisting(j)
	}
}
