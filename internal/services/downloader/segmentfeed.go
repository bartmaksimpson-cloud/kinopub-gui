package downloader

import (
	"io"
	"os"
	"sync"
)

// segmentFeed reads downloaded pieces one after another as a single stream, so
// the muxer can be fed the whole episode without it ever existing as one file.
//
// The bytes are exactly what joining the pieces on disk would have produced —
// HLS segments concatenate byte for byte, and the fMP4 init segment simply
// comes first — but nothing is written and nothing is read back. On a 4K
// episode that is the difference between eighteen gigabytes of extra disk
// traffic and none.
type segmentFeed struct {
	parts []string
	idx   int
	cur   *os.File

	mu  sync.Mutex
	err error
}

func newSegmentFeed(parts []string) *segmentFeed {
	return &segmentFeed{parts: parts}
}

// Read implements io.Reader over the pieces in order.
func (f *segmentFeed) Read(p []byte) (int, error) {
	for {
		if f.cur == nil {
			if f.idx >= len(f.parts) {
				return 0, io.EOF
			}
			file, err := os.Open(f.parts[f.idx])
			if err != nil {
				f.setErr(err)
				return 0, err
			}
			f.cur = file
			f.idx++
		}
		n, err := f.cur.Read(p)
		if n > 0 {
			return n, nil
		}
		if err == io.EOF {
			f.cur.Close()
			f.cur = nil
			continue // next piece
		}
		if err != nil {
			f.setErr(err)
			return 0, err
		}
	}
}

// Close releases the piece currently open. Safe to call more than once.
func (f *segmentFeed) Close() error {
	if f.cur != nil {
		err := f.cur.Close()
		f.cur = nil
		return err
	}
	return nil
}

// Err reports the first read failure, if any. A muxer that fails while its
// input was breaking should say which one it was.
func (f *segmentFeed) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *segmentFeed) setErr(err error) {
	f.mu.Lock()
	if f.err == nil {
		f.err = err
	}
	f.mu.Unlock()
}

// joinParts writes the pieces into one file, in order — the old behaviour, kept
// as the fallback for a muxer that refuses a non-seekable input.
func joinParts(parts []string, out string) error {
	dst, err := os.Create(out)
	if err != nil {
		return err
	}
	defer dst.Close()

	feed := newSegmentFeed(parts)
	defer feed.Close()

	buf := make([]byte, 1<<20)
	if _, err := io.CopyBuffer(dst, feed, buf); err != nil {
		return err
	}
	return dst.Close()
}
