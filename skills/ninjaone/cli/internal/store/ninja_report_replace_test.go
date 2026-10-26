// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Class report-row-identity: ReplaceReportRows swaps a position-keyed report for one whole walk
// in ONE transaction.

func stampedRows(t *testing.T, resourceType string, rows ...string) []json.RawMessage {
	t.Helper()
	items := make([]json.RawMessage, len(rows))
	for i, r := range rows {
		items[i] = json.RawMessage(r)
	}
	return NewReportRowOrdinals(resourceType).Stamp(items)
}

func batches(bs ...[]json.RawMessage) func() ([]json.RawMessage, bool, error) {
	i := 0
	return func() ([]json.RawMessage, bool, error) {
		if i == len(bs) {
			return nil, false, nil
		}
		i++
		return bs[i-1], true, nil
	}
}

// tableCounts is the row count of every table in the store, keyed by table name.
func tableCounts(t *testing.T, s *Store) map[string]int {
	t.Helper()
	rows, err := s.DB().Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'resources_fts_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	rows.Close()
	out := map[string]int{}
	for _, n := range names {
		var c int
		if err := s.DB().QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, strings.ReplaceAll(n, `"`, `""`))).Scan(&c); err != nil {
			t.Fatal(err)
		}
		out[n] = c
	}
	return out
}

func partitionDump(t *testing.T, s *Store, resourceType string) string {
	t.Helper()
	rows, err := s.DB().Query(`SELECT id, data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, data string
		if err := rows.Scan(&id, &data); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%q %s", id, data))
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// A replace that fails part way (here, the staged walk cannot be read after its first batch)
// rolls back whole: the partition keeps the previous walk's rows exactly, never a mix.
func TestReplaceReportRowsIsOneTransaction(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	previous := stampedRows(t, "queries-software",
		`{"deviceId":7,"name":"Alpha","version":"1"}`,
		`{"deviceId":7,"name":"Beta","version":"1"}`,
		`{"deviceId":7,"name":"Gamma","version":"1"}`)
	if _, _, err := s.UpsertBatch("queries-software", previous); err != nil {
		t.Fatal(err)
	}
	before := partitionDump(t, s, "queries-software")
	walk := stampedRows(t, "queries-software", `{"deviceId":7,"name":"Alpha","version":"2"}`, `{"deviceId":7,"name":"Gamma","version":"2"}`)
	calls := 0
	next := func() ([]json.RawMessage, bool, error) {
		calls++
		if calls == 1 {
			return walk[:1], true, nil
		}
		return nil, false, errors.New("staged walk unreadable")
	}
	if _, _, err := s.ReplaceReportRows("queries-software", "", next); err == nil {
		t.Fatal("a replace whose staged walk failed part way returned no error")
	}
	if after := partitionDump(t, s, "queries-software"); after != before {
		t.Fatalf("a failed replace left a mix of two walks:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	stored, deleted, err := s.ReplaceReportRows("queries-software", "", batches(walk[:1], walk[1:]))
	if err != nil || stored != 2 || deleted != 1 {
		t.Fatalf("replace = stored %d, deleted %d, err %v; want 2, 1, nil", stored, deleted, err)
	}
	if got := partitionDump(t, s, "queries-software"); !strings.Contains(got, `"name":"Gamma","version":"2"`) || strings.Contains(got, "Beta") || strings.Count(got, "\n") != 1 {
		t.Fatalf("after the replace the partition holds:\n%s", got)
	}
	if _, _, err := s.ReplaceReportRows("queries-os-patches", "", batches()); err == nil {
		t.Fatal("ReplaceReportRows accepted a report whose rows carry their own id")
	}
	unstamped := []json.RawMessage{json.RawMessage(`{"deviceId":7,"name":"Alpha"}`)}
	if _, _, err := s.ReplaceReportRows("queries-software", "", batches(unstamped)); err == nil {
		t.Fatal("ReplaceReportRows stored a row with no ordinal")
	}
}

// ReplaceReportRows writes every table UpsertBatchDetailed writes for each ordinal report (the
// typed projection included), so a reprint that gives another ordinal report a typed table
// fails here instead of leaving that table stale.
func TestReplaceReportRowsWritesWhatUpsertBatchWrites(t *testing.T) {
	for resourceType := range reportRowOrdinalReports {
		t.Run(resourceType, func(t *testing.T) {
			rows := stampedRows(t, resourceType,
				`{"deviceId":7,"name":"one","id":1,"uid":"u-1","system_name":"PC"}`,
				`{"deviceId":7,"name":"two","id":2,"uid":"u-2","system_name":"PC"}`)
			a, err := Open(filepath.Join(t.TempDir(), "a.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			b, err := Open(filepath.Join(t.TempDir(), "b.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			base := tableCounts(t, a)
			if stored, _, typed, err := a.UpsertBatchDetailed(resourceType, rows); err != nil || stored != 2 || typed != 0 {
				t.Fatalf("UpsertBatchDetailed = %d stored, %d typed failures, %v", stored, typed, err)
			}
			if stored, _, err := b.ReplaceReportRows(resourceType, "", batches(rows)); err != nil || stored != 2 {
				t.Fatalf("ReplaceReportRows = %d stored, %v", stored, err)
			}
			want, got := tableCounts(t, a), tableCounts(t, b)
			for name, n := range want {
				if got[name] != n {
					t.Errorf("table %s: UpsertBatchDetailed wrote %d rows (from %d), ReplaceReportRows %d", name, n, base[name], got[name])
				}
			}
		})
	}
}
