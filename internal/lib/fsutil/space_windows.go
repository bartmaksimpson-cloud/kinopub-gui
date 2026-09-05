//go:build windows

package fsutil

import "golang.org/x/sys/windows"

// FreeSpace reports the bytes still available on the volume holding path — a
// mapped network drive (Z:) answers for the share behind it, which is exactly
// the number that matters when the output folder lives on a NAS.
func FreeSpace(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(existingParent(path))
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}
