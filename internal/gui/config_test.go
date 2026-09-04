package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

func TestSplitShellArgs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"-c copy", []string{"-c", "copy"}},
		{"  -c   copy  ", []string{"-c", "copy"}},
		{`-metadata title="My Movie"`, []string{"-metadata", "title=My Movie"}},
		{`-x 'single quoted arg'`, []string{"-x", "single quoted arg"}},
		{`a"b"c`, []string{"abc"}},                       // quotes are removed, parts joined
		{"\t-a\tb", []string{"-a", "b"}},                 // tabs are separators
		{`-f "a b" 'c d'`, []string{"-f", "a b", "c d"}}, // mixed quoting
		{`"unterminated`, []string{"unterminated"}},      // unterminated quote → still flushed
	}
	for _, c := range cases {
		got := splitShellArgs(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitShellArgs(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestDefaultSettings(t *testing.T) {
	s := defaultSettings()
	if s.Quality != "1080p" || s.Container != "mkv" {
		t.Errorf("unexpected defaults: %+v", s)
	}
	if s.Verbosity != "normal" || s.Theme != "cinematic" {
		t.Errorf("unexpected defaults: %+v", s)
	}
}

func TestSettingsSaveClamps(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store := newSettingsStore()

	cases := []struct {
		name string
		in   Settings
		want string // expected container
	}{
		{"bad container → mkv", Settings{Container: "avi"}, "mkv"},
		{"empty container → mkv", Settings{}, "mkv"},
		{"mp4 preserved", Settings{Container: "mp4"}, "mp4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := store.save(c.in)
			if err != nil {
				t.Fatalf("save: %v", err)
			}
			if got.Container != c.want {
				t.Errorf("Container = %q, want %q", got.Container, c.want)
			}
		})
	}
}

func TestSettingsSavePersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	store := newSettingsStore()

	in := Settings{
		OutputPath:  "/tmp/out",
		Quality:     "720p",
		Container:   "mp4",
		Proxy:       "socks5://localhost:1080",
		Verbosity:   "verbose",
		Theme:       "dark",
		LibraryDirs: []string{"/a", "/b"},
	}
	if _, err := store.save(in); err != nil {
		t.Fatalf("save: %v", err)
	}

	// A fresh store loads the persisted file (merged over defaults).
	reloaded := newSettingsStore()
	got := reloaded.get()
	if got.OutputPath != "/tmp/out" || got.Quality != "720p" || got.Container != "mp4" {
		t.Errorf("reloaded mismatch: %+v", got)
	}
	if got.Proxy != "socks5://localhost:1080" || got.Verbosity != "verbose" {
		t.Errorf("reloaded mismatch: %+v", got)
	}
	if !reflect.DeepEqual(got.LibraryDirs, []string{"/a", "/b"}) {
		t.Errorf("reloaded libraryDirs mismatch: %+v", got)
	}
}

func TestSettingsLoadMergesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Write a partial settings file with only one field set; the rest must default.
	cfgDir := filepath.Join(dir, "kinopub")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// "concurrency"/"retries"/"maxActiveJobs" are leftovers from when these were
	// user-facing knobs; a file written by an older build must still load, with
	// the retired keys simply dropped.
	partial := map[string]any{"quality": "480p", "concurrency": 8, "retries": 2, "maxActiveJobs": 100}
	data, _ := json.Marshal(partial)
	if err := os.WriteFile(filepath.Join(cfgDir, "gui.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	store := newSettingsStore()
	got := store.get()
	if got.Quality != "480p" {
		t.Errorf("Quality = %q, want 480p", got.Quality)
	}
	// Defaulted fields.
	if got.Container != "mkv" || got.Verbosity != "normal" || got.Theme != "cinematic" {
		t.Errorf("defaults not merged: %+v", got)
	}
}

func TestSettingsLoadIgnoresBadJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgDir := filepath.Join(dir, "kinopub")
	_ = os.MkdirAll(cfgDir, 0o755)
	_ = os.WriteFile(filepath.Join(cfgDir, "gui.json"), []byte("{not json"), 0o644)

	store := newSettingsStore()
	got := store.get()
	// Falls back to defaults rather than crashing.
	if got.Quality != "1080p" || got.Container != "mkv" {
		t.Errorf("bad JSON should yield defaults, got %+v", got)
	}
}

// The download tuning that used to be user-editable is now fixed, so every run
// gets it regardless of what the UI sends. Pin it here: a regression would
// silently change how hard every download hits the CDN.
func TestBuildRunConfig_FixedTuning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := buildRunConfig(RunRequest{URL: "https://kino.watch/item/view/1"})
	if err != nil {
		t.Fatalf("buildRunConfig: %v", err)
	}
	if cfg.MaxConcurrency != episodeConcurrency {
		t.Errorf("MaxConcurrency = %d, want %d", cfg.MaxConcurrency, episodeConcurrency)
	}
	if cfg.MaxRetries != episodeRetries {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, episodeRetries)
	}
	if cfg.MinIntervalMS != 0 {
		t.Errorf("MinIntervalMS = %d, want 0 (no artificial throttle)", cfg.MinIntervalMS)
	}
}

