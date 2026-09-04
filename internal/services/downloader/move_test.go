package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveFile_SameFilesystem(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "a.tmp")
	to := filepath.Join(dir, "a.mkv")
	if err := os.WriteFile(from, []byte("кино"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(from, to); err != nil {
		t.Fatalf("moveFile: %v", err)
	}
	got, err := os.ReadFile(to)
	if err != nil || string(got) != "кино" {
		t.Fatalf("файл не доехал: %v %q", err, got)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Error("исходный файл не удалён")
	}
}

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

// Перенос не должен оставлять после себя половину файла под финальным именем.
func TestMoveFile_MissingSourceLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "out.mkv")
	if err := moveFile(filepath.Join(dir, "нет-такого"), to); err == nil {
		t.Fatal("перенос несуществующего файла прошёл успешно")
	}
	if _, err := os.Stat(to); !os.IsNotExist(err) {
		t.Error("под финальным именем что-то осталось")
	}
	if _, err := os.Stat(to + ".moving"); !os.IsNotExist(err) {
		t.Error("промежуточный файл не убран")
	}
}
