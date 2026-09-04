package hlsdownloader

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The parser handled TYPE=AUDIO and silently dropped everything else, which is
// why subtitles never reached a download although the muxer was ready for them.
func TestParseMasterPlaylist_ReadsSubtitleRenditions(t *testing.T) {
	master := strings.Join([]string{
		"#EXTM3U",
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a1",NAME="Русский",LANGUAGE="rus",URI="audio/rus.m3u8"`,
		`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="s1",NAME="Русские",LANGUAGE="rus",URI="subs/rus.m3u8"`,
		`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="s1",NAME="Forced",LANGUAGE="eng",URI="subs/eng.m3u8"`,
		`#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080,CODECS="avc1.640028"`,
		"video/1080.m3u8",
	}, "\n")

	got, err := parseMasterPlaylist(strings.NewReader(master), "https://cdn.example/base/x.m3u8")
	if err != nil {
		t.Fatalf("parseMasterPlaylist: %v", err)
	}
	if len(got.Audio) != 1 {
		t.Errorf("аудиодорожек %d, ожидалась 1 — разбор звука не должен был пострадать", len(got.Audio))
	}
	if len(got.Subtitles) != 2 {
		t.Fatalf("субтитровых дорожек %d, ожидалось 2", len(got.Subtitles))
	}
	first := got.Subtitles[0]
	if first.Language != "rus" || first.Name != "Русские" || first.GroupID != "s1" {
		t.Errorf("первая дорожка разобрана неверно: %+v", first)
	}
	// Relative URIs must be resolved against the master, or nothing can fetch them.
	if !strings.HasSuffix(first.URI, "/base/subs/rus.m3u8") {
		t.Errorf("URI не приведён к абсолютному: %q", first.URI)
	}
}

// A declaration without a URI names a track the server does not serve; keeping
// it would create a phantom input that fails the whole mux.
func TestParseMasterPlaylist_SkipsSubtitlesWithoutURI(t *testing.T) {
	master := strings.Join([]string{
		"#EXTM3U",
		`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="s1",NAME="Нет ссылки",LANGUAGE="rus"`,
		"#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=1x1",
		"v.m3u8",
	}, "\n")

	got, err := parseMasterPlaylist(strings.NewReader(master), "https://cdn.example/x.m3u8")
	if err != nil {
		t.Fatalf("parseMasterPlaylist: %v", err)
	}
	if len(got.Subtitles) != 0 {
		t.Errorf("дорожка без URI попала в список: %+v", got.Subtitles)
	}
}

// A mixed-codec master lists the same subtitle track once per video group, and
// a variant that names a group must not drag in another group's tracks.
func TestSubtitleRenditionsFor_FiltersGroupAndDeduplicates(t *testing.T) {
	master := &MasterPlaylist{Subtitles: []SubtitleRendition{
		{GroupID: "s1", Name: "Русские", Language: "rus", URI: "a.m3u8"},
		{GroupID: "s1", Name: "Русские", Language: "rus", URI: "a-copy.m3u8"},
		{GroupID: "s1", Name: "Forced", Language: "rus", URI: "f.m3u8"},
		{GroupID: "s2", Name: "Чужие", Language: "eng", URI: "o.m3u8"},
	}}

	got := subtitleRenditionsFor(master, Variant{SubtitleGroup: "s1"})
	if len(got) != 2 || got[0].Name != "Русские" || got[1].Name != "Forced" {
		t.Fatalf("группа s1 разобрана неверно: %+v", got)
	}

	// No group on the variant: the master has one set, take it whole.
	all := subtitleRenditionsFor(master, Variant{})
	if len(all) != 3 {
		t.Fatalf("без группы ожидалось 3 дорожки после дедупликации, получено %d", len(all))
	}
}

// Concatenated WebVTT segments each bring their own header; a player stops at
// the second one, so the joined file must keep exactly the first.
func TestJoinWebVTT_KeepsOneHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub.vtt")
	concat := "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n\n" +
		"00:00:01.000 --> 00:00:02.000\nПривет\n\n" +
		"WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:900000,LOCAL:00:00:00.000\n\n" +
		"00:00:11.000 --> 00:00:12.000\nПока\n"
	if err := os.WriteFile(path, []byte(concat), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := joinWebVTT(path); err != nil {
		t.Fatalf("joinWebVTT: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.HasPrefix(s, "WEBVTT\n") {
		t.Fatalf("файл не начинается с заголовка: %q", s[:20])
	}
	if strings.Count(s, "WEBVTT") != 1 {
		t.Errorf("заголовков %d, ожидался 1:\n%s", strings.Count(s, "WEBVTT"), s)
	}
	if strings.Contains(s, "X-TIMESTAMP-MAP") {
		t.Errorf("остался X-TIMESTAMP-MAP:\n%s", s)
	}
	for _, cue := range []string{"00:00:01.000 --> 00:00:02.000", "Привет", "00:00:11.000 --> 00:00:12.000", "Пока"} {
		if !strings.Contains(s, cue) {
			t.Errorf("потеряно %q:\n%s", cue, s)
		}
	}
}

// fMP4 subtitle segments are not text; rewriting them would corrupt the file.
func TestJoinWebVTT_LeavesNonVTTAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub.mp4")
	raw := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p'}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := joinWebVTT(path); err != nil {
		t.Fatalf("joinWebVTT: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, raw) {
		t.Errorf("двоичный файл переписан: %v", got)
	}
}

// An empty file would fail the whole mux; the caller drops the track instead.
func TestJoinWebVTT_RejectsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub.vtt")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := joinWebVTT(path); err == nil {
		t.Error("пустой файл принят как годная дорожка")
	}
}
