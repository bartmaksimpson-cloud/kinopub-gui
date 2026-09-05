package gui

import (
	"encoding/json"
	"testing"

	"github.com/ZioSHik/kinopub-gui/internal/services/kinopubapi"
)

// «Дюна: Часть вторая» лежит на kino.watch одним фильмом с двумя видео —
// «24 fps» и «48 fps». Это альтернативы, а не части: без явного выбора движок
// раскладывал их в эпизоды S1E1/S1E2 и качал оба, то есть два 4K-файла вместо
// одного, причём второй — тот самый HFR, который телевизор не тянет.
func TestCollectVersions_MovieWithSeveralFiles(t *testing.T) {
	it := kinopubapi.Item{
		Videos: []kinopubapi.Video{
			{Number: 1, Title: "24 fps", Duration: 10016},
			{Number: 2, Title: "48 fps", Duration: 9949},
		},
	}
	got := collectVersions(it)
	if len(got) != 2 {
		t.Fatalf("версий %d, ожидалось 2: %+v", len(got), got)
	}
	if got[0].Title != "24 fps" || got[0].Episode != 1 {
		t.Errorf("первая версия разобрана неверно: %+v", got[0])
	}
	if got[0].DurationMin != 166 {
		t.Errorf("длительность %d мин, ожидалось 166", got[0].DurationMin)
	}
}

// Обычный фильм одним файлом спрашивать не о чем, и сериал — тем более.
func TestCollectVersions_NothingToAsk(t *testing.T) {
	single := kinopubapi.Item{Videos: []kinopubapi.Video{{Number: 1, Title: "Фильм"}}}
	if got := collectVersions(single); got != nil {
		t.Errorf("для одного файла вернулось %+v", got)
	}

	serial := kinopubapi.Item{
		Seasons: []kinopubapi.Season{{Number: 1, Episodes: []kinopubapi.Episode{{Number: 1}, {Number: 2}}}},
	}
	if got := collectVersions(serial); got != nil {
		t.Errorf("для сериала вернулось %+v", got)
	}
}

// Поле уезжает в интерфейс только когда есть о чём спрашивать: иначе карточка
// фильма показывала бы пустой раздел «Версия».
func TestDiscoverDetail_VersionsOmittedWhenSingle(t *testing.T) {
	b, err := json.Marshal(DiscoverDetail{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "" && contains(string(b), "\"versions\"") {
		t.Errorf("пустые версии попали в ответ: %s", b)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
