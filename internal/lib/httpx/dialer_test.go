package httpx

import (
	"testing"
	"time"
)

func TestNewDialer_Config(t *testing.T) {
	d := NewDialer()
	if d == nil {
		t.Fatal("nil dialer")
	}
	// The connect timeout is deliberately short. A blocked host (kino.watch
	// without a VPN) must fail fast: at 30s each such request pinned a browser
	// socket for half a minute, and browsers allow only ~6 per origin, so a
	// handful of them starved unrelated pages. This bounds the TCP connect only,
	// never the transfer, so slow-but-working links are unaffected.
	if d.Timeout != dialTimeout {
		t.Errorf("Timeout = %v, want %v", d.Timeout, dialTimeout)
	}
	if d.Timeout > 10*time.Second {
		t.Errorf("Timeout = %v: too long to keep a socket held on an unreachable host", d.Timeout)
	}
	if d.KeepAlive != 30*time.Second {
		t.Errorf("KeepAlive = %v, want 30s", d.KeepAlive)
	}
}

func TestNewDialer_ReturnsDistinctInstances(t *testing.T) {
	a := NewDialer()
	b := NewDialer()
	if a == b {
		t.Error("expected distinct dialer instances")
	}
}

func TestNewDialerAlias_MatchesUnexported(t *testing.T) {
	pub := NewDialer()
	priv := newDialer()
	if pub.Timeout != priv.Timeout || pub.KeepAlive != priv.KeepAlive {
		t.Errorf("NewDialer and newDialer differ: %+v vs %+v", pub, priv)
	}
}
