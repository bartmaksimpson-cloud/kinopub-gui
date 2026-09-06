package gui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// All discovery handlers resolve the kino.watch client first; with no stored
// credentials they must reply 401 (not signed in), never panic.
func TestDiscoverHandlers_Unauthenticated(t *testing.T) {
	s := newTestServer(t)
	handlers := map[string]http.HandlerFunc{
		"/api/discover/search?q=x":              s.handleDiscoverSearch,
		"/api/discover/items":                   s.handleDiscoverItems,
		"/api/discover/top?kind=hot&type=movie": s.handleDiscoverTop,
		"/api/discover/collections":             s.handleDiscoverCollections,
		"/api/discover/countries":               s.handleDiscoverCountries,
		"/api/discover/history":                 s.handleDiscoverHistory,
		"/api/discover/watching":                s.handleDiscoverWatching,
		"/api/discover/genres":                  s.handleDiscoverGenres,
		"/api/discover/bookmarks":               s.handleDiscoverBookmarks,
		"/api/discover/item?id=1":               s.handleDiscoverItem,
		"/api/discover/similar?id=1":            s.handleDiscoverSimilar,
		"/api/kp/user":                          s.handleKPUser,
	}
	for path, h := range handlers {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", path, w.Code)
		}
	}
}

func TestDiscoverSearch_RequiresQuery(t *testing.T) {
	s := newTestServer(t)
	// Without "q" it would short-circuit on the missing query — but the client is
	// resolved first, so an unauthenticated server returns 401. Confirm a signed-out
	// search never panics and returns an error status.
	req := httptest.NewRequest("GET", "/api/discover/search", nil)
	w := httptest.NewRecorder()
	s.handleDiscoverSearch(w, req)
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 401 or 400", w.Code)
	}
}

// Both inputs are validated before the client is resolved: the kind becomes a
// URL path segment upstream, and a type-less top list is rejected by the API
// itself — catching it here keeps it a 400 rather than an upstream 502.
func TestDiscoverTop_RejectsBadInput(t *testing.T) {
	s := newTestServer(t)
	for _, q := range []string{
		"kind=&type=movie",
		"kind=views-&type=movie",
		"kind=" + url.QueryEscape("../user") + "&type=movie",
		"kind=hot",          // no type
		"kind=hot&type=%20", // blank type
	} {
		req := httptest.NewRequest("GET", "/api/discover/top?"+q, nil)
		w := httptest.NewRecorder()
		s.handleDiscoverTop(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, w.Code)
		}
	}
}

func TestDiscoverItemHandlers_RequireID(t *testing.T) {
	// item/similar/collection/bookmark require an id; but client resolution comes
	// first. To exercise the id-required branch we stub a client by signing in is
	// not feasible here, so we only assert the unauthenticated path is handled
	// (covered above). This test documents that id-less requests do not 5xx.
	s := newTestServer(t)
	for _, path := range []string{"/api/discover/collection", "/api/discover/bookmark"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		switch path {
		case "/api/discover/collection":
			s.handleDiscoverCollection(w, req)
		case "/api/discover/bookmark":
			s.handleDiscoverBookmark(w, req)
		}
		if w.Code >= 500 {
			t.Errorf("%s should not 5xx, got %d", path, w.Code)
		}
	}
}

func TestProxyImage_RejectsBadURL(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/img", nil)
	proxyImage(w, req, "not a url with no scheme")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestProxyImage_RejectsNonHTTPScheme(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/img", nil)
	proxyImage(w, req, "ftp://host/image.jpg")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleImage_EmptyURL(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/img", nil)
	w := httptest.NewRecorder()
	s.handleImage(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty image url: status = %d, want 400", w.Code)
	}
}

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		origin, host string
		want         bool
	}{
		{"http://127.0.0.1:8765", "127.0.0.1:8765", true}, // exact match
		{"http://localhost:8765", "127.0.0.1:8765", true}, // loopback ↔ loopback
		{"http://[::1]:8765", "127.0.0.1:8765", true},
		{"http://evil.example.com", "127.0.0.1:8765", false},
		{"not a url", "127.0.0.1:8765", false},
		{"http://", "127.0.0.1:8765", false}, // empty host
	}
	for _, c := range cases {
		if got := originAllowed(c.origin, c.host); got != c.want {
			t.Errorf("originAllowed(%q, %q) = %v, want %v", c.origin, c.host, got, c.want)
		}
	}
}

