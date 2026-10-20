// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "error-text-secret-funnel" entry.

package client

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"saas-alerts-pp-cli/internal/config"
	"saas-alerts-pp-cli/internal/platform"
)

// funnelBS is a backslash, built at run time so no escape sequence in this
// file can be decoded before it reaches the compiler.
var funnelBS = string(rune(92))

// funnelEncodings are the ways a server might quote a secret back.
var funnelEncodings = map[string]func(string) string{
	"raw":            func(s string) string { return s },
	"json-all-upper": func(s string) string { return funnelUTF16(s, "u%04X") },
	"json-all-lower": func(s string) string { return funnelUTF16(s, "u%04x") },
	"json-special": func(s string) string {
		return strings.NewReplacer(funnelBS, funnelBS+funnelBS, `"`, funnelBS+`"`, "/", funnelBS+"/", "+", funnelBS+"u002B", "<", funnelBS+"u003c", ">", funnelBS+"u003e", "&", funnelBS+"u0026", "=", funnelBS+"u003d").Replace(s)
	},
	"pct-all-upper": func(s string) string { return funnelBytes(s, "%%%02X") },
	"pct-all-lower": func(s string) string { return funnelBytes(s, "%%%02x") },
	"pct-query":     url.QueryEscape,
	"pct-path":      url.PathEscape,
	"html-escape":   html.EscapeString,
	"html-decimal":  func(s string) string { return funnelEach(s, func(r rune) string { return fmt.Sprintf("&#%d;", r) }) },
	"html-hex":      func(s string) string { return funnelEach(s, func(r rune) string { return fmt.Sprintf("&#x%x;", r) }) },
	"json-of-pct":   func(s string) string { return strings.ReplaceAll(funnelBytes(s, "%%%02X"), "%", funnelBS+"u0025") },
	"pct-5-deep": func(s string) string {
		head := fmt.Sprintf("%%%02X", s[0])
		for i := 0; i < 4; i++ {
			head = url.QueryEscape(head)
		}
		return head + s[1:]
	},
	// The display deletes control characters and invalid UTF-8; an echo split
	// by one must not come out joined.
	"ctrl-split": func(s string) string { return s[:3] + "\x01" + s[3:] },
	"utf8-split": func(s string) string { return s[:3] + "\xff" + s[3:] },
	"json-entity-split": func(s string) string {
		return funnelUTF16(s[:3], "u%04X") + "&#1;" + funnelUTF16(s[3:], "u%04X")
	},
	"pct-of-html": func(s string) string {
		return url.QueryEscape(funnelEach(s, func(r rune) string { return fmt.Sprintf("&#%d;", r) }))
	},
}

// funnelUTF16 escapes every UTF-16 unit, as a JSON encoder does (an astral
// character becomes a surrogate pair).
func funnelUTF16(s, format string) string {
	var b strings.Builder
	for _, u := range utf16.Encode([]rune(s)) {
		b.WriteString(funnelBS + fmt.Sprintf(format, u))
	}
	return b.String()
}

func funnelEach(s string, f func(rune) string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteString(f(r))
	}
	return b.String()
}

func funnelBytes(s, format string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		b.WriteString(fmt.Sprintf(format, s[i]))
	}
	return b.String()
}

