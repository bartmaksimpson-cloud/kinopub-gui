// Package downloader implements domain.Downloader — it orchestrates ffmpeg
// invocations to download and mux media for a single episode (Req 7, 8, 9).
// For progressive MP4 sources, it supports a chunked HTTP download mode with
// resume capability, falling back to direct ffmpeg streaming on failure.
package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
	"github.com/ZioSHik/kinopub-gui/internal/lib/fsutil"
)

// Compile-time interface assertions.
var (
	_ domain.Downloader  = (*Downloader)(nil)
	_ domain.JobExecutor = (*Downloader)(nil)
	_ domain.HLSMuxer    = (*Downloader)(nil)
)

// RunFunc is a function that runs a command, streaming stdout to the provided
// writer. It blocks until the command completes. The writer receives the
// command's stdout (used for -progress pipe:1 output).
type RunFunc func(ctx context.Context, name string, args, env []string, stdout io.Writer) error

// DownloadMode indicates which download strategy was used.
type DownloadMode string

const (
	ModeChunked DownloadMode = "chunked" // HTTP Range-based with resume
	ModeDirect  DownloadMode = "direct"  // ffmpeg stream copy from URL
)

// Downloader implements domain.Downloader and domain.JobExecutor.
type Downloader struct {
	run           RunFunc
	proxy         domain.ProxyProvider
	logger        domain.Logger
	ffmpegPath    string
	auth          domain.RequestAuth
	extraArgs     []string
	transcodeHEVC bool
	maxHeight     int
	maxFPS        float64
	workDir       string
	outputRoot    string
	noChunked     bool
	httpClient    *http.Client
}

// Option configures the Downloader.
type Option func(*Downloader)

// WithTranscodeHEVC re-encodes sources that are not already HEVC. A source that
// carries it is left alone whatever this says: converting HEVC to HEVC would
// cost hours and lose quality for nothing.
func WithTranscodeHEVC(v bool) Option {
	return func(d *Downloader) {
		d.transcodeHEVC = v
	}
}

// WithMaxHeight caps the frame height of finished files: anything taller is
// scaled down to it (aspect kept). 0 leaves every frame as it came. See
// domain.RunConfig.MaxHeight for why a height limit and not a "4K" switch.
func WithMaxHeight(n int) Option {
	return func(d *Downloader) {
		if n > 0 {
			d.maxHeight = n
		}
	}
}

// WithWorkDir puts the intermediate files — the raw download, the segments, the
// muxer's .tmp — in their own folder instead of next to the finished file. On a
// spinning disk that is the difference between one head seeking between two
// files and two drives each doing one sequential stream. Empty keeps the old
// behaviour.
func WithWorkDir(dir string) Option {
	return func(d *Downloader) {
		d.workDir = dir
	}
}

// WithOutputRoot tells the downloader which folder the output paths are built
// under, so the work folder can mirror that structure instead of flattening
// every episode of every title into one pile.
func WithOutputRoot(root string) Option {
	return func(d *Downloader) {
		d.outputRoot = root
	}
}

// workPath is where an intermediate file for this job goes. The directory is
// created on the way: a work folder the user typed once may not exist yet, and
// neither do the series/season folders mirrored inside it.
func (d *Downloader) workPath(outPath, suffix string) string {
	p := domain.WorkPathFor(d.workDir, d.outputRoot, outPath) + suffix
	if d.workDir != "" {
		if err := fsutil.EnsureDir(filepath.Dir(p)); err != nil {
			// Unusable work folder must not fail the download: fall back to the
			// output folder, which is known to work — it is where the file goes.
			d.logger.Warn("work folder unusable, keeping temp files next to the output",
				domain.F("dir", d.workDir),
				domain.F("error", err.Error()),
			)
			return outPath + suffix
		}
	}
	return p
}

// WithMaxFPS caps the frame rate of finished files whose frame is 4K-class:
// above the cap the rate is halved until it fits (48→24, 60→30), which keeps
// the original cadence. 0 leaves the frame rate alone.
func WithMaxFPS(f float64) Option {
	return func(d *Downloader) {
		if f > 0 {
			d.maxFPS = f
		}
	}
}

