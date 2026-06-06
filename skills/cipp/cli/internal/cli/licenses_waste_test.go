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

func seedLicensedUser(t *testing.T, db *store.Store, id, tenant, sku, lastSignIn string) {
	t.Helper()
	obj := map[string]any{
		"id":                id,
		"_tenant":           tenant,
		"userPrincipalName": id + "@" + tenant,
		"assignedLicenses":  []any{map[string]any{"skuPartNumber": sku}},
	}
	if lastSignIn != "" {
		obj["LastSignInDateTime"] = lastSignIn
	}
	raw, _ := json.Marshal(obj)
	if err := db.Upsert("users", id, raw); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func runWaste(t *testing.T, dbPath string) []wasteRow {
	t.Helper()
	var flags rootFlags
	flags.asJSON = true
	cmd := newNovelLicensesWasteCmd(&flags)
	cmd.SetArgs([]string{"--db", dbPath})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute waste: %v", err)
	}
	var rows []wasteRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal waste output %q: %v", out.String(), err)
	}
	return rows
}

func TestLicensesWasteDetectsDormant(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	tenant := "contoso.onmicrosoft.com"
	old := time.Now().AddDate(0, 0, -90).UTC().Format(time.RFC3339)
	recent := time.Now().AddDate(0, 0, -2).UTC().Format(time.RFC3339)
	// Two users on the same SKU: one dormant (90d), one active (2d).
	seedLicensedUser(t, db, "dormant", tenant, "ENTERPRISEPACK", old)
	seedLicensedUser(t, db, "active", tenant, "ENTERPRISEPACK", recent)
	db.Close()

	rows := runWaste(t, dbPath)
	if len(rows) != 1 {
		t.Fatalf("expected 1 waste row, got %d: %#v", len(rows), rows)
	}
	r := rows[0]
	if r.Tenant != tenant || r.LicenseName != "ENTERPRISEPACK" || r.Assigned != 2 || r.Unused != 1 {
		t.Fatalf("waste row wrong: %#v", r)
	}
	fmt.Printf("licenses waste detected: %+v\n", r)
}

func TestLicensesWasteEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close()

	var flags rootFlags
	flags.asJSON = true
	cmd := newNovelLicensesWasteCmd(&flags)
	cmd.SetArgs([]string{"--db", dbPath})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute (empty): %v", err)
	}
	var rows []wasteRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("unmarshal empty output %q: %v", out.String(), err)
	}
	if len(rows) != 0 {
		t.Fatalf("empty store should yield 0 waste rows, got %d", len(rows))
	}
}

func TestUserLastSignInNestedSignInActivity(t *testing.T) {
	// Graph/CIPP commonly ship sign-in data ONLY as the nested signInActivity
	// object; the flat LastSignInDateTime field is absent. A string-only
	// extractor returns has=false here, which makes `licenses waste`
	// under-report and `users stale` over-report.
	obj := map[string]any{
		"userPrincipalName": "dormant@contoso.com",
		"signInActivity": map[string]any{
			"lastSignInDateTime":               "2026-01-15T08:30:00Z",
			"lastNonInteractiveSignInDateTime": "2026-02-01T10:00:00Z",
		},
	}
	ts, ok := userLastSignIn(obj)
	if !ok {
		t.Fatal("nested signInActivity object was not extracted")
	}
	want := time.Date(2026, 1, 15, 8, 30, 0, 0, time.UTC)
	if !ts.Equal(want) {
		t.Fatalf("expected lastSignInDateTime %v, got %v", want, ts)
	}

	// Timezone-less nested shape also parses.
	obj2 := map[string]any{
		"signInActivity": map[string]any{"lastSignInDateTime": "2026-01-15T08:30:00"},
	}
	if _, ok := userLastSignIn(obj2); !ok {
		t.Fatal("timezone-less nested timestamp was not parsed")
	}
}
