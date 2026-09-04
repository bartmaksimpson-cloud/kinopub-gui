package downloader

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// Живая проверка всего пути на настоящем ffmpeg: кадр выше предела приезжает
// уменьшенным до 2160 и в HEVC, а не копируется как есть. Проверять сборкой
// строки аргументов мало — порядок «-c copy … -vf … -c:v» ffmpeg трактует
// сам, и ошибка здесь тихо отдала бы файл, который телевизор снова не возьмёт.
func TestFitPipeline_RealFFmpeg(t *testing.T) {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s не найден", bin)
		}
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	mk := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=3840x2880:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "3", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest", "-y", src)
	if out, err := mk.CombinedOutput(); err != nil {
		t.Fatalf("не удалось собрать источник: %v\n%s", err, out)
	}

	d := New(func(ctx context.Context, name string, args, env []string, stdout io.Writer) error {
		cmd := exec.CommandContext(ctx, name, args...)
		if stdout != nil {
			cmd.Stdout = stdout
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("ffmpeg упал: %v\n%s", err, out)
		}
		return err
	}, &testProxy{}, testLogger{}, WithFFmpegPath("ffmpeg"), WithMaxHeight(2160))

	outPath := filepath.Join(dir, "out.mkv")
	job := domain.Job{
		Episode: domain.Episode{Key: domain.EpisodeKey{Series: "s", Season: 1, Episode: 1}, Duration: 3 * time.Second},
		OutPath: outPath,
	}
	hls := &domain.HLSDownloadResult{VideoPath: src, Resolution: "3840x2880", BitrateKbps: 12000}

	if err := d.MuxHLS(context.Background(), job, hls); err != nil {
		t.Fatalf("MuxHLS: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("файла нет: %v", err)
	}

	probe, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height", "-of", "csv=p=0", outPath).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	got := strings.TrimSpace(string(probe))
	t.Logf("получено: %s", got)
	if !strings.Contains(got, "2880,2160") {
		t.Errorf("кадр не приведён к 2880x2160: %s", got)
	}
	if !strings.HasPrefix(got, "hevc") {
		t.Errorf("не HEVC: %s", got)
	}
}