// TestErrorTextNeverCarriesASecret is the oracle for the class: every secret
// (the partner key, a webhook token the request sent), in every encoding, at
// every position around the 160-rune title cut and the 4096-byte body cut, in
// JSON (with and without trailing content), plain text and HTML answers, on
// error and 2xx-HTML paths, must never put any of the secret's text into the
// error. The secrets share a prefix that appears nowhere else.
func TestErrorTextNeverCarriesASecret(t *testing.T) {
	var mu sync.Mutex
	status, ctype, answer := 400, "application/json", ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", ctype)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	const marker = "Q7zXw"
	key := marker + "Key+ab/cd=ef!(gh)*ij~kl"
	tokens := []string{marker + `Tok+ab/cd="ef"<gh>&ij`, marker + "Tok-é€ñ-0000-unicode", marker + "Tok-" + string(rune(0x1F600)) + "-0000-astral"}
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: key, SaasAlertsApiKey: key}, 5*time.Second, 0)
	c.NoCache = true

	type call struct {
		name   string
		secret string
		run    func() error
	}
	calls := []call{{"partner-key", key, func() error {
		_, err := c.Get(context.Background(), "/reportApi/api/v1/customers", nil)
		return err
	}}, {"respond-mint", key, func() error {
		_, err := c.Get(context.Background(), "/respondApi/api/v1/states", nil)
		return err
	}}}
	for _, tok := range tokens {
		tok := tok
		calls = append(calls, call{"webhook-token", tok, func() error {
			_, _, err := c.Post(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", map[string]any{"url": "https://hooks.example.com/x", "token": tok})
			return err
		}})
	}
	wrappers := []struct {
		status int
		ctype  string
		wrap   func(pad, enc string) string
	}{
		{400, "application/json", func(pad, enc string) string { return `{"message":"` + pad + enc + `"}` }},
		{401, "application/json", func(pad, enc string) string { return `{"message":"` + pad + enc + `"}` + "\n" + `{"code":1}` }},
		{400, "text/plain", func(pad, enc string) string { return pad + enc + " rejected" }},
		{422, "text/html", func(pad, enc string) string {
			return "<html><head><title>" + pad + enc + "</title></head><body>x</body></html>"
		}},
		{200, "text/html", func(pad, enc string) string {
			return "<html><head><title>" + pad + enc + "</title></head><body>x</body></html>"
		}},
	}
	pads := []int{0, 140, 150, 158, 4060, 4080, 4090}
	checked := 0
	for _, cl := range calls {
		for encName, enc := range funnelEncodings {
			for _, w := range wrappers {
				for _, n := range pads {
					mu.Lock()
					status, ctype, answer = w.status, w.ctype, w.wrap(strings.Repeat("x", n), enc(cl.secret))
					mu.Unlock()
					err := cl.run()
					if err == nil {
						t.Fatalf("%s/%s/%d/%s: want an error", cl.name, encName, w.status, w.ctype)
					}
					if out := err.Error(); strings.Contains(funnelNormalize(out), marker) || strings.Contains(strings.ToLower(out), strings.ToLower(enc(marker))) {
						t.Fatalf("%s %s in a %d %s answer at pad %d leaked: ...%s", cl.name, encName, w.status, w.ctype, n, funnelTail(err.Error()))
					}
					checked++
				}
			}
		}
	}
	if checked < 1000 {
		t.Fatalf("only %d cases ran", checked)
	}
}

// funnelNormalize decodes the shown error text the way a reader (or a JSON
// parser) would, independently of the funnel's own decoders.
func funnelNormalize(s string) string {
	uEsc := regexp.MustCompile(funnelBS + funnelBS + "u([0-9a-fA-F]{4})")
	for i := 0; i < 5; i++ {
		before := s
		s = strings.Map(func(r rune) rune {
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
				return -1
			}
			return r
		}, strings.ToValidUTF8(s, ""))
		s = uEsc.ReplaceAllStringFunc(s, func(m string) string {
			v, _ := strconv.ParseUint(m[2:], 16, 32)
			return string(rune(v))
		})
		s = strings.ReplaceAll(s, funnelBS+"/", "/")
		if u, err := url.PathUnescape(s); err == nil {
			s = u
		}
		s = html.UnescapeString(s)
		if s == before {
			break
		}
	}
	return s
}

func funnelTail(s string) string {
	if len(s) > 200 {
		return s[len(s)-200:]
	}
	return s
}

