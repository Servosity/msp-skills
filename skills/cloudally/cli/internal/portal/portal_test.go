// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// shellHTML is the SPA shell with the CSRF meta pair the real portal serves.
const shellHTML = `<!DOCTYPE html><html><head>` +
	`<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>` +
	`<meta name="_csrf" content="csrf-value-1"/>` +
	`<meta name="_csrf_header" content="X-CSRF-TOKEN"/>` +
	`</head><body><div id="root"></div></body></html>`

// fakePortal is a stand-in for the customer portal: it spends a one-time
// handoff token for a session cookie, serves the shell with a CSRF token,
// and answers service calls only for an authenticated session.
type fakePortal struct {
	t *testing.T
	// validToken is the handoff token that has not been spent yet.
	validToken string
	spent      bool
	// handler answers one POST service operation.
	handler func(op string, body []byte, reqID string) string
	// getHandler answers one GET service operation, when set.
	getHandler func(op string, query map[string][]string) string
	// calls records each service operation invoked.
	calls []string
	// sessionCSRF is handed out with the shell.
	csrf string
}

func newFakePortal(t *testing.T, token string) *fakePortal {
	return &fakePortal{t: t, validToken: token, csrf: "csrf-value-1"}
}

func (f *fakePortal) server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/newapp", func(w http.ResponseWriter, r *http.Request) {
		if tok := r.URL.Query().Get(handoffParam); tok != "" {
			if tok != f.validToken || f.spent {
				// A refused or re-used token still gets a session, but an
				// unauthenticated one, exactly like the real portal.
				http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "anon"})
				_, _ = w.Write([]byte(shellHTML))
				return
			}
			f.spent = true
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "authed"})
			http.Redirect(w, r, "/newapp", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(shellHTML))
	})
	mux.HandleFunc(servicePath, func(w http.ResponseWriter, r *http.Request) {
		op := strings.TrimPrefix(r.URL.Path, servicePath)
		if r.Method == http.MethodGet && f.getHandler != nil {
			c, cerr := r.Cookie("JSESSIONID")
			if cerr != nil || c.Value != "authed" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if got := r.Header.Get("X-CSRF-TOKEN"); got != f.csrf {
				f.t.Errorf("service %s: csrf header = %q", op, got)
				w.WriteHeader(http.StatusForbidden)
				return
			}
			f.calls = append(f.calls, op)
			_, _ = w.Write([]byte(f.getHandler(op, r.URL.Query())))
			return
		}
		c, err := r.Cookie("JSESSIONID")
		if err != nil || c.Value != "authed" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("X-CSRF-TOKEN"); got != f.csrf {
			f.t.Errorf("service %s: csrf header = %q, want %q", op, got, f.csrf)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			f.t.Errorf("service %s: content-type = %q", op, ct)
		}
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		f.calls = append(f.calls, op)
		_, _ = w.Write([]byte(f.handler(op, buf, r.URL.Query().Get("reqId"))))
	})
	return httptest.NewServer(mux)
}

func TestOpenSpendsTokenAndReadsCSRF(t *testing.T) {
	f := newFakePortal(t, "good-token")
	f.handler = func(op string, _ []byte, _ string) string {
		return `{"success":true,"data":{"id":"acct-1"}}`
	}
	srv := f.server()
	defer srv.Close()

	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "good-token")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if s.csrfToken != "csrf-value-1" || s.csrfHeader != "X-CSRF-TOKEN" {
		t.Fatalf("csrf = %q/%q", s.csrfHeader, s.csrfToken)
	}
	raw, err := s.Call(context.Background(), "getLoggedUser", map[string]any{})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if string(raw) != `{"id":"acct-1"}` {
		t.Fatalf("data = %s", raw)
	}
	if s.Calls() != 1 {
		t.Fatalf("Calls = %d", s.Calls())
	}
}

func TestOpenRejectsSpentToken(t *testing.T) {
	f := newFakePortal(t, "good-token")
	f.handler = func(string, []byte, string) string { return `{"success":true,"data":null}` }
	srv := f.server()
	defer srv.Close()

	if _, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "good-token"); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	// The second attempt reaches an unauthenticated session, so the service
	// layer refuses it.
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "good-token")
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	_, err = s.Call(context.Background(), "getLoggedUser", map[string]any{})
	if !errors.Is(err, ErrNoSession) {
		t.Fatalf("want ErrNoSession, got %v", err)
	}
}

func TestOpenWithoutCSRFFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>404</title></head></html>`))
	}))
	defer srv.Close()

	_, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if !errors.Is(err, ErrNoCSRF) {
		t.Fatalf("want ErrNoCSRF, got %v", err)
	}
}

func TestOpenRequiresToken(t *testing.T) {
	if _, err := Open(context.Background(), Config{BaseURL: "https://example.invalid"}, "  "); err == nil {
		t.Fatal("want an error for an empty handoff token")
	}
}

func TestCallRejectsNonIdentifierOperation(t *testing.T) {
	s := &Session{csrfHeader: "X-CSRF-TOKEN", csrfToken: "x"}
	for _, op := range []string{"../admin", "get/snapshots", "", "get snapshots"} {
		if _, err := s.Call(context.Background(), op, nil); err == nil {
			t.Fatalf("operation %q was accepted", op)
		}
	}
}

func TestCallSurfacesUnsuccessfulEnvelope(t *testing.T) {
	f := newFakePortal(t, "tok")
	f.handler = func(string, []byte, string) string {
		return `{"success":false,"message":"no such task"}`
	}
	srv := f.server()
	defer srv.Close()

	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = s.Call(context.Background(), "getSnapshotsCalendar", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "no such task") {
		t.Fatalf("want the envelope message, got %v", err)
	}
}

func TestCallPollsDeferredResult(t *testing.T) {
	f := newFakePortal(t, "tok")
	polls := 0
	f.handler = func(_ string, _ []byte, reqID string) string {
		if reqID == "" {
			return `{"success":true,"result":"WAIT","id":"req-7"}`
		}
		if reqID != "req-7" {
			f.t.Errorf("reqId = %q", reqID)
		}
		polls++
		if polls < 2 {
			return `{"success":true,"result":"WAIT","id":"req-7"}`
		}
		return `{"success":true,"data":[{"dateTime":20251231043522,"succeed":true}]}`
	}
	srv := f.server()
	defer srv.Close()

	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	raw, err := s.Call(ctx, "getSnapshotsCalendar", map[string]any{})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(string(raw), "20251231043522") {
		t.Fatalf("data = %s", raw)
	}
	if polls != 2 {
		t.Fatalf("polls = %d, want 2", polls)
	}
}

func TestParseCSRFMetaIgnoresOtherTags(t *testing.T) {
	page := `<meta name="robots" content=""/><meta content="tok-9" name="_csrf"/>` +
		`<meta name="_csrf_header" content="X-Other"/>`
	token, header := parseCSRFMeta([]byte(page))
	if token != "tok-9" || header != "X-Other" {
		t.Fatalf("got %q/%q", header, token)
	}
}

func TestScrubURLErrorDropsTheURL(t *testing.T) {
	inner := errors.New("connection refused")
	wrapped := &url.Error{
		Op:  "Get",
		URL: "https://portal/newapp?accessToken=secret-handoff-token",
		Err: inner,
	}
	if !strings.Contains(wrapped.Error(), "secret-handoff-token") {
		t.Fatal("fixture should carry the token before scrubbing")
	}
	err := scrubURLError(wrapped)
	if strings.Contains(err.Error(), "secret-handoff-token") {
		t.Fatalf("token leaked: %v", err)
	}
	if !errors.Is(err, inner) {
		t.Fatalf("scrubbed error lost its cause: %v", err)
	}
}

// TestOpenErrorOmitsTheHandoffToken covers the whole path: a transport
// failure during the exchange must not put the token in the error.
func TestOpenErrorOmitsTheHandoffToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // nothing is listening, so the exchange fails at the transport
	_, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "secret-handoff-token")
	if err == nil {
		t.Fatal("want a transport error")
	}
	if strings.Contains(err.Error(), "secret-handoff-token") {
		t.Fatalf("token leaked: %v", err)
	}
}
