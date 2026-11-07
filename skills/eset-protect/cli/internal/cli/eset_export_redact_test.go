// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestExportRedactsCredentialTier (handfixes.json: export-redacts-credential-tier):
// `export installers --output <file>` wrote each installer's enrollment
// downloadUrl to disk unredacted, although the mirror and the cache redact it
// and governance.md says those links are never written to disk.
func TestExportRedactsCredentialTier(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"installers":[{"uuid":"00000000-0000-0000-0000-000000000001","displayName":"Acme Agent","downloadUrl":"https://x.invalid/enroll?tok=SENTINELLINK"}],"nextPageToken":""}`)
	})
	for _, format := range []string{"jsonl", "json"} {
		file := filepath.Join(t.TempDir(), "installers."+format)
		if _, errOut, err := captureExecuteAll(t, "export", "installers", "--format", format, "--output", file); err != nil {
			t.Fatalf("export --format %s: %v\n%s", format, err, errOut)
		}
		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(got, []byte("SENTINELLINK")) {
			t.Fatalf("export --format %s wrote the enrollment link to disk: %s", format, got)
		}
		if !bytes.Contains(got, []byte("[redacted]")) || !bytes.Contains(got, []byte("Acme Agent")) {
			t.Fatalf("export --format %s lost the row or the redaction marker: %s", format, got)
		}
	}
}

// TestExportDetailSubstitutesItemID (handfixes.json:
// detail-path-skips-template-vars): resourceDetailPath replaced the FIRST
// placeholder, which on ESET's absolute https://{region}.<svc>... paths is the
// region, so `export policies <uuid>` failed with "unresolved path parameter
// {policyUuid}". The item ID must land in the item placeholder.
func TestExportDetailSubstitutesItemID(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	const id = "00000000-0000-0000-0000-000000000001"
	var mu sync.Mutex
	var paths []string
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"policy":{"uuid":"`+id+`","displayName":"Base policy"}}`)
	})
	out, errOut, err := captureExecuteAll(t, "export", "policies", id, "--format", "json")
	if err != nil {
		t.Fatalf("export policies <uuid>: %v\n%s", err, errOut)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || paths[0] != "/v2/policies/"+id {
		t.Fatalf("requested %v, want [/v2/policies/%s]", paths, id)
	}
	if !bytes.Contains(out, []byte("Base policy")) {
		t.Fatalf("export lost the item: %s", out)
	}
}

// TestExportEmptyListPageIsZeroRecords (handfixes.json:
// export-empty-list-is-zero-records): ESET omits an empty repeated field, so
// an empty devices page is `{}`; export emitted the envelope as one fabricated
// record.
func TestExportEmptyListPageIsZeroRecords(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	body := `{}`
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
	for _, b := range []string{`{}`, `{"nextPageToken":"","totalSize":0}`} {
		body = b
		out, errOut, err := captureExecuteAll(t, "export", "devices", "--format", "json")
		if err != nil {
			t.Fatalf("%s: %v\n%s", b, err, errOut)
		}
		if strings.TrimSpace(string(out)) != "[]" {
			t.Fatalf("an empty page %s exported %q, want []", b, out)
		}
	}
}

// TestExportDryRunAndTokenOnlyPages (export-empty-list-is-zero-records):
// the empty-page check rejected the {"dry_run": true} sentinel, so every
// `export <resource> --dry-run` exited 1; and a page holding only a
// next-page token ended the export early instead of following the token.
func TestExportDryRunAndTokenOnlyPages(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = io.WriteString(w, `{"nextPageToken":"p2"}`)
			return
		}
		_, _ = io.WriteString(w, `{"devices":[{"uuid":"00000000-0000-0000-0000-000000000001","displayName":"PAGE-TWO"}],"nextPageToken":""}`)
	})
	if _, errOut, err := captureExecuteAll(t, "export", "devices", "--dry-run", "--format", "json"); err != nil {
		t.Fatalf("export --dry-run: %v\n%s", err, errOut)
	}
	out, errOut, err := captureExecuteAll(t, "export", "devices", "--format", "jsonl", "--no-cache")
	if err != nil {
		t.Fatalf("export: %v\n%s", err, errOut)
	}
	if !strings.Contains(string(out), "PAGE-TWO") {
		t.Fatalf("export stopped at a token-only page: %q", out)
	}
}

// TestExportStopsOnCyclingCursors (export-empty-list-is-zero-records): once
// empty token-only pages stopped ending an export, a server cycling tokens
// over empty pages (A, B, A, ...) would page until the 100000-page cap.
func TestExportStopsOnCyclingCursors(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	var n int32
	respondFakeESET(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&n, 1)%2 == 1 {
			_, _ = io.WriteString(w, `{"nextPageToken":"A"}`)
			return
		}
		_, _ = io.WriteString(w, `{"nextPageToken":"B"}`)
	})
	if _, _, err := captureExecuteAll(t, "export", "devices", "--format", "jsonl", "--no-cache"); err == nil {
		t.Fatal("cycling cursors did not stop the export")
	}
	if got := atomic.LoadInt32(&n); got > 5 {
		t.Fatalf("export made %d requests before stopping on a cycling cursor", got)
	}
}
