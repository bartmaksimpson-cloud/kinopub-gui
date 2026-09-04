package downloader

import (
	"fmt"
	"io"
	"os"
)

// moveFile moves a finished file to its final place. A rename is instant, but it
// only works inside one filesystem: with the work folder on another drive — the
// whole point of having one — it fails with EXDEV and the download would be
// lost. The fallback copies through a neighbour of the destination and renames
// that, so the final path never exists half-written.
func moveFile(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}

	staging := to + ".moving"
	if err := copyFile(from, staging); err != nil {
		os.Remove(staging)
		return fmt.Errorf("copy across filesystems: %w", err)
	}
	if err := os.Rename(staging, to); err != nil {
		os.Remove(staging)
		return err
	}
	os.Remove(from)
	return nil
}

// copyFile writes src to dst, flushing to disk before it reports success: the
// caller renames the result into place and then deletes the source.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
