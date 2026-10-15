// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json fanout-malformed-collection-is-failed-read.

package cli

import (
	"encoding/json"
	"testing"
)

// A 2xx body whose collection is null, a scalar or a list of non-objects is
// a failed read, never zero devices, zero endpoints or zero incidents.
func TestListRowsRejectsMalformedCollections(t *testing.T) {
	for _, bad := range []string{`{"data":null}`, `{"data":"unavailable"}`, `{"data":{"count":0}}`, `{"data":[1,2]}`, `["x"]`, `{"other":[]}`} {
		if rows, err := listRows(json.RawMessage(bad), "data"); err == nil {
			t.Errorf("listRows(%s) = %d rows, nil error; want a failed read", bad, len(rows))
		}
	}
	for _, good := range []string{`{"data":[]}`, `{"data":[{"id":"1"}]}`, `[{"id":"1"}]`, `[{"data":{"id":"1"},"total_items":1}]`} {
		if _, err := listRows(json.RawMessage(good), "data"); err != nil {
			t.Errorf("listRows(%s): %v", good, err)
		}
	}
	for _, bad := range []string{`{"items":null}`, `{"items":"unavailable"}`, `{"items":[1]}`, `{"groups":{"open":null}}`, `{"groups":[]}`, `{"page":1}`} {
		if rows, err := incidentRows(json.RawMessage(bad)); err == nil {
			t.Errorf("incidentRows(%s) = %d rows, nil error; want a failed read", bad, len(rows))
		}
	}
	rows, err := incidentRows(json.RawMessage(`{"items":[{"id":"a"}],"groups":{"open":[{"id":"b"}],"closed":[]},"pageControls":{"totalItems":2}}`))
	if err != nil || len(rows) != 2 {
		t.Fatalf("incidentRows items+groups: rows=%d err=%v", len(rows), err)
	}
	if rows, err := incidentRows(json.RawMessage(`{"items":[]}`)); err != nil || len(rows) != 0 {
		t.Fatalf("incidentRows empty items: rows=%d err=%v", len(rows), err)
	}
}
