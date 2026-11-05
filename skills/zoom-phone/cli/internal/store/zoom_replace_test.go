// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func zoomStoredRow(t *testing.T, sync bool, rows ...string) map[string]any {
	t.Helper()
	if sync {
		defer BeginSyncReplace()()
	}
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, row := range rows {
		if _, _, err := s.UpsertBatch("numbers", []json.RawMessage{json.RawMessage(row)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Get("numbers", "n1")
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if err := json.Unmarshal(got, &obj); err != nil {
		t.Fatal(err)
	}
	return obj
}

// A sync shows the current state: an unassigned number loses its assignee and
// an emptied array stays empty. Outside sync the generated merge still keeps
// the richer row (detail write-through).
func TestZoomSyncReplacesStoredRow(t *testing.T) {
	first := `{"id":"n1","number":"+14155550101","assignee":{"id":"u1","type":"user"},"tags":["a"]}`
	later := `{"id":"n1","number":"+14155550101","tags":[]}`
	obj := zoomStoredRow(t, true, first, later)
	if _, stale := obj["assignee"]; stale {
		t.Fatalf("sync kept a stale assignee: %v", obj)
	}
	if tags, _ := obj["tags"].([]any); len(tags) != 0 {
		t.Fatalf("sync kept old array values: %v", obj)
	}
	if merged := zoomStoredRow(t, false, first, later); merged["assignee"] == nil {
		t.Fatalf("write-through lost the richer row: %v", merged)
	}
	if zoomSyncReplaceDepth.Load() != 0 {
		t.Fatal("sync replace depth leaked")
	}
}

func TestZoomSecretsNeverStored(t *testing.T) {
	row := `{"id":"n1","provision":{"sip_accounts":[{"password":"SIP-SECRET","authorization_id":"a1"}]},"desk_phone":{"pin_code":"4321"},"chat_channels":[{"token":"chat-tok"}],"activation_code":"AC-99","sync_token":"keep","pin":""}`
	obj := zoomStoredRow(t, true, row)
	raw, _ := json.Marshal(obj)
	for _, secret := range []string{"SIP-SECRET", "4321", "chat-tok", "AC-99"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("secret %q stored: %s", secret, raw)
		}
	}
	if !strings.Contains(string(raw), `"sync_token":"keep"`) || !strings.Contains(string(raw), `"authorization_id":"a1"`) {
		t.Fatalf("non-secret fields were altered: %s", raw)
	}
}

// An older row written before redaction must not carry its secret forward
// through a later merge (write-through outside sync).
func TestZoomMergeDropsOlderSecrets(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	legacy := `{"id":"d1","name":"Lobby phone","provision":{"sip_accounts":[{"password":"LEGACY-SIP-SECRET"}]}}`
	if _, err := s.DB().Exec(`INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES ('d1','devices',?,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`, legacy); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch("devices", []json.RawMessage{json.RawMessage(`{"id":"d1","name":"Lobby phone 2"}`)}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("devices", "d1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "LEGACY-SIP-SECRET") || !strings.Contains(string(got), "Lobby phone 2") {
		t.Fatalf("merge kept the legacy secret or lost the update: %s", got)
	}
}
