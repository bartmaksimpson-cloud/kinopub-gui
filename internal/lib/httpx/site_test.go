package httpx

import "testing"

func TestIsSiteHost(t *testing.T) {
	cases := map[string]bool{
		"kino.watch":         true,
		"KINO.WATCH":         true,
		"cdn.kino.watch":     true,
		"a.b.kino.watch":     true,
		"kino.watch:443":     true, // a port must not defeat the match
		"kino.watch.":        true, // trailing root dot
		"kino.pub":           true, // the older domain is still the site
		"cdn.kino.pub":       true,
		"kino.watch.evil.io": false,
		"kino.pub.evil.io":   false,
		"notkino.watch":      false,
		"notkino.pub":        false,
		"evil.com":           false,
		"":                   false,
	}
	for host, want := range cases {
		if got := IsSiteHost(host); got != want {
			t.Errorf("IsSiteHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestCanonicalSiteURL(t *testing.T) {
	cases := map[string]string{
		"https://kino.pub/item/view/42":   "https://kino.watch/item/view/42",
		"http://kino.pub/device":          "http://kino.watch/device",
		"https://cdn.kino.pub/a.ts?x=1":   "https://cdn.kino.watch/a.ts?x=1",
		"https://kino.pub:8443/item/1":    "https://kino.watch:8443/item/1",
		"  https://kino.pub/item/view/7 ": "https://kino.watch/item/view/7",

		// Already canonical, or not ours: returned verbatim.
		"https://kino.watch/item/view/42": "https://kino.watch/item/view/42",
		"https://kino.pub.evil.io/item/1": "https://kino.pub.evil.io/item/1",
		"https://example.com/x":           "https://example.com/x",
		"38290":                           "38290",
		"":                                "",
	}
	for in, want := range cases {
		if got := CanonicalSiteURL(in); got != want {
			t.Errorf("CanonicalSiteURL(%q) = %q, want %q", in, got, want)
		}
	}
}
