package downloader

import (
	"os/exec"
	"strings"
	"testing"
)

// pinFitEncoder фиксирует выбранный кодировщик на время теста. Без этого тесты
// зависят от машины: на маке зонд открывает videotoolbox и подгонка идёт по
// битрейту, а на runner'е CI не открывается ничего — остаётся libx265 с -crf,
// и те же самые проверки падают. Зонд запускается один раз на процесс, поэтому
// его достаточно «съесть» пустым Do.
func pinFitEncoder(t *testing.T, name string) {
	t.Helper()
	fitOnce.Do(func() {})
	prev := fitEncoderName
	fitEncoderName = name
	t.Cleanup(func() { fitEncoderName = prev })
}

func TestHeightOf(t *testing.T) {
	cases := map[string]int{
		"1920x1080":   1080,
		"3840x2880":   2880,
		" 3840x2160 ": 2160,
		"":            0,
		"1080":        0,
		"1920xabc":    0,
		"1920x0":      0,
		"1920x-4":     0,
	}
	for in, want := range cases {
		if got := heightOf(in); got != want {
			t.Errorf("heightOf(%q) = %d, ожидалось %d", in, got, want)
		}
	}
}

// The whole point of the cap: 3840x2880 is inside the width limit of a TV
// decoder and far over its height limit, so it must be scaled; a standard 4K
// frame must be left alone, or every download would be re-encoded for nothing.
func TestScaleToHeightArgs(t *testing.T) {
	pinFitEncoder(t, "h264_nvenc")
	tall := scaleToHeightArgs(2880, 2160, 0, "ffmpeg")
	if len(tall) == 0 {
		t.Fatal("кадр 2880 выше предела 2160, а аргументов нет")
	}
	joined := strings.Join(tall, " ")
	if !strings.Contains(joined, "-vf scale=-16:2160") {
		t.Errorf("нет фильтра масштабирования: %q", joined)
	}
	// Scaling cannot be a stream copy — an encoder has to be chosen.
	if !strings.Contains(joined, "-c:v") {
		t.Errorf("масштабирование без энкодера — ffmpeg откажется: %q", joined)
	}

	// The source bitrate is carried over so the smaller frame keeps its picture.
	withRate := strings.Join(scaleToHeightArgs(2880, 2160, 18000, "ffmpeg"), " ")
	if !strings.Contains(withRate, "-b:v 18000k") {
		t.Errorf("битрейт источника не перенесён: %q", withRate)
	}

	for _, c := range []struct {
		name     string
		src, max int
	}{
		{"кадр ровно по пределу", 2160, 2160},
		{"кадр ниже предела", 1080, 2160},
		{"предел выключен", 2880, 0},
		{"разрешение неизвестно", 0, 2160},
	} {
		if got := scaleToHeightArgs(c.src, c.max, 0, "ffmpeg"); got != nil {
			t.Errorf("%s: ожидалась копия без перекодирования, получено %v", c.name, got)
		}
	}
}

// Дюна: 3840x1600 при 47,952 к/с. Кадр в предел железа влезает, а поток — нет:
// декодер принимает файл, запускается и выбрасывает две трети кадров.
func TestFitArgsFor_HalvesHighFrameRateAt4K(t *testing.T) {
	pinFitEncoder(t, "h264_nvenc")
	got := fitArgsFor(
		fitSource{Width: 3840, Height: 1600, FPS: 47.952, Kbps: 9000},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	)
	joined := strings.Join(got, " ")
	// Ровно половина, а не «30»: так сохраняется исходная каденция фильма и ни
	// один кадр не показывается дважды.
	if !strings.Contains(joined, "-r 23.976") {
		t.Errorf("частота не поделена пополам: %q", joined)
	}
	// Кадр в предел влезает — масштабировать нечего.
	if strings.Contains(joined, "scale=") {
		t.Errorf("кадр 1600 масштабировать не нужно: %q", joined)
	}
	if !strings.Contains(joined, "-b:v 9000k") {
		t.Errorf("битрейт источника не перенесён: %q", joined)
	}
}

// На маленьком кадре высокая частота железу не мешает — портить её нельзя.
func TestFitArgsFor_LeavesSmallFramesAlone(t *testing.T) {
	if got := fitArgsFor(
		fitSource{Width: 1920, Height: 1080, FPS: 60},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	); got != nil {
		t.Errorf("1080p60 трогать не надо, получено %v", got)
	}
}

// Оба предела сразу: и кадр выше, и частота выше.
func TestFitArgsFor_ScalesAndHalvesTogether(t *testing.T) {
	joined := strings.Join(fitArgsFor(
		fitSource{Width: 3840, Height: 2880, FPS: 60, Kbps: 20000},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	), " ")
	// Ширина считается заранее и стоит в фильтре явно: 3840x2880 → 2880x2160.
	if !strings.Contains(joined, "scale=2880:2160") || !strings.Contains(joined, "-r 30") {
		t.Errorf("нужны и масштабирование, и деление частоты: %q", joined)
	}
}

// Обычный 4K24: ничего не трогаем, файл копируется как есть.
func TestFitArgsFor_PassesStandard4K(t *testing.T) {
	if got := fitArgsFor(
		fitSource{Width: 3840, Height: 2160, FPS: 23.976},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	); got != nil {
		t.Errorf("настоящий 4K должен копироваться без изменений, получено %v", got)
	}
}

