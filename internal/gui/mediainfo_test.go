package gui

import "testing"

func TestParseFFprobe(t *testing.T) {
	// Дорожка в mkv обычно без битрейта — берётся общий по файлу.
	const out = `{
	  "streams": [{"width":3840,"height":2160,"codec_name":"hevc","bit_rate":"N/A","avg_frame_rate":"24000/1001"}],
	  "format": {"bit_rate":"26836000"}
	}`
	got := parseFFprobe([]byte(out))
	want := mediaInfo{Resolution: "3840x2160", Codec: "HEVC", BitrateKbps: 26836, FPS: 24}
	if got != want {
		t.Fatalf("parseFFprobe = %+v, want %+v", got, want)
	}
}

// Реальный ответ ffprobe на mkv: поля bit_rate у дорожки нет вовсе.
func TestParseFFprobe_NoStreamBitrate(t *testing.T) {
	const out = `{"programs":[],"streams":[{"codec_name":"h264","width":1280,"height":720,"avg_frame_rate":"24000/1001"}],"format":{"bit_rate":"105510"}}`
	got := parseFFprobe([]byte(out))
	want := mediaInfo{Resolution: "1280x720", Codec: "H.264", BitrateKbps: 105, FPS: 24}
	if got != want {
		t.Fatalf("parseFFprobe = %+v, want %+v", got, want)
	}
}

func TestParseFrameRate(t *testing.T) {
	cases := map[string]float64{
		"24000/1001": 24,
		"30000/1001": 30,
		"60000/1001": 60,
		"25/1":       25,
		"0/0":        0,
		"":           0,
		"N/A":        0,
	}
	for in, want := range cases {
		if got := parseFrameRate(in); got != want {
			t.Errorf("parseFrameRate(%q) = %v, want %v", in, got, want)
		}
	}
}
