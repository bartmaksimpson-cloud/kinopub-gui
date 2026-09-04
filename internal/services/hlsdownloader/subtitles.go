package hlsdownloader

import (
	"bytes"
	"fmt"
	"os"
)

// joinWebVTT rewrites the byte-concatenation of WebVTT segments into a single
// valid WebVTT file. Every segment carries its own "WEBVTT" header (and usually
// an X-TIMESTAMP-MAP line); a player that meets the second header stops reading
// there, so all but the first must go. Cue timings are left untouched: VOD
// segmenters write them on the media timeline already.
//
// Non-WebVTT payloads (fMP4 or TTML segments) are left exactly as concatenated —
// they need no stitching.
//
// ponytail: X-TIMESTAMP-MAP offsets are dropped rather than applied, and a cue
// whose text is literally "WEBVTT" or "X-TIMESTAMP-MAP..." is dropped with them.
// If subtitles ever come out shifted by a constant, that is the line to honour.
func joinWebVTT(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("empty subtitle file %s", path)
	}
	if !bytes.HasPrefix(data, []byte("WEBVTT")) {
		return nil
	}

	var out bytes.Buffer
	out.WriteString("WEBVTT\n")
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if bytes.HasPrefix(line, []byte("WEBVTT")) || bytes.HasPrefix(line, []byte("X-TIMESTAMP-MAP")) {
			continue
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}