// TestSecretInAResponseHeaderErrorIsWithheld: server text that reaches an
// error through a header (an unsupported Content-Encoding) goes through the
// same funnel, raw or encoded.
func TestSecretInAResponseHeaderErrorIsWithheld(t *testing.T) {
	const key = "Q7zXwHdr+ab/cd=ef-0000"
	enc := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", enc)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: key, SaasAlertsApiKey: key}, 5*time.Second, 0)
	c.NoCache = true
	for _, e := range []string{key, url.QueryEscape(key), funnelBytes(key, "%%%02x")} {
		enc = e
		_, err := c.Get(context.Background(), "/reportApi/api/v1/customers", nil)
		if err == nil || strings.Contains(err.Error(), "Q7zX") || strings.Contains(err.Error(), "%51%37") || strings.Contains(err.Error(), "%51%37%7a") {
			t.Fatalf("Content-Encoding %q leaked: %v", e, err)
		}
	}
}

// TestWebhookTokenInARedirectLocationIsWithheld: a transport error carrying
// server text (a refused redirect's Location) goes through the funnel with the
// request's webhook tokens, not only the client's own credentials.
func TestWebhookTokenInARedirectLocationIsWithheld(t *testing.T) {
	const tok = "Q7zXwRedirect-0000-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "ftp://evil.example/?t="+tok)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key-0000", SaasAlertsApiKey: "test-key-0000"}, 5*time.Second, 0)
	_, _, err := c.Patch(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions/ch-1", map[string]any{"token": tok})
	if err == nil || strings.Contains(err.Error(), "Q7zX") {
		t.Fatalf("redirect Location leaked the webhook token: %v", err)
	}
}

// TestWithheldNoticeIsNotAnAuthError: withholding must not turn a validation
// 400 into an auth-looking message.
func TestWithheldNoticeIsNotAnAuthError(t *testing.T) {
	for _, w := range []string{"key", "token", "credential", "authenticat", "unauthorized", "forbidden", "api_key"} {
		if strings.Contains(strings.ToLower(echoWithheldText), w) {
			t.Fatalf("withheld notice contains auth word %q: %s", w, echoWithheldText)
		}
	}
}

// TestSuccessfulAnswerNeverCarriesASecret: a 2xx JSON answer that quotes back
// a secret (raw or escaped) has that string withheld before it reaches the
// caller, and so the cache, the store and MCP; other fields survive.
func TestSuccessfulAnswerNeverCarriesASecret(t *testing.T) {
	answer := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	const key = "Q7zXwKey-succ+ab/cd-0000"
	const tok = "Q7zXwTok-succ+ab/cd-0000"
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: key, SaasAlertsApiKey: key}, 5*time.Second, 0)
	c.NoCache = true
	for _, enc := range []func(string) string{funnelEncodings["raw"], funnelEncodings["json-special"], funnelEncodings["pct-all-lower"]} {
		answer = `{"id":"keep-me","message":"created for ` + enc(tok) + `","note":"key ` + enc(key) + `"}`
		data, _, err := c.Post(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", map[string]any{"url": "https://hooks.example.com/x", "token": tok})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(funnelNormalize(string(data)), "Q7zX") || !strings.Contains(string(data), "keep-me") {
			t.Fatalf("success answer: %s", data)
		}
	}
}

// TestRateLimitRequestIDIsFunnelled: a 429's request-id header is server text.
func TestRateLimitRequestIDIsFunnelled(t *testing.T) {
	const key = "Q7zXwKey-429+ab/cd-0000"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", url.QueryEscape(key))
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: key, SaasAlertsApiKey: key}, 5*time.Second, 0)
	c.NoCache = true
	// A write is not replayed after a 429, so it returns the typed error at once.
	_, _, err := c.Post(context.Background(), "/reportApi/api/v1/customers", map[string]any{"name": "x"})
	if err == nil {
		t.Fatal("want an error")
	}
	var rl *platform.RateLimitedError
	if !errors.As(err, &rl) {
		t.Fatalf("want a RateLimitedError, got %T: %v", err, err)
	}
	if strings.Contains(funnelNormalize(rl.RequestID), "Q7zX") {
		t.Fatalf("request id leaked: %q", rl.RequestID)
	}
	if strings.Contains(funnelNormalize(err.Error()), "Q7zX") {
		t.Fatalf("429 error leaked: %v", err)
	}
}
