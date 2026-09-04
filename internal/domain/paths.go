package domain

import "path/filepath"

// WorkPathFor returns the base path for the intermediate files belonging to one
// output file: inside workDir when it is set, next to the output otherwise.
//
// The output's own file name is kept, so two episodes downloading at once never
// collide and a file left behind by a crash still says what it belongs to.
func WorkPathFor(workDir, outPath string) string {
	if workDir == "" {
		return outPath
	}
	return filepath.Join(workDir, filepath.Base(outPath))
}
