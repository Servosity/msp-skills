// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"cipp-pp-cli/internal/store"
)

// seedUser stores one user row stamped with a tenant and MFA state.
func seedUser(t *testing.T, db *store.Store, id, tenant string, mfa bool) {
	t.Helper()
	obj := map[string]any{
		"id":                id,
		"_tenant":           tenant,
		"userPrincipalName": id + "@" + tenant,
		"MFARegistration":   mfa,
	}
	raw, _ := json.Marshal(obj)
	if err := db.Upsert("users", id, raw); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func TestPostureMFAMatrix(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Tenant A: 2 users, 1 with MFA. Tenant B: 1 user, MFA on.
	seedUser(t, db, "a1", "contoso.onmicrosoft.com", true)
	seedUser(t, db, "a2", "contoso.onmicrosoft.com", false)
	seedUser(t, db, "b1", "fabrikam.onmicrosoft.com", true)
	db.Close()

	var flags rootFlags
	flags.asJSON = true
	cmd := newNovelPostureCmd(&flags)
	cmd.SetArgs([]string{"--dimension", "mfa", "--db", dbPath})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var rows []postureRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal output %q: %v", out.String(), err)
	}
	got := map[string]map[string]int{}
	for _, r := range rows {
		got[r.Tenant] = r.Metrics
	}
	contoso, ok := got["contoso.onmicrosoft.com"]
	if !ok {
		t.Fatalf("missing contoso row; output=%s", out.String())
	}
	if contoso["users"] != 2 || contoso["mfa_enabled"] != 1 || contoso["mfa_missing"] != 1 {
		t.Fatalf("contoso metrics wrong: %#v (output=%s)", contoso, out.String())
	}
	fabrikam := got["fabrikam.onmicrosoft.com"]
	if fabrikam["users"] != 1 || fabrikam["mfa_enabled"] != 1 || fabrikam["mfa_missing"] != 0 {
		t.Fatalf("fabrikam metrics wrong: %#v", fabrikam)
	}
	fmt.Printf("posture mfa matrix: contoso=%v fabrikam=%v\n", contoso, fabrikam)
}

func TestPostureHonestEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close() // empty store

	var flags rootFlags
	flags.asJSON = true
	cmd := newNovelPostureCmd(&flags)
	cmd.SetArgs([]string{"--dimension", "mfa", "--db", dbPath})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute (empty) should not error, got: %v", err)
	}

	var rows []postureRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal empty output %q: %v", out.String(), err)
	}
	if len(rows) != 0 {
		t.Fatalf("empty store should yield 0 rows, got %d", len(rows))
	}
	if errBuf.Len() == 0 {
		t.Fatalf("expected honest-empty hint on stderr, got nothing")
	}
	fmt.Printf("posture honest-empty stderr: %s", errBuf.String())
}
