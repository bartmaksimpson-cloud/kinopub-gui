package domain

import (
	"path/filepath"
	"testing"
)

func TestWorkPathFor(t *testing.T) {
	out := filepath.Join("/nas", "Сериал", "S01E01.mkv")

	// Без рабочей папки ничего не меняется — временные файлы лежат рядом.
	if got := WorkPathFor("", out); got != out {
		t.Errorf("без рабочей папки = %q, ожидалось %q", got, out)
	}

	// С рабочей папкой имя файла сохраняется: два эпизода не столкнутся, и по
	// брошенному файлу видно, чей он.
	want := filepath.Join("/ssd/work", "S01E01.mkv")
	if got := WorkPathFor("/ssd/work", out); got != want {
		t.Errorf("получено %q, ожидалось %q", got, want)
	}
}
