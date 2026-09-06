package downloader

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// interlacedFieldOrders are the ffprobe answers that mean "two half-frames from
// two different moments". Everything else — "progressive", "unknown", an empty
// answer — is treated as progressive: guessing wrong towards deinterlacing
// would re-encode a file that could have been copied.
var interlacedFieldOrders = map[string]bool{"tt": true, "bb": true, "tb": true, "bt": true}

// ffprobeNear is the ffprobe that sits next to this ffmpeg. They ship together,
// and the app installs them into one folder, so deriving the path beats adding
// a second setting nobody would ever set differently.
func ffprobeNear(ffmpegPath string) string {
	if ffmpegPath == "" || ffmpegPath == "ffmpeg" {
		return "ffprobe"
	}
	dir, base := filepath.Split(ffmpegPath)
	return filepath.Join(dir, strings.Replace(base, "ffmpeg", "ffprobe", 1))
}

// isInterlacedFile reports whether the video in path is interlaced.
//
// Читается из самого файла, а не из подписи: чересстрочность нигде не
// объявлена — ни в плейлисте, ни в ярлыке качества, — и единственный честный
// источник это поле field_order у дорожки. Пробуется ЛОКАЛЬНЫЙ сегмент, уже
// лежащий на диске, поэтому проверка ничего не стоит и не ходит в сеть.
//
// Любая неудача — молчаливое «нет»: непрочитанный сегмент не повод
// перекодировать серию, которую можно было скопировать.
func isInterlacedFile(ffmpegPath, path string) bool {
	if path == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, ffprobeNear(ffmpegPath),
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=field_order",
		"-of", "default=nw=1:nk=1",
		path,
	).Output()
	if err != nil {
		return false
	}
	return interlacedFieldOrders[strings.ToLower(strings.TrimSpace(string(out)))]
}

// firstVideoFile is the piece of video to probe: the first downloaded segment
// when the video is streamed in parts, otherwise the joined file. An fMP4 init
// segment carries no frames, so it is skipped — probing it would answer
// nothing about the picture.
func firstVideoFile(hls *domain.HLSDownloadResult) string {
	for _, p := range hls.VideoParts {
		if strings.HasSuffix(strings.ToLower(p), "init.mp4") {
			continue
		}
		return p
	}
	return hls.VideoPath
}