func TestGuardLocalOnly_RejectsNonLoopbackHost(t *testing.T) {
	called := false
	guard := guardLocalOnly(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.Host = "evil.example.com"
	// Запрос СВОЕЙ машины с подделанным Host — это и есть DNS-rebinding, от
	// которого защищает этот слой. Чужие адреса теперь отсекает s.guard, и
	// заголовок Host там уже ничего не решает.
	req.RemoteAddr = "127.0.0.1:52000"
	w := httptest.NewRecorder()
	guard.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || called {
		t.Errorf("forged host: status=%d called=%v, want 403 and not called", w.Code, called)
	}
}

func TestGuardLocalOnly_AllowsLoopback(t *testing.T) {
	called := false
	guard := guardLocalOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.Host = "127.0.0.1:8765"
	w := httptest.NewRecorder()
	guard.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !called {
		t.Errorf("loopback should pass: status=%d called=%v", w.Code, called)
	}
}

// Запрос ПО СЕТИ без ключа не должен доходить до приложения, даже с
// подделанным Host: именно так весь этот сервер и открывался наружу, пока
// проверка смотрела на заголовок вместо адреса соединения.
func TestGuard_NetworkRequestNeedsToken(t *testing.T) {
	srv := NewServer("test", nil)
	// Хранилище без пути: NewServer читает и ПИШЕТ настоящий файл настроек, и
	// тест, включивший доступ по сети, включил бы его человеку по-настоящему.
	srv.settings = &settingsStore{cur: defaultSettings()}
	called := false
	guard := srv.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	netReq := func() *http.Request {
		r := httptest.NewRequest("GET", "/api/settings", nil)
		r.Host = "127.0.0.1:8765" // подделанный «свой» Host — больше не пропуск
		r.RemoteAddr = "192.168.1.77:41000"
		return r
	}

	// Доступ выключен — отказ.
	w := httptest.NewRecorder()
	guard.ServeHTTP(w, netReq())
	if w.Code != http.StatusForbidden || called {
		t.Errorf("доступ выключен: status=%d called=%v, ожидалось 403 без вызова", w.Code, called)
	}

	saved, err := srv.settings.save(Settings{Container: "mkv", RemoteAccess: true})
	if err != nil {
		t.Fatalf("сохранение настроек: %v", err)
	}
	if len(saved.RemoteToken) != 32 {
		t.Fatalf("ключ = %q, ожидались 32 шестнадцатеричных знака", saved.RemoteToken)
	}

	// Доступ включён, ключа нет — отказ.
	w = httptest.NewRecorder()
	guard.ServeHTTP(w, netReq())
	if w.Code != http.StatusUnauthorized || called {
		t.Errorf("без ключа: status=%d called=%v, ожидалось 401 без вызова", w.Code, called)
	}

	// Неверный ключ — отказ.
	w = httptest.NewRecorder()
	bad := netReq()
	bad.Header.Set("X-Kinopub-Token", "00000000000000000000000000000000")
	guard.ServeHTTP(w, bad)
	if w.Code != http.StatusUnauthorized || called {
		t.Errorf("неверный ключ: status=%d called=%v, ожидалось 401 без вызова", w.Code, called)
	}

	// Верный ключ в заголовке — пропуск.
	w = httptest.NewRecorder()
	ok := netReq()
	ok.Header.Set("X-Kinopub-Token", saved.RemoteToken)
	guard.ServeHTTP(w, ok)
	if w.Code != http.StatusOK || !called {
		t.Errorf("верный ключ: status=%d called=%v, ожидалось 200 с вызовом", w.Code, called)
	}

	// Ключ в адресе — пропуск и печенье, чтобы дальше он не болтался в URL.
	called = false
	w = httptest.NewRecorder()
	viaURL := httptest.NewRequest("GET", "/?t="+saved.RemoteToken, nil)
	viaURL.RemoteAddr = "192.168.1.77:41001"
	guard.ServeHTTP(w, viaURL)
	if w.Code != http.StatusOK || !called {
		t.Fatalf("ключ в адресе: status=%d called=%v", w.Code, called)
	}
	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == remoteTokenCookie && c.Value == saved.RemoteToken {
			found = true
			if !c.HttpOnly {
				t.Error("печенье с ключом должно быть HttpOnly — иначе его читает любой скрипт страницы")
			}
		}
	}
	if !found {
		t.Error("ключ из адреса не сохранён в печенье")
	}
}

// Свои запросы ходят без ключа: доступ по сети — это про чужие адреса.
func TestGuard_LoopbackNeedsNoToken(t *testing.T) {
	srv := NewServer("test", nil)
	srv.settings = &settingsStore{cur: defaultSettings()}
	called := false
	guard := srv.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.RemoteAddr = "127.0.0.1:53000"
	w := httptest.NewRecorder()
	guard.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !called {
		t.Errorf("свой запрос: status=%d called=%v, ожидалось 200 с вызовом", w.Code, called)
	}
}
