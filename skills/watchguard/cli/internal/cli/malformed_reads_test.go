// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

// hand-fix guard (handfixes.json fanout-partial-data-exit): a 2xx body
// without the expected collection is a failed read, and a protection row
// with no status fields is unknown, never healthy.
func TestMalformedListIsAFailedRead(t *testing.T) {
	for _, body := range []string{`{"unexpected":"upstream failure"}`, `"ok"`, `not json`} {
		if _, err := listRows(json.RawMessage(body), "data"); err == nil {
			t.Errorf("listRows(%s) = nil error; want a failed read", body)
		}
		if _, err := incidentRows(json.RawMessage(body)); err == nil {
			t.Errorf("incidentRows(%s) = nil error; want a failed read", body)
		}
	}
	for _, body := range []string{`{"data":[]}`, `[]`, `{"data":[{"id":"d1"}],"total_items":1}`} {
		if _, err := listRows(json.RawMessage(body), "data"); err != nil {
			t.Errorf("listRows(%s) = %v; want ok", body, err)
		}
	}
	if _, err := incidentRows(json.RawMessage(`{"items":[]}`)); err != nil {
		t.Errorf("empty items list rejected: %v", err)
	}
	if got := protectionIssues(map[string]any{"device_id": "d1", "host_name": "PC-1"}); len(got) == 0 {
		t.Error("a protection row with no status fields was treated as healthy")
	}
	if got := protectionIssues(map[string]any{"accumulated_protection_status": float64(1), "isolation_state": float64(0)}); len(got) != 0 {
		t.Errorf("healthy row flagged: %v", got)
	}
}
