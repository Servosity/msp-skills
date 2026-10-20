// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "webhook-subscription-token-withheld" entry.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/config"
)

func TestWithholdWebhookTokensBlanksEveryToken(t *testing.T) {
	in := `[{"channelId":"ch-1","url":"https://hooks.example.com/a?x=1&y=2","token":"test-secret-1"},{"channelId":"ch-2","nested":{"Token":"test-secret-2"},"token":null}]`
	out := string(withholdWebhookTokens("/outgoingWebhookApi/api/v1/subscriptions", []byte(in)))
	if strings.Contains(out, "test-secret") {
		t.Fatalf("token survived: %s", out)
	}
	if strings.Count(out, WebhookTokenWithheld) != 2 {
		t.Fatalf("want 2 withheld markers: %s", out)
	}
	if !strings.Contains(out, "https://hooks.example.com/a?x=1&y=2") {
		t.Fatalf("url was rewritten: %s", out)
	}
}

func TestWithholdWebhookTokensLeavesOtherAPIsAlone(t *testing.T) {
	in := `{"token":"keep-me"}`
	if out := string(withholdWebhookTokens("/reportApi/api/v1/customers", []byte(in))); out != in {
		t.Fatalf("non-webhook body changed: %s", out)
	}
	if out := string(withholdWebhookTokens("/outgoingWebhookApi/api/v1/subscriptions", []byte("not json"))); out != "not json" {
		t.Fatalf("non-JSON body changed: %s", out)
	}
}

// TestWebhookTokenNeverLeavesTheClient drives the real request path: the
// response a caller receives (and so the cache, sync and MCP) has no token.
func TestWebhookTokenNeverLeavesTheClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"channelId":"ch-1","token":"test-secret-wire"}]`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key"}, 5*time.Second, 0)
	c.NoCache = true
	data, err := c.Get(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "test-secret-wire") || !strings.Contains(string(data), WebhookTokenWithheld) {
		t.Fatalf("token reached the caller: %s", data)
	}
}

// TestWebhookTokenWithheldFromDryRunPreview: the --dry-run request preview of a
// subscription write prints [withheld], not the delivery token.
func TestWebhookTokenWithheldFromDryRunPreview(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	c := New(&config.Config{BaseURL: "http://127.0.0.1:1", AuthHeaderVal: "test-key"}, time.Second, 0)
	_, _, derr := c.dryRun("POST", "http://127.0.0.1:1/outgoingWebhookApi/api/v1/subscriptions", "/outgoingWebhookApi/api/v1/subscriptions", nil, []byte(`{"url":"https://hooks.example.com/x","token":"test-secret-preview"}`), nil, "test-key")
	w.Close()
	os.Stderr = saved
	if derr != nil {
		t.Fatal(derr)
	}
	out, _ := io.ReadAll(r)
	if strings.Contains(string(out), "test-secret-preview") || !strings.Contains(string(out), WebhookTokenWithheld) {
		t.Fatalf("dry-run preview leaked the token: %s", out)
	}
}

// TestWithheldPlaceholderIsNeverWritten: copying a listed subscription (token
// [withheld]) into an update must not overwrite the real token.
func TestWithheldPlaceholderIsNeverWritten(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key"}, 5*time.Second, 0)
	_, _, err := c.Patch(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions/ch-1", map[string]any{"enabled": true, "token": WebhookTokenWithheld})
	if err == nil || hits != 0 {
		t.Fatalf("placeholder token sent: err=%v hits=%d", err, hits)
	}
	if _, _, err := c.Patch(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions/ch-1", map[string]any{"enabled": true}); err != nil || hits != 1 {
		t.Fatalf("an update without token must pass: err=%v hits=%d", err, hits)
	}
}

// TestWebhookTokenWithheldFromErrorBodies: an error answer that echoes a
// subscription token does not carry it into the returned error.
func TestWebhookTokenWithheldFromErrorBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"exists","existing":{"channelId":"ch-1","token":"test-secret-error"}}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key"}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", nil)
	if err == nil || strings.Contains(err.Error(), "test-secret-error") || !strings.Contains(err.Error(), WebhookTokenWithheld) {
		t.Fatalf("token reached the error: %v", err)
	}
}

// TestWebhookTokenEchoedInErrorTextIsMasked: an error answer that quotes the
// request's token inside a message (not a token key) has it masked.
func TestWebhookTokenEchoedInErrorTextIsMasked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"token test-secret-echo is too short"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key"}, 5*time.Second, 0)
	_, _, err := c.Post(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", map[string]any{"url": "https://hooks.example.com/x", "token": "test-secret-echo"})
	if err == nil || strings.Contains(err.Error(), "test-secret-echo") {
		t.Fatalf("echoed request token reached the error: %v", err)
	}
	// A one-letter token is not masked, or every "a" in the error would be.
	_, _, err = c.Post(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", map[string]any{"url": "https://hooks.example.com/x", "token": "a"})
	if err == nil || !strings.Contains(err.Error(), `"message"`) {
		t.Fatalf("a short token garbled the error: %v", err)
	}
}

// TestWebhookTokenEchoedEscapedOrAtTheCutIsMasked: a token echoed with JSON
// escapes, or straddling the error-text cut, does not reach the error.
func TestWebhookTokenEchoedEscapedOrAtTheCutIsMasked(t *testing.T) {
	answer := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key"}, 5*time.Second, 0)
	post := func(token string) string {
		_, _, err := c.Post(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", map[string]any{"url": "https://hooks.example.com/x", "token": token})
		if err == nil {
			t.Fatal("want an error")
		}
		return err.Error()
	}
	answer = `{"message":"token test\"SECRET\u002Becho\/x is invalid"}`
	if msg := post(`test"SECRET+echo/x`); strings.Contains(msg, "SECRET") {
		t.Fatalf("escaped echo leaked: %s", msg)
	}
	// The cut keeps the first maxErrorBodyBytes bytes; start the token 12 bytes before it.
	answer = `{"message":"` + strings.Repeat("x", maxErrorBodyBytes-len(`{"message":"`)-12) + `test-secret-straddle"}`
	if msg := post("test-secret-straddle"); strings.Contains(msg, "test-secret") {
		t.Fatalf("token at the cut leaked a prefix: %s", msg[len(msg)-80:])
	}
}

