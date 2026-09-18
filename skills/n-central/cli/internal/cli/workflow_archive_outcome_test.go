// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Issue #275 finding 3 (upstream cli-printing-press #4410): `workflow archive`
// reported len(resources) as resources_synced and exited 0 even when every
// resource failed, so an agent reading the summary believed a broken tenant
// was archived. These tests drive the real command against a local HTTP
// double with dummy credentials and pin the three outcomes: all failed ->
// non-zero exit and a count of 0; partial -> exit 0 with the TRUE count;
// successful-but-empty -> exit 0.

type archiveDouble struct {
	srv   *httptest.Server
	mu    sync.Mutex
	paths map[string]bool
}

// newArchiveDouble serves the archive's resource requests. decide receives the
// request path (auth handshakes are answered before it is consulted) and
// returns the JSON body to serve with 200, or "" for a 401.
func newArchiveDouble(t *testing.T, decide func(path string) string) *archiveDouble {
	t.Helper()
	d := &archiveDouble{paths: map[string]bool{}}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// N-central's two-step auth: the JWT exchange always succeeds here so
		// the outcomes below are decided by the data endpoints, which is what
		// the archive aggregates over.
		if strings.HasPrefix(r.URL.Path, "/auth/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tokens":{"access":{"token":"not-a-real-access","type":"Bearer","expirySeconds":3600},"refresh":{"token":"not-a-real-refresh","type":"Body","expirySeconds":90000}}}`))
			return
		}
		d.mu.Lock()
		d.paths[r.URL.Path] = true
		d.mu.Unlock()
		body := decide(r.URL.Path)
		if body == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func (d *archiveDouble) distinctPaths() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.paths)
}

// runArchive executes `workflow archive` through the real root command under
// a throwaway HOME with dummy credentials pointed at the double.
func runArchive(t *testing.T, d *archiveDouble, extra ...string) (stdout, stderr string, err error) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.Unsetenv("PRINTING_PRESS_VERIFY")
	t.Setenv("N_CENTRAL_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("N_CENTRAL_BASE_URL", d.srv.URL)
	t.Setenv("NCENTRAL_JWT", "not-a-real-jwt")
	var out, errb bytes.Buffer
	flags := &rootFlags{}
	root := newRootCmd(flags)
	root.SetOut(&out)
	root.SetErr(&errb)
	args := append([]string{"workflow", "archive", "--db", filepath.Join(home, "archive.db")}, extra...)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errb.String(), err
}

var archiveFailedRE = regexp.MustCompile(`workflow archive failed: 0 of (\d+) resource\(s\) archived`)

func resourcesSyncedFromJSON(t *testing.T, stdout string) int {
	t.Helper()
	var got struct {
		ResourcesSynced int `json:"resources_synced"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("archive --json stdout is not the summary object: %v\n%s", err, stdout)
	}
	return got.ResourcesSynced
}

func TestWorkflowArchiveFailsWhenEveryResourceErrors(t *testing.T) {
	d := newArchiveDouble(t, func(string) string { return "" })

	stdout, stderr, err := runArchive(t, d, "--json")
	if err == nil {
		t.Fatalf("all-failed archive unexpectedly succeeded; stdout=%s stderr=%s", stdout, stderr)
	}
	m := archiveFailedRE.FindStringSubmatch(err.Error())
	if m == nil {
		t.Fatalf("all-failed archive error = %v, want the 0-of-N message; stderr=%s", err, stderr)
	}
	attempted, _ := strconv.Atoi(m[1])
	if attempted == 0 {
		t.Fatalf("all-failed archive reported 0 attempted resources: %v", err)
	}
	if got := resourcesSyncedFromJSON(t, stdout); got != 0 {
		t.Fatalf("all-failed archive resources_synced = %d, want 0", got)
	}
	if strings.Contains(stdout, "Archived 0 items") {
		t.Fatalf("all-failed archive stdout = %q, want no success-shaped human summary", stdout)
	}
	if !strings.Contains(stderr, "error:") {
		t.Fatalf("all-failed archive stderr = %q, want per-resource errors", stderr)
	}

	stdout, stderr, err = runArchive(t, d)
	if err == nil {
		t.Fatalf("all-failed human archive unexpectedly succeeded; stdout=%s stderr=%s", stdout, stderr)
	}
	if !archiveFailedRE.MatchString(err.Error()) {
		t.Fatalf("all-failed human archive error = %v; stderr=%s", err, stderr)
	}
	if strings.Contains(stdout, "Archived") {
		t.Fatalf("all-failed human archive stdout = %q, want no success-shaped summary", stdout)
	}
}

func TestWorkflowArchivePartialSuccessExitsZero(t *testing.T) {
	d := newArchiveDouble(t, func(path string) string {
		if path == "/customers" {
			return `[{"id":"a1"}]`
		}
		return ""
	})

	stdout, stderr, err := runArchive(t, d, "--json")
	if err != nil {
		t.Fatalf("partial archive: %v; stdout=%s stderr=%s", err, stdout, stderr)
	}
	if got := resourcesSyncedFromJSON(t, stdout); got != 1 {
		t.Fatalf("partial archive resources_synced = %d, want the true count 1; stderr=%s", got, stderr)
	}
	if !strings.Contains(stderr, "error:") {
		t.Fatalf("partial archive stderr = %q, want per-resource errors", stderr)
	}

	stdout, stderr, err = runArchive(t, d)
	if err != nil {
		t.Fatalf("partial human archive: %v; stdout=%s stderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "across 1 resources") {
		t.Fatalf("partial human archive stdout = %q, want the true count", stdout)
	}
}

func TestWorkflowArchiveSuccessfulEmptyExitsZero(t *testing.T) {
	d := newArchiveDouble(t, func(string) string { return "[]" })

	stdout, stderr, err := runArchive(t, d, "--json")
	if err != nil {
		t.Fatalf("empty archive: %v; stdout=%s stderr=%s", err, stdout, stderr)
	}
	got := resourcesSyncedFromJSON(t, stdout)
	if got == 0 || got != d.distinctPaths() {
		t.Fatalf("empty archive resources_synced = %d, want every attempted resource (%d distinct paths served); stderr=%s", got, d.distinctPaths(), stderr)
	}
	if strings.Contains(stderr, "error:") {
		t.Fatalf("empty archive stderr = %q, want no per-resource errors", stderr)
	}

	stdout, _, err = runArchive(t, d)
	if err != nil {
		t.Fatalf("empty human archive: %v", err)
	}
	if !strings.Contains(stdout, "Archived 0 items across "+strconv.Itoa(got)+" resources") {
		t.Fatalf("empty human archive stdout = %q, want the full count", stdout)
	}
}
