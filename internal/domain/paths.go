package domain

import "path/filepath"

// WorkPathFor returns the base path for the intermediate files belonging to one
// output file: inside workDir when it is set, next to the output otherwise.
//
// The output's layout is MIRRORED under the work folder — series/Season NN/file
// — rather than flattened into it. Two shows can have an "S01E01.mkv", and a
// flat work folder would have them writing over each other's segments; it is
// also unreadable when several titles are downloading at once.
//
// outputRoot is the download folder the path was built under. When the relative
// path cannot be worked out (a path from somewhere else entirely), the file name
// alone is used — still unique enough for one title, and never wrong.
func WorkPathFor(workDir, outputRoot, outPath string) string {
	if workDir == "" {
		return outPath
	}
	if outputRoot != "" {
		if rel, err := filepath.Rel(outputRoot, outPath); err == nil && !filepath.IsAbs(rel) && rel != "." && !hasParentEscape(rel) {
			return filepath.Join(workDir, rel)
		}
	}
	return filepath.Join(workDir, filepath.Base(outPath))
}

// hasParentEscape reports whether a relative path climbs out of its root, which
// would put work files somewhere nobody asked for.
func hasParentEscape(rel string) bool {
	return len(rel) >= 2 && rel[0] == '.' && rel[1] == '.'
}
