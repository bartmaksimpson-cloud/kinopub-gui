package domain

import "strings"

// SubtitleTrackInfo is one subtitle track offered by the source, as shown to
// the user before a download starts.
type SubtitleTrackInfo struct {
	Index    int
	Name     string
	Language string
	// Forced marks the track that carries signs and foreign lines only.
	Forced bool
}

// SubtitleSpec names one wanted subtitle track by language, and by whether it is
// the forced variant.
//
// Not by index, and not by name: the track list is per episode, the names carry
// running numbers ("RUS #13") that shift between episodes, and positions shift
// with them. The language plus the forced flag is what a viewer actually means
// and what survives to the next episode.
type SubtitleSpec struct {
	Language string `json:"lang"`
	Forced   bool   `json:"forced"`
}

// SubtitlePreference selects which subtitle tracks to download.
//
// Unlike audio, the zero value keeps NOTHING. kino.watch offers forty-odd
// subtitle languages per episode; downloading all of them because nobody said
// otherwise would be absurd, and a file without subtitles still plays.
// Subtitles are opt-in.
type SubtitlePreference struct {
	Keep []SubtitleSpec
}

// KeepsNothing reports whether this preference selects no tracks at all.
func (p SubtitlePreference) KeepsNothing() bool { return len(p.Keep) == 0 }

// SelectSubtitles returns the indices of the tracks the preference keeps, in
// source order. An empty preference selects nothing.
func SelectSubtitles(tracks []SubtitleTrackInfo, pref SubtitlePreference) []int {
	if len(tracks) == 0 || pref.KeepsNothing() {
		return nil
	}
	var out []int
	for i, t := range tracks {
		for _, spec := range pref.Keep {
			if spec.Forced != t.Forced {
				continue
			}
			if subtitleLanguageMatches(t, spec.Language) {
				out = append(out, i)
				break
			}
		}
	}
	return out
}

// subtitleLanguageMatches compares a track's language to a wanted one. The tag
// is matched against both the LANGUAGE attribute and the track name, because
// some sources leave the attribute empty and put the language in the name.
// An empty wanted language matches any track (of the right forced kind).
func subtitleLanguageMatches(t SubtitleTrackInfo, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(t.Language), want) {
		return true
	}
	return strings.Contains(strings.ToLower(t.Name), want)
}
