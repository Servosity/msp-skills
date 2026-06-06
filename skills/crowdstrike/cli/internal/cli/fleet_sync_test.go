// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"crowdstrike-pp-cli/internal/config"
)

// TestLoadFleetEntities_NullColumns round-trips entities through the real
// SQLite store. Hosts carry no severity and freshly-synced rows may omit
// status, so those columns are NULL — loadFleetEntities must scan them as ""
// rather than failing with "converting NULL to string is unsupported". Pure
// in-memory rollup tests can't catch this because they never touch the DB.
func TestLoadFleetEntities_NullColumns(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "fleet.db")

	st, err := openFleetStore(ctx, dbPath)
	if err != nil {
		t.Fatalf("openFleetStore: %v", err)
	}
	// A host (no severity, no status) plus a vuln (both set) exercises both
	// the NULL and non-NULL paths through the same scan.
	if _, err := upsertFleetEntities(ctx, st.DB(), []fleetEntity{
		{CID: "CID-A", Kind: kindHost, ID: "h1", Name: "web01"},
		{CID: "CID-B", Kind: kindVuln, ID: "v1", Name: "CVE-2026-1", Severity: "critical", Status: "open"},
	}); err != nil {
		st.Close()
		t.Fatalf("upsertFleetEntities: %v", err)
	}
	st.Close()

	ents, err := loadFleetEntities(ctx, dbPath, "")
	if err != nil {
		t.Fatalf("loadFleetEntities: %v", err)
	}
	if len(ents) != 2 {
		t.Fatalf("want 2 entities, got %d", len(ents))
	}
	byID := map[string]fleetEntity{}
	for _, e := range ents {
		byID[e.ID] = e
	}
	if h := byID["h1"]; h.Severity != "" || h.Status != "" || h.Name != "web01" {
		t.Errorf("host scanned wrong: %+v (want empty severity/status, name web01)", h)
	}
	if v := byID["v1"]; v.Severity != "critical" || v.Status != "open" {
		t.Errorf("vuln scanned wrong: %+v (want critical/open)", v)
	}
}

func TestExtractObjectResources(t *testing.T) {
	data := json.RawMessage(`{"meta":{},"resources":[{"id":"1","hostname":"a"},{"id":"2"}],"errors":[]}`)
	objs := extractObjectResources(data)
	if len(objs) != 2 || objs[0]["hostname"] != "a" {
		t.Fatalf("want 2 objects, got %+v", objs)
	}
	// bare array fallback
	bare := extractObjectResources(json.RawMessage(`[{"id":"x"}]`))
	if len(bare) != 1 || bare[0]["id"] != "x" {
		t.Errorf("bare array fallback failed: %+v", bare)
	}
}

func TestExtractStringResources(t *testing.T) {
	ids := extractStringResources(json.RawMessage(`{"resources":["cid-a","cid-b"]}`))
	if len(ids) != 2 || ids[0] != "cid-a" {
		t.Fatalf("want [cid-a cid-b], got %+v", ids)
	}
}

func TestAlertSeverity(t *testing.T) {
	cases := []struct {
		in   map[string]any
		want string
	}{
		{map[string]any{"severity_name": "High"}, "high"},
		{map[string]any{"severity": float64(95)}, "critical"},
		{map[string]any{"severity": float64(75)}, "high"},
		{map[string]any{"severity": float64(10)}, "low"},
		{map[string]any{}, ""},
	}
	for _, c := range cases {
		if got := alertSeverity(c.in); got != c.want {
			t.Errorf("alertSeverity(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestVulnSeverityAndName(t *testing.T) {
	o := map[string]any{"id": "v1", "cve": map[string]any{"id": "CVE-2026-1", "severity": "CRITICAL"}}
	if got := vulnSeverity(o); got != "critical" {
		t.Errorf("vulnSeverity = %q, want critical", got)
	}
	if got := vulnName(o); got != "CVE-2026-1" {
		t.Errorf("vulnName = %q, want CVE-2026-1", got)
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV("a, b ,,c")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("splitCSV bad parse: %+v", got)
	}
	if splitCSV("") != nil {
		t.Errorf("empty CSV should be nil")
	}
}

func TestScopedChildConfigInvariants(t *testing.T) {
	parent := &config.Config{
		ClientID:      "parent-id",
		ClientSecret:  "parent-secret",
		AccessToken:   "parent-token",
		AuthHeaderVal: "Bearer static-parent-header",
		TokenURL:      "https://api.crowdstrike.com/oauth2/token",
		BaseURL:       "https://api.crowdstrike.com",
		Path:          "/home/user/.config/crowdstrike-cli/config.toml",
		TokenExpiry:   time.Now().Add(20 * time.Minute),
	}
	scoped := scopedChildConfig(parent, "minted-child-token")

	if scoped.AccessToken != "minted-child-token" {
		t.Errorf("AccessToken = %q, want the minted member_cid token", scoped.AccessToken)
	}
	// Every remaining assertion closes a distinct path back to the PARENT
	// credential (config-sourced re-mint, static header override, env-var
	// re-mint via the 60s expiry gate, on-disk config clobber).
	if scoped.ClientID != "" || scoped.ClientSecret != "" {
		t.Error("ClientID/ClientSecret must be cleared on the scoped copy")
	}
	if scoped.AuthHeaderVal != "" {
		t.Error("AuthHeaderVal must be cleared so the static parent header cannot win over the minted token")
	}
	if !scoped.TokenExpiry.IsZero() {
		t.Error("TokenExpiry must be zero so needsClientCredentialsMint never re-mints an unscoped parent token")
	}
	if scoped.Path != "" {
		t.Error("Path must be cleared so a re-mint can never persist a parent token over the user's config.toml")
	}
	// The parent config must be untouched (value copy).
	if parent.ClientID != "parent-id" || parent.AuthHeaderVal != "Bearer static-parent-header" {
		t.Error("scopedChildConfig must not mutate the parent config")
	}
}