// fitArgs are the arguments that bring one source inside what a player can
// decode, or nil when it already fits. resolution and fps come from the master
// playlist; srcKbps is the bitrate to carry over, 0 when unknown.
func (d *Downloader) fitArgs(resolution string, fps float64, srcKbps int, codec string) []string {
	w, h := sizeOf(resolution)
	return fitArgsFor(
		fitSource{Width: w, Height: h, FPS: fps, Kbps: srcKbps, Codec: codec},
		fitLimits{Height: d.maxHeight, FPS: d.maxFPS},
		d.ffmpegPath,
	)
}

// effectiveArgs is the ffmpeg tail for one job: the encoder preset first (only
// when this job actually needs converting), then the user's manual arguments so
// they can still override it.
func (d *Downloader) effectiveArgs(job domain.Job) []string {
	var args []string
	if fit := d.fitArgs(job.Media.Video.Resolution, 0, job.Media.Video.BitRate, job.Media.Source.Codec); len(fit) > 0 {
		// Scaling re-encodes anyway, and it encodes to HEVC — adding the HEVC
		// preset on top would only repeat the same options.
		args = append(args, fit...)
	} else if d.transcodeHEVC && !isHEVCSource(job.Media.Source.Codec) {
		args = append(args, hevcEncoderArgs(d.ffmpegPath)...)
	}
	return append(args, d.extraArgs...)
}

// WithFFmpegPath sets a custom ffmpeg binary path.
func WithFFmpegPath(path string) Option {
	return func(d *Downloader) {
		d.ffmpegPath = path
	}
}

// WithAuth sets the request authentication (Cookie, User-Agent, extra headers)
// propagated to ffmpeg so its requests pass Cloudflare and kino.watch auth.
func WithAuth(auth domain.RequestAuth) Option {
	return func(d *Downloader) {
		d.auth = auth
	}
}

// WithExtraArgs sets additional ffmpeg arguments injected before the output
// path. This allows users to override encoding settings (e.g. -c:v libx265)
// or add filters on the fly.
func WithExtraArgs(args []string) Option {
	return func(d *Downloader) {
		d.extraArgs = args
	}
}

// WithNoChunked disables the chunked download mode, forcing all downloads
// through ffmpeg directly.
func WithNoChunked(noChunked bool) Option {
	return func(d *Downloader) {
		d.noChunked = noChunked
	}
}

// WithHTTPClient sets the HTTP client used for chunked downloads.
func WithHTTPClient(client *http.Client) Option {
	return func(d *Downloader) {
		d.httpClient = client
	}
}

