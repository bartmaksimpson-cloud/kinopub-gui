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
