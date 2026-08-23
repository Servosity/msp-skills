// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"microsoft-graph-pp-cli/internal/store"
)

// seedStore creates a temp store, populates the domain tables that the
// transcendence commands read, and returns its path. It exercises the real
// typed Upsert path so the test covers the same store schema the live `pull`
// writes.
func seedStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	st, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	mustJSON := func(v any) json.RawMessage {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return b
	}

	// licenses: one wasteful SKU, one fully consumed
	if err := st.UpsertLicenses(mustJSON(map[string]any{"id": "sku-ent", "skuId": "ent", "skuPartNumber": "ENTERPRISEPACK", "consumedUnits": 80, "prepaidUnits": map[string]any{"enabled": 100}})); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertLicenses(mustJSON(map[string]any{"id": "sku-ems", "skuId": "ems", "skuPartNumber": "EMS", "consumedUnits": 10, "prepaidUnits": map[string]any{"enabled": 10}})); err != nil {
		t.Fatal(err)
	}

	// users: an active member, a disabled licensed member (orphan), a guest
	if err := st.UpsertUsers(mustJSON(map[string]any{"id": "u1", "userPrincipalName": "active@contoso.com", "accountEnabled": true, "userType": "Member"})); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUsers(mustJSON(map[string]any{"id": "u2", "userPrincipalName": "ex@contoso.com", "displayName": "Ex Employee", "accountEnabled": false, "userType": "Member", "assignedLicenses": []map[string]any{{"skuId": "ent"}}})); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUsers(mustJSON(map[string]any{"id": "u3", "userPrincipalName": "guest@partner.com", "accountEnabled": true, "userType": "Guest"})); err != nil {
		t.Fatal(err)
	}

	// directory role with an embedded guest admin member
	if err := st.UpsertDirectoryRoles(mustJSON(map[string]any{
		"id": "r1", "displayName": "Global Administrator",
		"members": []map[string]any{
			{"displayName": "Guest Admin", "userPrincipalName": "guest@partner.com", "userType": "Guest", "accountEnabled": true},
		},
	})); err != nil {
		t.Fatal(err)
	}

	// Both time-sensitive fixtures below mean "an hour ago", and are derived
	// from the clock so they keep meaning that on every run date. They used
	// to be written as the literal 2099-01-01, which stands in for "recent"
	// only while the calendar is short of it.
	recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)

	// security alert (open, high, raised inside any reasonable look-back)
	if err := st.UpsertSecurity(mustJSON(map[string]any{"id": "a1", "title": "Suspicious sign-in", "severity": "high", "status": "new", "serviceSource": "microsoftDefenderForEndpoint", "createdDateTime": recent})); err != nil {
		t.Fatal(err)
	}

	// managed device: recently checked in, but noncompliant + unencrypted
	if err := st.UpsertManagedDevices(mustJSON(map[string]any{"id": "d1", "deviceName": "LAPTOP-1", "userPrincipalName": "ex@contoso.com", "complianceState": "noncompliant", "isEncrypted": false, "lastSyncDateTime": recent})); err != nil {
		t.Fatal(err)
	}

	// groups: one healthy, one ownerless, one without association data
	if err := st.UpsertGroups(mustJSON(map[string]any{
		"id": "g1", "displayName": "Healthy Team",
		"members": []map[string]any{{"id": "u1", "userPrincipalName": "active@contoso.com", "userType": "Member"}},
		"owners":  []map[string]any{{"id": "u1"}},
	})); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertGroups(mustJSON(map[string]any{
		"id": "g2", "displayName": "Orphaned Project",
		"members": []map[string]any{{"id": "u3", "userPrincipalName": "guest@partner.com", "userType": "Guest"}},
		"owners":  []map[string]any{},
	})); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertGroups(mustJSON(map[string]any{"id": "g3", "displayName": "Unsynced Associations"})); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func runNovel(t *testing.T, dbPath string, build func(*rootFlags) *cobra.Command, extraArgs ...string) string {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := build(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(append([]string{"--db", dbPath}, extraArgs...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %s: %v\noutput: %s", cmd.Name(), err, buf.String())
	}
	return buf.String()
}

func TestLicensesWasteCommand(t *testing.T) {
	out := runNovel(t, seedStore(t), newNovelLicensesWasteCmd)
	if !strings.Contains(out, "ENTERPRISEPACK") {
		t.Errorf("expected wasteful SKU ENTERPRISEPACK in output, got: %s", out)
	}
	if strings.Contains(out, "EMS") {
		t.Errorf("fully-consumed EMS should be omitted, got: %s", out)
	}
}

func TestLicensesOrphansCommand(t *testing.T) {
	out := runNovel(t, seedStore(t), newNovelLicensesOrphansCmd)
	if !strings.Contains(out, "ex@contoso.com") || !strings.Contains(out, "ENTERPRISEPACK") {
		t.Errorf("expected disabled-licensed orphan with resolved SKU, got: %s", out)
	}
	if strings.Contains(out, "active@contoso.com") {
		t.Errorf("active licensed user should not be an orphan, got: %s", out)
	}
}

func TestAdminsAuditCommand(t *testing.T) {
	out := runNovel(t, seedStore(t), newNovelAdminsAuditCmd)
	if !strings.Contains(out, "Global Administrator") || !strings.Contains(out, "guest-admin") {
		t.Errorf("expected guest-admin risk under Global Administrator, got: %s", out)
	}
}

func TestSecurityTriageCommand(t *testing.T) {
	// The fixture alert is seeded an hour ago, so it sits inside the 24h
	// look-back on any run date (since = now-24h is always before now-1h).
	out := runNovel(t, seedStore(t), newNovelSecurityTriageCmd, "--since", "24h")
	if !strings.Contains(out, "Suspicious sign-in") || !strings.Contains(out, "high") {
		t.Errorf("expected open high-severity alert in triage, got: %s", out)
	}
}

func TestManagedDevicesDriftCommand(t *testing.T) {
	out := runNovel(t, seedStore(t), newNovelManagedDevicesDriftCmd, "--days", "1")
	if !strings.Contains(out, "LAPTOP-1") || !strings.Contains(out, "noncompliant") || !strings.Contains(out, "unencrypted") {
		t.Errorf("expected drifted device with noncompliant+unencrypted reasons, got: %s", out)
	}
	// The device checked in an hour ago, so it must NOT also be flagged
	// stale against a 1-day window. Pinning the absence keeps the seeded
	// sync time load-bearing rather than decorative.
	if strings.Contains(out, `"stale"`) {
		t.Errorf("device synced an hour ago must not be flagged stale, got: %s", out)
	}
}

func TestTenantSnapshotCommand(t *testing.T) {
	out := runNovel(t, seedStore(t), newNovelTenantSnapshotCmd)
	var snap map[string]any
	if err := json.Unmarshal([]byte(out), &snap); err != nil {
		t.Fatalf("snapshot output is not valid JSON: %v\n%s", err, out)
	}
	if snap["users"].(float64) != 3 {
		t.Errorf("expected 3 users in snapshot, got %v", snap["users"])
	}
	if snap["unusedLicenseSeats"].(float64) != 20 {
		t.Errorf("expected 20 unused seats, got %v", snap["unusedLicenseSeats"])
	}
}
