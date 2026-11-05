// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "sync-json-only-bounded" and "all-default-page-size" entries. See
// docs/reprint-survival.md.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	syncBinaryBodyBytes = int64(200 << 20) // larger than the client's whole-body cap
	syncBodyNeverRead   = int64(16 << 20)  // room for socket buffers, far below the body
)

// serveSyncFixture answers every path with an empty JSON list, except the
// paths in routes. Each resource test names only what it changes.
func serveSyncFixture(t *testing.T, routes map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	isolateNovelStore(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := routes[r.URL.Path]; ok {
			h(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":0,"next":null,"previous":null,"results":[]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SERVOSITY_MSP_BASE_URL", srv.URL)
	t.Setenv("SERVOSITY_MSP_TOKEN", "fixture-token")
	return srv
}

// streamBody writes size bytes in chunks, stops when the client hangs up, and
// counts what it managed to send.
func streamBody(w http.ResponseWriter, contentType string, size int64, sent *atomic.Int64) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	chunk := make([]byte, 64<<10)
	for remaining := size; remaining > 0; {
		n := min(remaining, int64(len(chunk)))
		if _, err := w.Write(chunk[:n]); err != nil {
			return
		}
		sent.Add(n)
		remaining -= n
	}
}

func syncSummaryEvent(t *testing.T, stderr string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(stderr, "\n") {
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) == nil && ev["event"] == "sync_summary" {
			return ev
		}
	}
	t.Fatalf("no sync_summary event:\n%s", stderr)
	return nil
}

func syncWarnedNames(ev map[string]any) []string {
	raw, _ := ev["warned_resources"].([]any)
	names := []string{}
	for _, v := range raw {
		if s, ok := v.(string); ok {
			names = append(names, s)
		}
	}
	return names
}

func syncWarningEvent(stderr, resource string) map[string]any {
	for _, line := range strings.Split(stderr, "\n") {
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) == nil && ev["event"] == "sync_warning" && ev["resource"] == resource {
			return ev
		}
	}
	return nil
}

func syncCompleted(stderr, resource string, total float64) bool {
	for _, line := range strings.Split(stderr, "\n") {
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) == nil && ev["event"] == "sync_complete" && ev["resource"] == resource {
			return ev["total"] == total
		}
	}
	return false
}

// The installer endpoint is not a resource. The spec gives it no response type
// to tell it from a JSON read with the same empty schema, so it is named here,
// and the runtime guard below covers any other endpoint like it.
func TestDownloadIsNotASyncResource(t *testing.T) {
	if slices.Contains(defaultSyncResources(), "download") {
		t.Error("default sync includes download")
	}
	if slices.Contains(knownSyncResourceNames(), "download") {
		t.Error("known sync resources include download")
	}
	if _, err := syncResourcePath("download"); err == nil {
		t.Error("syncResourcePath accepted download")
	}
	for _, name := range defaultSyncResources() {
		path, err := syncResourcePath(name)
		if err != nil {
			t.Errorf("default resource %s has no path: %v", name, err)
		}
		if strings.Contains(path, "/download/") {
			t.Errorf("default resource %s reads a download path %s", name, path)
		}
	}
}