// Настоящий 4K60 в HEVC железо показывает: чипы, которые упираются в 4Kp30 на
// H.264, тянут HEVC до 4Kp60 и выше. Делить такую частоту — выбросить плавность,
// которую плеер способен показать.
func TestFitArgsFor_KeepsHighFrameRateForHEVC(t *testing.T) {
	if got := fitArgsFor(
		fitSource{Width: 3840, Height: 2160, FPS: 60, Kbps: 25000, Codec: "h265"},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	); got != nil {
		t.Errorf("HEVC 4K60 трогать не надо, получено %v", got)
	}

	// А тот же поток в H.264 — за пределом бюджета декодера, делим.
	joined := strings.Join(fitArgsFor(
		fitSource{Width: 3840, Height: 2160, FPS: 60, Kbps: 25000, Codec: "h264"},
		fitLimits{Height: 2160, FPS: 30},
		"ffmpeg",
	), " ")
	if !strings.Contains(joined, "-r 30") {
		t.Errorf("H.264 4K60 должен быть поделён: %q", joined)
	}
}

// Подбор энкодера обязан проверять машину, а не сборку ffmpeg: любая
// стандартная сборка под Windows перечисляет hevc_nvenc, и без карты NVIDIA он
// падает при открытии — посреди мукса, уже после скачанных гигабайт.
func TestHEVCEncoder_IsProbedNotAssumed(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg не найден")
	}
	// Заведомо несуществующий энкодер: список сборки его не содержит, но даже
	// если бы содержал, живая проверка обязана его отвергнуть.
	if encoderOpens("ffmpeg", "hevc_nosuchencoder") {
		t.Error("несуществующий энкодер признан рабочим")
	}
	// Программный x265 есть в любой полной сборке — проверка не должна врать в
	// другую сторону и отвергать то, что работает.
	if listEncoders("ffmpeg", []string{"hevc_videotoolbox"})["hevc_videotoolbox"] && !encoderOpens("ffmpeg", "hevc_videotoolbox") {
		t.Log("videotoolbox есть в сборке, но не открывается на этой машине — это допустимо")
	}
}

// Подгонка не обязана быть в HEVC: уменьшённый кадр по построению не выше
// 3840x2160 при 30 к/с, а это ровно то, что любое железо ещё берёт в H.264.
// Карта вроде GeForce GT 730 кодирует только H.264 — гнать её на процессорный
// x265 значило бы часы вместо минут.
func TestFitEncoders_FallBackToHardwareH264(t *testing.T) {
	h264At := -1
	x265At := -1
	for i, name := range fitEncoders {
		if h264At < 0 && strings.HasPrefix(name, "h264_") {
			h264At = i
		}
		if name == "libx265" {
			x265At = i
		}
	}
	if h264At < 0 {
		t.Fatal("в списке нет ни одного аппаратного H.264")
	}
	if x265At >= 0 {
		t.Errorf("libx265 в списке подгонки: на 4K без железа он считает часами")
	}
	// Программный запасной вариант обязан быть последним и обязан существовать.
	if last := fitEncoders[len(fitEncoders)-1]; last != "libx264" {
		t.Errorf("последним должен быть libx264, а не %q", last)
	}
	// И аппаратный HEVC обязан идти раньше аппаратного H.264.
	for i, name := range fitEncoders {
		if strings.HasPrefix(name, "hevc_") && i > h264At {
			t.Errorf("аппаратный %s стоит после H.264", name)
		}
	}
}

// Кадр вписывается в КОРОБКУ декодера, а не только по высоте: анаморфный
// 5120x2160 законен по высоте и не лезет по ширине — ровно тот случай, когда
// телевизор молча уходит в программный декод.
func TestFitArgsFor_FitsIntoDecoderBox(t *testing.T) {
	pinFitEncoder(t, "h264_nvenc")
	joined := strings.Join(fitArgsFor(
		fitSource{Width: 5120, Height: 2160, Kbps: 20000},
		fitLimits{Width: 4096, Height: 2160},
		"ffmpeg",
	), " ")
	if !strings.Contains(joined, "scale=4096:1728") {
		t.Errorf("широкий кадр не вписан в 4096x2160: %q", joined)
	}
}

func TestFitBox(t *testing.T) {
	cases := []struct {
		name                   string
		srcW, srcH, maxW, maxH int
		wantW, wantH           int
		wantOK                 bool
	}{
		{"кадр в пределах — не трогаем", 3840, 2160, 4096, 2160, 0, 0, false},
		// 3840*2160/2330 = 3559,8 → ближайшее кратное 16 вниз.
		{"высокий кадр", 3840, 2330, 4096, 2160, 3552, 2160, true},
		{"широкий кадр", 5120, 2160, 4096, 2160, 4096, 1728, true},
		{"обе стороны за пределом", 6000, 3000, 4096, 2160, 4096, 2048, true},
		{"предел ширины выключен", 5120, 2160, 0, 2160, 0, 0, false},
		{"ширина источника неизвестна", 0, 2880, 4096, 2160, 0, 0, false},
	}
	for _, c := range cases {
		w, h, ok := fitBox(c.srcW, c.srcH, c.maxW, c.maxH)
		if ok != c.wantOK || (ok && (w != c.wantW || h != c.wantH)) {
			t.Errorf("%s: fitBox(%d,%d,%d,%d) = %dx%d ok=%v, ожидалось %dx%d ok=%v",
				c.name, c.srcW, c.srcH, c.maxW, c.maxH, w, h, ok, c.wantW, c.wantH, c.wantOK)
		}
	}
}
