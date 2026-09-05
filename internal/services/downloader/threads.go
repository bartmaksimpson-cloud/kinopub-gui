package downloader

import (
	"runtime"
	"strconv"
	"time"
)

// nightFrom and nightTo bound the hours when a re-encode may take the whole
// machine: midnight to nine in the morning. Nobody is typing at four in the
// morning; at four in the afternoon somebody is, and one free core is the
// difference between "slow" and "cannot move the mouse".
const (
	nightFrom = 0
	nightTo   = 9
)

// encodeThreads is how many CPU threads one re-encode may use, decided by the
// clock when it starts: everything at night, everything but one core by day.
//
// It is fixed at the start on purpose. A pass that begins in the evening and
// runs past midnight keeps its limit — changing thread counts mid-encode is not
// something ffmpeg supports, and a job that silently speeds up at midnight is
// harder to reason about than one that keeps its promise.
func encodeThreads(now time.Time) int {
	n := runtime.NumCPU()
	if n < 2 {
		return 1
	}
	if h := now.Hour(); h >= nightFrom && h < nightTo {
		return n
	}
	return n - 1
}

// withDecodeThreads inserts the thread limit where it binds to DECODING: a
// per-stream option placed before the first input applies to that input's
// streams. Decoding a 4K source and scaling it is where the CPU actually goes —
// with a hardware encoder the encoder itself costs almost nothing — so limiting
// only the output side would leave the machine just as busy.
//
// -filter_threads bounds the scaler, which is the other half of that work.
func withDecodeThreads(args []string, threads int) []string {
	if threads <= 0 || len(args) == 0 {
		return args
	}
	out := make([]string, 0, len(args)+4)
	out = append(out, args[0]) // "-y"
	out = append(out, "-threads", strconv.Itoa(threads), "-filter_threads", strconv.Itoa(threads))
	return append(out, args[1:]...)
}