// TestWebhookTokenEchoedEncodedIsMasked: a token echoed URL-encoded, inside an
// HTML error title, or in a JSON key is masked; trailing content after a JSON
// answer survives the rewrite.
func TestWebhookTokenEchoedEncodedIsMasked(t *testing.T) {
	answer, ctype := "", "application/json"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ctype)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key"}, 5*time.Second, 0)
	post := func() string {
		_, _, err := c.Post(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", map[string]any{"url": "https://hooks.example.com/x", "token": "abcd+efgh/ijkl"})
		if err == nil {
			t.Fatal("want an error")
		}
		return err.Error()
	}
	for _, tc := range []struct{ ctype, answer string }{
		{"application/json", `{"message":"bad url https://h/x?token=abcd%2Befgh%2Fijkl"}`},
		{"text/html", `<html><head><title>Invalid token abcd&#43;efgh&#47;ijkl</title></head><body>x</body></html>`},
		{"application/json", `{"abcd\u002Befgh\/ijkl":"x"}`},
	} {
		ctype, answer = tc.ctype, tc.answer
		if msg := post(); strings.Contains(msg, "efgh") {
			t.Fatalf("%s echo leaked: %s", tc.ctype, msg)
		}
	}
	ctype, answer = "application/json", `{"message":"abcd+efgh/ijkl is invalid"} {"second":"error"}`
	if msg := post(); strings.Contains(msg, "efgh") || !strings.Contains(msg, echoWithheldText) {
		t.Fatalf("an echo followed by trailing content must be withheld whole: %s", msg)
	}
}

