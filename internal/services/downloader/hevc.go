package downloader

import (
	"fmt"
	"os/exec"
	"strconv"
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

// scaleToHeightArgs returns the ffmpeg arguments that shrink a frame taller than
// maxHeight down to it. Nil when nothing needs shrinking, so a file that already
// fits is stream-copied untouched — the point is to keep real 4K wherever it
// already plays, not to re-encode everything.
//
// The width follows the aspect ratio and is rounded to a multiple of 16 (that is
// what "-16" means to the scale filter): macroblock-aligned frames are what
// hardware decoders are built for, and the rounding moves the picture by at most
// a fraction of a percent. So 3840x2880 becomes 2880x2160 and 3840x2314 becomes
// 3584x2160 — both inside the 4096x2176 a TV decoder declares.
//
// Scaling always means re-encoding, and it encodes to HEVC: the players this
// exists for decode HEVC in hardware, and the file comes out smaller. srcKbps is
// the source variant's bitrate — carrying it over keeps the picture, because the
// same budget now covers fewer pixels in a more efficient codec. 0 falls back to
// the encoder's quality preset.
func scaleToHeightArgs(srcHeight, maxHeight, srcKbps int, ffmpegPath string) []string {
	if maxHeight <= 0 || srcHeight <= 0 || srcHeight <= maxHeight {
		return nil
	}
	args := []string{"-vf", fmt.Sprintf("scale=-16:%d", maxHeight)}
	return append(args, hevcEncoderAt(ffmpegPath, srcKbps)...)
}

// hevcEncoderAt is the HEVC encoder this machine has, told to hold a bitrate
// instead of a quality level. Every encoder in the list takes -b:v, and a
// number carried over from the source is the safest way to not lose picture.
func hevcEncoderAt(ffmpegPath string, kbps int) []string {
	if kbps <= 0 {
		return hevcEncoderArgs(ffmpegPath)
	}
	// hevcEncoderArgs always starts with "-c:v <name>"; the rest is the quality
	// preset this replaces.
	preset := hevcEncoderArgs(ffmpegPath)
	if len(preset) < 2 {
		return preset
	}
	return []string{"-c:v", preset[1], "-b:v", fmt.Sprintf("%dk", kbps), "-tag:v", "hvc1"}
}

// heightOf reads the height out of an "1920x1080" resolution string. 0 when it
// is missing or unparseable — the caller then leaves the frame alone rather
// than guessing.
func heightOf(resolution string) int {
	_, h, ok := strings.Cut(resolution, "x")
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || n <= 0 {
		return 0
	}
	return n
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
