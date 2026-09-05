package outputlayout

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// Compile-time interface satisfaction check.
var _ domain.OutputLayout = (*Layout)(nil)

func TestEpisodePath_Basic(t *testing.T) {
	l := New(domain.ContainerMKV)

	series := domain.Series{
		ID:    "12345",
		Title: "Тест Сериал",
	}
	ep := domain.Episode{
		Key: domain.EpisodeKey{
			Series:  "12345",
			Season:  1,
			Episode: 8,
		},
	}

	got, err := l.EpisodePath("/output", series, ep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("/output", "Тест Сериал", "Season 01", "S01E08.mkv")
	if got != want {
		t.Errorf("EpisodePath = %q, want %q", got, want)
	}
}

func TestEpisodePath_MP4Container(t *testing.T) {
	l := New(domain.ContainerMP4)

	series := domain.Series{
		ID:    "99",
		Title: "Show",
	}
	ep := domain.Episode{
		Key: domain.EpisodeKey{
			Series:  "99",
			Season:  12,
			Episode: 3,
		},
	}

	got, err := l.EpisodePath("/root", series, ep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("/root", "Show", "Season 12", "S12E03.mp4")
	if got != want {
		t.Errorf("EpisodePath = %q, want %q", got, want)
	}
}

func TestEpisodePath_SanitizesTitle(t *testing.T) {
	l := New(domain.ContainerMKV)

	series := domain.Series{
		ID:    "42",
		Title: "Bad/Title:With*Chars",
	}
	ep := domain.Episode{
		Key: domain.EpisodeKey{
			Series:  "42",
			Season:  2,
			Episode: 5,
		},
	}

	got, err := l.EpisodePath("/out", series, ep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("/out", "Bad_Title_With_Chars", "Season 02", "S02E05.mkv")
	if got != want {
		t.Errorf("EpisodePath = %q, want %q", got, want)
	}
}

func TestEpisodePath_EmptyTitleFallback(t *testing.T) {
	l := New(domain.ContainerMKV)

	series := domain.Series{
		ID:    "777",
		Title: "", // empty title → fallback
	}
	ep := domain.Episode{
		Key: domain.EpisodeKey{
			Series:  "777",
			Season:  1,
			Episode: 1,
		},
	}

	got, err := l.EpisodePath("/out", series, ep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("/out", "series_777", "Season 01", "S01E01.mkv")
	if got != want {
		t.Errorf("EpisodePath = %q, want %q", got, want)
	}
}

func TestEpisodePath_ZeroPadding(t *testing.T) {
	l := New(domain.ContainerMKV)

	series := domain.Series{
		ID:    "1",
		Title: "X",
	}
	ep := domain.Episode{
		Key: domain.EpisodeKey{
			Series:  "1",
			Season:  3,
			Episode: 14,
		},
	}

	got, err := l.EpisodePath("/", series, ep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("/", "X", "Season 03", "S03E14.mkv")
	if got != want {
		t.Errorf("EpisodePath = %q, want %q", got, want)
	}
}

func TestEnsureDirs_CreatesDirectories(t *testing.T) {
	tmp := t.TempDir()
	l := New(domain.ContainerMKV)

	path := filepath.Join(tmp, "a", "b", "c", "file.mkv")
	if err := l.EnsureDirs(path); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory, got file")
	}
}

func TestEnsureDirs_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	l := New(domain.ContainerMKV)

	path := filepath.Join(tmp, "x", "y", "file.mkv")

	// Call twice — second call should not error.
	if err := l.EnsureDirs(path); err != nil {
		t.Fatalf("first EnsureDirs failed: %v", err)
	}
	if err := l.EnsureDirs(path); err != nil {
		t.Fatalf("second EnsureDirs failed: %v", err)
	}
}

func TestEnsureDirs_UnwritableError(t *testing.T) {
	// Relies on /dev/null being a file so a directory cannot be created beneath
	// it. Windows has no equivalent always-unwritable path, so skip there.
	if runtime.GOOS == "windows" {
		t.Skip("no /dev/null-style unwritable path on Windows")
	}
	// Use a path that cannot be created (e.g., under /dev/null on Unix).
	l := New(domain.ContainerMKV)

	path := "/dev/null/impossible/path/file.mkv"
	err := l.EnsureDirs(path)
	if err == nil {
		t.Fatal("expected error for unwritable path, got nil")
	}
	if !errors.Is(err, domain.ErrOutputDirUnwritable) {
		t.Errorf("expected ErrOutputDirUnwritable, got: %v", err)
	}
}

// Фильм — это файл, а не сериал из одной серии. Раньше он ложился как
// «Фильм/Season 01/S01E01.mkv»: папка ни о чём, и имя файла ничего не говорит.
func TestEpisodePath_MovieIsAFile(t *testing.T) {
	l := New(domain.ContainerMKV)
	movie := domain.Series{ID: "100468", Title: "Дюна: Часть вторая", IsMovie: true}

	got, err := l.EpisodePath("/out", movie, domain.Episode{
		Key:   domain.EpisodeKey{Series: "100468", Season: 1, Episode: 1},
		Title: "24 fps",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Двоеточие — запрещённый в именах файлов символ, санитайзер меняет его на «_».
	if want := filepath.Join("/out", "Дюна_ Часть вторая.mkv"); got != want {
		t.Errorf("получено %q, ожидалось %q", got, want)
	}
}

// Когда сервис выкладывает фильм несколькими файлами, второй и последующие
// называют себя сами — иначе они затирали бы друг друга.
func TestEpisodePath_MovieVariantsDoNotCollide(t *testing.T) {
	l := New(domain.ContainerMKV)
	movie := domain.Series{ID: "100468", Title: "Дюна: Часть вторая", IsMovie: true}

	first, _ := l.EpisodePath("/out", movie, domain.Episode{
		Key: domain.EpisodeKey{Season: 1, Episode: 1}, Title: "24 fps",
	})
	second, _ := l.EpisodePath("/out", movie, domain.Episode{
		Key: domain.EpisodeKey{Season: 1, Episode: 2}, Title: "48 fps",
	})
	if first == second {
		t.Fatalf("обе версии легли в один файл: %q", first)
	}
	if want := filepath.Join("/out", "Дюна_ Часть вторая (48 fps).mkv"); second != want {
		t.Errorf("вторая версия: получено %q, ожидалось %q", second, want)
	}

	// Без собственного имени версия всё равно должна отличаться.
	noName, _ := l.EpisodePath("/out", movie, domain.Episode{Key: domain.EpisodeKey{Season: 1, Episode: 3}})
	if noName == first {
		t.Errorf("безымянная версия совпала с первой: %q", noName)
	}
}

// У сериала раскладка по сезонам остаётся, но название серии больше не теряется.
func TestEpisodePath_SerialKeepsEpisodeTitle(t *testing.T) {
	l := New(domain.ContainerMKV)
	serial := domain.Series{ID: "8634", Title: "Южный Парк"}

	got, _ := l.EpisodePath("/out", serial, domain.Episode{
		Key:   domain.EpisodeKey{Season: 1, Episode: 2},
		Title: "Вулкан",
	})
	want := filepath.Join("/out", "Южный Парк", "Season 01", "S01E02 - Вулкан.mkv")
	if got != want {
		t.Errorf("получено %q, ожидалось %q", got, want)
	}
}

// Пустое и служебное название не добавляется: «S01E04 - Серия 4» — это шум,
// номер уже стоит в начале имени.
func TestEpisodePath_SkipsGenericEpisodeTitles(t *testing.T) {
	l := New(domain.ContainerMKV)
	serial := domain.Series{ID: "8634", Title: "Сериал"}
	for _, title := range []string{"", "   ", "Серия 4", "серия 4", "Episode 4", "Эпизод 4"} {
		got, _ := l.EpisodePath("/out", serial, domain.Episode{
			Key: domain.EpisodeKey{Season: 1, Episode: 4}, Title: title,
		})
		want := filepath.Join("/out", "Сериал", "Season 01", "S01E04.mkv")
		if got != want {
			t.Errorf("название %q: получено %q, ожидалось %q", title, got, want)
		}
	}
}

// Длинное название не должно упереться в предел файловой системы: 255 БАЙТ,
// а кириллица тратит по два на символ.
func TestEpisodePath_TruncatesLongTitles(t *testing.T) {
	l := New(domain.ContainerMKV)
	long := strings.Repeat("длинное название ", 20)
	got, _ := l.EpisodePath("/out", domain.Series{ID: "1", Title: "Сериал"}, domain.Episode{
		Key: domain.EpisodeKey{Season: 1, Episode: 1}, Title: long,
	})
	if n := len([]byte(filepath.Base(got))); n > 255 {
		t.Errorf("имя файла %d байт — файловая система его не примет", n)
	}
}
