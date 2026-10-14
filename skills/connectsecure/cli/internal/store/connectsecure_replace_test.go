// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json local-report-rows-replaced.

package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestClearedFieldIsNotKeptOnResync: a suppression whose approver was
// cleared must not keep the old approver, or suppression-audit misses it.
func TestClearedFieldIsNotKeptOnResync(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, row := range []string{`{"id":7,"approved_by":"bob","reason":"x"}`, `{"id":7,"approved_by":null,"reason":"x"}`} {
		if _, _, err := s.UpsertBatch("suppress_vulnerability", []json.RawMessage{json.RawMessage(row)}); err != nil {
			t.Fatal(err)
		}
	}
	var data string
	if err := s.DB().QueryRow(`SELECT data FROM resources WHERE resource_type = 'suppress_vulnerability'`).Scan(&data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(data, "bob") {
		t.Fatalf("cleared approver kept after resync: %s", data)
	}
}
