// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-fix (handfixes.json sync-forbidden-not-partial). The first two tests match
// tools/maintainer/handfix_replay/sync_outcome_test.go.tmpl; this connector's
// syncResource takes a concrete GraphQL client, so its walk runs through the CLI.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/store"
)

func TestSyncForbiddenKeyOnlyForAnOutrightRefusal(t *testing.T) {
	forbidden := &accessWarning{Status: 403, Reason: "forbidden"}
	if got := syncForbiddenKey(forbidden, 0, "acme"); got != "acme" {
		t.Fatalf("403 on the first page = %q, want the key", got)
	}
	if got := syncForbiddenKey(forbidden, 1, "acme"); got != "" {
		t.Fatalf("403 after a page was read = %q: the rows read so far are a partial walk", got)
	}
	if got := syncForbiddenKey(&accessWarning{Status: 400, Reason: "argument_missing"}, 0, "acme"); got != "" {
		t.Fatalf("400 = %q: the resource is readable with the right argument", got)
	}
}

func TestSyncOutcomesNameWhatWasSkippedAndWhatFailed(t *testing.T) {
	o := &syncOutcomes{skippedForbidden: []string{"tags", "overviews"}, failed: []string{"tickets"}}
	want := `,"skipped_forbidden":["overviews","tags"],"failed":["tickets"]`
	if got := o.summaryJSON(); got != want {
		t.Fatalf("summary fields = %s, want %s", got, want)
	}
	if got := (&syncOutcomes{}).summaryJSON(); got != "" {
		t.Fatalf("a clean run added %s to its summary", got)
	}
}

// A real sync against a vendor that refuses one resource with HTTP 403.
func TestSuperopsSyncRecordsAForbiddenResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden for Acme Corp"}`))
	}))
	defer srv.Close()
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_BASE_URL", srv.URL)
	t.Setenv("SUPEROPS_API_TOKEN", "sop-test-token-4471b9")
	dbPath := filepath.Join(t.TempDir(), "sync.db")
	out, _ := runSuperOpsCLI(t, "sync", "--db", dbPath, "--resources", "clients", "--json")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	defer db.Close()
	got, err := db.SyncCoverage()
	if err != nil {
		t.Fatalf("reading sync coverage: %v", err)
	}
	if !reflect.DeepEqual(got.SkippedForbidden, []string{"clients"}) || len(got.Incomplete) != 0 {
		t.Fatalf("coverage after a 403 = %+v, want clients skipped and nothing incomplete; output:\n%s", got, out)
	}
}
