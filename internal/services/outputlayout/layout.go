// Package outputlayout derives filesystem paths for episode output and ensures
// the required directory structure exists.
package outputlayout

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
	"github.com/ZioSHik/kinopub-gui/internal/lib/fsutil"
)

// Layout implements domain.OutputLayout.
type Layout struct {
	ext string // file extension including the leading dot, e.g. ".mkv"
}

// New creates a Layout for the given container format.
// The container determines the output file extension (.mkv or .mp4).
func New(container domain.Container) *Layout {
	ext := ".mkv"
	if container == domain.ContainerMP4 {
		ext = ".mp4"
	}
	return &Layout{ext: ext}
}

// EpisodePath builds the full output path for one downloaded file.
//
// A serial keeps the nested layout, now with the episode's own name:
//
//	root/<series title>/Season <NN>/S<NN>E<NN> - <episode title>.<ext>
//
// A film is a file, not a series of one:
//
//	root/<film title>.<ext>
//
// It used to be laid out like a serial — a folder per film, "Season 01" inside
// it and a file called S01E01 — which said nothing about what was in it. When
// the service publishes a film as several files (Дюна: "24 fps" and "48 fps"),
// the second and later ones carry their own name in brackets so they cannot
// collide.
func (l *Layout) EpisodePath(root string, series domain.Series, ep domain.Episode) (string, error) {
	fallback := fmt.Sprintf("series_%s", string(series.ID))
	title := fsutil.SanitizeComponent(series.Title, fallback)

	if series.IsMovie {
		name := title
		// The first file is the film itself; the rest are alternatives and say
		// which one they are.
		if ep.Key.Episode > 1 {
			variant := episodeName(ep)
			if variant == "" {
				variant = fmt.Sprintf("версия %d", ep.Key.Episode)
			}
			name = fmt.Sprintf("%s (%s)", title, variant)
		}
		return filepath.Join(root, fsutil.SanitizeComponent(name, fallback)+l.ext), nil
	}

	seasonDir := fmt.Sprintf("Season %02d", ep.Key.Season)
	filename := fmt.Sprintf("S%02dE%02d", ep.Key.Season, ep.Key.Episode)
	if name := episodeName(ep); name != "" {
		filename += " - " + name
	}
	filename = fsutil.SanitizeComponent(filename, fmt.Sprintf("S%02dE%02d", ep.Key.Season, ep.Key.Episode))

	return filepath.Join(root, title, seasonDir, filename+l.ext), nil
}

// maxTitleRunes caps the part of a name that comes from the source. Filesystems
// stop at 255 bytes, and a Cyrillic title spends two of them per character, so a
// long name would be truncated by the OS — or refused outright.
const maxTitleRunes = 80

// episodeName is the episode's own title, or "" when the source has nothing to
// add. A generic "Серия 4" is nothing to add: the file already says E04, and
// repeating it only makes the name longer.
func episodeName(ep domain.Episode) string {
	name := strings.TrimSpace(ep.Title)
	if name == "" || genericEpisodeTitle.MatchString(name) {
		return ""
	}
	if r := []rune(name); len(r) > maxTitleRunes {
		name = strings.TrimSpace(string(r[:maxTitleRunes]))
	}
	return name
}

// genericEpisodeTitle matches the placeholder names the service (and this app)
// fill in when an episode has no title of its own.
var genericEpisodeTitle = regexp.MustCompile(`(?i)^(серия|эпизод|episode|ep\.?)\s*\d+$`)

// EnsureDirs creates all directories in the path up to and including the
// directory containing the file at path. It is idempotent: existing directories
// are not an error. Returns domain.ErrOutputDirUnwritable if the directories
// cannot be created.
func (l *Layout) EnsureDirs(path string) error {
	dir := filepath.Dir(path)
	if err := fsutil.EnsureDir(dir); err != nil {
		return fmt.Errorf("%w: %s", domain.ErrOutputDirUnwritable, err.Error())
	}
	return nil
}
