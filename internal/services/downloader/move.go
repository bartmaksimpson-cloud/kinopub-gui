package downloader

import "github.com/ZioSHik/kinopub-gui/internal/lib/fsutil"

// moveFile moves a finished file to its final place. Живёт в fsutil, потому что
// то же самое понадобилось движку: он переносит файлы, дождавшиеся вернувшейся
// сетевой папки, и вторая копия той же осторожности была бы лишней.
func moveFile(from, to string) error { return fsutil.Move(from, to) }
