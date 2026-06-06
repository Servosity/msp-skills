// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"cipp-pp-cli/internal/store"
)

// seedStaleUser stores a user with a license and an optional last sign-in.
func seedStaleUser(t *testing.T, db *store.Store, id, tenant string, licensed bool, lastSignIn string) {
	t.Helper()
	obj := map[string]any{
		"id":                id,
		"_tenant":           tenant,
		"userPrincipalName": id + "@" + tenant,
		"displayName":       id,
	}
	if licensed {
		obj["assignedLicenses"] = []any{map[string]any{"skuPartNumber": "ENTERPRISEPACK"}}
	}
	if lastSignIn != "" {
		obj["LastSignInDateTime"] = lastSignIn
	}
	raw, _ := json.Marshal(obj)
	if err := db.Upsert("users", id, raw); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func TestUsersStaleFlagsOnlyStaleLicensed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	tenant := "contoso.onmicrosoft.com"
	old := time.Now().AddDate(0, 0, -200).UTC().Format(time.RFC3339)
	recent := time.Now().AddDate(0, 0, -5).UTC().Format(time.RFC3339)

	// stale-licensed: licensed, signed in 200d ago -> flagged for --days 90
	seedStaleUser(t, db, "stale-licensed", tenant, true, old)
	// recent-licensed: licensed, signed in 5d ago -> NOT flagged
	seedStaleUser(t, db, "recent-licensed", tenant, true, recent)
	// stale-unlicensed: NOT licensed, old sign-in -> NOT flagged (only licensed)
	seedStaleUser(t, db, "stale-unlicensed", tenant, false, old)
	// never-licensed: licensed, no sign-in at all -> flagged (null = stale)
	seedStaleUser(t, db, "never-signed-in", tenant, true, "")
	db.Close()

	var flags rootFlags
	flags.asJSON = true
	cmd := newNovelUsersStaleCmd(&flags)
	cmd.SetArgs([]string{"--days", "90", "--db", dbPath})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var rows []staleRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal output %q: %v", out.String(), err)
	}
	got := map[string]staleRow{}
	for _, r := range rows {
		got[r.UserPrincipalName] = r
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 stale licensed rows, got %d: %#v", len(rows), rows)
	}
	if _, ok := got["stale-licensed@"+tenant]; !ok {
		t.Fatalf("stale-licensed user not flagged; output=%s", out.String())
	}
	if _, ok := got["never-signed-in@"+tenant]; !ok {
		t.Fatalf("never-signed-in licensed user not flagged; output=%s", out.String())
	}
	if _, ok := got["recent-licensed@"+tenant]; ok {
		t.Fatalf("recent-licensed user should NOT be flagged")
	}
	if _, ok := got["stale-unlicensed@"+tenant]; ok {
		t.Fatalf("unlicensed user should NOT be flagged")
	}
	fmt.Printf("users stale flagged: %v\n", func() []string {
		var n []string
		for k := range got {
			n = append(n, k)
		}
		return n
	}())
}
