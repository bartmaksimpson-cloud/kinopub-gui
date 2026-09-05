package downloader

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

func TestMoveFile_SameFilesystem(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "a.tmp")
	to := filepath.Join(dir, "a.mkv")
	if err := os.WriteFile(from, []byte("кино"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(from, to); err != nil {
		t.Fatalf("moveFile: %v", err)
	}
	got, err := os.ReadFile(to)
	if err != nil || string(got) != "кино" {
		t.Fatalf("файл не доехал: %v %q", err, got)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Error("исходный файл не удалён")
	}
}

// Путь, ради которого всё и написано: рабочая папка на другом диске, где
// os.Rename падает с EXDEV. Разные файловые системы в тесте не поднять,
// поэтому проверяется сама копия — то, чем перенос подстраховывается.
func TestCopyFile_KeepsContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	body := make([]byte, 1<<20) // мегабайт: не один Read
	for i := range body {
		body[i] = byte(i)
	}
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(body) {
		t.Fatalf("размер %d, ожидался %d", len(got), len(body))
	}
	for i := range got {
		if got[i] != body[i] {
			t.Fatalf("байт %d испорчен", i)
		}
	}
}

// Перенос не должен оставлять после себя половину файла под финальным именем.
func TestMoveFile_MissingSourceLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "out.mkv")
	if err := moveFile(filepath.Join(dir, "нет-такого"), to); err == nil {
		t.Fatal("перенос несуществующего файла прошёл успешно")
	}
	if _, err := os.Stat(to); !os.IsNotExist(err) {
		t.Error("под финальным именем что-то осталось")
	}
	if _, err := os.Stat(to + ".moving"); !os.IsNotExist(err) {
		t.Error("промежуточный файл не убран")
	}
}

// Половина смысла раздельных папок — в том, ЧТО и КУДА пишется на каждом шаге.
// Временный файл склейки должен лежать рядом с готовым файлом, а не в рабочей
// папке: иначе ffmpeg читает и пишет один и тот же диск, а следом ещё и
// переносит результат целиком. Правильная раскладка: читаем с рабочего диска,
// пишем на выходной, перенос вырождается в переименование.
func TestMuxTempSitsNextToOutput(t *testing.T) {
	work := t.TempDir()
	out := filepath.Join(t.TempDir(), "Сериал", "Season 01", "S01E01.mkv")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}

	var seenTemp string
	run := func(_ context.Context, _ string, args []string, _ []string, _ io.Writer, _ io.Reader) error {
		seenTemp = args[len(args)-1]
		return os.WriteFile(seenTemp, make([]byte, 4096), 0o644)
	}
	d := New(run, &testProxy{}, testLogger{},
		WithFFmpegPath("ffmpeg"), WithWorkDir(work), WithOutputRoot(filepath.Dir(filepath.Dir(out))))

	job := domain.Job{
		Episode: domain.Episode{Key: domain.EpisodeKey{Series: "s", Season: 1, Episode: 1}},
		OutPath: out,
	}
	if err := d.MuxHLS(context.Background(), job, &domain.HLSDownloadResult{VideoPath: filepath.Join(work, "video.ts")}); err != nil {
		t.Fatalf("мукс: %v", err)
	}

	if seenTemp != out+".tmp" {
		t.Errorf("ffmpeg писал в %q, а должен был рядом с готовым файлом (%q)", seenTemp, out+".tmp")
	}
	if strings.HasPrefix(seenTemp, work) {
		t.Errorf("временный файл склейки оказался в рабочей папке: %q", seenTemp)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("готовый файл не появился: %v", err)
	}
}
