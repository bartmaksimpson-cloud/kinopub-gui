package kinopub

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// Имя временного каталога выводится из имени готового файла, поэтому каждое
// изменение именования осиротляет недокачанное. За один день имя менялось
// дважды — добавились названия серий, потом структура в рабочей папке, — и
// пользователь, обновившийся посреди загрузки, начинал качать гигабайты заново.
func TestAdoptLegacyTemp_PicksUpOldLayouts(t *testing.T) {
	series := domain.Series{ID: "8634", Title: "Южный Парк"}
	ep := domain.Episode{Key: domain.EpisodeKey{Series: "8634", Season: 1, Episode: 2}, Title: "Вулкан"}

	cases := []struct {
		name  string
		build func(root, work string) string // создаёт старый каталог, возвращает его путь
		work  bool
	}{
		{
			name: "старое имя без названия серии, рядом с выходным файлом",
			build: func(root, _ string) string {
				return filepath.Join(root, "Южный Парк", "Season 01", "S01E02.mkv.ts.hls-tmp")
			},
		},
		{
			name: "рабочая папка, плоская укладка",
			work: true,
			build: func(_, work string) string {
				return filepath.Join(work, "S01E02.mkv.ts.hls-tmp")
			},
		},
		{
			name: "рабочая папка, старое имя в зеркальной структуре",
			work: true,
			build: func(_, work string) string {
				return filepath.Join(work, "Южный Парк", "Season 01", "S01E02.mkv.ts.hls-tmp")
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			work := ""
			if c.work {
				work = t.TempDir()
			}
			cfg := domain.RunConfig{OutputPath: root, WorkPath: work, Container: domain.ContainerMKV}

			old := c.build(root, work)
			if err := os.MkdirAll(filepath.Join(old, "video"), 0o755); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(old, "video", "seg_00000.ts")
			if err := os.WriteFile(marker, []byte("сегмент"), 0o644); err != nil {
				t.Fatal(err)
			}

			// Путь, который ждёт текущая версия.
			outPath := filepath.Join(root, "Южный Парк", "Season 01", "S01E02 - Вулкан.mkv")
			tsPath := domain.WorkPathFor(work, root, outPath) + ".ts"

			adoptLegacyTemp(cfg, series, ep, tsPath, &mockLogger{})

			want := filepath.Join(tsPath+".hls-tmp", "video", "seg_00000.ts")
			if _, err := os.Stat(want); err != nil {
				t.Errorf("сегменты не подобраны: %v", err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Error("старый каталог остался на месте — сегменты задвоятся")
			}
		})
	}
}

// Если сегменты текущей версии уже лежат где надо, трогать ничего нельзя.
func TestAdoptLegacyTemp_LeavesCurrentAlone(t *testing.T) {
	root := t.TempDir()
	cfg := domain.RunConfig{OutputPath: root, Container: domain.ContainerMKV}
	series := domain.Series{ID: "1", Title: "Сериал"}
	ep := domain.Episode{Key: domain.EpisodeKey{Season: 1, Episode: 1}}

	outPath := filepath.Join(root, "Сериал", "Season 01", "S01E01.mkv")
	tsPath := outPath + ".ts"
	current := tsPath + ".hls-tmp"
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "свой.ts"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	adoptLegacyTemp(cfg, series, ep, tsPath, &mockLogger{})

	if _, err := os.Stat(filepath.Join(current, "свой.ts")); err != nil {
		t.Errorf("текущие сегменты пострадали: %v", err)
	}
}
