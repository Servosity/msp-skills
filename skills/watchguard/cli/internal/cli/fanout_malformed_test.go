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

// The transaction reads accept every documented shape and fail closed on a
// malformed one: wait never skips a bad row to reach an older, finished
// transaction, and a malformed body is a failed read (exit 5), not "not found".
func TestTransactionsWaitRejectsMalformedRows(t *testing.T) {
	const bad = `[{"data":null},{"data":{"id":"old","device":"FB-1","status":"complete","inception_time":1}}]`
	for _, body := range []string{bad, `{"data":{"data":[{"data":null},{"id":"old","status":"complete"}]}}`, `{"data":{"data":[{"data":null},{"data":{"id":"old","status":"complete"}}]}}`} {
		if got := firstObject(json.RawMessage(body), "data"); got != nil {
			t.Fatalf("firstObject(%s) skipped a malformed row to %v", body, got)
		}
	}
	list, byID := "", map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/transactions") {
			_, _ = w.Write([]byte(list))
			return
		}
		_, _ = w.Write([]byte(byID[r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]]))
	}))
	defer srv.Close()
	spEnv(t, srv)
	byID["old"] = `{"id":"old","device":"FB-1","status":"complete"}` // the documented bare object
	byID["new"] = `{"id":"new","device":"FB-1","status":"complete"}`
	wait := func(args ...string) (map[string]any, int) {
		return runJSON(t, append(append([]string{"firebox-transactions", "wait"}, args...), "--interval", "2s", "--max-wait", "20s", "--json")...)
	}
	list = bad
	if _, code := wait("--device", "FB-1"); code != 5 {
		t.Errorf("wait --device on a malformed list: exit %d, want 5", code)
	}
	list = `{"count":2,"more":false,"objects":[{"id":"old","device":"FB-1","inception_time":1},{"id":"new","device":"FB-1","inception_time":2}]}`
	if doc, code := wait("--device", "FB-1"); code != 0 || doc["id"] != "new" {
		t.Errorf("wait --device on the documented objects list: exit %d, id %v; want 0, new", code, doc["id"])
	}
	for id, body := range map[string]string{
		"m1": bad,
		"m2": `{"data":{"data":[{"data":null},{"id":"old","status":"complete"}]}}`,
		"m3": `[{"id":"old","status":"complete"},{"id":"other","status":"complete"}]`,
	} {
		byID[id] = body
		if _, code := wait(id); code != 5 {
			t.Errorf("wait %s on %s: exit %d, want 5", id, body, code)
		}
	}
	byID["pick"] = `[{"id":"other","status":"failed"},{"id":"pick","status":"complete"}]`
	if doc, code := wait("pick"); code != 0 || doc["id"] != "pick" {
		t.Errorf("wait pick on a two-row list: exit %d, id %v; want 0, pick", code, doc["id"])
	}
}
