// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Execution + parsing tests: resolve-by-URL / by-name.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"hudu-pp-cli/internal/store"
)

func TestParseResolveInput(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantPath string
		wantSlug string
		wantURL  bool
	}{
		{"plain name", "DC01", "", "", false},
		{"asset url", "https://docs.example.huducloud.com/a/dc01-abc123", "/a/dc01-abc123", "dc01-abc123", true},
		{"trailing slash", "https://docs.example.com/companies/42/", "/companies/42", "42", true},
		{"url with query", "https://docs.example.com/kba/runbook?draft=1", "/kba/runbook", "runbook", true},
		{"scheme only host falls back to name search", "https://docs.example.com", "", "", false},
		{"unparseable url-shaped input falls back", "foo://", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, slug, isURL := parseResolveInput(tc.input)
			if path != tc.wantPath || slug != tc.wantSlug || isURL != tc.wantURL {
				t.Errorf("parseResolveInput(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.input, path, slug, isURL, tc.wantPath, tc.wantSlug, tc.wantURL)
			}
		})
	}
}

func TestNovelResolveRunsEmptyMirror(t *testing.T) {
	out := runNovelCmd(t, newNovelResolveCmd, "no-such-object")
	var view resolveView
	if err := json.Unmarshal(out, &view); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, out)
	}
	if view.Query != "no-such-object" {
		t.Errorf("envelope must echo the query, got %q", view.Query)
	}
	if len(view.Matches) != 0 {
		t.Errorf("expected no matches on empty mirror, got %d", len(view.Matches))
	}
	if view.Note == "" {
		t.Error("empty result must carry an explanatory note")
	}
}

func TestNovelResolveURLInputRuns(t *testing.T) {
	out := runNovelCmd(t, newNovelResolveCmd, "https://docs.example.huducloud.com/a/dc01-abc123")
	var view resolveView
	if err := json.Unmarshal(out, &view); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, out)
	}
	if view.Query == "" {
		t.Error("envelope must echo the query")
	}
}

func TestNovelResolveRejectsBadType(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelResolveCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "resolve-test.db"), "--type", "bogus", "DC01"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for --type bogus, got nil")
	}
}

func TestNovelResolveRequiresInput(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelResolveCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	// A flag is set but no positional argument: must fail with usage error,
	// not silently return help.
	cmd.SetArgs([]string{"--db", filepath.Join(t.TempDir(), "resolve-test.db")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected a usage error when url-or-name is missing, got nil")
	}
}

// runNovelCmdWithDB is runNovelCmd against a pre-seeded database path.
func runNovelCmdWithDB(t *testing.T, dbPath string, build func(*rootFlags) *cobra.Command, extraArgs ...string) []byte {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := build(flags)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(append([]string{"--db", dbPath}, extraArgs...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error: %v\nstdout: %s\nstderr: %s", err, out.String(), errBuf.String())
	}
	return out.Bytes()
}

func seedResolveStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "seeded.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("opening seed store: %v", err)
	}
	defer db.Close()
	seed := map[string][]json.RawMessage{
		"companies": {json.RawMessage(`{"id": 1, "name": "Acme", "slug": "acme"}`)},
		"assets":    {json.RawMessage(`{"id": 2, "name": "DC01", "company_id": 1, "asset_layout_id": 5, "slug": "dc01-xyz", "url": "https://docs.example.huducloud.com/a/dc01-xyz"}`)},
		// UpsertBatch routes typed tables by the hyphenated API resource name.
		"asset-passwords": {json.RawMessage(`{"id": 3, "name": "VaultEntry", "username": "admin", "company_id": 1, "password": "sup3r-s3cr3t-value", "updated_at": "2020-01-01T00:00:00Z"}`)},
	}
	for resource, items := range seed {
		if _, _, err := db.UpsertBatch(resource, items); err != nil {
			t.Fatalf("seeding %s: %v", resource, err)
		}
	}
	return dbPath
}

func TestNovelResolveSeededByName(t *testing.T) {
	dbPath := seedResolveStore(t)
	out := runNovelCmdWithDB(t, dbPath, newNovelResolveCmd, "Acme")
	var view resolveView
	if err := json.Unmarshal(out, &view); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, out)
	}
	if len(view.Matches) == 0 {
		t.Fatalf("expected a company match for Acme, got none: %s", out)
	}
	if view.Matches[0].Kind != "company" || view.Matches[0].Name != "Acme" {
		t.Errorf("expected company/Acme first, got %+v", view.Matches[0])
	}
}

func TestNovelResolveSeededByURL(t *testing.T) {
	dbPath := seedResolveStore(t)
	out := runNovelCmdWithDB(t, dbPath, newNovelResolveCmd, "https://docs.example.huducloud.com/a/dc01-xyz")
	var view resolveView
	if err := json.Unmarshal(out, &view); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, out)
	}
	if len(view.Matches) == 0 {
		t.Fatalf("expected an asset match for the portal URL, got none: %s", out)
	}
	m := view.Matches[0]
	if m.Kind != "asset" || m.ID != 2 || m.Name != "DC01" {
		t.Errorf("expected asset #2 DC01, got %+v", m)
	}
}

// TestNoSecretValueInOutputs is the headline-risk regression test: a vault row
// with a real password value must never leak the secret through any
// password-touching command surface.
func TestNoSecretValueInOutputs(t *testing.T) {
	dbPath := seedResolveStore(t)
	const secret = "sup3r-s3cr3t-value"
	outputs := map[string][]byte{
		"resolve --type password": runNovelCmdWithDB(t, dbPath, newNovelResolveCmd, "VaultEntry", "--type", "password"),
		"audit stale-passwords":   runNovelCmdWithDB(t, dbPath, newNovelAuditStalePasswordsCmd, "--older-than", "30d"),
		"audit summary":           runNovelCmdWithDB(t, dbPath, newNovelAuditSummaryCmd),
	}
	for label, out := range outputs {
		if strings.Contains(string(out), secret) {
			t.Errorf("%s output leaked the password value:\n%s", label, out)
		}
	}
	// Sanity: the stale-passwords audit did see the seeded row (name only).
	if !strings.Contains(string(outputs["audit stale-passwords"]), "VaultEntry") {
		t.Errorf("stale-passwords audit did not report the seeded vault entry: %s", outputs["audit stale-passwords"])
	}
}
