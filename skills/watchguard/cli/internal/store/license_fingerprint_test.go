// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json license-keys-fingerprinted.

package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// A synced software allocation's license key must not land anywhere in the
// SQLite file (generic blob, typed column, FTS), while a Firebox allocation's
// serial number stays readable for allocations reconcile and the mirror.
func TestLicenseKeysNeverPersisted(t *testing.T) {
	const key = "SENT-LICENSE-KEY-7e2a"
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	rows := []json.RawMessage{
		json.RawMessage(`{"accountId":"WGC-1-0001","productName":"endPoint","allocationType":"linked_to_license","serialOrLicense":"` + key + `","quantityTotal":5}`),
		json.RawMessage(`{"accountId":"WGC-1-0001","productName":"firebox","serialOrLicense":"80B50000SERIAL","modelName":"T45"}`),
	}
	if _, _, err := s.UpsertBatch("allocations", rows); err != nil {
		t.Fatalf("UpsertBatch allocations: %v", err)
	}
	tables, err := s.DB().Query(`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var n string
		_ = tables.Scan(&n)
		names = append(names, n)
	}
	tables.Close()
	serialSeen := false
	for _, tbl := range names {
		r, err := s.DB().Query(`SELECT * FROM "` + tbl + `"`)
		if err != nil {
			continue
		}
		cols, _ := r.Columns()
		for r.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := r.Scan(ptrs...); err != nil {
				continue
			}
			for i, v := range vals {
				text := ""
				switch x := v.(type) {
				case string:
					text = x
				case []byte:
					text = string(x)
				}
				if strings.Contains(text, key) {
					t.Errorf("license key persisted in %s.%s: %s", tbl, cols[i], text)
				}
				if strings.Contains(text, "80B50000SERIAL") {
					serialSeen = true
				}
			}
		}
		r.Close()
	}
	if !serialSeen {
		t.Fatalf("the Firebox serial number was dropped from the mirror")
	}
}
