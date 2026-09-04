//go:build !windows

package gui

import "errors"

// listDrives only means something on Windows (drive letters). listDir never
// reaches this on other platforms, since filepath.Dir("/") == "/" no longer
// triggers the drivesSentinel branch there — see system.go.
func listDrives() (FSListing, error) {
	return FSListing{}, errors.New("drive listing is only available on Windows")
}
