// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"cipp-pp-cli/internal/store"
)

func openTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func rawRows(t *testing.T, objs ...map[string]any) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, 0, len(objs))
	for _, o := range objs {
		b, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func TestFanoutRefusesActionEndpoints(t *testing.T) {
	for _, e := range []string{"/ExecRunBackup", "/ExecSendOrgMessage", "/RemoveStandard", "/RemoveTenantCapabilitiesCache",
		"/AddUser", "/ListUsers?tenantFilter=x", "/ListUsers/../ExecRunBackup", "/List Users"} {
		if _, err := resolveFanoutEndpoints(e, ""); err == nil {
			t.Errorf("fanout accepted %q; only List*/Get* reads may fan out", e)
		}
	}
	for _, e := range []string{"/ListUsers", "ListMFAUsers", "/GetVersion", "/BestPracticeAnalyser_List", "/listStandardTemplates", "/PublicPing"} {
		if _, err := resolveFanoutEndpoints(e, ""); err != nil {
			t.Errorf("fanout refused read endpoint %q: %v", e, err)
		}
	}
	for name, eps := range fanoutPresets {
		for _, e := range eps {
			if err := checkFanoutEndpoint(e); err != nil {
				t.Errorf("preset %s carries a non-read endpoint: %v", name, err)
			}
		}
	}
}

func TestStampTenantGivesIDLessRowsAStableID(t *testing.T) {
	in := rawRows(t,
		map[string]any{"UPN": "a@contoso.com", "Name": "Junk", "Identity": "a@contoso.com\\1"},
		map[string]any{"UPN": "b@contoso.com", "Name": "Junk", "Identity": "b@contoso.com\\1"},
		map[string]any{"id": "real-id", "Name": "x"},
	)
	out := stampTenant(in, "contoso.com")
	var a, b, c map[string]any
	_ = json.Unmarshal(out[0], &a)
	_ = json.Unmarshal(out[1], &b)
	_ = json.Unmarshal(out[2], &c)
	if a["_id"] == nil || a["_id"] == b["_id"] {
		t.Fatalf("id-less rows sharing a Name need distinct _id values: %v vs %v", a["_id"], b["_id"])
	}
	if _, ok := c["_id"]; ok {
		t.Fatalf("a row with a real id must not get a synthetic _id: %v", c)
	}
	if a["_tenant"] != "contoso.com" {
		t.Fatalf("_tenant not stamped: %v", a)
	}
}

func TestTenantScopedRowsDoNotOverwriteEachOther(t *testing.T) {
	db, _ := openTestStore(t)
	store.TenantScopeResource("licenses")
	for _, tenant := range []string{"a.onmicrosoft.com", "b.onmicrosoft.com"} {
		rows := stampTenant(rawRows(t, map[string]any{"id": "sku-e3", "skuPartNumber": "ENTERPRISEPACK"}), tenant)
		if _, _, err := db.UpsertBatch("licenses", rows); err != nil {
			t.Fatal(err)
		}
	}
	got, err := readResourceRows(db, "licenses")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("two tenants sharing a SKU id must keep two rows, got %d: %v", len(got), got)
	}
}

func TestPickSnapshotPair(t *testing.T) {
	snaps := []string{"2026-10-01T00:00:00Z", "2026-10-10T00:00:00Z", "2026-10-20T00:00:00Z"}
	if b, c, ok := pickSnapshotPair(snaps, time.Time{}); !ok || b != snaps[1] || c != snaps[2] {
		t.Fatalf("default pair = %s,%s,%v", b, c, ok)
	}
	cutoff, _ := time.Parse(time.RFC3339, "2026-10-05T00:00:00Z")
	if b, _, ok := pickSnapshotPair(snaps, cutoff); !ok || b != snaps[0] {
		t.Fatalf("--since base = %s,%v, want %s", b, ok, snaps[0])
	}
	early, _ := time.Parse(time.RFC3339, "2026-09-01T00:00:00Z")
	if _, _, ok := pickSnapshotPair(snaps, early); ok {
		t.Fatal("no snapshot older than the window must yield no pair, not a guess")
	}
	if _, _, ok := pickSnapshotPair(snaps[:1], time.Time{}); ok {
		t.Fatal("one snapshot must yield no pair")
	}
}

