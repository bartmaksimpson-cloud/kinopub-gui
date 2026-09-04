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
	return fitArgsFor(fitSource{Height: srcHeight, Kbps: srcKbps}, fitLimits{Height: maxHeight}, ffmpegPath)
}

// fitSource is what the master playlist says about the stream we are about to
// hand to a player: frame size, declared frame rate, bitrate.
type fitSource struct {
	Width  int
	Height int
	FPS    float64
	Kbps   int
	// Codec is what the source carries ("h264", "h265", …). It decides whether a
	// high frame rate is a problem at all: hardware decoders budget AVC and HEVC
	// separately, and the AVC budget is the small one.
	Codec string
}

// fitLimits is what the player can actually take. Height is the decoder's frame
// limit; FPS is its throughput limit at a large frame. 0 disables either.
type fitLimits struct {
	Height int
	FPS    float64
}

// hfrWidth is where the frame-rate limit starts to apply. A decoder that stalls
// on 4K at 48 fps handles 1080p at 60 without noticing, so capping small frames
// would throw away smoothness for nothing.
const hfrWidth = 1920

// fitArgsFor returns the ffmpeg arguments that bring one source inside what the
// player can decode, or nil when it already fits — in which case the file is
// stream-copied and real 4K is kept untouched.
//
// Two independent limits, both measured on a TCL/Realtek TV:
//   - Frame too tall (3840x2880): the decoder refuses the frame outright, falls
//     back to software, and a quarter of the frames are lost.
//   - Frame rate too high (3840x1600 at 48 fps): the decoder accepts it, runs,
//     and still drops two thirds of the frames — it simply cannot keep up at a
//     4K frame. The panel is 60/30 Hz anyway, so 48 fps could never be shown
//     evenly even if it did.
//
// The frame rate is halved rather than set to the limit: 48→24, 50→25, 60→30
// keeps the original cadence exactly, so no frame is ever shown twice or
// interpolated.
func fitArgsFor(src fitSource, lim fitLimits, ffmpegPath string) []string {
	var filters []string
	if lim.Height > 0 && src.Height > lim.Height {
		filters = append(filters, fmt.Sprintf("scale=-16:%d", lim.Height))
	}

	rate := 0.0
	// The width to judge by is the one that comes out, not the one that went in.
	outWidth := src.Width
	if outWidth == 0 {
		outWidth = hfrWidth + 1 // unknown width: a height limit only fires on big frames anyway
	}
	// HEVC is exempt: the same chips that stop at 4Kp30 for H.264 do 4Kp60 and
	// beyond for HEVC (Realtek rtd6748, Amlogic S905X4/X5M — all of them), so
	// halving a genuine 4K60 HEVC stream would throw away smoothness the player
	// can actually show.
	if lim.FPS > 0 && src.FPS > lim.FPS && outWidth > hfrWidth && !isHEVCSource(src.Codec) {
		rate = src.FPS
		for rate > lim.FPS {
			rate /= 2
		}
	}

	if len(filters) == 0 && rate == 0 {
		return nil
	}

	var args []string
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}
	if rate > 0 {
		args = append(args, "-r", strconv.FormatFloat(rate, 'f', -1, 64))
	}
	return append(args, hevcEncoderAt(ffmpegPath, src.Kbps)...)
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

// sizeOf reads width and height out of an "1920x1080" resolution string. Zeroes
// when it is missing or unparseable — the caller then leaves the frame alone
// rather than guessing.
func sizeOf(resolution string) (int, int) {
	wStr, hStr, ok := strings.Cut(resolution, "x")
	if !ok {
		return 0, 0
	}
	w, werr := strconv.Atoi(strings.TrimSpace(wStr))
	h, herr := strconv.Atoi(strings.TrimSpace(hStr))
	if werr != nil || herr != nil || w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}

// heightOf is sizeOf when only the height matters.
func heightOf(resolution string) int {
	_, h := sizeOf(resolution)
	return h
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
