package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

// modulePrefix marks our own frames in a stack trace, so panicOrigin can skip
// the runtime's and name the line that actually failed.
const modulePrefix = "github.com/ZioSHik/kinopub-gui/"

// panicOrigin returns "pkg/file.go:123" for the frame that actually panicked,
// or "" when the trace holds none. A recovered panic otherwise reaches the
// user as a bare "invalid memory address or nil pointer dereference", which
// says nothing about where it happened — and on Windows the GUI subsystem has
// no console, so nothing else records it either.
//
// Everything above runtime.gopanic is the recovery machinery itself (this
// file, then the deferred closure that called it), so the scan starts below
// gopanic — otherwise every panic would be reported at its recover site.
func panicOrigin(stack []byte) string {
	lines := strings.Split(string(stack), "\n")

	start := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "panic(") || strings.HasPrefix(line, "runtime.gopanic") {
			start = i + 1
			break
		}
	}

	for i := start; i+1 < len(lines); i++ {
		if !strings.Contains(lines[i], modulePrefix) {
			continue
		}
		// The location sits on the line after the function name, indented and
		// followed by the frame's PC offset: "\t/path/file.go:123 +0x1c".
		loc := strings.TrimSpace(lines[i+1])
		if !strings.Contains(loc, ".go:") {
			continue
		}
		loc, _, _ = strings.Cut(loc, " ")
		return filepath.Base(filepath.Dir(loc)) + "/" + filepath.Base(loc)
	}
	return ""
}

// logPanic appends a recovered panic and its stack to crash.log next to the
// settings, and returns a one-line description for the UI. Best effort: a
// panic must not be lost just because the log cannot be written.
func logPanic(where string, r any) string {
	stack := debug.Stack()
	origin := panicOrigin(stack)

	desc := fmt.Sprintf("%v", r)
	if origin != "" {
		desc += " at " + origin
	}

	if dir, err := configDir(); err == nil {
		// The config dir may not exist yet — a crash before the first settings
		// save would otherwise leave nothing behind at all.
		_ = os.MkdirAll(dir, 0o700)
		if f, err := os.OpenFile(filepath.Join(dir, "crash.log"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			fmt.Fprintf(f, "\n=== %s | %s | %v\n%s\n",
				time.Now().Format(time.RFC3339), where, r, stack)
			f.Close()
		}
	}
	return desc
}
