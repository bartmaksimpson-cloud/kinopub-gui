//go:build android

package httpx

import (
	"context"
	"net"
	"time"
)

// newDialer returns a net.Dialer with a reliable DNS resolver for Android/Termux.
// Android's netd DNS proxy at [::1]:53 is inaccessible via raw UDP sockets
// from non-system processes (like Termux), so we dial 8.8.8.8 directly.
// NewDialer is the exported alias used by other packages.
func NewDialer() *net.Dialer { return newDialer() }

// dialTimeout matches the non-Android dialer: see dialer_other.go for why a
// blocked connect must fail fast rather than pin a socket for half a minute.
const dialTimeout = 8 * time.Second

func newDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, "8.8.8.8:53")
			},
		},
	}
}
