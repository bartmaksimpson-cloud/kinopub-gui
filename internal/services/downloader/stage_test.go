package downloader

import (
	"strings"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// Подпись стадии читается из тех же аргументов, что уходят в ffmpeg, — иначе
// карточка обещала бы одно, а получалось бы другое.
func TestDescribeFit(t *testing.T) {
	hls := &domain.HLSDownloadResult{Resolution: "3840x2880"}
	got := describeFit([]string{"-vf", "scale=-16:2160", "-c:v", "hevc_videotoolbox", "-b:v", "18000k"}, hls)
	if !strings.Contains(got, "2880x2160") {
		t.Errorf("нет итогового кадра: %q", got)
	}
	if !strings.Contains(got, "HEVC") || !strings.Contains(got, "18000 kbps") {
		t.Errorf("нет кодека или битрейта: %q", got)
	}

	// Случай «только частота»: кадр не меняется, и обещать его изменение нельзя.
	got = describeFit([]string{"-r", "23.976", "-c:v", "h264_nvenc", "-b:v", "9000k"}, &domain.HLSDownloadResult{Resolution: "3840x1600"})
	if strings.Contains(got, "x") {
		t.Errorf("кадр не масштабируется, а в подписи он есть: %q", got)
	}
	if !strings.Contains(got, "23.976 fps") || !strings.Contains(got, "H.264") {
		t.Errorf("нет частоты или кодека: %q", got)
	}
}

// Ширина в подписи считается по тем же пропорциям и с тем же округлением до 16,
// что и в фильтре: 3840x2314 → 3584x2160.
func TestDescribeFit_KeepsAspect(t *testing.T) {
	got := describeFit([]string{"-vf", "scale=-16:2160", "-c:v", "hevc_amf"}, &domain.HLSDownloadResult{Resolution: "3840x2314"})
	if !strings.Contains(got, "3584x2160") {
		t.Errorf("ширина посчитана неверно: %q", got)
	}
}