// TestPartnerKeyEchoedEscapedOrAtTheCutIsMasked: the class, not one secret:
// the partner key echoed JSON-escaped, or straddling the error-text cut, on a
// non-webhook API is masked too.
func TestPartnerKeyEchoedEscapedOrAtTheCutIsMasked(t *testing.T) {
	answer := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	key := "test-key/SECRET+0000"
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: key, SaasAlertsApiKey: key}, 5*time.Second, 0)
	c.NoCache = true
	get := func() string {
		_, err := c.Get(context.Background(), "/reportApi/api/v1/customers", nil)
		if err == nil {
			t.Fatal("want an error")
		}
		return err.Error()
	}
	answer = `{"message":"key test-key\/SECRET\u002B0000 rejected"}`
	if msg := get(); strings.Contains(msg, "SECRET") {
		t.Fatalf("escaped key echo leaked: %s", msg)
	}
	answer = `{"message":"` + strings.Repeat("x", maxErrorBodyBytes-len(`{"message":"`)-12) + key + `"}`
	if msg := get(); strings.Contains(msg, "test-key/SEC") {
		t.Fatalf("key at the cut leaked a prefix: %s", msg[len(msg)-60:])
	}
}

// TestPartnerKeyInA200HTMLTitleIsMasked: a 200 HTML page whose title quotes
// the partner key does not carry it into the "expected JSON" error.
func TestPartnerKeyInA200HTMLTitleIsMasked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Signed in as test-key-SECRET-0000</title></head><body>portal</body></html>`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key-SECRET-0000", SaasAlertsApiKey: "test-key-SECRET-0000"}, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/reportApi/api/v1/customers", nil)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("key in an HTML title leaked: %v", err)
	}
}

// TestWebhookTokenWithheldFromEveryJSONValue: an answer holding several JSON
// values has every token withheld and no record dropped; text that is not
// JSON but carries a token key is withheld whole.
func TestWebhookTokenWithheldFromEveryJSONValue(t *testing.T) {
	const path = "/outgoingWebhookApi/api/v1/subscriptions"
	out := string(withholdWebhookTokens(path, []byte("{\"id\":1}\n{\"id\":2,\"token\":\"test-secret-second\"}")))
	if strings.Contains(out, "test-secret-second") || !strings.Contains(out, `"id":1`) || !strings.Contains(out, `"id":2`) {
		t.Fatalf("multi-value answer: %s", out)
	}
	out = string(withholdWebhookTokens(path, []byte(`{"id":1} {"token":"test-secret-broken"`)))
	if strings.Contains(out, "test-secret-broken") {
		t.Fatalf("broken JSON with a token key leaked: %s", out)
	}
	if out := string(withholdWebhookTokens(path, []byte(`"token" is required`))); out != `"token" is required` {
		t.Fatalf("a validation message naming the token field was withheld: %s", out)
	}
	if out := string(withholdWebhookTokens(path, []byte("Invalid token"))); out != "Invalid token" {
		t.Fatalf("plain text without a token key changed: %s", out)
	}
}

// TestWebhookTokenBackstopCatchesWhatTheWalkMisses: a duplicate key whose last
// copy is null, or an escaped key name in text that is not JSON, still
// withholds the whole answer.
func TestWebhookTokenBackstopCatchesWhatTheWalkMisses(t *testing.T) {
	const path = "/outgoingWebhookApi/api/v1/subscriptions"
	bs := string(rune(92))
	for _, body := range []string{
		`{"token":"test-secret-dup","token":null}`,
		`{"items":[{"id":1,"` + bs + `u0074oken":"test-secret-escaped"`,
		`{"id":1,"Token": "test-secret-cased"} trailing`,
	} {
		if out := string(withholdWebhookTokens(path, []byte(body))); strings.Contains(out, "test-secret") {
			t.Fatalf("%s leaked: %s", body, out)
		}
	}
	twice := `{"payload":"{` + bs + `"a` + bs + `":` + bs + `"{` + bs + bs + bs + `"token` + bs + bs + bs + `":` + bs + bs + bs + `"test-secret-twice` + bs + bs + bs + `"}` + bs + `"}"}`
	if out := string(withholdWebhookTokens(path, []byte(twice))); strings.Contains(out, "test-secret") {
		t.Fatalf("twice-escaped token leaked: %s", out)
	}
	for _, body := range []string{`[{"channelId":"ch-1","token":null}]`, `[{"channelId":"ch-1"}]`,
		`{"message":"` + bs + `"token` + bs + `": must be a string"}`} {
		if out := string(withholdWebhookTokens(path, []byte(body))); out != body {
			t.Fatalf("%s changed: %s", body, out)
		}
	}
}

// TestWebhookTokenWithheldFromEveryAnswerShape: a stored token in a 2xx HTML
// page or a plain-text error on a Webhooks path never reaches the caller.
func TestWebhookTokenWithheldFromEveryAnswerShape(t *testing.T) {
	status, ctype, answer := 200, "", ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ctype)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key-0000", SaasAlertsApiKey: "test-key-0000"}, 5*time.Second, 0)
	c.NoCache = true
	for _, tc := range []struct {
		status        int
		ctype, answer string
	}{
		{200, "text/html", `<html><head><title>{"token":"test-secret-html"}</title></head><body>x</body></html>`},
		{400, "text/plain", `bad subscription {"token":"test-secret-text"}`},
		{400, "text/plain", `Error: bad subscription { channelId: 'ch-1', token: 'test-secret-inspect' }`},
		{200, "text/html", `<html><head><title>&quot;token&quot;:&quot;test-secret-entity&quot;</title></head><body>x</body></html>`},
		{200, "text/html", `<html><head><title>"token":&nbsp;"test-secret-nbsp"</title></head><body>x</body></html>`},
		{400, "text/html", `<html><head><title>"token":<b>"test-secret-tag"</b></title></head><body>x</body></html>`},
		{400, "text/plain", "bad {\"token\":" + string(rune(0xA0)) + "\"test-secret-nbsp-text\"}"},
		{400, "text/html", `<html><head><title>&quot;token&quot;:&quot;test-secret-entity&quot;</title></head><body>x</body></html>`},
	} {
		status, ctype, answer = tc.status, tc.ctype, tc.answer
		data, err := c.Get(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions", nil)
		got := string(data)
		if err != nil {
			got += err.Error()
		}
		if strings.Contains(got, "test-secret") {
			t.Fatalf("%d %s leaked: %s", tc.status, tc.ctype, got)
		}
	}
}

// TestWebhookHTMLPageStaysAnErrorWhenItsTokenIsWithheld: withholding a token
// from a 2xx HTML page must not turn the page into a successful answer.
func TestWebhookHTMLPageStaysAnErrorWhenItsTokenIsWithheld(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Sign in</title><script>window.cfg={"token":"test-secret-csrf"}</script></head><body>x</body></html>`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "test-key-0000", SaasAlertsApiKey: "test-key-0000"}, 5*time.Second, 0)
	_, _, err := c.Patch(context.Background(), "/outgoingWebhookApi/api/v1/subscriptions/ch-1", map[string]any{"enabled": true})
	if err == nil || !IsAnswerUnreadable(err) || strings.Contains(err.Error(), "test-secret") || !strings.Contains(err.Error(), webhookWithheldText) {
		t.Fatalf("a 2xx HTML page with a token must stay an unreadable-answer error, withheld: %v", err)
	}
}

// TestWebhookBackstopKeepsFreeTextAnswers: on a JSON answer only a JSON-shaped
// token key counts, so free text that mentions a token keeps the list intact.
func TestWebhookBackstopKeepsFreeTextAnswers(t *testing.T) {
	const path = "/outgoingWebhookApi/api/v1/queue"
	bs := string(rune(92))
	for _, body := range []string{
		`[{"channelId":"ch-1","failedAttempts":[{"message":"401 Unauthorized: missing token:"}],"url":"https://h/x?token=abc"}]`,
		`[{"jointDesc":"Refresh token: 'revoked' for user"},{"jointDesc":"OAuth token: ` + bs + `"Outlook Mobile` + bs + `" granted"}]`,
	} {
		if out := string(withholdWebhookTokens(path, []byte(body))); out != body {
			t.Fatalf("free text was withheld:\n in: %s\nout: %s", body, out)
		}
	}
}