// New creates a new Downloader.
//   - run: function to execute ffmpeg, streaming stdout to a writer
//   - proxy: provides proxy environment for ffmpeg
//   - logger: structured logger
func New(run RunFunc, proxy domain.ProxyProvider, logger domain.Logger, opts ...Option) *Downloader {
	d := &Downloader{
		run:        run,
		proxy:      proxy,
		logger:     logger.Component("downloader"),
		ffmpegPath: "ffmpeg",
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Download runs the download for one episode. For progressive MP4 sources
// (when chunked mode is enabled), it first downloads the raw file via HTTP
// Range requests with resume capability, then remuxes with ffmpeg to add
// metadata and labels. For HLS sources or when chunked is disabled, it uses
// the traditional ffmpeg-based streaming approach.
func (d *Downloader) Download(ctx context.Context, job domain.Job, sink domain.ProgressSink) error {
	// Determine if we can use chunked mode.
	// Determine if we can use chunked mode.
	// Skip chunked for local files (no http:// prefix) — they're already on disk.
	isRemoteURL := strings.HasPrefix(job.Media.Source.URL, "http://") ||
		strings.HasPrefix(job.Media.Source.URL, "https://")

	useChunked := !d.noChunked &&
		d.httpClient != nil &&
		job.Media.Source.Kind == domain.MediaProgressive &&
		isRemoteURL &&
		len(d.effectiveArgs(job)) == 0 // transcoding rules out chunked: it needs one continuous ffmpeg pass

	if useChunked {
		err := d.downloadChunked(ctx, job, sink)
		if err == nil {
			return nil
		}
		// Chunked failed — fall back to direct ffmpeg.
		d.logger.Warn("chunked download failed, falling back to direct ffmpeg",
			domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
			domain.F("error", err.Error()),
		)
	}

	return d.downloadDirect(ctx, job, sink)
}

// downloadChunked implements the chunked HTTP Range download + ffmpeg remux.
func (d *Downloader) downloadChunked(ctx context.Context, job domain.Job, sink domain.ProgressSink) error {
	d.logger.Info("starting chunked download",
		domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
		domain.F("mode", string(ModeChunked)),
		domain.F("output", job.OutPath),
	)

	// 1. Download raw file via chunked HTTP.
	rawPath := d.workPath(job.OutPath, ".raw")
	chunked := NewChunked(d.httpClient, d.auth, d.logger)

	if err := chunked.Download(ctx, job.Media.Source.URL, rawPath, job.Episode.Key, sink); err != nil {
		return fmt.Errorf("chunked download: %w", err)
	}

	// 2. Remux with ffmpeg: local file → final container with metadata.
	d.logger.Info("remuxing downloaded file",
		domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
	)

	if err := d.remuxLocal(ctx, job, rawPath); err != nil {
		// Clean up raw file on remux failure.
		os.Remove(rawPath)
		return fmt.Errorf("remux: %w", err)
	}

	// 3. Clean up raw file after successful remux.
	os.Remove(rawPath)

	d.logger.Info("download completed",
		domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
		domain.F("mode", string(ModeChunked)),
		domain.F("output", job.OutPath),
	)

	return nil
}

// remuxLocal runs ffmpeg to remux a local raw file into the final container
// with all metadata, poster, and audio/subtitle labels.
func (d *Downloader) remuxLocal(ctx context.Context, job domain.Job, rawPath string) error {
	return d.RemuxLocal(ctx, job, rawPath)
}

// MuxHLS combines a downloaded HLS video file with separate audio tracks into
// the final container at job.OutPath. For demuxed HLS, video and audio come
// from separate files; this maps them all together with -c copy and applies
// per-track labels/languages.
func (d *Downloader) MuxHLS(ctx context.Context, job domain.Job, hls *domain.HLSDownloadResult) error {
	return d.MuxHLSProgress(ctx, job, hls, nil)
}

// MuxHLSProgress is MuxHLS with progress reporting, used when the mux is not a
// plain copy: scaling a too-tall frame re-encodes the whole episode, and that
// takes long enough that a silent job reads as a hang.
func (d *Downloader) MuxHLSProgress(ctx context.Context, job domain.Job, hls *domain.HLSDownloadResult, sink domain.ProgressSink) error {
	d.logger.Info("muxing HLS streams",
		domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
		domain.F("audio_tracks", len(hls.AudioTracks)),
		domain.F("subtitle_tracks", len(hls.Subtitles)),
		domain.F("output", job.OutPath),
	)

	tempPath := d.workPath(job.OutPath, ".tmp")
	// The master playlist already told us the frame size, so a source over the
	// height cap is scaled here, in the pass that runs anyway.
	fit := d.fitArgs(hls.Resolution, hls.FrameRate, hls.BitrateKbps, hls.Codec)
	if len(fit) > 0 {
		d.logger.Info("source beyond what a player decodes, fitting while muxing",
			domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
			domain.F("source", hls.Resolution),
			domain.F("source_fps", hls.FrameRate),
			domain.F("max_height", d.maxHeight),
			domain.F("max_fps", d.maxFPS),
			// The bitrate is carried over rather than re-guessed: fewer pixels in
			// a better codec on the same budget is what keeps the picture.
			domain.F("bitrate_kbps", hls.BitrateKbps),
		)
	}
	// Say what is happening now: a copy takes seconds, a fit re-encodes the whole
	// episode, and the two look identical from outside without this.
	threads := 0
	if len(fit) > 0 {
		threads = encodeThreads(time.Now())
	}
	if stager, ok := sink.(domain.EpisodeStageSink); ok {
		stage := domain.EpisodeStage{Phase: "mux"}
		if len(fit) > 0 {
			stage.Phase = "encode"
			stage.Format = describeFit(fit, hls)
			stage.Encoder = encoderLabel(fit)
			stage.Threads = threads
		}
		stager.EpisodeStage(job.Episode.Key, stage)
	}

	// Progress is only wired for the re-encoding case: a stream copy finishes
	// before the first report would arrive.
	var (
		stdout io.Writer
		parser *progressParser
	)
	if len(fit) > 0 && sink != nil {
		if dur := muxDuration(job); dur > 0 {
			parser = newProgressParser(sink, job.Episode.Key, domain.TrackRef{Kind: domain.TrackVideo, Index: 0}, dur)
			stdout = parser
		}
	}

	// Attempts from best to merely acceptable. Each drops the one thing that can
	// make ffmpeg refuse the job, and each is still a usable file:
	// a subtitle track it will not read, or the scaling pass — whose encoder may
	// be missing, broken or refused by the driver on this machine (nvenc has
	// been seen failing to open at all). Losing the fit means the file plays
	// badly on a TV; losing the DOWNLOAD means gigabytes fetched again.
	type attempt struct {
		why  string
		hls  *domain.HLSDownloadResult
		fit  []string
		pipe bool
	}
	withoutSubs := *hls
	withoutSubs.Subtitles = nil

	attempts := []attempt{{why: "", hls: hls, fit: fit, pipe: true}}
	if len(hls.Subtitles) > 0 {
		attempts = append(attempts, attempt{why: "без субтитров", hls: &withoutSubs, fit: fit, pipe: true})
	}
	if len(fit) > 0 {
		attempts = append(attempts, attempt{why: "без подгонки под плеер", hls: hls, fit: nil})
		if len(hls.Subtitles) > 0 {
			attempts = append(attempts, attempt{why: "без подгонки и без субтитров", hls: &withoutSubs, fit: nil})
		}
	}

	var runErr error
	for i, a := range attempts {
		if i > 0 {
			if ctx.Err() != nil {
				break
			}
			d.logger.Warn("mux failed, retrying with less",
				domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
				domain.F("retry", a.why),
				domain.F("error", runErr.Error()),
			)
			os.Remove(tempPath)
		}

		args := a.fit
		if a.pipe && parser != nil {
			args = append(append([]string{}, args...), "-progress", "pipe:1")
		}
		full := withDecodeThreads(BuildHLSMuxArgs(job, a.hls, tempPath, args...), threads)

		var out io.Writer
		if a.pipe {
			out = stdout
		}
		runErr = d.run(ctx, d.ffmpegPath, full, nil, out)
		if runErr == nil {
			if i > 0 {
				d.logger.Warn("episode muxed with a fallback",
					domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
					domain.F("gave_up", a.why),
				)
			}
			break
		}
		// The arguments are the first thing anyone needs when ffmpeg refuses a
		// job on someone else's machine, and they carry no secrets here: every
		// input is a local file.
		d.logger.Debug("ffmpeg mux failed",
			domain.F("args", strings.Join(full, " ")),
			domain.F("error", runErr.Error()),
		)
	}
	if parser != nil {
		_ = parser.Close()
	}
	if runErr != nil {
		os.Remove(tempPath)
		return fmt.Errorf("%w: %v", domain.ErrFFmpegFailed, runErr)
	}

	info, err := os.Stat(tempPath)
	if err != nil || info.Size() == 0 {
		os.Remove(tempPath)
		return domain.ErrEmptyOutput
	}

	// A stream copy must come out about the size of what went in. When it does
	// not, ffmpeg stopped early — a bad timestamp, a truncated input — and
	// exited 0 anyway, which used to be reported as a finished episode: sixteen
	// gigabytes downloaded, one and a half in the library, job green. Better to
	// fail loudly and keep the segments for a retry than to call that success.
	// Re-encoding legitimately changes the size, so the check applies only to
	// the copy path.
	if len(fit) == 0 && hls.TotalBytes > 0 {
		if min := hls.TotalBytes / 2; info.Size() < min {
			d.logger.Error("muxed file is far smaller than what was downloaded",
				domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
				domain.F("downloaded", formatBytes(hls.TotalBytes)),
				domain.F("muxed", formatBytes(info.Size())),
			)
			os.Remove(tempPath)
			return fmt.Errorf("%w: склейка дала %s из скачанных %s — ffmpeg остановился раньше времени",
				domain.ErrFFmpegFailed, formatBytes(info.Size()), formatBytes(hls.TotalBytes))
		}
	}

	// Across disks the move is a full copy — minutes for a 4K episode — so it
	// gets its own stage rather than looking like the job stalled at the finish.
	// Within one filesystem it is a rename and the label is gone instantly.
	if stager, ok := sink.(domain.EpisodeStageSink); ok && d.workDir != "" {
		stager.EpisodeStage(job.Episode.Key, domain.EpisodeStage{
			Phase:  "move",
			Format: formatBytes(info.Size()),
		})
	}
	if err := moveFile(tempPath, job.OutPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("rename temp to final: %w", err)
	}

	return nil
}

// RemuxLocal remuxes a local media file (e.g. a concatenated HLS .ts) into the
// final container at job.OutPath. It copies ALL streams (video + every audio
// track + subtitles) using -map 0, applies container metadata and poster, and
// does NOT inject any HTTP auth options (the input is a local file).
func (d *Downloader) RemuxLocal(ctx context.Context, job domain.Job, localPath string) error {
	d.logger.Info("remuxing local file",
		domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
		domain.F("input", localPath),
		domain.F("output", job.OutPath),
	)

	tempPath := d.workPath(job.OutPath, ".tmp")
	args := BuildRemuxArgs(job, localPath, tempPath)

	// Run ffmpeg (no proxy env, no auth — local file).
	runErr := d.run(ctx, d.ffmpegPath, args, nil, nil)
	if runErr != nil {
		os.Remove(tempPath)
		return fmt.Errorf("%w: %v", domain.ErrFFmpegFailed, runErr)
	}

	info, err := os.Stat(tempPath)
	if err != nil || info.Size() == 0 {
		os.Remove(tempPath)
		return domain.ErrEmptyOutput
	}

	if err := moveFile(tempPath, job.OutPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("rename temp to final: %w", err)
	}

	return nil
}

// downloadDirect is the traditional ffmpeg-based download (stream from URL).
func (d *Downloader) downloadDirect(ctx context.Context, job domain.Job, sink domain.ProgressSink) error {
	d.logger.Info("starting direct download",
		domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
		domain.F("mode", string(ModeDirect)),
		domain.F("output", job.OutPath),
	)

	// 1. Get proxy env for ffmpeg.
	proxyEnv, err := d.proxy.FFmpegEnv()
	if err != nil {
		return fmt.Errorf("proxy env: %w", err)
	}

	// 2. Compute temp path.
	tempPath := d.workPath(job.OutPath, ".tmp")

	// 3. Build ffmpeg args.
	args := BuildFFmpegArgs(job, proxyEnv, d.auth, tempPath, d.effectiveArgs(job))

	// 4. Set up progress parsing.
	duration := estimateDuration(job)

	var stdout io.Writer
	var parser *progressParser

	if sink != nil && duration > 0 {
		track := domain.TrackRef{Kind: domain.TrackVideo, Index: 0}
		parser = newProgressParser(sink, job.Episode.Key, track, duration)
		stdout = parser
	}

	// 5. Run ffmpeg.
	d.logger.Debug("running ffmpeg",
		domain.F("args_count", len(args)),
		domain.F("proxy_env_count", len(proxyEnv)),
	)

	runErr := d.run(ctx, d.ffmpegPath, args, proxyEnv, stdout)

	// Close the progress parser to flush remaining data.
	if parser != nil {
		_ = parser.Close()
	}

	// 6. Handle failure.
	if runErr != nil {
		d.logger.Error("ffmpeg failed",
			domain.F("error", runErr.Error()),
			domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
		)
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: %v", domain.ErrFFmpegFailed, runErr)
	}

	// 7. Verify temp file exists and size > 0.
	info, err := os.Stat(tempPath)
	if err != nil || info.Size() == 0 {
		d.logger.Error("output file missing or empty",
			domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
			domain.F("temp_path", tempPath),
		)
		_ = os.Remove(tempPath)
		return domain.ErrEmptyOutput
	}

	// 7b. Verify duration: if we know the expected duration, check that the
	// downloaded file is at least 85% of it.
	if parser != nil && duration > 0 {
		lastPct := parser.lastPercent()
		if lastPct < 85 {
			d.logger.Error("download appears truncated",
				domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
				domain.F("last_progress_percent", lastPct),
				domain.F("expected_duration", duration.String()),
				domain.F("file_size", info.Size()),
			)
			_ = os.Remove(tempPath)
			return fmt.Errorf("%w: download truncated at %d%% (CDN may have dropped the connection)", domain.ErrFFmpegFailed, lastPct)
		}
	}

	// 8. Atomic rename to final path.
	if err := moveFile(tempPath, job.OutPath); err != nil {
		d.logger.Error("rename failed",
			domain.F("error", err.Error()),
			domain.F("from", tempPath),
			domain.F("to", job.OutPath),
		)
		_ = os.Remove(tempPath)
		return fmt.Errorf("rename temp to final: %w", err)
	}

	d.logger.Info("download completed",
		domain.F("episode", fmt.Sprintf("S%02dE%02d", job.Episode.Key.Season, job.Episode.Key.Episode)),
		domain.F("mode", string(ModeDirect)),
		domain.F("output", job.OutPath),
		domain.F("size", info.Size()),
	)

	return nil
}

// Execute implements domain.JobExecutor. It delegates to Download with a no-op
// ProgressSink.
func (d *Downloader) Execute(ctx context.Context, job domain.Job) error {
	return d.Download(ctx, job, nil)
}

// estimateDuration returns the expected duration of the media for progress
// computation. It uses the resolved media's duration field obtained from ffprobe.
// Returns 0 if duration cannot be determined.
func estimateDuration(job domain.Job) time.Duration {
	return job.Media.Duration
}

// describeFit renders what the re-encode produces: the frame it scales to, the
// codec it encodes with, the frame rate when it changes one, and the bitrate it
// carries over. Read off the arguments themselves so the label cannot drift
// from what ffmpeg is actually told to do.
func describeFit(fit []string, hls *domain.HLSDownloadResult) string {
	var parts []string
	height := ""
	for i := 0; i < len(fit)-1; i++ {
		switch fit[i] {
		case "-vf":
			if _, h, ok := strings.Cut(fit[i+1], ":"); ok {
				height = h
			}
		case "-c:v":
			name := fit[i+1]
			switch {
			case strings.HasPrefix(name, "hevc"), name == "libx265":
				parts = append(parts, "HEVC")
			case strings.HasPrefix(name, "h264"), name == "libx264":
				parts = append(parts, "H.264")
			default:
				parts = append(parts, name)
			}
		case "-r":
			parts = append(parts, fit[i+1]+" fps")
		case "-b:v":
			parts = append(parts, strings.TrimSuffix(fit[i+1], "k")+" kbps")
		}
	}
	if height != "" {
		if w, _ := sizeOf(hls.Resolution); w > 0 {
			if _, srcH := sizeOf(hls.Resolution); srcH > 0 {
				if h, err := strconv.Atoi(height); err == nil {
					// Ширина считается по тем же пропорциям, что и в фильтре.
					parts = append([]string{fmt.Sprintf("%dx%s", roundTo16(w*h/srcH), height)}, parts...)
					return strings.Join(parts, " · ")
				}
			}
		}
		parts = append([]string{"↓" + height}, parts...)
	}
	return strings.Join(parts, " · ")
}

// encoderLabel names who does the encoding the way a person would look it up in
// a task manager: the vendor, not the ffmpeg codec name. Empty when the pass is
// a plain copy.
func encoderLabel(fit []string) string {
	for i := 0; i < len(fit)-1; i++ {
		if fit[i] != "-c:v" {
			continue
		}
		switch name := fit[i+1]; {
		case strings.HasSuffix(name, "_nvenc"):
			return "NVIDIA (NVENC)"
		case strings.HasSuffix(name, "_amf"):
			return "AMD (AMF)"
		case strings.HasSuffix(name, "_qsv"):
			return "Intel (Quick Sync)"
		case strings.HasSuffix(name, "_videotoolbox"):
			return "Apple (VideoToolbox)"
		default:
			return "процессор"
		}
	}
	return ""
}

// roundTo16 rounds a computed width to the nearest multiple of 16, matching the
// "-16" the scale filter is given.
func roundTo16(w int) int { return (w + 8) / 16 * 16 }

// muxDuration is how long the episode runs, for percentages during a re-encode.
// The mux job is built from the episode alone (there is no resolved media by
// then), so the episode's own duration is the number that is actually there.
func muxDuration(job domain.Job) time.Duration {
	if job.Media.Duration > 0 {
		return job.Media.Duration
	}
	return job.Episode.Duration
}

// noopSink is a ProgressSink that discards all updates.
type noopSink struct{}

func (noopSink) TrackProgress(_ domain.EpisodeKey, _ domain.TrackRef, _ int) {}
