package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

// Путь, ради которого всё и написано: рабочая папка на другом диске, где
// os.Rename падает с EXDEV. Разные файловые системы в тесте не поднять,
// поэтому проверяется сама копия — то, чем перенос подстраховывается.
func TestCopyFile_KeepsContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	body := make([]byte, 1<<20) // мегабайт: не один Read
	for i := range body {
		body[i] = byte(i)
	}
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(body) {
		t.Fatalf("размер %d, ожидался %d", len(got), len(body))
	}
	for i := range got {
		if got[i] != body[i] {
			t.Fatalf("байт %d испорчен", i)
		}
	}
}