func TestChangesReportsAddedRemovedModified(t *testing.T) {
	db, path := openTestStore(t)
	ctx := context.Background()
	tenant := "contoso.onmicrosoft.com"
	if err := appendResourceSnapshot(ctx, db, "2026-10-01T00:00:00Z", "roles", tenant, rawRows(t,
		map[string]any{"id": "r1", "DisplayName": "Global Administrator", "Members": "a@contoso.com"},
		map[string]any{"id": "r2", "DisplayName": "Exchange Administrator"},
	)); err != nil {
		t.Fatal(err)
	}
	if err := appendResourceSnapshot(ctx, db, "2026-10-08T00:00:00Z", "roles", tenant, rawRows(t,
		map[string]any{"id": "r1", "DisplayName": "Global Administrator", "Members": "a@contoso.com, mallory@contoso.com"},
		map[string]any{"id": "r3", "DisplayName": "Security Administrator"},
	)); err != nil {
		t.Fatal(err)
	}
	db.Close()

	var flags rootFlags
	flags.asJSON = true
	cmd := newNovelChangesCmd(&flags)
	cmd.SetArgs([]string{"--resource", "/ListRoles", "--db", path})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var rows []changeRow
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	got := map[string]changeRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	if got["r1"].Change != "modified" || len(got["r1"].ChangedFields) != 1 || got["r1"].ChangedFields[0] != "Members" {
		t.Fatalf("r1 = %+v, want modified Members", got["r1"])
	}
	if got["r2"].Change != "removed" || got["r3"].Change != "added" || len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestSnapshotRetentionPrunes(t *testing.T) {
	db, _ := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < snapshotRetention+5; i++ {
		ts := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		if err := appendResourceSnapshot(ctx, db, ts, "roles", "t", rawRows(t, map[string]any{"id": "r1"})); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := loadResourceSnapshots(db, "roles", "t")
	if err != nil {
		t.Fatal(err)
	}
	snaps := map[string]bool{}
	for _, r := range rows {
		snaps[r.ts] = true
	}
	if len(snaps) != snapshotRetention {
		t.Fatalf("kept %d snapshots, want %d", len(snaps), snapshotRetention)
	}
}

func TestOffboardCheckFindsLeftovers(t *testing.T) {
	tenant := "contoso.onmicrosoft.com"
	users := []map[string]any{
		{"_tenant": tenant, "userPrincipalName": "Gone@contoso.com", "accountEnabled": false,
			"assignedLicenses": []any{map[string]any{"skuId": "e3"}}},
		{"_tenant": tenant, "userPrincipalName": "here@contoso.com", "accountEnabled": true},
		{"_tenant": tenant, "userPrincipalName": "clean@contoso.com", "accountEnabled": false},
	}
	fwd := []map[string]any{{"_tenant": tenant, "UPN": "gone@contoso.com", "ForwardingSmtpAddress": "smtp:x@evil.example"}}
	rules := []map[string]any{{"_tenant": tenant, "UserPrincipalName": "gone@contoso.com", "Name": "r", "Enabled": true}}
	roles := []map[string]any{{"_tenant": tenant, "DisplayName": "Global Administrator", "Members": "Gone User (gone@contoso.com), ann@contoso.com"},
		{"_tenant": "other.onmicrosoft.com", "DisplayName": "Leak", "Members": "gone@contoso.com"}}

	out := computeOffboardLeftovers(users, fwd, rules, roles, false)
	if len(out) != 1 {
		t.Fatalf("want only the disabled account holding things, got %+v", out)
	}
	r := out[0]
	if r.Forwarding == "" || r.InboxRules != 1 || len(r.Roles) != 1 || r.Roles[0] != "Global Administrator" || len(r.Licenses) == 0 {
		t.Fatalf("leftovers wrong (a role in another tenant must not count): %+v", r)
	}
	if len(r.Unchecked) != 0 {
		t.Fatalf("all sources were stored, unchecked = %v", r.Unchecked)
	}
	all := computeOffboardLeftovers(users, nil, nil, nil, true)
	for _, row := range all {
		if row.UserPrincipalName == "here@contoso.com" {
			t.Fatal("enabled accounts are not offboarded")
		}
		if row.UserPrincipalName == "clean@contoso.com" && len(row.Unchecked) != 3 {
			t.Fatalf("absent sources must be reported unchecked: %+v", row)
		}
	}
}

func TestUsersStaleSkipsDisabledAccounts(t *testing.T) {
	users := []map[string]any{
		{"_tenant": "t", "userPrincipalName": "off@t", "accountEnabled": false, "signInActivity": nil, "assignedLicenses": []any{map[string]any{"skuId": "e3"}}},
		{"_tenant": "t", "userPrincipalName": "on@t", "accountEnabled": true, "signInActivity": nil, "assignedLicenses": []any{map[string]any{"skuId": "e3"}}},
		// No sign-in field at all (/ListUsers without IncludeLogonDetails):
		// unknown, never reported as stale.
		{"_tenant": "t", "userPrincipalName": "unknown@t", "accountEnabled": true, "assignedLicenses": []any{map[string]any{"skuId": "e3"}}},
	}
	out := computeStaleUsers(users, time.Now(), 90)
	if len(out) != 1 || out[0].UserPrincipalName != "on@t" {
		t.Fatalf("stale = %+v, want only the enabled licensed account with sign-in data", out)
	}
}

func TestScorecardMarksMissingSignalsNoData(t *testing.T) {
	db, _ := openTestStore(t)
	tenant := "contoso.onmicrosoft.com"
	seedUser(t, db, "u1", tenant, true)
	seedUser(t, db, "u2", tenant, false)
	seedUser(t, db, "u3", "other.onmicrosoft.com", false)
	rep, err := buildScorecard(db, tenant, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	sig := map[string]scorecardSignal{}
	for _, s := range rep.Signals {
		sig[s.Signal] = s
	}
	mfa := sig["posture.mfa"]
	m, _ := mfa.Result.(map[string]int)
	if mfa.NoData || m["users"] != 2 || m["mfa_enabled"] != 1 || mfa.SyncedAt == "" {
		t.Fatalf("posture.mfa = %+v", mfa)
	}
	for _, name := range []string{"posture.ca", "ca.baseline_diff", "standards.drift", "gdap.relationships"} {
		if !sig[name].NoData {
			t.Fatalf("%s has nothing stored and must say no_data: %+v", name, sig[name])
		}
	}
}

func TestBulkIsNotAnMCPTool(t *testing.T) {
	var flags rootFlags
	cmd := newNovelBulkCmd(&flags)
	if cmd.Annotations["mcp:hidden"] != "true" || cmd.Annotations["mcp:read-flags"] != "from" {
		t.Fatalf("bulk reads a host CSV and writes without the MCP confirm; annotations = %v", cmd.Annotations)
	}
}
