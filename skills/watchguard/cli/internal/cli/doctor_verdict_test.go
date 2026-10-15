// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// doctorStub serves a token endpoint and answers every other request by mode.
func doctorStub(t *testing.T, mode string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"DOCTORCACHESENTINEL_0123456789","expires_in":3600}`))
			return
		}
		switch mode {
		case "healthy":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accountId":"ACC-1","name":"MSP","results":[]}`))
		case "expired":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		case "wrong-base":
			if r.URL.Path == "/" {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(`<!doctype html><html><head><title>Portal</title></head><body>header was ` + r.Header.Get("Authorization") + `</body></html>`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		case "empty":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func doctorEnv(t *testing.T, srv *httptest.Server, accountID string) string {
	t.Helper()
	isolateAuthEnv(t)
	home := t.TempDir()
	t.Setenv("WATCHGUARD_HOME", home)
	t.Setenv("WATCHGUARD_BASE_URL", srv.URL)
	t.Setenv("WATCHGUARD_TOKEN_URL", srv.URL+"/oauth/token")
	t.Setenv("WATCHGUARD_CLIENT_ID", "rw")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "rwsec")
	t.Setenv("WATCHGUARD_API_KEY", "KEY")
	t.Setenv("WATCHGUARD_ACCOUNT_ID", accountID)
	return home
}

func positiveVerdict(v any) bool {
	s := strings.ToLower(strings.TrimSpace(strings.TrimSpace(func() string { x, _ := v.(string); return x }())))
	if s == "" || strings.HasPrefix(s, "error") || strings.HasPrefix(s, "warn") || strings.HasPrefix(s, "info") {
		return false
	}
	for _, p := range []string{"not verified", "not configured", "unreachable", "invalid", "rejected", "scope-limited", "skipped", "not there", "error"} {
		if strings.Contains(s, p) {
			return false
		}
	}
	return true
}

// hand-fix (handfixes.json doctor-verdict-tracks-reality, ported from
// connectsecure): doctor verifies the credential with an authenticated GET.
// It used to print "present, not verified" in every state and call an HTML
// app shell at / a reachable API.
func TestDoctorVerdictTracksReality(t *testing.T) {
	for _, accountID := range []string{"ACC-1", ""} {
		for _, tc := range []struct {
			mode           string
			credsPositive  bool
			apiPositive    bool
			wantCredsShape string
		}{
			{"healthy", true, true, "valid (verified with GET"},
			{"expired", false, true, "ERROR rejected (HTTP 401"},
			{"wrong-base", false, false, "ERROR not verified (HTTP 404"},
			{"empty", false, true, "ERROR not verified: GET"},
		} {
			srv := doctorStub(t, tc.mode)
			doctorEnv(t, srv, accountID)
			doc := doctorJSON(t)
			creds, api := doc["credentials"], doc["api"]
			if positiveVerdict(creds) != tc.credsPositive || !strings.HasPrefix(func() string { s, _ := creds.(string); return s }(), tc.wantCredsShape) {
				t.Errorf("account=%q %s: credentials=%q, want positive=%v starting %q", accountID, tc.mode, creds, tc.credsPositive, tc.wantCredsShape)
			}
			if positiveVerdict(api) != tc.apiPositive {
				t.Errorf("account=%q %s: api=%q, want positive=%v", accountID, tc.mode, api, tc.apiPositive)
			}
			wantPath := doctorTokenProbePath
			if accountID != "" {
				wantPath = doctorAccountProbePath
			}
			if s, _ := creds.(string); !strings.Contains(s, wantPath) {
				t.Errorf("account=%q %s: probe %q, want %s", accountID, tc.mode, s, wantPath)
			}
		}
	}
}

// hand-fix (handfixes.json doctor-html-probe-uncached, ported from
// connectsecure): doctor's HTML-accepting `GET /` probe skips the JSON echo
// guard, and it wrote the page (here echoing the bearer token) into the
// on-disk response cache.
func TestDoctorWritesNoCache(t *testing.T) {
	srv := doctorStub(t, "wrong-base")
	home := doctorEnv(t, srv, "ACC-1")
	_ = doctorJSON(t)
	_ = filepath.WalkDir(filepath.Join(home, "cache"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, _ := os.ReadFile(p); strings.Contains(string(b), "DOCTORCACHESENTINEL") {
			t.Errorf("doctor wrote the echoed bearer token to the response cache: %s", p)
		}
		return nil
	})
}
