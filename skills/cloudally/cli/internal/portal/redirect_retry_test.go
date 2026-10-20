// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// trapServer is a second origin that must never receive a portal request.
type trapServer struct {
	srv  *httptest.Server
	hits atomic.Int32
	mu   sync.Mutex
	csrf []string
	body []string
}

func newTrapServer(t *testing.T) *trapServer {
	t.Helper()
	tr := &trapServer{}
	tr.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tr.hits.Add(1)
		raw, _ := io.ReadAll(r.Body)
		tr.mu.Lock()
		tr.csrf = append(tr.csrf, r.Header.Get("X-CSRF-TOKEN"))
		tr.body = append(tr.body, string(raw))
		tr.mu.Unlock()
		// Answer like a working portal, so following the redirect would
		// look like success to the caller.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"ok":true}}`))
	}))
	t.Cleanup(tr.srv.Close)
	return tr
}

// redirectingPortal serves a working handoff and CSRF shell, then answers
// every service call with a redirect to target (a full origin) at the same
// path and query.
func redirectingPortal(t *testing.T, status int, target string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(bootstrapPath, func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "authed"})
		_, _ = w.Write([]byte(shellHTML))
	})
	mux.HandleFunc(servicePath, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target+r.URL.RequestURI(), status)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// A service call that the portal redirects to another origin must not reach
// it: the CSRF header (and, on 307/308, the body) would go with it.
func TestServiceCallRefusesCrossOriginRedirect(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, method := range []string{"Call", "CallRead", "Get"} {
			t.Run(fmt.Sprintf("%d/%s", status, method), func(t *testing.T) {
				trap := newTrapServer(t)
				srv := redirectingPortal(t, status, trap.srv.URL)
				s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
				if err != nil {
					t.Fatalf("Open: %v", err)
				}
				switch method {
				case "Call":
					_, err = s.Call(context.Background(), "startRestoreOrDownload", map[string]any{"type": "EXPORT"})
				case "CallRead":
					_, err = s.CallRead(context.Background(), "getSnapshotsCalendar", map[string]any{"taskId": "t"})
				case "Get":
					_, err = s.Get(context.Background(), "getDownloadUrl", url.Values{"taskId": {"x"}})
				}
				if !errors.Is(err, ErrRedirectRefused) {
					t.Fatalf("want ErrRedirectRefused, got %v", err)
				}
				// The create reached the server before the redirect, so its
				// outcome is unknown; a read's is not.
				if got := errors.Is(err, ErrOutcomeUnknown); got != (method == "Call") {
					t.Fatalf("ErrOutcomeUnknown = %v for %s: %v", got, method, err)
				}
				if n := trap.hits.Load(); n != 0 {
					t.Fatalf("the other origin received %d request(s) (csrf %q, body %q)", n, trap.csrf, trap.body)
				}
				if strings.Contains(err.Error(), "csrf-value-1") {
					t.Fatalf("error leaked the CSRF token: %v", err)
				}
			})
		}
	}
}

// The handoff GET carries the single-use token in its query; a redirect to
// another origin must not be followed, and the error must not echo it.
func TestOpenRefusesCrossOriginHandoffRedirect(t *testing.T) {
	trap := newTrapServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc(bootstrapPath, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, trap.srv.URL+r.URL.RequestURI(), http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "secret-handoff")
	if !errors.Is(err, ErrRedirectRefused) {
		t.Fatalf("want ErrRedirectRefused, got %v", err)
	}
	if n := trap.hits.Load(); n != 0 {
		t.Fatalf("the other origin received %d request(s)", n)
	}
	if strings.Contains(err.Error(), "secret-handoff") {
		t.Fatalf("error leaked the handoff token: %v", err)
	}
	if !strings.Contains(err.Error(), "CLOUDALLY_PORTAL_URL") {
		t.Fatalf("error should name the override: %v", err)
	}
}

