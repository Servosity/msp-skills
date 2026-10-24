// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/acronis/handfixes.json under
// the "credential-fields-never-persisted" entry. See docs/reprint-survival.md.

package cache

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSetNeverWritesACredential pins that the file cache refuses a value
// carrying a credential field (and drops the stale entry) while a plain value
// is still written.
func TestSetNeverWritesACredential(t *testing.T) {
	s := New(t.TempDir(), time.Minute)
	s.Set("plain", json.RawMessage(`{"id":"t-1","name":"Acme"}`))
	if _, ok := s.Get("plain"); !ok {
		t.Fatal("a plain value was not cached; the check below proves nothing")
	}
	s.Set("k", json.RawMessage(`{"id":"c-1","client_id":"c"}`))
	s.Set("k", json.RawMessage(`{"id":"c-1","client_secret":"test-secret-aaaaaaaaaaaa"}`))
	if got, ok := s.Get("k"); ok {
		t.Fatalf("a credential-bearing value (or the stale one) was served: %s", got)
	}
}
