// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

// Review round 1 (Codex, 2026-10-28) [P1] class credential persisted in the
// store: a record's ID (and its parent ID, its typed columns, a sync cursor)
// was extracted before any withholding ran, so `sync` stored a credential
// unchanged in resources.id, resources_fts.id and the typed table. Every
// write entry point the store exposes is driven here with a value that
// repeats a held credential, as written and as ROT13 (an encoding the
// client's masker knows), and the database files are then read byte by byte.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "kaseya-vsa-pp-cli/internal/client" // links the client masker into the chokepoint, as both binaries do
	"kaseya-vsa-pp-cli/internal/cliutil"
)

const r1aSecret = "Secret" + "Alpha901" // split: a synthetic fixture, not a credential for secret scanners

func r1aRot13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case r >= 'A' && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}

func r1aJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestR1AHeldValueNeverReachesAnyStoreColumn(t *testing.T) {
	cliutil.FunnelHold(r1aSecret)
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "r1a.db"))
	if err != nil {
		t.Fatal(err)
	}
	forms := []string{r1aSecret, r1aRot13(r1aSecret)}
	for i, form := range forms {
		ok := "org-ok-" + string(rune('a'+i))
		// Batch upsert (sync and the write-through cache): a record whose ID
		// repeats the credential beside an ordinary one.
		if _, _, err := s.UpsertBatch("organizations", []json.RawMessage{
			r1aJSON(t, map[string]any{"Id": form, "Name": "Acme"}),
			r1aJSON(t, map[string]any{"Id": ok, "Name": "Beta Industries"}),
		}); err != nil {
			t.Logf("UpsertBatch: %v", err)
		}
		// A parent ID is part of a dependent record's storage key.
		_, _, _, _ = s.UpsertBatchDetailed("devices_notifications", []json.RawMessage{
			r1aJSON(t, map[string]any{"Id": "n-ok", "parent_id": form, "Message": "disk"}),
		})
		// Generic single upsert, with the ID supplied by the caller.
		_ = s.Upsert("organizations", form, json.RawMessage(`{"Name":"single"}`))
		// Every typed entry point (UpsertAssets, UpsertDevices, ...).
		rec := r1aJSON(t, map[string]any{"Id": form, "Identifier": form, "AuditLogRecordId": form,
			"Name": "typed", "parent_id": "p-1"})
		typed := 0
		v := reflect.ValueOf(s)
		for m := 0; m < v.NumMethod(); m++ {
			name := v.Type().Method(m).Name
			fn, isTyped := v.Method(m).Interface().(func(json.RawMessage) error)
			if !isTyped || !strings.HasPrefix(name, "Upsert") {
				continue
			}
			typed++
			_ = fn(rec)
		}
		if typed < 17 {
			t.Fatalf("found %d typed upsert entry points, want at least 17", typed)
		}
		// Sync cursors and scope keys.
		_ = s.SaveSyncState("organizations", form, 1)
		_ = s.SaveSyncProgress("organizations", form, 1)
		_ = s.SaveSyncStateInScope("devices", form, form, 1)
		_ = s.SaveSyncProgressInScope("devices", form, form, 1)
		_ = s.SaveSyncCursor("sites", form)
		// Any other write through the store's database handle (learnings,
		// lookups, playbooks and events use the same handle).
		_, _ = s.DB().Exec(`INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES (?, 'raw', '{}', '', '')`, form)
		_, _ = s.DB().Exec(`INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES ('raw-1', 'raw', ?, '', '')`, `{"note":"`+form+`"}`)
		if _, err := s.Get("organizations", ok); err != nil {
			t.Fatalf("the ordinary record %s was not stored, so this test proves nothing: %v", ok, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var scanned int
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		scanned++
		low := bytes.ToLower(data)
		for _, form := range forms {
			if bytes.Contains(low, []byte(strings.ToLower(form))) {
				t.Errorf("%s holds the credential as %q", e.Name(), form)
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no database file to scan")
	}
}
