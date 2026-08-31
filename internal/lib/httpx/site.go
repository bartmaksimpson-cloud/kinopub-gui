// Package httpx — site.go knows which hosts belong to the service. The site is
// reachable under more than one domain: kino.watch is the current one, kino.pub
// the historical one that still shows up in links saved before the rename. Both
// have to be recognised, or the authenticated Cookie is withheld from a
// legitimate host and old queue entries stop looking like site links.
package httpx

import (
	"net"
	"net/url"
	"strings"
)

// PrimarySiteDomain is the canonical domain — the one freshly built links use.
const PrimarySiteDomain = "kino.watch"

// SiteDomains lists every registrable domain the service is served from, the
// canonical one first. Older domains stay in the list so links stored under
// them keep working.
var SiteDomains = []string{PrimarySiteDomain, "kino.pub"}

// IsSiteHost reports whether host belongs to the site — one of SiteDomains or a
// sub-domain of one (cdn.kino.watch). host may carry a port.
func IsSiteHost(host string) bool { return siteDomainOf(host) != "" }

// CanonicalSiteURL rewrites a URL on an older site domain to the canonical one,
// keeping the sub-domain, port, path and query intact:
// https://kino.pub/item/view/42 → https://kino.watch/item/view/42. Anything that
// is not a site URL — another host, or an unparseable string — comes back
// unchanged, so this is safe to apply to arbitrary user input.
func CanonicalSiteURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return raw
	}
	d := siteDomainOf(u.Hostname())
	if d == "" || d == PrimarySiteDomain {
		return raw
	}

	host := strings.TrimSuffix(normalizeHost(u.Hostname()), d) + PrimarySiteDomain
	if port := u.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	}
	u.Host = host
	return u.String()
}

// siteDomainOf returns the site domain host belongs to, or "" for none. A match
// is the domain itself or a sub-domain of it — never a look-alike such as
// "notkino.watch" or "kino.watch.evil.io", so the authenticated Cookie cannot
// leak to an attacker-chosen host.
func siteDomainOf(host string) string {
	host = normalizeHost(host)
	for _, d := range SiteDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return d
		}
	}
	return ""
}

// normalizeHost lower-cases host and strips a port and the trailing root dot.
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(host, ".")
}
