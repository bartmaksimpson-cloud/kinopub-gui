package downloader

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/domain"
)

// muxProgress turns ffmpeg's "-progress" output into stage reports.
//
// A mux is a copy of the whole episode — ten minutes and more for a 4K file on
// a hard drive — and ffmpeg is the only one who knows how far along it is. The
// card used to show the download's leftover estimate all the way through it,
// which is how "2 seconds left" ended up next to a twenty-minute wait.
type muxProgress struct {
	sink     domain.EpisodeStageSink
	key      domain.EpisodeKey
	phase    string
	format   string
	duration time.Duration

	pr   *io.PipeReader
	pw   *io.PipeWriter
	done chan struct{}
}

func newMuxProgress(sink domain.EpisodeStageSink, key domain.EpisodeKey, phase, format string, duration time.Duration) *muxProgress {
	pr, pw := io.Pipe()
	m := &muxProgress{
		sink: sink, key: key, phase: phase, format: format, duration: duration,
		pr: pr, pw: pw, done: make(chan struct{}),
	}
	go m.parse()
	return m
}

func (m *muxProgress) Write(p []byte) (int, error) { return m.pw.Write(p) }

// Close stops parsing and waits for the reader to finish, so no report races
// with the stage that comes after this one.
func (m *muxProgress) Close() error {
	err := m.pw.Close()
	<-m.done
	return err
}

func (m *muxProgress) parse() {
	defer close(m.done)
	var last time.Time
	sc := bufio.NewScanner(m.pr)
	for sc.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || key != "out_time_us" {
			continue
		}
		us, err := strconv.ParseInt(value, 10, 64)
		if err != nil || us < 0 {
			continue
		}
		// Twice a second is enough for a progress line; more is just noise.
		if time.Since(last) < 500*time.Millisecond {
			continue
		}
		last = time.Now()
		m.sink.EpisodeStage(m.key, domain.EpisodeStage{
			Phase:  m.phase,
			Format: m.format,
			Done:   us / 1_000_000,
			Total:  int64(m.duration / time.Second),
		})
	}
}
