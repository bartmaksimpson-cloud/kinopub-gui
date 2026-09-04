package httpx

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRoundTripH2AdoptsDialedConn covers the HTTP/2 half of the browser
// transport without leaving the machine. The pre-existing h2 tests all sit
// behind the netintegration build tag and dial live hosts, so nothing in the
// default suite ever entered roundTripH2 — which is how a nil pointer panic on
// every single download shipped.
func TestRoundTripH2AdoptsDialedConn(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("server saw HTTP/%d.%d, want HTTP/2", r.ProtoMajor, r.ProtoMinor)
		}
		io.WriteString(w, "pong")
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	addr := strings.TrimPrefix(srv.URL, "https://")

	conn, err := tls.Dial("tcp", addr, &tls.Config{RootCAs: pool, NextProtos: []string{"h2"}})
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	if got := conn.ConnectionState().NegotiatedProtocol; got != "h2" {
		t.Fatalf("negotiated %q, want h2", got)
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	bt := &browserTransport{}
	resp, err := bt.roundTripH2(req, conn, addr)
	if err != nil {
		t.Fatalf("roundTripH2: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "pong" {
		t.Fatalf("got %d %q, want 200 \"pong\"", resp.StatusCode, body)
	}
}
