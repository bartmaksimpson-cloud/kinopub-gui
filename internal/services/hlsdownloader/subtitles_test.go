package hlsdownloader

import (
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