// A redirect that stays on the portal origin is followed, with the CSRF
// header, so the handoff and any same-origin hop keep working.
func TestServiceCallFollowsSameOriginRedirect(t *testing.T) {
	var landed atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc(bootstrapPath, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get(handoffParam) != "" {
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "authed"})
			http.Redirect(w, r, bootstrapPath, http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(shellHTML))
	})
	mux.HandleFunc(servicePath+"getMoved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, servicePath+"getHere", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc(servicePath+"getHere", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-CSRF-TOKEN") != "csrf-value-1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		landed.Add(1)
		_, _ = w.Write([]byte(`{"success":true,"data":{"here":true}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open through a same-origin handoff redirect: %v", err)
	}
	raw, err := s.CallRead(context.Background(), "getMoved", map[string]any{})
	if err != nil {
		t.Fatalf("CallRead through a same-origin redirect: %v", err)
	}
	if string(raw) != `{"here":true}` || landed.Load() != 1 {
		t.Fatalf("data = %s, landed = %d", raw, landed.Load())
	}
}

func TestRedirectPolicyRules(t *testing.T) {
	mustURL := func(raw string) *url.URL {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	req := func(raw string) *http.Request { return &http.Request{URL: mustURL(raw)} }
	base := mustURL("https://app.cloudally.com")

	cases := []struct {
		name   string
		via    []string
		target string
		refuse bool
	}{
		{"same origin", []string{"https://app.cloudally.com/newapp?accessToken=x"}, "https://app.cloudally.com/newapp", false},
		{"same origin explicit port", []string{"https://app.cloudally.com/a"}, "https://APP.cloudally.com:443/b", false},
		{"other host", []string{"https://app.cloudally.com/application/service/x"}, "https://evil.example/application/service/x", true},
		{"same host other port", []string{"https://app.cloudally.com/a"}, "https://app.cloudally.com:8443/a", true},
		{"https to http on the portal", []string{"https://app.cloudally.com/a"}, "http://app.cloudally.com/a", true},
		{"second hop leaves the portal", []string{"https://app.cloudally.com/a", "https://app.cloudally.com/b"}, "https://evil.example/c", true},
		{"storage link changes host", []string{"https://bucket.s3.amazonaws.com/k?sig=1"}, "https://bucket.s3.us-west-2.amazonaws.com/k?sig=1", false},
		{"storage link drops https", []string{"https://bucket.s3.amazonaws.com/k?sig=1"}, "http://bucket.s3.amazonaws.com/k?sig=1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var via []*http.Request
			for _, v := range tc.via {
				via = append(via, req(v))
			}
			nextCalled := false
			policy := redirectPolicy(base, func(*http.Request, []*http.Request) error {
				nextCalled = true
				return nil
			})
			err := policy(req(tc.target), via)
			if tc.refuse {
				if !errors.Is(err, ErrRedirectRefused) {
					t.Fatalf("want ErrRedirectRefused, got %v", err)
				}
				if nextCalled {
					t.Fatal("caller policy ran after a refusal")
				}
				if strings.Contains(err.Error(), "sig=1") || strings.Contains(err.Error(), "accessToken") {
					t.Fatalf("error echoed a path or query: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("want follow, got %v", err)
			}
			if !nextCalled {
				t.Fatal("caller policy was skipped")
			}
		})
	}

	var long []*http.Request
	for i := 0; i < maxRedirects; i++ {
		long = append(long, req("https://app.cloudally.com/hop"))
	}
	if err := redirectPolicy(base, nil)(req("https://app.cloudally.com/hop"), long); !errors.Is(err, ErrRedirectRefused) {
		t.Fatalf("want a refusal after %d redirects, got %v", maxRedirects, err)
	}
}

// throttledSession opens a session whose service calls are answered by fn.
func throttledSession(t *testing.T, fn roundTripFunc) *Session {
	t.Helper()
	f := newFakePortal(t, "tok")
	f.handler = func(string, []byte, string) string { return `{"success":true}` }
	srv := f.server()
	t.Cleanup(srv.Close)
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.hc.Transport = fn
	return s
}

// A 503 does not say whether the server acted. A call that creates a job is
// therefore sent exactly once; one download start must never become four.
func TestCallNeverRetriesAThrottledCreate(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var attempts atomic.Int32
			s := throttledSession(t, func(*http.Request) (*http.Response, error) {
				attempts.Add(1)
				return jsonResponse(status, `{}`, "0"), nil
			})
			_, err := s.Call(context.Background(), startOperation, map[string]any{"type": jobTypeExport})
			if !errors.Is(err, ErrThrottled) {
				t.Fatalf("want ErrThrottled, got %v", err)
			}
			if !strings.Contains(err.Error(), "not retried") {
				t.Fatalf("error should say it was not retried: %v", err)
			}
			if n := attempts.Load(); n != 1 {
				t.Fatalf("create sent %d times, want 1", n)
			}
		})
	}
}

// StartDownload is the create path the reviewer drove: one 503 used to turn
// into four startRestoreOrDownload jobs.
func TestStartDownloadSendsOneCreateOn503(t *testing.T) {
	var creates atomic.Int32
	s := throttledSession(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, servicePath+startOperation) {
			creates.Add(1)
		}
		return jsonResponse(http.StatusServiceUnavailable, `{}`, "0"), nil
	})
	err := s.StartDownload(context.Background(), DownloadRequest{
		Entity:  Entity{TaskID: "task-1", AccountID: "jane.doe@contoso.com"},
		Source:  "O365MAIL",
		DateRaw: 20221231120000,
		Format:  FormatZip,
	})
	if !errors.Is(err, ErrThrottled) {
		t.Fatalf("want ErrThrottled, got %v", err)
	}
	if n := creates.Load(); n != 1 {
		t.Fatalf("startRestoreOrDownload sent %d times, want 1", n)
	}
}

// Reads stay resilient: a GET and a CallRead are retried after throttling.
func TestReadsAreStillRetriedAfterThrottling(t *testing.T) {
	for _, method := range []string{"CallRead", "Get"} {
		t.Run(method, func(t *testing.T) {
			var attempts atomic.Int32
			s := throttledSession(t, func(*http.Request) (*http.Response, error) {
				if attempts.Add(1) <= 2 {
					return jsonResponse(http.StatusServiceUnavailable, `{}`, "1"), nil
				}
				return jsonResponse(http.StatusOK, `{"success":true,"data":{"ok":true}}`, ""), nil
			})
			var err error
			if method == "Get" {
				_, err = s.Get(context.Background(), "getDownloadInfo", url.Values{"taskId": {"x"}})
			} else {
				_, err = s.CallRead(context.Background(), "getRestoreDownloadStatus", map[string]any{})
			}
			if err != nil {
				t.Fatalf("%s: %v", method, err)
			}
			if n := attempts.Load(); n != 3 {
				t.Fatalf("attempts = %d, want 3", n)
			}
		})
	}
}

// A known read reached through Call keeps its throttle retry, so the job
// listing that download reconciliation depends on survives one 503.
func TestCallRetriesKnownReadOperations(t *testing.T) {
	var attempts atomic.Int32
	s := throttledSession(t, func(*http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return jsonResponse(http.StatusServiceUnavailable, `{}`, "1"), nil
		}
		return jsonResponse(http.StatusOK, `{"success":true,"data":[]}`, ""), nil
	})
	if _, err := s.DownloadJobs(context.Background()); err != nil {
		t.Fatalf("DownloadJobs after one 503: %v", err)
	}
	if n := attempts.Load(); n != 2 {
		t.Fatalf("attempts = %d, want 2", n)
	}
}

// A create that fails in a way that does not prove it was not acted on is
// marked ErrOutcomeUnknown, so a caller reconciles instead of resending.
func TestCreateFailuresReportAnUnknownOutcome(t *testing.T) {
	cases := []struct {
		name    string
		rt      roundTripFunc
		unknown bool
	}{
		{"503", func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusServiceUnavailable, `{}`, "0"), nil
		}, true},
		{"502", func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusBadGateway, `{}`, ""), nil
		}, true},
		{"transport error", func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection reset")
		}, true},
		{"200 unreadable", func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{"success":tr`, ""), nil
		}, true},
		{"400", func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusBadRequest, `{}`, ""), nil
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := throttledSession(t, tc.rt)
			_, err := s.Call(context.Background(), startOperation, map[string]any{"type": jobTypeExport})
			if err == nil {
				t.Fatal("want an error")
			}
			if got := errors.Is(err, ErrOutcomeUnknown); got != tc.unknown {
				t.Fatalf("ErrOutcomeUnknown = %v, want %v (%v)", got, tc.unknown, err)
			}
			// The same failure on a read is an ordinary error. (A throttled
			// read is retried, which the retry tests cover.)
			if tc.name == "503" {
				return
			}
			if _, rerr := s.CallRead(context.Background(), "getRestoreDownloadStatus", map[string]any{}); errors.Is(rerr, ErrOutcomeUnknown) {
				t.Fatalf("a read was marked outcome-unknown: %v", rerr)
			}
		})
	}
}
