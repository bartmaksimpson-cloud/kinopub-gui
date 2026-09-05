package domain

import "testing"

// Субтитры — по желанию: пустое предпочтение не берёт ничего. У kino.watch
// в мастер-плейлисте бывает сорок с лишним языков, и качать их все «потому что
// никто не возражал» — абсурд.
func TestSelectSubtitles_EmptyPreferenceKeepsNothing(t *testing.T) {
	tracks := []SubtitleTrackInfo{
		{Index: 0, Name: "RUS #13", Language: "rus"},
		{Index: 1, Name: "ENG #21", Language: "eng"},
	}
	if got := SelectSubtitles(tracks, SubtitlePreference{}); len(got) != 0 {
		t.Errorf("пустое предпочтение выбрало %v", got)
	}
}

// Выбор запоминается словами, а не номерами: в другом эпизоде тот же язык
// стоит на другом месте списка.
func TestSelectSubtitles_MatchesByNameOrLanguage(t *testing.T) {
	tracks := []SubtitleTrackInfo{
		{Index: 0, Name: "VIE #01", Language: "vie"},
		{Index: 1, Name: "RUS #12 Forced", Language: "rus", Forced: true},
		{Index: 2, Name: "RUS #13", Language: "rus"},
		{Index: 3, Name: "ENG #21", Language: "eng"},
	}

	// Обычная русская — только она, форсированная остаётся за бортом.
	got := SelectSubtitles(tracks, SubtitlePreference{Keep: []SubtitleSpec{{Language: "rus"}}})
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("выбрано %v, ожидалась только полная русская дорожка", got)
	}

	// Форсированная — отдельный выбор, хотя язык тот же.
	got = SelectSubtitles(tracks, SubtitlePreference{Keep: []SubtitleSpec{{Language: "rus", Forced: true}}})
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("выбрано %v, ожидалась только форсированная", got)
	}

	// Обе сразу —два выбора — два правила.
	got = SelectSubtitles(tracks, SubtitlePreference{Keep: []SubtitleSpec{{Language: "rus"}, {Language: "rus", Forced: true}}})
	if len(got) != 2 {
		t.Errorf("выбрано %v, ожидались обе русские", got)
	}
}