func TestBuildRunConfig_AudioSpecsSupersede(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := buildRunConfig(RunRequest{
		URL: "https://kino.watch/item/view/1",
		AudioSpecs: []AudioSpecDTO{
			{Require: []string{"LostFilm"}, Forbid: []string{"AC3"}},
			{Require: nil}, // empty Require → skipped
		},
	})
	if err != nil {
		t.Fatalf("buildRunConfig: %v", err)
	}
	if len(cfg.AudioPref.Specs) != 1 {
		t.Fatalf("expected 1 spec (empty-require skipped), got %d", len(cfg.AudioPref.Specs))
	}
	got := cfg.AudioPref.Specs[0]
	if !reflect.DeepEqual(got.Require, []string{"LostFilm"}) || !reflect.DeepEqual(got.Forbid, []string{"AC3"}) {
		t.Errorf("spec = %+v", got)
	}
}

func TestBuildRunConfig_DefaultUserAgentAndTimeout(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := buildRunConfig(RunRequest{URL: "https://kino.watch/item/view/1", UserAgent: "  "})
	if err != nil {
		t.Fatalf("buildRunConfig: %v", err)
	}
	if cfg.UserAgent != defaultUserAgent {
		t.Errorf("blank UA should default, got %q", cfg.UserAgent)
	}
	if cfg.AudioMenuTimeout == 0 {
		t.Error("AudioMenuTimeout should be defaulted to non-zero")
	}
	if !cfg.UseAPI {
		t.Error("UseAPI should be true")
	}
}

func TestBuildRunConfig_VerbosityMapping(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// "quiet" must survive end-to-end: buildRunConfig maps it to VerbosityQuiet
	// and ApplyDefaults preserves it, because VerbosityNormal (not Quiet) is the
	// zero value treated as "unset". Empty/unknown verbosity falls back to normal.
	cases := map[string]domain.Verbosity{
		"quiet":   domain.VerbosityQuiet,
		"verbose": domain.VerbosityVerbose,
		"":        domain.VerbosityNormal,
		"weird":   domain.VerbosityNormal,
	}
	for in, want := range cases {
		cfg, err := buildRunConfig(RunRequest{URL: "https://kino.watch/item/view/1", Verbosity: in})
		if err != nil {
			t.Fatalf("buildRunConfig(%q): %v", in, err)
		}
		if cfg.Verbosity != want {
			t.Errorf("verbosity %q → %v, want %v", in, cfg.Verbosity, want)
		}
	}
}

func TestBuildRunConfig_FFmpegArgsSplit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := buildRunConfig(RunRequest{
		URL:        "https://kino.watch/item/view/1",
		FFmpegArgs: `-threads 2`,
	})
	if err != nil {
		t.Fatalf("buildRunConfig: %v", err)
	}
	if !reflect.DeepEqual(cfg.FFmpegExtraArgs, []string{"-threads", "2"}) {
		t.Errorf("FFmpegExtraArgs = %v", cfg.FFmpegExtraArgs)
	}
}

// Settings used to be merged field by field, and only the strings were copied:
// every bool and number in the file — transcodeHevc, and now maxHeight — was
// silently reset to its default on restart.
// Авторежим: ограничение работает без единой настройки со стороны пользователя.
func TestDefaultSettings_LimitsHeightForHardwareDecoders(t *testing.T) {
	if got := defaultSettings().MaxHeight; got != 2160 {
		t.Errorf("по умолчанию maxHeight = %d, ожидалось 2160", got)
	}
}

func TestSettingsLoad_KeepsNonStringFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gui.json")
	body := `{"outputPath":"/tmp/out","transcodeHevc":true,"maxHeight":2160,"quality":""}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &settingsStore{cur: defaultSettings(), path: path}
	s.load()

	got := s.get()
	if !got.TranscodeHEVC {
		t.Error("transcodeHevc не пережил перезапуск")
	}
	if got.MaxHeight != 2160 {
		t.Errorf("maxHeight = %d, ожидалось 2160", got.MaxHeight)
	}
	if got.OutputPath != "/tmp/out" {
		t.Errorf("outputPath = %q", got.OutputPath)
	}
	// A field written empty must fall back to the default, not stay empty.
	if got.Quality != defaultSettings().Quality {
		t.Errorf("пустое quality затёрло значение по умолчанию: %q", got.Quality)
	}
}

// An impossible cap would scale every download to nothing.
func TestSettingsSave_ClampsMaxHeight(t *testing.T) {
	s := &settingsStore{cur: defaultSettings()}
	for _, bad := range []int{-1, 10000} {
		saved, err := s.save(Settings{Container: "mkv", MaxHeight: bad})
		if err != nil {
			t.Fatal(err)
		}
		if saved.MaxHeight != 0 {
			t.Errorf("maxHeight %d не сброшен, сохранено %d", bad, saved.MaxHeight)
		}
	}
}

// Авторежим: частота кадров для 4K тоже ограничена без настройки руками.
func TestDefaultSettings_LimitsFrameRate(t *testing.T) {
	if got := defaultSettings().MaxFPS; got != 30 {
		t.Errorf("по умолчанию maxFps = %v, ожидалось 30", got)
	}
}
