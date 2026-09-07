package kinopub

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
	"github.com/ZioSHik/kinopub-gui/internal/lib/fsutil"
)

// «Сетевого пути нет» и «сюда нельзя» лечатся по-разному: первое — временем,
// второе — настройками. Спутать их значит либо валить очередь на ровном месте,
// либо бесконечно ждать того, что не наступит.
func TestOutputUnavailable(t *testing.T) {
	temporary := []string{
		"mkdir Z:\\Сериал: The network path was not found.",
		"open \\\\nas\\video: The specified network name is no longer available.",
		"stat D:\\: device is not ready",
		"dial tcp: lookup nas: no such host",
		"read: i/o timeout",
	}
	for _, msg := range temporary {
		if !outputUnavailable(errString(msg)) {
			t.Errorf("не распознано как временное: %q", msg)
		}
	}
	permanent := []string{
		"mkdir /out: permission denied",
		"open /out: read-only file system",
		"no space left on device",
	}
	for _, msg := range permanent {
		if outputUnavailable(errString(msg)) {
			t.Errorf("принято за временное, хотя ждать бесполезно: %q", msg)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestStagedPathRoundTrip(t *testing.T) {
	cfg := domain.RunConfig{OutputPath: filepath.FromSlash("/nas/video"), WorkPath: filepath.FromSlash("/work")}
	out := filepath.FromSlash("/nas/video/Сериал/Season 01/S01E01.mkv")

	staged := stagedPathFor(cfg, out)
	want := filepath.FromSlash("/work/Сериал/Season 01/S01E01.mkv")
	if staged != want {
		t.Fatalf("stagedPathFor = %q, ожидалось %q", staged, want)
	}
	if back := outputPathFor(cfg, staged); back != out {
		t.Errorf("outputPathFor = %q, ожидалось %q", back, out)
	}
	// Без рабочей папки складывать некуда, и притворяться нечем.
	if got := stagedPathFor(domain.RunConfig{OutputPath: "/nas"}, out); got != "" {
		t.Errorf("без рабочей папки ожидалась пустая строка, получено %q", got)
	}
	// Файл вне зеркала переносить некуда.
	if got := outputPathFor(cfg, filepath.FromSlash("/somewhere/else.mkv")); got != "" {
		t.Errorf("файл вне рабочей папки: %q", got)
	}
}

// Главное поведение: то, что собралось в рабочей папке, пока сетевой диск
// отсутствовал, уезжает на место, когда он возвращается.
func TestFlushStaged(t *testing.T) {
	work := t.TempDir()
	out := t.TempDir()
	cfg := domain.RunConfig{OutputPath: out, WorkPath: work}

	ready := filepath.Join(work, "Сериал", "Season 01", "S01E01.mkv")
	if err := os.MkdirAll(filepath.Dir(ready), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{ready,
		filepath.Join(work, "Сериал", "Season 01", "S01E02.mkv.tmp"), // недоделанная склейка
		filepath.Join(work, "Сериал", "Season 01", "video.ts"),       // промежуточный поток
	} {
		if err := os.WriteFile(f, []byte("кино"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Папка с сегментами: внутри init.mp4, который по имени выглядит как кино.
	// Именно его первая версия сборщика утащила на NAS — вместо серии там
	// появлялась папка с одним заголовком fMP4 внутри.
	segs := filepath.Join(work, "Сериал", "Season 01", "S01E10 - Двухпалатный.mkv.ts.hls-tmp")
	if err := os.MkdirAll(segs, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"init.mp4", "seg_00001.ts"} {
		if err := os.WriteFile(filepath.Join(segs, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Уже лежащий на месте файл трогать нельзя.
	taken := filepath.Join(work, "Сериал", "Season 01", "S01E03.mkv")
	if err := os.WriteFile(taken, []byte("новое"), 0o644); err != nil {
		t.Fatal(err)
	}
	target3 := filepath.Join(out, "Сериал", "Season 01", "S01E03.mkv")
	if err := os.MkdirAll(filepath.Dir(target3), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target3, []byte("старое"), 0o644); err != nil {
		t.Fatal(err)
	}

	if n := FlushStaged(context.Background(), cfg, fsutil.Move, &mockLogger{}); n != 1 {
		t.Fatalf("перенесено файлов: %d, ожидался один", n)
	}

	moved := filepath.Join(out, "Сериал", "Season 01", "S01E01.mkv")
	if _, err := os.Stat(moved); err != nil {
		t.Errorf("готовый файл не доехал: %v", err)
	}
	if _, err := os.Stat(ready); !os.IsNotExist(err) {
		t.Error("исходник остался в рабочей папке")
	}
	if _, err := os.Stat(filepath.Join(work, "Сериал", "Season 01", "video.ts")); err != nil {
		t.Error("промежуточный поток унесли вместе с готовым файлом")
	}
	if body, _ := os.ReadFile(target3); string(body) != "старое" {
		t.Error("файл на месте перезаписан тем, что лежало в рабочей папке")
	}
	if _, err := os.Stat(filepath.Join(segs, "init.mp4")); err != nil {
		t.Error("заголовок fMP4 унесли из папки сегментов — серия перестанет докачиваться")
	}
	if _, err := os.Stat(filepath.Join(out, "Сериал", "Season 01", "S01E10 - Двухпалатный.mkv.ts.hls-tmp")); !os.IsNotExist(err) {
		t.Error("в папке назначения появилась папка сегментов")
	}
}
