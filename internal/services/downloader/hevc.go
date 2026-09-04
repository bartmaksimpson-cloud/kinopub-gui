package downloader

import (
	"os/exec"
	"strings"
	"sync"
)

// hevcSourceCodecs marks a source that already carries HEVC video, so asking for
// HEVC needs no encoder at all. Checked explicitly rather than as "not H.264":
// an unknown codec must fall through to converting, never skip it silently.
func isHEVCSource(codec string) bool {
	c := strings.ToLower(codec)
	return strings.Contains(c, "265") || strings.Contains(c, "hev") || strings.Contains(c, "hvc")
}

// hardwareHEVCEncoders are tried in order of how much work they take off the CPU.
// Software x265 closes the list: correct everywhere, and slow enough on 4K that
// it is a last resort rather than a default.
var hardwareHEVCEncoders = []struct {
	name string
	args []string
}{
	{"hevc_videotoolbox", []string{"-q:v", "60"}},          // Apple silicon and Intel Macs
	{"hevc_nvenc", []string{"-preset", "p5", "-cq", "28"}}, // NVIDIA
	{"hevc_qsv", []string{"-global_quality", "28"}},        // Intel Quick Sync
	{"hevc_amf", []string{"-quality", "balanced"}},         // AMD
}

var (
	encoderOnce sync.Once
	encoderArgs []string
)

// hevcEncoderArgs returns ffmpeg arguments that re-encode video to HEVC and
// leave everything else alone. They are appended after the "-c copy" the muxer
// emits, and ffmpeg honours the last option for a stream type, so audio tracks
// and subtitles are still copied verbatim.
//
// The encoder is picked from what this ffmpeg build actually offers rather than
// from the operating system: a Windows box may have NVIDIA, Intel or AMD, and
// guessing wrong means either a failed run or hours of software encoding.
func hevcEncoderArgs(ffmpegPath string) []string {
	encoderOnce.Do(func() {
		available := listEncoders(ffmpegPath)
		for _, e := range hardwareHEVCEncoders {
			if available[e.name] {
				encoderArgs = append([]string{"-c:v", e.name}, e.args...)
				encoderArgs = append(encoderArgs, "-tag:v", "hvc1")
				return
			}
		}
		encoderArgs = []string{"-c:v", "libx265", "-crf", "22", "-preset", "medium", "-tag:v", "hvc1"}
	})
	return encoderArgs
}

// listEncoders asks ffmpeg which encoders it was built with. A name present here
// is not proof the hardware works, but its absence is proof it does not — enough
// to avoid choosing an encoder that cannot start.
func listEncoders(ffmpegPath string) map[string]bool {
	out := map[string]bool{}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	b, err := exec.Command(ffmpegPath, "-hide_banner", "-encoders").Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		for _, e := range hardwareHEVCEncoders {
			if strings.Contains(line, e.name) {
				out[e.name] = true
			}
		}
	}
	return out
}
