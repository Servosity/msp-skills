// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "doctor-verdict-tracks-reality", "env-api-key-wins" and
// "dry-run-never-prints-a-credential" entries.

package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/store"
)

// r18DoctorProbe serves /v1/current_user with the given status, content
// type and body, and returns doctor --json's report and the --fail-on warn
// result.
func r18DoctorProbe(t *testing.T, status int, contentType, body string) (map[string]any, error) {
	t.Helper()
	testenv.Isolate(t)
	t.Setenv("DNSFILTER_API_KEY", "raw-key-r18")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	return runR17JSON(t, "doctor", "--json", "--fail-on", "warn")
}

// A /v1/current_user answer below 400 is proof of valid credentials only
// when it is the current-user record: a 304 with no body, a 200 plain-text
// "Access denied", an empty object or a null data member is not verified,
// and --fail-on warn trips on it.
func TestDoctorCurrentUserProbeNeedsARecord(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		status                  int
	}{
		{"304 with no body", "", "", http.StatusNotModified},
		{"200 plain-text Access denied", "text/plain", "Access denied", http.StatusOK},
		{"200 empty object", "application/json", "{}", http.StatusOK},
		{"200 null data", "application/json", `{"data":null}`, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := r18DoctorProbe(t, tc.status, tc.contentType, tc.body)
			if creds, _ := report["credentials"].(string); creds == "valid" || !strings.HasPrefix(creds, "WARN not verified") {
				t.Fatalf("credentials verdict = %q, want WARN not verified", creds)
			}
			if err == nil {
				t.Fatal("doctor --fail-on warn passed on an unverified key")
			}
		})
	}
	report, err := r18DoctorProbe(t, http.StatusOK, "application/json", `{"data":{"id":"7","type":"users","attributes":{"email":"tech@example.com"}}}`)
	if err != nil || report["credentials"] != "valid" {
		t.Fatalf("a current-user record: credentials %v (%v), want valid", report["credentials"], err)
	}
}

// A 403 whose body says the key lacks scope or permission is scope-limited
// (WARN), not an invalid key; a 403 that says the request was not
// authenticated is still invalid.
func TestDoctorForbiddenScopeIsNotInvalid(t *testing.T) {
	invalid := []string{
		`{"errors":[{"detail":"Invalid API token"}]}`,
		`{"detail":"Authentication credentials were not provided."}`,
		`{"error":"Authentication required"}`,
		`{"error":"API key is invalid"}`,
		`{"error":"Token has expired"}`,
		`{"error":"unauthenticated"}`,
		`{"error":"Unauthorized"}`,
		`{"error":"No API key provided"}`,
		`{"error":"API key required"}`,
	}
	scope := []string{
		`{"error":"This API key lacks the required scope for this endpoint"}`,
		`{"error":"API key does not have the required permissions"}`,
		`{"error":"You do not have permission to perform this action"}`,
		`{"error":"The authenticated user does not have access to this organization"}`,
		`{"error":"Unauthorized to access this organization"}`,
	}
	for _, body := range invalid {
		if !doctorForbiddenAuthFailure.MatchString(body) {
			t.Errorf("not read as an auth failure: %s", body)
		}
	}
	for _, body := range scope {
		if doctorForbiddenAuthFailure.MatchString(body) {
			t.Errorf("a permission refusal read as an invalid key: %s", body)
		}
	}
	t.Setenv(cliutil.DogfoodEnvVar, "1")
	report, err := r18DoctorProbe(t, http.StatusForbidden, "application/json", scope[0])
	if creds, _ := report["credentials"].(string); !strings.HasPrefix(creds, "WARN scope-limited") {
		t.Fatalf("403 scope body: credentials = %q, want WARN scope-limited", creds)
	}
	if doctorExitForFailOn("error", report) != nil {
		t.Fatalf("doctor --fail-on error tripped on a valid scope-limited key: %v (warn gate: %v)", report["credentials"], err)
	}
}

// A mirror whose latest walk did not finish is not fresh, even when its last
// successful timestamp is recent: the cache status says incomplete, names the
// collection, and --fail-on stale trips on it.
func TestDoctorIncompleteSyncIsNotFresh(t *testing.T) {
	testenv.Isolate(t, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	dbPath := defaultDBPath("dnsfilter-cli")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, step := range []func() error{
		func() error { return db.SaveSyncStateAt("organizations", "", 2, now) },
		func() error { return db.SaveSyncStateAt("networks", "", 3, now) },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	report := collectCacheReport(context.Background(), "")
	if report["status"] != "fresh" || doctorExitForFailOn("stale", map[string]any{"cache": report}) != nil {
		t.Fatalf("complete recent walks: cache status %v, want fresh and no --fail-on stale", report["status"])
	}
	// A capped walk after the complete one keeps the recent timestamp.
	if err := db.SaveSyncProgress("networks", "next-page", 5); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	report = collectCacheReport(context.Background(), "")
	if report["status"] != "incomplete" || fmt.Sprint(report["incomplete"]) != "[networks]" {
		t.Fatalf("after a capped walk: cache status %v (incomplete %v), want incomplete naming networks", report["status"], report["incomplete"])
	}
	if doctorExitForFailOn("stale", map[string]any{"cache": report}) == nil {
		t.Fatal("doctor --fail-on stale passed on an incomplete mirror")
	}
	var out strings.Builder
	renderCacheReport(&out, report)
	if !strings.Contains(out.String(), "Cache: incomplete") || !strings.Contains(out.String(), "networks: 5 rows") || !strings.Contains(out.String(), "latest sync incomplete") {
		t.Fatalf("human cache report:\n%s", out.String())
	}
}

// With a stored auth_header and DNSFILTER_API_KEY set, doctor sends the
// environment key and names the environment as the source.
func TestDoctorSendsTheKeyItNames(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("auth_header = \"TENANT-A-KEY\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sent = append(sent, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"id":"7"}}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "TENANT-B-KEY")
	report, err := runR17JSON(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if report["auth_source"] != "env:DNSFILTER_API_KEY" || len(sent) == 0 {
		t.Fatalf("auth_source %v, %d requests", report["auth_source"], len(sent))
	}
	for _, got := range sent {
		if got != "TENANT-B-KEY" {
			t.Fatalf("doctor named env:DNSFILTER_API_KEY but sent %q", got)
		}
	}
}

// `enterprise-connections create --stdin --dry-run` previews the request
// without printing the client secret piped in on stdin.
func TestDryRunStdinBodyNeverPrintsTheSecret(t *testing.T) {
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("DNSFILTER_API_KEY", "R18-DRYRUN-KEY")
	in := filepath.Join(home, "body.json")
	if err := os.WriteFile(in, []byte(`{"organization_id":1,"strategy":"oidc","options":{"client_secret":"R18-LIVE-SECRET","client_id":"r18-client"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	oldIn := os.Stdin
	os.Stdin = f
	defer func() {
		os.Stdin = oldIn
		_ = f.Close()
	}()
	stdout, stderr, err := executeArgs(t, "enterprise-connections", "create", "--stdin", "--dry-run")
	if err != nil {
		t.Fatalf("dry-run: %v\n%s%s", err, stdout, stderr)
	}
	if strings.Contains(stdout+stderr, "R18-LIVE-SECRET") || strings.Contains(stdout+stderr, "R18-DRYRUN-KEY") {
		t.Fatalf("--dry-run printed a credential:\n%s%s", stdout, stderr)
	}
	if !strings.Contains(stderr, `"client_secret": "****"`) || !strings.Contains(stderr, "r18-client") {
		t.Fatalf("--dry-run preview lost the request shape:\n%s", stderr)
	}
}