func TestSyncRefusesNamedDownloadBeforeAnyRequest(t *testing.T) {
	var hits atomic.Int32
	serveSyncFixture(t, map[string]http.HandlerFunc{
		"/download/servosity-one-windows/": func(http.ResponseWriter, *http.Request) { hits.Add(1) },
	})
	_, _, err := runDRF(t, "sync", "--resources", "download", "--json")
	if err == nil || !strings.Contains(err.Error(), "download is not synced") {
		t.Fatalf("err = %v, want the refusal", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("%d request(s) reached the installer endpoint", n)
	}
}

// A plain sync over a resource that redirects to a large binary: the body is
// not read, the run finishes, the resource is named, and JSON resources land.
func TestPlainSyncSkipsResourceThatRedirectsToBinary(t *testing.T) {
	var sent atomic.Int64
	serveSyncFixture(t, map[string]http.HandlerFunc{
		"/stats/": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/installer.bin", http.StatusFound)
		},
		"/installer.bin": func(w http.ResponseWriter, _ *http.Request) {
			streamBody(w, "binary/octet-stream", syncBinaryBodyBytes, &sent)
		},
		"/companies/": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"count":1,"next":null,"previous":null,"results":[{"id":1,"name":"synthetic"}]}`))
		},
	})
	started := time.Now()
	_, stderr, err := runDRF(t, "sync", "--json")
	if err != nil {
		t.Fatalf("plain sync failed: %v\n%s", err, stderr)
	}
	if elapsed := time.Since(started); elapsed > time.Minute {
		t.Errorf("sync took %s", elapsed)
	}
	if n := sent.Load(); n > syncBodyNeverRead {
		t.Errorf("sync read %d bytes of a binary body", n)
	}
	if !slices.Contains(syncWarnedNames(syncSummaryEvent(t, stderr)), "stats") {
		t.Errorf("summary does not name the skipped resource:\n%s", stderr)
	}
	warning := syncWarningEvent(stderr, "stats")
	if warning == nil || warning["reason"] != "non_json_response" || !strings.Contains(warning["message"].(string), "binary/octet-stream") {
		t.Errorf("warning for stats = %v\n%s", warning, stderr)
	}
	if !syncCompleted(stderr, "companies", 1) {
		t.Errorf("a JSON resource did not sync:\n%s", stderr)
	}
}

// The installer's real route: a 302 to another host. Nothing is fetched there.
func TestSyncDoesNotFollowRedirectOffOrigin(t *testing.T) {
	var hits atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(elsewhere.Close)
	var requests atomic.Int32
	serveSyncFixture(t, map[string]http.HandlerFunc{
		"/stats/": func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			http.Redirect(w, r, elsewhere.URL+"/installer.bin", http.StatusFound)
		},
	})
	_, stderr, err := runDRF(t, "sync", "--resources", "stats,companies", "--json")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("%d request(s) reached the other host", n)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("the refused redirect was retried: %d requests", n)
	}
	warning := syncWarningEvent(stderr, "stats")
	if warning == nil || warning["reason"] != "redirect_off_origin" {
		t.Errorf("warning for stats = %v\n%s", warning, stderr)
	}
	if !slices.Contains(syncWarnedNames(syncSummaryEvent(t, stderr)), "stats") {
		t.Errorf("summary does not name stats:\n%s", stderr)
	}
}

// The refusal reads the header, whatever the endpoint is called, and only a
// JSON media type passes.
func TestSyncContentTypeDecidesEveryResponse(t *testing.T) {
	const body = `{"count":1,"next":null,"previous":null,"results":[{"id":1,"name":"synthetic"}]}`
	for _, tc := range []struct {
		contentType string
		synced      bool
	}{
		{"application/json", true},
		{"Application/JSON; charset=utf-8", true},
		{"text/json", true},
		{"application/vnd.api+json", true},
		{"text/html; charset=utf-8", false},
		{"text/plain", false},
		{"application/zip", false},
		{"application/jsonl", false},
		{"", false},
	} {
		t.Run("type="+tc.contentType, func(t *testing.T) {
			serveSyncFixture(t, map[string]http.HandlerFunc{
				"/companies/": func(w http.ResponseWriter, _ *http.Request) {
					if tc.contentType == "" {
						w.Header()["Content-Type"] = nil // stop net/http sniffing one in
					} else {
						w.Header().Set("Content-Type", tc.contentType)
					}
					_, _ = w.Write([]byte(body))
				},
			})
			_, stderr, err := runDRF(t, "sync", "--resources", "companies,stats", "--json")
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, stderr)
			}
			named := slices.Contains(syncWarnedNames(syncSummaryEvent(t, stderr)), "companies")
			if synced := syncCompleted(stderr, "companies", 1); synced != tc.synced || named == tc.synced {
				t.Fatalf("synced=%v named=%v, want synced=%v\n%s", synced, named, tc.synced, stderr)
			}
		})
	}
}

// A body with nothing in it has no type to check.
func TestSyncAcceptsEmptyBodyWithoutContentType(t *testing.T) {
	serveSyncFixture(t, map[string]http.HandlerFunc{
		"/companies/": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) },
	})
	_, stderr, err := runDRF(t, "sync", "--resources", "companies", "--json")
	if syncWarningEvent(stderr, "companies") != nil {
		t.Fatalf("an empty 204 was refused (err=%v):\n%s", err, stderr)
	}
}

// A JSON-typed body past the cap is cut at the cap and named, not buffered.
func TestSyncBoundsJSONBody(t *testing.T) {
	const capBytes = 1 << 20
	t.Setenv("SERVOSITY_MSP_MAX_RESPONSE_BYTES", strconv.Itoa(capBytes))
	var sent atomic.Int64
	serveSyncFixture(t, map[string]http.HandlerFunc{
		"/stats/": func(w http.ResponseWriter, _ *http.Request) {
			streamBody(w, "application/json", syncBinaryBodyBytes, &sent)
		},
	})
	_, stderr, err := runDRF(t, "sync", "--resources", "stats,companies", "--json")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	if n := sent.Load(); n > syncBodyNeverRead {
		t.Errorf("sync read %d bytes past a %d byte cap", n, capBytes)
	}
	warning := syncWarningEvent(stderr, "stats")
	if warning == nil || warning["reason"] != "response_too_large" {
		t.Errorf("warning for stats = %v\n%s", warning, stderr)
	}
	if !slices.Contains(syncWarnedNames(syncSummaryEvent(t, stderr)), "stats") {
		t.Errorf("summary does not name the oversized resource:\n%s", stderr)
	}
}

// Missing data must not pass --strict, and a run with nothing else synced must
// not exit 0.
func TestSyncRefusedResourceFailsStrictAndEmptyRuns(t *testing.T) {
	html := map[string]http.HandlerFunc{
		"/stats/": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><title>sign in</title></html>"))
		},
	}
	serveSyncFixture(t, html)
	if _, stderr, err := runDRF(t, "sync", "--resources", "stats,companies", "--strict", "--json"); err == nil || !strings.Contains(err.Error(), "stats") {
		t.Errorf("--strict err = %v, want it to name stats\n%s", err, stderr)
	}
	if _, stderr, err := runDRF(t, "sync", "--resources", "stats", "--json"); err == nil {
		t.Errorf("a run that stored nothing exited 0\n%s", stderr)
	}
}

// An error status keeps its own handling: a 403 HTML page is an access warning,
// not a Content-Type refusal.
func TestSyncErrorStatusIsNotAContentTypeRefusal(t *testing.T) {
	serveSyncFixture(t, map[string]http.HandlerFunc{
		"/stats/": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html>forbidden</html>"))
		},
	})
	_, stderr, err := runDRF(t, "sync", "--resources", "stats,companies", "--json")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	warning := syncWarningEvent(stderr, "stats")
	if warning == nil || warning["reason"] == "non_json_response" || warning["status"] != float64(403) {
		t.Fatalf("warning for stats = %v\n%s", warning, stderr)
	}
}

// The guard belongs to sync. A direct read of the same endpoint still works.
func TestDirectDownloadStillReadsBinary(t *testing.T) {
	var sent atomic.Int64
	serveSyncFixture(t, map[string]http.HandlerFunc{
		"/download/servosity-one-windows/": func(w http.ResponseWriter, _ *http.Request) {
			streamBody(w, "application/octet-stream", 1024, &sent)
		},
	})
	stdout, stderr, err := runDRF(t, "download", "--json")
	if err != nil {
		t.Fatalf("download: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, `application/octet-stream`) || sent.Load() != 1024 {
		t.Fatalf("sent=%d stdout=%s", sent.Load(), stdout)
	}
}

func pageSizesSent(t *testing.T, requests []string) []string {
	t.Helper()
	sizes := []string{}
	for _, uri := range requests {
		u, err := url.ParseRequestURI(uri)
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, u.Query().Get("page_size"))
	}
	return sizes
}

// --all with no --page-size asks for 100 rows a page on every list command, and
// every page of the walk says so. The short-page heuristic stays off: the
// server's next link still ends the walk (TestAllSurvivesServerCapBelowDefault).
func TestAllSendsDefaultPageSize(t *testing.T) {
	for _, base := range drfListCommands {
		name := strings.Join(base, " ")
		t.Run(name, func(t *testing.T) {
			isolateNovelStore(t)
			srv := newDRFServer(t, drfHonest)
			args := append(append([]string{}, base...), "--all", "--json")
			if _, stderr, err := runDRF(t, args...); err != nil {
				t.Fatalf("%v\n%s", err, stderr)
			}
			sizes := pageSizesSent(t, srv.requests())
			if len(sizes) == 0 {
				t.Fatal("no requests")
			}
			for _, size := range sizes {
				if size != "100" {
					t.Fatalf("page_size = %q, want 100; requests: %v", size, srv.requests())
				}
			}
		})
	}
}

func TestAllKeepsOperatorPageSize(t *testing.T) {
	for _, value := range []string{"7", "1000"} {
		t.Run(value, func(t *testing.T) {
			isolateNovelStore(t)
			srv := newDRFServer(t, drfHonest)
			stdout, stderr, err := runDRF(t, "companies", "list", "--all", "--json", "--page-size", value)
			if err != nil {
				t.Fatalf("%v\n%s", err, stderr)
			}
			if got, _ := drfListRows(t, stdout); got != drfRows {
				t.Fatalf("rows = %d, want %d", got, drfRows)
			}
			for _, size := range pageSizesSent(t, srv.requests()) {
				if size != value {
					t.Fatalf("page_size = %q, want %s; requests: %v", size, value, srv.requests())
				}
			}
		})
	}
}

// Without --all a list still sends only what the operator asked for.
func TestPlainListSendsNoPageSize(t *testing.T) {
	isolateNovelStore(t)
	srv := newDRFServer(t, drfHonest)
	if _, stderr, err := runDRF(t, "companies", "list", "--json"); err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	for _, size := range pageSizesSent(t, srv.requests()) {
		if size != "" {
			t.Fatalf("page_size = %q without --all", size)
		}
	}
}

// A server that caps page_size below 100 keeps paging by its next link.
func TestAllSurvivesServerCapBelowDefault(t *testing.T) {
	isolateNovelStore(t)
	srv := newDRFServer(t, drfHonest)
	stdout, stderr, err := runDRF(t, "companies", "list", "--all", "--json")
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr)
	}
	if got, _ := drfListRows(t, stdout); got != drfRows {
		t.Fatalf("rows = %d, want %d", got, drfRows)
	}
	if n := len(srv.requests()); n != 3 {
		t.Fatalf("%d requests, want 3 pages at the server's cap: %v", n, srv.requests())
	}
}
