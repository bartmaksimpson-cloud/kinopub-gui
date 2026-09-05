// Package fsutil provides filesystem utilities: atomic file writes and
// path-component sanitization.
package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// AtomicWrite writes data to a temporary file in the same directory as path,
// then atomically renames it to path. On failure the temp file is cleaned up.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("fsutil: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// Ensure cleanup on any failure path.
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("fsutil: chmod temp file: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("fsutil: write temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("fsutil: sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("fsutil: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("fsutil: rename temp to target: %w", err)
	}

	success = true
	return nil
}

// AtomicRename renames src to dst atomically (os.Rename wrapper for clarity).
func AtomicRename(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("fsutil: rename %q -> %q: %w", src, dst, err)
	}
	return nil
}

// reservedChars contains characters that are reserved or invalid in filesystem
// path components across common operating systems.
const reservedChars = "/\\:*?\"<>|"

// SanitizeComponent replaces characters that are reserved or invalid in
// filesystem path components with an underscore. It preserves valid Unicode
// including Cyrillic. Leading/trailing whitespace and dots are trimmed. If the
// result is empty, the fallback is returned. The fallback itself must be
// non-empty; if it is empty, "unnamed" is used.
func SanitizeComponent(name string, fallback string) string {
	if fallback == "" {
		fallback = "unnamed"
	}

	var b strings.Builder
	b.Grow(len(name))

	for _, r := range name {
		switch {
		case r == 0: // NUL
			b.WriteByte('_')
		case unicode.IsControl(r):
			b.WriteByte('_')
		case strings.ContainsRune(reservedChars, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}

	result := b.String()

	// Trim leading/trailing whitespace and dots.
	result = strings.TrimFunc(result, func(r rune) bool {
		return unicode.IsSpace(r) || r == '.'
	})

	if result == "" {
		return fallback
	}
	return result
}

// EnsureDir creates a directory and every parent it needs, treating "it is
// already there" as success even when the filesystem says otherwise.
//
// os.MkdirAll re-checks with Lstat when mkdir reports ERROR_ALREADY_EXISTS, and
// on a network share (a mapped Z:, an SMB mount) that re-check is not reliable:
// two episodes creating "Season 01" at the same moment, or a server answering a
// beat late, surface as "Cannot create a file when that file already exists"
// and fail a download whose folder is sitting right there. Asking the
// filesystem what is actually at the path is the honest answer.
func EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err == nil {
		return nil
	} else if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		// The directory exists: whatever mkdir complained about, the job is done.
		return nil
	} else {
		return err
	}
}
