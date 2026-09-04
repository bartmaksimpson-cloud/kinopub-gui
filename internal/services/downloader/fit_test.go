package downloader

import (
	"strings"
	"testing"
)

func TestHeightOf(t *testing.T) {
	cases := map[string]int{
		"1920x1080":   1080,
		"3840x2880":   2880,
		" 3840x2160 ": 2160,
		"":            0,
		"1080":        0,
		"1920xabc":    0,
		"1920x0":      0,
		"1920x-4":     0,
	}
	for in, want := range cases {
		if got := heightOf(in); got != want {
			t.Errorf("heightOf(%q) = %d, ожидалось %d", in, got, want)
		}
	}
}

// The whole point of the cap: 3840x2880 is inside the width limit of a TV
// decoder and far over its height limit, so it must be scaled; a standard 4K
// frame must be left alone, or every download would be re-encoded for nothing.
func TestScaleToHeightArgs(t *testing.T) {
	tall := scaleToHeightArgs(2880, 2160, 0, "ffmpeg")
	if len(tall) == 0 {
		t.Fatal("кадр 2880 выше предела 2160, а аргументов нет")
	}
	joined := strings.Join(tall, " ")
	if !strings.Contains(joined, "-vf scale=-16:2160") {
		t.Errorf("нет фильтра масштабирования: %q", joined)
	}
	// Scaling cannot be a stream copy — an encoder has to be chosen.
	if !strings.Contains(joined, "-c:v") {
		t.Errorf("масштабирование без энкодера — ffmpeg откажется: %q", joined)
	}

	// The source bitrate is carried over so the smaller frame keeps its picture.
	withRate := strings.Join(scaleToHeightArgs(2880, 2160, 18000, "ffmpeg"), " ")
	if !strings.Contains(withRate, "-b:v 18000k") {
		t.Errorf("битрейт источника не перенесён: %q", withRate)
	}

	for _, c := range []struct {
		name     string
		src, max int
	}{
		{"кадр ровно по пределу", 2160, 2160},
		{"кадр ниже предела", 1080, 2160},
		{"предел выключен", 2880, 0},
		{"разрешение неизвестно", 0, 2160},
	} {
		if got := scaleToHeightArgs(c.src, c.max, 0, "ffmpeg"); got != nil {
			t.Errorf("%s: ожидалась копия без перекодирования, получено %v", c.name, got)
		}
	}
}

// Дюна: 3840x1600 при 47,952 к/с. Кадр в предел железа влезает, а поток — нет:
// декодер принимает файл, запускается и выбрасывает две трети кадров.
func TestFitArgsFor_HalvesHighFrameRateAt4K(t *testing.T) {
	got := fitArgsFor(
		fitSource{Width: 3840, Height: 1600, FPS: 47.952, Kbps: 9000},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	)
	joined := strings.Join(got, " ")
	// Ровно половина, а не «30»: так сохраняется исходная каденция фильма и ни
	// один кадр не показывается дважды.
	if !strings.Contains(joined, "-r 23.976") {
		t.Errorf("частота не поделена пополам: %q", joined)
	}
	// Кадр в предел влезает — масштабировать нечего.
	if strings.Contains(joined, "scale=") {
		t.Errorf("кадр 1600 масштабировать не нужно: %q", joined)
	}
	if !strings.Contains(joined, "-b:v 9000k") {
		t.Errorf("битрейт источника не перенесён: %q", joined)
	}
}

// На маленьком кадре высокая частота железу не мешает — портить её нельзя.
func TestFitArgsFor_LeavesSmallFramesAlone(t *testing.T) {
	if got := fitArgsFor(
		fitSource{Width: 1920, Height: 1080, FPS: 60},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	); got != nil {
		t.Errorf("1080p60 трогать не надо, получено %v", got)
	}
}

// Оба предела сразу: и кадр выше, и частота выше.
func TestFitArgsFor_ScalesAndHalvesTogether(t *testing.T) {
	joined := strings.Join(fitArgsFor(
		fitSource{Width: 3840, Height: 2880, FPS: 60, Kbps: 20000},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	), " ")
	if !strings.Contains(joined, "scale=-16:2160") || !strings.Contains(joined, "-r 30") {
		t.Errorf("нужны и масштабирование, и деление частоты: %q", joined)
	}
}

// Обычный 4K24: ничего не трогаем, файл копируется как есть.
func TestFitArgsFor_PassesStandard4K(t *testing.T) {
	if got := fitArgsFor(
		fitSource{Width: 3840, Height: 2160, FPS: 23.976},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	); got != nil {
		t.Errorf("настоящий 4K должен копироваться без изменений, получено %v", got)
	}
}

// Настоящий 4K60 в HEVC железо показывает: чипы, которые упираются в 4Kp30 на
// H.264, тянут HEVC до 4Kp60 и выше. Делить такую частоту — выбросить плавность,
// которую плеер способен показать.
func TestFitArgsFor_KeepsHighFrameRateForHEVC(t *testing.T) {
	if got := fitArgsFor(
		fitSource{Width: 3840, Height: 2160, FPS: 60, Kbps: 25000, Codec: "h265"},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	); got != nil {
		t.Errorf("HEVC 4K60 трогать не надо, получено %v", got)
	}

	// А тот же поток в H.264 — за пределом бюджета декодера, делим.
	joined := strings.Join(fitArgsFor(
		fitSource{Width: 3840, Height: 2160, FPS: 60, Kbps: 25000, Codec: "h264"},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	), " ")
	if !strings.Contains(joined, "-r 30") {
		t.Errorf("H.264 4K60 должен быть поделён: %q", joined)
	}
}
