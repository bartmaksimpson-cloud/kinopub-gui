package gui

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// issueRepo receives crash reports. It is deliberately separate from
// updateRepo: releases are served from the public repo so self-update keeps
// working without a token, while issues go where the maintainer actually
// reads them.
const issueRepo = "bartmaksimpson-cloud/kinopub-gui"

// maxIssueURL keeps the prefilled issue link inside what GitHub accepts;
// browsers and the server both stop honouring very long URLs, and a truncated
// tail is far better than a link that silently 414s.
const maxIssueURL = 7500

// secretPatterns strip credentials out of a crash report before it can be
// pasted into a public issue. kino.watch takes its token in the query string
// (see kinopubapi.methods: "?access_token=..."), so a stack or a wrapped URL
// error can carry a live one — this is not hypothetical tidying.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)((?:access|refresh|api|device)_token=)[^&\s"'` + "`" + `]+`),
	regexp.MustCompile(`(?i)("(?:access|refresh|api_access|device)_token"\s*:\s*")[^"]*`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)(authorization:\s*\S+\s+)\S+`),
}

// redactCrash removes credentials and the user's home directory from a crash
// report. Home paths go too: they carry the account name, and a stack trace
// full of C:\Users\<name>\... names its owner in a public issue.
func redactCrash(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "${1}REDACTED")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, form := range []string{home, filepath.ToSlash(home)} {
			if form != "" {
				s = strings.ReplaceAll(s, form, "~")
			}
		}
	}
	return s
}

// lastCrashEntry returns the most recent block written by logPanic, or "" when
// crash.log holds none. Blocks start with the "=== " header logPanic writes.
func lastCrashEntry() string {
	dir, err := configDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "crash.log"))
	if err != nil {
		return ""
	}
	idx := strings.LastIndex(string(data), "\n=== ")
	if idx < 0 {
		if !strings.HasPrefix(string(data), "=== ") {
			return ""
		}
		return strings.TrimSpace(string(data))
	}
	return strings.TrimSpace(string(data)[idx:])
}

// crashReportURL builds a prefilled GitHub issue for the last crash: the user
// reviews it in their own browser and decides whether to submit. Nothing is
// sent from the app itself, so no token has to ship inside the binary — one
// extracted from a published release would let anyone write to the repo.
func crashReportURL(version string) string {
	entry := redactCrash(lastCrashEntry())
	if entry == "" {
		return ""
	}

	title := "crash: " + firstLineOf(entry)
	body := "**Version:** " + version + "\n\n" +
		"<!-- Paths and tokens are already removed. Add what you were doing when it happened. -->\n\n" +
		"```\n" + entry + "\n```"

	base := "https://github.com/" + issueRepo + "/issues/new?"
	for {
		u := base + url.Values{"title": {title}, "body": {body}}.Encode()
		if len(u) <= maxIssueURL || len(body) < 500 {
			return u
		}
		body = body[:len(body)-(len(u)-maxIssueURL)-16] + "\n… truncated\n```"
	}
}

// firstLineOf summarises an entry for the issue title: logPanic's header line
// ends with the panic value, which is the useful half.
func firstLineOf(entry string) string {
	line, _, _ := strings.Cut(entry, "\n")
	if _, rest, ok := strings.Cut(line, " | "); ok {
		line = rest
	}
	if len(line) > 120 {
		line = line[:120]
	}
	return strings.TrimSpace(line)
}
