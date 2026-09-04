package gui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ZioSHik/kinopub-gui/internal/lib/credstore"
)

// githubAPI is the API root, overridden in tests. Reports go to the same host
// the updater talks to; nothing is ever posted anywhere else.
var githubAPI = "https://api.github.com"

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

// maxDetail caps what the UI may hand us. A job error is a line or two; the
// cap is only here so a runaway string cannot become the issue body.
const maxDetail = 16000

// reportBody picks what to report and cleans it. detail is a failure the UI
// knows about — an ordinary job error, which never reaches crash.log because
// nothing panicked — and takes precedence over the last recorded crash. It is
// redacted exactly like a stack trace: a wrapped URL error carries the
// kino.watch token in its query string.
func reportBody(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > maxDetail {
		detail = detail[:maxDetail] + "\n… truncated"
	}
	if detail == "" {
		detail = lastCrashEntry()
	}
	return redactCrash(detail)
}

// crashReportURL builds a prefilled GitHub issue: the user reviews it in their
// own browser and decides whether to submit. Nothing is sent from the app
// itself, so no token has to ship inside the binary — one extracted from a
// published release would let anyone write to the repo.
func crashReportURL(version, detail string) string {
	entry := reportBody(detail)
	if entry == "" {
		return ""
	}

	title := reportTitle(entry, detail)
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

// crashToken returns the stored GitHub token, or "" when none is set. The same
// token the updater uses — it just needs Issues: write added to it.
func crashToken() string {
	creds, err := credstore.Load()
	if err != nil {
		return ""
	}
	return creds.GitHubToken
}

// reportedMarkPath holds the digest of the last crash filed, so a panic that
// repeats — a loop recover can fire every tick — does not open one issue per
// occurrence.
func reportedMarkPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "crash.reported"), nil
}

// errAlreadyReported is returned when this exact crash was already filed.
var errAlreadyReported = fmt.Errorf("this crash has already been reported")

// postCrashIssue files the last crash as an issue and returns its URL. It
// posts only what crashReportURL would have shown the user in the browser:
// the same redacted text, so the automatic path can never leak more than the
// manual one.
func postCrashIssue(ctx context.Context, version, detail string) (string, error) {
	token := crashToken()
	if token == "" {
		return "", fmt.Errorf("no GitHub token configured")
	}
	entry := reportBody(detail)
	if entry == "" {
		return "", fmt.Errorf("nothing to report")
	}

	sum := sha256.Sum256([]byte(entry))
	digest := hex.EncodeToString(sum[:])
	markPath, err := reportedMarkPath()
	if err == nil {
		if prev, readErr := os.ReadFile(markPath); readErr == nil && strings.TrimSpace(string(prev)) == digest {
			return "", errAlreadyReported
		}
	}

	body, err := json.Marshal(map[string]string{
		"title": reportTitle(entry, detail),
		"body": "**Version:** " + version + "\n\n" +
			"_Filed from the app. Paths and tokens are removed._\n\n" +
			"```\n" + entry + "\n```",
	})
	if err != nil {
		return "", err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		githubAPI+"/repos/"+issueRepo+"/issues", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "kinopub-gui")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("GitHub HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var out struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	if markPath != "" {
		_ = os.WriteFile(markPath, []byte(digest), 0o600)
	}
	return out.HTMLURL, nil
}

// reportTitle labels the issue honestly: a recovered panic is a crash, a job
// that failed on its own is not, and calling both "crash:" makes the issue
// list useless for telling them apart.
func reportTitle(entry, detail string) string {
	if strings.TrimSpace(detail) != "" {
		return "download failed: " + firstLineOf(entry)
	}
	return "crash: " + firstLineOf(entry)
}
