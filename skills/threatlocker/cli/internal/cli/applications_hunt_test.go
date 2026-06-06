// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"threatlocker-pp-cli/internal/store"
)

const huntTestHash = "3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1b"

// seedHuntStore creates a temp store with the same file pending in two tenants
// and trusted by one application file rule.
func seedHuntStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "hunt-test.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("opening test store: %v", err)
	}
	defer db.Close()
	stmts := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO approvals (id, data, organization_id, organization_name, computer_name, file_name, full_path, hash, status_id, status, date_requested)
			VALUES (?, '{}', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			[]any{"a1", "org-1", "Tenant Alpha", "WS-ALPHA-01", "tool.exe", `C:\tools\tool.exe`, huntTestHash, 1, "Pending", "2026-06-01T12:00:00Z"}},
		{`INSERT INTO approvals (id, data, organization_id, organization_name, computer_name, file_name, full_path, hash, status_id, status, date_requested)
			VALUES (?, '{}', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			[]any{"a2", "org-2", "Tenant Beta", "WS-BETA-07", "tool.exe", `C:\tools\tool.exe`, huntTestHash, 2, "Approved", "2026-05-30T08:00:00Z"}},
		{`INSERT INTO applications (id, data, application_id, name) VALUES (?, '{}', ?, ?)`,
			[]any{"app1", "app-guid-1", "Example Tool"}},
		{`INSERT INTO application_files (id, data, application_file_id, application_id, full_path, hash, cert, process_path)
			VALUES (?, '{}', ?, ?, ?, ?, ?, ?)`,
			[]any{"af1", "af-guid-1", "app-guid-1", `C:\tools\tool.exe`, huntTestHash, "CN=Example Corp", ""}},
	}
	for _, s := range stmts {
		if _, err := db.DB().ExecContext(context.Background(), s.sql, s.args...); err != nil {
			t.Fatalf("seeding store: %v", err)
		}
	}
	return dbPath
}

func runHunt(t *testing.T, args ...string) huntView {
	t.Helper()
	var flags rootFlags
	root := newRootCmd(&flags)
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"applications", "hunt", "--json"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("hunt execute: %v", err)
	}
	var view huntView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("hunt output is not valid JSON: %v\n%s", err, out.String())
	}
	return view
}

func TestApplicationsHuntByHashSpansTenants(t *testing.T) {
	dbPath := seedHuntStore(t)
	view := runHunt(t, "--hash", huntTestHash, "--db", dbPath)
	if len(view.Approvals) != 2 {
		t.Fatalf("expected 2 approval hits, got %d", len(view.Approvals))
	}
	if len(view.TenantRollup) != 2 {
		t.Fatalf("expected 2 tenants in rollup, got %d", len(view.TenantRollup))
	}
	// Pending tenant ranks first.
	if view.TenantRollup[0].Organization != "Tenant Alpha" || view.TenantRollup[0].Pending != 1 {
		t.Errorf("expected Tenant Alpha pending=1 first, got %+v", view.TenantRollup[0])
	}
	if view.TenantRollup[1].Processed != 1 {
		t.Errorf("expected Tenant Beta processed=1, got %+v", view.TenantRollup[1])
	}
	if len(view.TrustedFiles) != 1 || view.TrustedFiles[0].Application != "Example Tool" {
		t.Errorf("expected 1 trusted file for Example Tool, got %+v", view.TrustedFiles)
	}
}

func TestApplicationsHuntCaseInsensitiveHash(t *testing.T) {
	dbPath := seedHuntStore(t)
	view := runHunt(t, "--hash", "3A7BD3E2360A3D29EEA436FCFB7E44C735D117C42D1C1835420B6B9942DD4F1B", "--db", dbPath)
	if len(view.Approvals) != 2 {
		t.Errorf("uppercase hash should match (case-insensitive), got %d approvals", len(view.Approvals))
	}
}

func TestApplicationsHuntByPathSubstring(t *testing.T) {
	dbPath := seedHuntStore(t)
	view := runHunt(t, "--path", "tool.exe", "--db", dbPath)
	if len(view.Approvals) != 2 || len(view.TrustedFiles) != 1 {
		t.Errorf("path substring should match both surfaces, got approvals=%d trustedFiles=%d", len(view.Approvals), len(view.TrustedFiles))
	}
}

func TestApplicationsHuntNoMatchReturnsHonestNote(t *testing.T) {
	dbPath := seedHuntStore(t)
	view := runHunt(t, "--hash", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "--db", dbPath)
	if len(view.Approvals) != 0 || len(view.TrustedFiles) != 0 {
		t.Errorf("mismatching hash must not return rows, got approvals=%d trustedFiles=%d", len(view.Approvals), len(view.TrustedFiles))
	}
	if view.Note == "" {
		t.Error("zero-match result must carry an honest note")
	}
}

func TestApplicationsHuntCertResolvesToApprovalsViaHash(t *testing.T) {
	dbPath := seedHuntStore(t)
	view := runHunt(t, "--cert", "Example Corp", "--db", dbPath)
	if len(view.TrustedFiles) != 1 {
		t.Fatalf("cert should match 1 trusted file rule, got %d", len(view.TrustedFiles))
	}
	// The cert's file rule carries the hash; approvals must be matched via
	// that resolved hash, not via a filename-substring fallback.
	if len(view.Approvals) != 2 {
		t.Errorf("cert hunt should resolve to hash and find both tenants' approvals, got %d", len(view.Approvals))
	}
	if len(view.TenantRollup) != 2 {
		t.Errorf("cert hunt rollup should span 2 tenants, got %d", len(view.TenantRollup))
	}
}

func TestApplicationsHuntCertOnlyUnresolvedSkipsApprovals(t *testing.T) {
	dbPath := seedHuntStore(t)
	// Cert matches nothing → no hashes to resolve → approvals must be skipped
	// with an explicit note, not silently matched by filename.
	view := runHunt(t, "--cert", "No Such Issuer", "--db", dbPath)
	if len(view.Approvals) != 0 || len(view.TrustedFiles) != 0 {
		t.Errorf("unmatched cert must return no rows, got approvals=%d trustedFiles=%d", len(view.Approvals), len(view.TrustedFiles))
	}
	if view.Note == "" {
		t.Error("unmatched cert-only hunt must carry an explanatory note")
	}
}

func TestApplicationsHuntRollupCountsBeyondLimit(t *testing.T) {
	dbPath := seedHuntStore(t)
	// Seed 5 more pending approvals for the same hash in a third tenant.
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("reopening store: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := db.DB().ExecContext(context.Background(),
			`INSERT INTO approvals (id, data, organization_id, organization_name, computer_name, file_name, full_path, hash, status_id, status, date_requested)
			 VALUES (?, '{}', 'org-3', 'Tenant Gamma', ?, 'tool.exe', 'C:\tools\tool.exe', ?, 1, 'Pending', '2026-06-02T09:00:00Z')`,
			"g"+string(rune('0'+i)), "WS-GAMMA-0"+string(rune('0'+i)), huntTestHash); err != nil {
			t.Fatalf("seeding extra approvals: %v", err)
		}
	}
	db.Close()
	// --limit 3 caps the detail list, but the rollup must still count all 7
	// matching rows (2 from seedHuntStore + 5 here) across 3 tenants.
	view := runHunt(t, "--hash", huntTestHash, "--db", dbPath, "--limit", "3")
	if len(view.Approvals) != 3 {
		t.Errorf("detail rows should be capped at --limit 3, got %d", len(view.Approvals))
	}
	if len(view.TenantRollup) != 3 {
		t.Fatalf("rollup should span 3 tenants regardless of --limit, got %d", len(view.TenantRollup))
	}
	total := 0
	for _, r := range view.TenantRollup {
		total += r.Pending + r.Processed
	}
	if total != 7 {
		t.Errorf("rollup must count all 7 matching rows (blast radius), got %d", total)
	}
	if view.Note == "" {
		t.Error("capped detail list must carry the truncation note")
	}
}

func TestApplicationsHuntRequiresAQueryFlag(t *testing.T) {
	dbPath := seedHuntStore(t)
	var flags rootFlags
	root := newRootCmd(&flags)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"applications", "hunt", "--json", "--db", dbPath})
	if err := root.Execute(); err == nil {
		t.Error("hunt without --hash/--cert/--path must fail with a usage error")
	}
}
