// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json fanout-malformed-collection-is-failed-read.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A 2xx body whose collection is null, a scalar or a list of non-objects is
// a failed read, never zero devices, zero endpoints or zero incidents.
func TestListRowsRejectsMalformedCollections(t *testing.T) {
	for _, bad := range []string{`{"data":null}`, `{"data":"unavailable"}`, `{"data":{"count":0}}`, `{"data":[1,2]}`, `["x"]`, `{"other":[]}`, `[{"data":null}]`, `[{"data":"unavailable","total_items":1}]`, `{"data":[{"data":null}]}`, `[{"data":[1]}]`, `[{"data":{"data":null}}]`} {
		if rows, err := listRows(json.RawMessage(bad), "data"); err == nil {
			t.Errorf("listRows(%s) = %d rows, nil error; want a failed read", bad, len(rows))
		}
	}
	for _, good := range []string{`{"data":[]}`, `{"data":[{"id":"1"}]}`, `[{"id":"1"}]`, `[{"data":{"id":"1"},"total_items":1}]`, `[{"data":[{"id":"1"}]}]`, `[{"data":[]}]`} {
		if _, err := listRows(json.RawMessage(good), "data"); err != nil {
			t.Errorf("listRows(%s): %v", good, err)
		}
	}
	for _, bad := range []string{`{"items":null}`, `{"items":"unavailable"}`, `{"items":[1]}`, `{"groups":{"open":null}}`, `{"groups":[]}`, `{"page":1}`, `[{"items":null}]`, `[{"items":"unavailable"}]`, `[1]`} {
		if rows, err := incidentRows(json.RawMessage(bad)); err == nil {
			t.Errorf("incidentRows(%s) = %d rows, nil error; want a failed read", bad, len(rows))
		}
	}
	rows, err := incidentRows(json.RawMessage(`{"items":[{"id":"a"}],"groups":{"open":[{"id":"b"}],"closed":[]},"pageControls":{"totalItems":2}}`))
	if err != nil || len(rows) != 2 {
		t.Fatalf("incidentRows items+groups: rows=%d err=%v", len(rows), err)
	}
	if rows, err := incidentRows(json.RawMessage(`[{"items":{"id":"a"}},{"id":"b"}]`)); err != nil || len(rows) != 2 {
		t.Fatalf("incidentRows wrapped list: rows=%d err=%v", len(rows), err)
	}
	if rows, err := incidentRows(json.RawMessage(`{"items":[]}`)); err != nil || len(rows) != 0 {
		t.Fatalf("incidentRows empty items: rows=%d err=%v", len(rows), err)
	}
}

// A malformed wrapper row never lets the wait skip to an older, finished
// transaction: wait --device fails the read, and firstObject finds nothing.
func TestTransactionsWaitRejectsMalformedRows(t *testing.T) {
	const bad = `[{"data":null},{"data":{"id":"old","device":"FB-1","status":"complete","inception_time":1}}]`
	if got := firstObject(json.RawMessage(bad), "data"); got != nil {
		t.Fatalf("firstObject skipped a malformed row to %v", got)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/transactions/old") { // the by-ID read is well formed
			_, _ = w.Write([]byte(`[{"id":"old","device":"FB-1","status":"complete"}]`))
			return
		}
		_, _ = w.Write([]byte(bad))
	}))
	defer srv.Close()
	spEnv(t, srv)
	if _, code := runJSON(t, "firebox-transactions", "wait", "--device", "FB-1", "--interval", "2s", "--max-wait", "20s", "--json"); code == 0 {
		t.Fatalf("wait --device exited 0 on a malformed transaction list")
	}
}
