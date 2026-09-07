// Package proc spawns child processes without letting them open a window.
//
// Приложение под Windows собрано как графическое (-H windowsgui) и своей
// консоли не имеет. Когда оно запускает консольную программу — ffmpeg, ffprobe,
// tar, wmic, — Windows выдаёт ей НОВУЮ консоль, и та вспыхивает поверх
// интерфейса чёрным окном. Каждая проба кодировщика, каждый ffprobe — окно.
//
// Хуже того, окно можно закрыть: тогда всей группе процессов уходит
// CTRL_CLOSE_EVENT, и человек, убравший с экрана «мусорное» окошко, убивает
// идущую склейку.
package proc

import (
	"context"
	"os/exec"
)

// Command is exec.Command that never opens a console window.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	hide(cmd)
	return cmd
}

// CommandContext is exec.CommandContext that never opens a console window.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	return cmd
}

// Hide applies the same treatment to a command built elsewhere.
func Hide(cmd *exec.Cmd) { hide(cmd) }
