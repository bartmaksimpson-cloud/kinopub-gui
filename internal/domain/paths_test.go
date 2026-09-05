package domain

import (
	"path/filepath"
	"testing"
)

func TestWorkPathFor(t *testing.T) {
	out := filepath.Join("/nas", "Сериал", "S01E01.mkv")

	// Без рабочей папки ничего не меняется — временные файлы лежат рядом.
	if got := WorkPathFor("", "/nas", out); got != out {
		t.Errorf("без рабочей папки = %q, ожидалось %q", got, out)
	}

	// С рабочей папкой повторяется структура папки загрузки: сериал/сезон/файл.
	// Плоская укладка сталкивала бы два сериала с одинаковыми именами серий.
	want := filepath.Join("/ssd/work", "Сериал", "S01E01.mkv")
	if got := WorkPathFor("/ssd/work", "/nas", out); got != want {
		t.Errorf("получено %q, ожидалось %q", got, want)
	}

	// Путь не из папки загрузки: структуру взять неоткуда, остаётся имя файла.
	outside := filepath.Join("/somewhere", "else", "S01E01.mkv")
	want = filepath.Join("/ssd/work", "S01E01.mkv")
	if got := WorkPathFor("/ssd/work", "/nas", outside); got != want {
		t.Errorf("для чужого пути получено %q, ожидалось %q", got, want)
	}
}
