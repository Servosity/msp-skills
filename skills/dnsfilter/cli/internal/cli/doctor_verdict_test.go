// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

func runDoctorJSON(t *testing.T, key string, h http.HandlerFunc) (map[string]any, []string) {
	t.Helper()
	testenv.Isolate(t)
	var auth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", key)
	cmd := RootCmd()
	cmd.SetArgs([]string{"doctor", "--json"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor: %v\n%s%s", err, out.String(), errOut.String())
	}
	report := map[string]any{}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out.String())
	}
	return report, auth
}

// A base_url aimed at a web UI (an app shell at /, 404 on every API path) or
// a host serving HTML at the probe path is not reported as a reachable API.
func TestDoctorAPIVerdictTracksReality(t *testing.T) {
	report, _ := runDoctorJSON(t, "k", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, "<!doctype html><html><body>app</body></html>")
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	})
	if api, _ := report["api"].(string); !strings.HasPrefix(api, "ERROR") || !strings.Contains(api, "not the DNSFilter API root") {
		t.Fatalf("wrong-base api verdict = %q, want an ERROR naming the base_url", api)
	}
	report, _ = runDoctorJSON(t, "k", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<!doctype html><html><head><title>Sign in</title></head><body>app</body></html>")
	})
	if api, _ := report["api"].(string); !strings.HasPrefix(api, "WARN") {
		t.Fatalf("web page api verdict = %q, want WARN", api)
	}
	report, auth := runDoctorJSON(t, "raw-key-123", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"id":"1","attributes":{"email":"tech@example.com"}}}`)
	})
	if report["api"] != "reachable" || report["credentials"] != "valid" || report["auth"] != "configured" {
		t.Fatalf("healthy report = api %v, credentials %v, auth %v", report["api"], report["credentials"], report["auth"])
	}
	// DNSFilter's Authorization header value is the API key itself.
	for _, a := range auth {
		if a != "raw-key-123" {
			t.Fatalf("Authorization = %q, want the raw API key with no scheme prefix", a)
		}
	}
}

// A key saved with a pasted "Bearer " prefix would be sent verbatim; doctor
// says so instead of reporting a clean configuration.
func TestDoctorWarnsOnSchemePrefixedKey(t *testing.T) {
	report, _ := runDoctorJSON(t, "Bearer raw-key-123", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"Not Authorized"}`)
	})
	if a, _ := report["auth"].(string); !strings.HasPrefix(a, "WARN") || !strings.Contains(a, `"Bearer "`) {
		t.Fatalf("auth verdict = %q, want a WARN naming the Bearer prefix", a)
	}
	for key, want := range map[string]string{"Bearer x": "Bearer", "token x": "Token", "Bearerx": "", "abc": "", "Token\tx": "Token"} {
		if got := apiKeySchemePrefix(key); got != want {
			t.Errorf("apiKeySchemePrefix(%q) = %q, want %q", key, got, want)
		}
	}
}

// doctor never answers from the response cache: after a healthy run, a host
// that now 404s, or refuses connections, is reported as such.
func TestDoctorProbesBypassResponseCache(t *testing.T) {
	testenv.Isolate(t)
	var gone atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gone.Load() {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"not found"}`)
			return
		}
		fmt.Fprint(w, `{"data":{"id":"1","attributes":{"email":"tech@example.com"}}}`)
	}))
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "k")
	run := func() map[string]any {
		cmd := RootCmd()
		cmd.SetArgs([]string{"doctor", "--json"})
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("doctor: %v\n%s%s", err, out.String(), errOut.String())
		}
		report := map[string]any{}
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatalf("doctor --json: %v\n%s", err, out.String())
		}
		return report
	}
	if r := run(); r["api"] != "reachable" || r["credentials"] != "valid" {
		t.Fatalf("healthy run: api=%v credentials=%v", r["api"], r["credentials"])
	}
	gone.Store(true)
	if r := run(); !strings.HasPrefix(fmt.Sprint(r["api"]), "ERROR") || r["credentials"] == "valid" {
		t.Fatalf("after the host began to 404: api=%v credentials=%v (served from cache?)", r["api"], r["credentials"])
	}
	gone.Store(false)
	run()
	srv.Close()
	if r := run(); !strings.HasPrefix(fmt.Sprint(r["api"]), "unreachable") {
		t.Fatalf("after the host went away: api=%v (served from cache?)", r["api"])
	}
}
