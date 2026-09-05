package downloader

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// Ночью машина ничья — можно занимать всё. Днём одно ядро остаётся свободным,
// иначе за компьютером нельзя работать, пока считается эпизод.
func TestEncodeThreads_ByTimeOfDay(t *testing.T) {
	n := runtime.NumCPU()
	if n < 2 {
		t.Skip("на одноядерной машине делить нечего")
	}
	day := time.Date(2026, 9, 5, 14, 0, 0, 0, time.Local)
	night := time.Date(2026, 9, 5, 3, 0, 0, 0, time.Local)

	if got := encodeThreads(night); got != n {
		t.Errorf("ночью потоков %d, ожидалось %d (все)", got, n)
	}
	if got := encodeThreads(day); got != n-1 {
		t.Errorf("днём потоков %d, ожидалось %d (все минус одно)", got, n-1)
	}
	// Границы: 00:00 уже ночь, 09:00 уже день.
	if got := encodeThreads(time.Date(2026, 9, 5, 0, 0, 0, 0, time.Local)); got != n {
		t.Errorf("в полночь потоков %d, ожидалось %d", got, n)
	}
	if got := encodeThreads(time.Date(2026, 9, 5, 9, 0, 0, 0, time.Local)); got != n-1 {
		t.Errorf("в девять утра потоков %d, ожидалось %d", got, n-1)
	}
}

// Ограничение должно связываться с ДЕКОДИРОВАНИЕМ: именно там уходит процессор,
// когда кодирует видеокарта. Опция перед первым входом относится к нему.
func TestWithDecodeThreads_BindsToInput(t *testing.T) {
	args := withDecodeThreads([]string{"-y", "-i", "video.ts", "-c", "copy", "out.mkv"}, 3)
	joined := strings.Join(args, " ")
	if !strings.HasPrefix(joined, "-y -threads 3 -filter_threads 3 -i ") {
		t.Errorf("ограничение стоит не перед входом: %q", joined)
	}
	// Без ограничения аргументы не трогаем вовсе.
	plain := []string{"-y", "-i", "video.ts"}
	if got := withDecodeThreads(plain, 0); strings.Join(got, " ") != strings.Join(plain, " ") {
		t.Errorf("нулевое ограничение изменило аргументы: %v", got)
	}
}

// Метка энкодера — то, что человек найдёт в диспетчере задач, а не имя кодека.
func TestEncoderLabel(t *testing.T) {
	cases := map[string]string{
		"h264_nvenc":        "NVIDIA (NVENC)",
		"hevc_amf":          "AMD (AMF)",
		"hevc_qsv":          "Intel (Quick Sync)",
		"hevc_videotoolbox": "Apple (VideoToolbox)",
		"libx264":           "процессор",
	}
	for enc, want := range cases {
		if got := encoderLabel([]string{"-vf", "scale=-16:2160", "-c:v", enc, "-b:v", "9000k"}); got != want {
			t.Errorf("%s → %q, ожидалось %q", enc, got, want)
		}
	}
	if got := encoderLabel(nil); got != "" {
		t.Errorf("для чистой копии метка должна быть пустой, получено %q", got)
	}
}
