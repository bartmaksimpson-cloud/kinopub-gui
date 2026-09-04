package gui

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

// boom panics from a known line so the origin can be checked against it.
func boom() { var p *int; _ = *p }

// TestPanicOriginNamesTheFailingLine is the point of the whole file: the
// reported location must be where the panic happened, not the recover site.
// Everything above runtime.gopanic is recovery machinery, and reporting that
// would make every crash look like it came from jobs.go's deferred func.
func TestPanicOriginNamesTheFailingLine(t *testing.T) {
	var origin string
	func() {
		defer func() {
			if r := recover(); r != nil {
				origin = panicOrigin(debug.Stack())
			}
		}()
		boom()
	}()

	if !strings.HasPrefix(origin, "gui/panics_test.go:") {
		t.Fatalf("origin = %q, want the line inside boom() in gui/panics_test.go", origin)
	}
}

// TestLogPanicWritesCrashLog checks the durable half: on Windows the GUI has
// no console, so crash.log next to the settings is the only place a stack can
// survive for the user to send on.
func TestLogPanicWritesCrashLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var desc string
	func() {
		defer func() {
			if r := recover(); r != nil {
				desc = logPanic("unit test", r)
			}
		}()
		boom()
	}()

	if !strings.Contains(desc, "nil pointer dereference") || !strings.Contains(desc, " at gui/panics_test.go:") {
		t.Fatalf("desc = %q, want the panic text plus its origin", desc)
	}

	data, err := os.ReadFile(filepath.Join(dir, "kinopub", "crash.log"))
	if err != nil {
		t.Fatalf("read crash.log: %v", err)
	}
	for _, want := range []string{"unit test", "nil pointer dereference", "panics_test.go"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("crash.log missing %q, got:\n%s", want, data)
		}
	}
}
