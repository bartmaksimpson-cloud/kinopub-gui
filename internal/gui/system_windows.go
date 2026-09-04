//go:build windows

package gui

import "golang.org/x/sys/windows"

// listDrives enumerates the available drive letters (via GetLogicalDrives) as
// an FSListing, so DirPicker.tsx can render them as ordinary clickable entries
// with no frontend changes — it already just renders whatever Dirs it gets.
func listDrives() (FSListing, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return FSListing{}, err
	}
	dirs := []FSEntry{}
	for i := range 26 {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + ":\\"
		dirs = append(dirs, FSEntry{Name: root, Path: root})
	}
	return FSListing{Path: "This PC", Parent: "", Dirs: dirs}, nil
}
