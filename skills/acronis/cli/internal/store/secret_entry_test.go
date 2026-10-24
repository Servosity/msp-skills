// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/acronis/handfixes.json under
// the "credential-fields-never-persisted" entry. Every store write cleans the
// whole incoming row once, at its entry, before any envelope unwrapping or id
// lookup can pick a credential-named subtree as the row.

package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	entryOrdinary1 = "ordinary-client-0001"
	entryOrdinary2 = "ordinary-client-0002"
	entryOrdinary3 = "ordinary-client-0003"
	entryOrdinary4 = "ordinary-client-0004"
)

// The stop-report input: a credential-named wrapper whose value carries an id,
// so envelope unwrapping would make the credential the row.
var entryWrappedRow = `{"password":{"id":"entry-1","value":"test-secret-wrapped-aaaa"}}`

// Plural and compound secret names, at depth and inside an id-bearing wrapper.
var entryPluralWrapped = `{"wrapper":{"id":"w-1","client_id":"` + entryOrdinary2 + `",` +
	`"secrets":{"value":"test-secret-plural-bbbb"},"api_keys":["test-secret-plural-cccc"],` +
	`"credentials":{"user":"u","value":"test-secret-plural-dddd"},"tokens":[{"value":"test-secret-plural-eeee"}],` +
	`"passwords":["test-secret-plural-ffff"],"private_keys":["test-secret-plural-gggg"],` +
	`"apikeys":["test-secret-plural-hhhh"],"credential":"test-secret-plural-iiii"}}`
var entryPluralDeep = `{"id":"c-3","client_id":"` + entryOrdinary3 + `","nested":{"list":[{"Secrets":["test-secret-plural-jjjj"],"PrivateKeys":"test-secret-plural-kkkk"}]}}`

// A credential-named field is withheld whatever its value: a number under
// api_keys and a letter-only code (Client.data allows any property). The count
// and the documented result word beside them are kept.
var entryValueShaped = `{"id":"c-4","client_id":"` + entryOrdinary4 + `","data":{"api_keys":98765432109876543210,` +
	`"code":"TestSecretLetterCode","total_tokens":42,"result":{"code":"ok"}}}`
var entryOrdinaryRow = `{"id":"c-ordinary","client_id":"` + entryOrdinary1 + `","type":"api_client"}`

var entrySentinels = []string{
	"test-secret-wrapped-aaaa", "test-secret-plural-bbbb", "test-secret-plural-cccc", "test-secret-plural-dddd",
	"test-secret-plural-eeee", "test-secret-plural-ffff", "test-secret-plural-gggg", "test-secret-plural-hhhh",
	"test-secret-plural-iiii", "test-secret-plural-jjjj", "test-secret-plural-kkkk",
	"98765432109876543210", "TestSecretLetterCode",
}

// TestEveryStoreWriteCleansTheRowAtItsEntry drives each write entry point
// (the batch path that unwraps envelopes, the generic Upsert, and the typed
// writers) with the stop-report input and the plural cases, then reads every
// column and the database files: no credential, and the ordinary rows stored.
func TestEveryStoreWriteCleansTheRowAtItsEntry(t *testing.T) {
	for _, path := range []string{"batch", "generic", "typed"} {
		t.Run(path, func(t *testing.T) {
			dir := t.TempDir()
			s, err := Open(filepath.Join(dir, "data.db"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			rows := []string{entryWrappedRow, entryPluralWrapped, entryPluralDeep, entryValueShaped, entryOrdinaryRow}
			switch path {
			case "batch":
				raw := make([]json.RawMessage, len(rows))
				for i, r := range rows {
					raw[i] = json.RawMessage(r)
				}
				if _, _, err := s.UpsertBatch("clients", raw); err != nil {
					t.Fatalf("UpsertBatch: %v", err)
				}
			case "generic":
				for i, r := range rows {
					if err := s.Upsert("clients", fmt.Sprintf("row-%d", i), json.RawMessage(r)); err != nil {
						t.Fatalf("Upsert row %d: %v", i, err)
					}
				}
			case "typed":
				for _, r := range rows {
					// A row whose only identity was the credential has none left: refused, not stored.
					_ = s.UpsertClients(json.RawMessage(r))
				}
			}
			assertEntryClean(t, s, dir, path)
			if err := s.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			assertEntryFilesClean(t, dir, path)
		})
	}
}

func assertEntryClean(t *testing.T, s *Store, dir, path string) {
	t.Helper()
	tables, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var names []string
	for tables.Next() {
		var n string
		_ = tables.Scan(&n)
		names = append(names, n)
	}
	_ = tables.Close()
	for _, table := range names {
		r, err := s.db.Query(fmt.Sprintf(`SELECT * FROM "%s"`, table))
		if err != nil {
			continue
		}
		cols, _ := r.Columns()
		for r.Next() {
			cells := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range cells {
				ptrs[i] = &cells[i]
			}
			if err := r.Scan(ptrs...); err != nil {
				t.Fatalf("scan %s: %v", table, err)
			}
			for i, cell := range cells {
				var text string
				switch v := cell.(type) {
				case []byte:
					text = string(v)
				case string:
					text = v
				default:
					continue
				}
				for _, sentinel := range entrySentinels {
					if strings.Contains(text, sentinel) {
						t.Errorf("%s: %s.%s holds %s", path, table, cols[i], sentinel)
					}
				}
			}
		}
		_ = r.Close()
	}
}

func assertEntryFilesClean(t *testing.T, dir, path string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var all []byte
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, sentinel := range entrySentinels {
			if bytes.Contains(body, []byte(sentinel)) {
				t.Errorf("%s: %s holds %s", path, e.Name(), sentinel)
			}
		}
		all = append(all, body...)
	}
	want := []string{entryOrdinary1, entryOrdinary2, entryOrdinary3, entryOrdinary4, `"total_tokens":42`, `"code":"ok"`}
	if path == "typed" {
		// The typed writers do not unwrap envelopes, so the wrapped row has no
		// identity there and is refused whole.
		want = []string{entryOrdinary1, entryOrdinary3, entryOrdinary4, `"total_tokens":42`, `"code":"ok"`}
	}
	for _, ordinary := range want {
		if !bytes.Contains(all, []byte(ordinary)) {
			t.Errorf("%s: no store file holds the ordinary value %s; the check proves nothing", path, ordinary)
		}
	}
}
