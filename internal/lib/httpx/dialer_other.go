//go:build !android

package httpx

import (
	"net"
	"time"
)

// NewDialer is the exported alias used by other packages.
func NewDialer() *net.Dialer { return newDialer() }

// dialTimeout bounds only the TCP connect, not the transfer that follows, so a
// slow-but-working link is unaffected. It is deliberately short: kino.watch is
// unreachable without a VPN, and at 30s every blocked request pinned a browser
// socket for half a minute. Browsers allow ~6 connections per origin, so a few
// of those starved the pool and unrelated pages — a local library scan that the
// server answers in 11ms — queued behind them.
const dialTimeout = 8 * time.Second

func newDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}
}
