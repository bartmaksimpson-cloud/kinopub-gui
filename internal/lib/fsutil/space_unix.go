//go:build !windows

package fsutil

import "golang.org/x/sys/unix"

// FreeSpace reports the bytes still available on the filesystem holding path.
// The path itself need not exist — the nearest existing parent answers, which
// is what a caller checking "will this download fit" actually means.
func FreeSpace(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(existingParent(path), &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
