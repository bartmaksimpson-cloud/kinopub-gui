package gui

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// О содержимом скачанного файла состояние знает не всё: разрешение и битрейт
// туда пишутся с версии, которая их считала, а частота кадров не писалась
// никогда. Спрашивать сам файл — единственный способ показать одно и то же для
// серии, скачанной вчера и полгода назад.
//
// Заголовок читается один раз на файл: результат живёт в памяти, пока файл не
// поменяется. Библиотека пересканируется на каждый запрос страницы, и без
// этого сетевая папка опрашивалась бы заново десятками ffprobe.

// mediaInfo is what the video track of a downloaded file turned out to be.
type mediaInfo struct {
	Resolution  string  `json:"resolution,omitempty"`  // 3840x2160
	Codec       string  `json:"videoCodec,omitempty"`  // HEVC
	BitrateKbps int     `json:"bitrateKbps,omitempty"` // 26836
	FPS         float64 `json:"fps,omitempty"`         // 24
}

type cachedMediaInfo struct {
	info  mediaInfo
	size  int64
	mtime time.Time
}

var mediaCache = struct {
	sync.Mutex
	m map[string]cachedMediaInfo
}{m: map[string]cachedMediaInfo{}}

// mediaProbeTimeout bounds one ffprobe. Отвалившаяся на середине сетевая папка
// не должна подвесить показ библиотеки.
const mediaProbeTimeout = 5 * time.Second

// mediaProbeWorkers bounds how many files are probed at once.
const mediaProbeWorkers = 8

// mediaProbeBudget bounds one whole scan. Медленная шара не должна держать
// открытие библиотеки: что не успели прочитать, прочитается на следующем
// сканировании — прочитанное уже лежит в кэше.
//
// ponytail: кэш живёт только в памяти. Если после перезапуска первое открытие
// библиотеки станет заметно долгим — писать разбор в файл состояния рядом с
// фильмом, как это делает backfill метаданных.
const mediaProbeBudget = 20 * time.Second

// fillMediaInfo probes the files behind the episodes and fills their media
// details. Пропавшие файлы пропускаются: спрашивать нечего.
func fillMediaInfo(eps []LibraryEpisode) {
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		return
	}
	ctx, cancel := contextWithTimeout(mediaProbeBudget)
	defer cancel()
	sem := make(chan struct{}, mediaProbeWorkers)
	var wg sync.WaitGroup
	for i := range eps {
		if !eps[i].Exists || eps[i].Path == "" {
			continue
		}
		wg.Add(1)
		go func(ep *LibraryEpisode) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			info := probeMedia(ctx, probe, ep.Path)
			if info.Resolution != "" {
				ep.Resolution = info.Resolution
			}
			ep.Codec = info.Codec
			ep.BitrateKbps = info.BitrateKbps
			ep.FPS = info.FPS
		}(&eps[i])
	}
	wg.Wait()
}

// probeMedia reads the video track's headers, through the cache.
func probeMedia(ctx context.Context, ffprobePath, path string) mediaInfo {
	st, err := os.Stat(path)
	if err != nil {
		return mediaInfo{}
	}
	mediaCache.Lock()
	c, ok := mediaCache.m[path]
	mediaCache.Unlock()
	if ok && c.size == st.Size() && c.mtime.Equal(st.ModTime()) {
		return c.info
	}

	info := runFFprobe(ctx, ffprobePath, path)

	mediaCache.Lock()
	mediaCache.m[path] = cachedMediaInfo{info: info, size: st.Size(), mtime: st.ModTime()}
	mediaCache.Unlock()
	return info
}

func runFFprobe(parent context.Context, ffprobePath, path string) mediaInfo {
	ctx, cancel := context.WithTimeout(parent, mediaProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,codec_name,bit_rate,avg_frame_rate",
		"-show_entries", "format=bit_rate",
		"-of", "json",
		path,
	)
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return mediaInfo{}
	}
	return parseFFprobe(out)
}

func parseFFprobe(data []byte) mediaInfo {
	var probed struct {
		Streams []struct {
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			CodecName string `json:"codec_name"`
			BitRate   string `json:"bit_rate"`
			AvgFrame  string `json:"avg_frame_rate"`
		} `json:"streams"`
		Format struct {
			BitRate string `json:"bit_rate"`
		} `json:"format"`
	}
	if json.Unmarshal(data, &probed) != nil || len(probed.Streams) == 0 {
		return mediaInfo{}
	}
	s := probed.Streams[0]
	var info mediaInfo
	if s.Width > 0 && s.Height > 0 {
		info.Resolution = strconv.Itoa(s.Width) + "x" + strconv.Itoa(s.Height)
	}
	info.Codec = prettyCodec(s.CodecName)
	// У дорожки в mkv битрейта обычно нет — контейнер его не хранит. Тогда
	// берётся общий битрейт файла: с звуком он на процент-другой выше, и это
	// ближе к правде, чем пустое место.
	bits := s.BitRate
	if bits == "" || bits == "N/A" {
		bits = probed.Format.BitRate
	}
	if n, err := strconv.ParseInt(bits, 10, 64); err == nil && n > 0 {
		info.BitrateKbps = int(n / 1000)
	}
	info.FPS = parseFrameRate(s.AvgFrame)
	return info
}

// parseFrameRate turns "24000/1001" into 24. Кинематографические 23.976 и 29.97
// — это те же 24 и 30 с телевизионной поправкой, и показывать дробь незачем.
func parseFrameRate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		return 0
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 || n <= 0 {
		return 0
	}
	fps := n / d
	// Телевизионная поправка — это ровно деление на 1001/1000, то есть отставание
	// на одну тысячную. Допуск задан долей, а не абсолютом: иначе 59.94 не
	// округлилось бы до 60, отстав уже на 0.06.
	if r := math.Round(fps); r > 0 && math.Abs(fps-r)/r < 0.002 {
		return r
	}
	return math.Round(fps*100) / 100
}

var codecNames = map[string]string{
	"hevc":       "HEVC",
	"h264":       "H.264",
	"av1":        "AV1",
	"vp9":        "VP9",
	"mpeg4":      "MPEG-4",
	"mpeg2video": "MPEG-2",
	"vc1":        "VC-1",
}

func prettyCodec(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "n/a" {
		return ""
	}
	if p, ok := codecNames[name]; ok {
		return p
	}
	return strings.ToUpper(name)
}
