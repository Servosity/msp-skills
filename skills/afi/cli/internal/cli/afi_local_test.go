// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"afi-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// newTestStore opens a throwaway store and returns its path for --db flags.
func newTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("opening test store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func seed(t *testing.T, db *store.Store, rtype, id, body string) {
	t.Helper()
	if err := db.Upsert(rtype, id, json.RawMessage(body)); err != nil {
		t.Fatalf("seeding %s/%s: %v", rtype, id, err)
	}
}

// runNovel executes a novel command constructor with JSON output captured.
func runNovel(t *testing.T, build func(*rootFlags) *cobra.Command, args ...string) (string, error) {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := build(flags)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func decodeView[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decoding output %q: %v", raw, err)
	}
	return v
}

func TestJSONInt64(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
		ok   bool
	}{
		{"string int64", "25", 25, true},
		{"float64", float64(7), 7, true},
		{"json.Number", json.Number("42"), 42, true},
		{"empty string", "", 0, false},
		{"garbage", "abc", 0, false},
		{"nil", nil, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := jsonInt64(c.in)
			if got != c.want || ok != c.ok {
				t.Errorf("jsonInt64(%v) = %d,%v want %d,%v", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestCSVSet(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"a, b ,c", 3},
		{"a,,b", 2},
	}
	for _, c := range cases {
		if got := len(csvSet(c.in)); got != c.want {
			t.Errorf("csvSet(%q) size = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseTaskStats(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantFailed int64
		wantOK     bool
	}{
		{"normal", `{"total":{"done":10,"failed":2,"warnings":1}}`, 2, true},
		{"string ints", `{"total":{"done":"10","failed":"2","warnings":"1"}}`, 2, true},
		{"missing total", `{"by_action":{}}`, 0, true},
		{"garbage", `not json`, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			total, _, ok := parseTaskStats([]byte(c.body))
			if ok != c.wantOK || total.Failed != c.wantFailed {
				t.Errorf("parseTaskStats(%s) failed=%d ok=%v, want failed=%d ok=%v", c.body, total.Failed, ok, c.wantFailed, c.wantOK)
			}
		})
	}
}
