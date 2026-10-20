// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/config"
	"cloudally-pp-cli/internal/store"
)

// seedTenantADataDB writes an older tenant A's unscoped data.db holding one
// restore batch, with no owner marker, and returns its path.
func seedTenantADataDB(t *testing.T) string {
	t.Helper()
	unscoped := currentStorePathUnscoped(t)
	if err := os.MkdirAll(filepath.Dir(unscoped), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenWithContext(context.Background(), unscoped)
	if err != nil {
		t.Fatalf("open tenant A data.db: %v", err)
	}
	defer db.Close()
	if err := ledgerInsertBatch(db.DB(), restoreBatch{
		BatchID: "tenant-a-batch", BatchKey: "k", ResumeKey: "r", CreatedAt: time.Now(),
		Scope: scopeAccount, TaskID: "tenant-a-task", Source: "MS365",
		Dates: []string{"2025-12-31"}, Options: map[string]any{},
	}); err != nil {
		t.Fatalf("seed tenant A batch: %v", err)
	}
	if _, err := os.Stat(unscoped + ".owner"); !os.IsNotExist(err) {
		t.Fatalf("tenant A data.db should start unowned: %v", err)
	}
	return unscoped
}

// Round three: every credential path, the raw-token fallbacks included,
// keeps out of an unowned unscoped data.db another tenant may have filled.
// The repro: tenant A's data.db, no auth record, tenant B through
// CLOUDALLY_ACCESS_TOKEN, then restore history and the MCP store path.
func TestRawTokenPathsNeverAdoptAnotherTenantsDataDB(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{"environment token, no auth record", func(t *testing.T) {
			t.Setenv("CLOUDALLY_ACCESS_TOKEN", "tenant-b-token")
		}},
		{"set-token, no auth record", func(t *testing.T) {
			if _, _, err := runAuthCmd(t, "tenant-b-token\n", "auth", "set-token"); err != nil {
				t.Fatalf("set-token: %v", err)
			}
		}},
		{"raw auth header in the config", func(t *testing.T) {
			cfg, err := config.Load("")
			if err != nil {
				t.Fatal(err)
			}
			cfg.AuthHeaderVal = "Bearer tenant-b-token"
			if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "", "", time.Time{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"known token with no known user", func(t *testing.T) {
			cfg, err := config.Load("")
			if err != nil {
				t.Fatal(err)
			}
			if err := cfg.SaveTokens(cfg.ClientID, cfg.ClientSecret, "tenant-b-token", "", time.Time{}); err != nil {
				t.Fatal(err)
			}
			bindSidecar(t, authScopeAccount, "tenant-b-token", "")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateAuthState(t)
			resetStoreScope(t)
			t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
			t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
			t.Setenv("CLOUDALLY_BASE_URL", scopeAPI)
			unscoped := seedTenantADataDB(t)
			tc.setup(t)

			stdout, _, err := runAuthCmd(t, "", "restore", "history", "--json")
			if err != nil {
				t.Fatalf("restore history: %v", err)
			}
			if strings.Contains(stdout, "tenant-a-batch") {
				t.Fatalf("tenant B's restore history shows tenant A's batch: %s", stdout)
			}
			if got := currentStorePath(); got == unscoped {
				t.Fatalf("tenant B's CLI store is tenant A's data.db")
			}
			mcpPath, err := MCPStorePath()
			if err != nil {
				t.Fatalf("MCPStorePath: %v", err)
			}
			if mcpPath == unscoped {
				t.Fatalf("MCP sql/search would open tenant A's data.db")
			}
			if data, err := os.ReadFile(unscoped + ".owner"); err != nil || !strings.HasPrefix(string(data), "unclaimed") {
				t.Fatalf("data.db owner marker = %q, %v", data, err)
			}
		})
	}
}

// The other direction: with an empty data directory an environment-token
// run works normally, and nothing is marked because there is no data.db.
func TestRawTokenRunWithAnEmptyDataDir(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", scopeAPI)
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "tenant-b-token")

	stdout, _, err := runAuthCmd(t, "", "restore", "history", "--json")
	if err != nil {
		t.Fatalf("restore history: %v", err)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Fatalf("restore history = %q, want []", stdout)
	}
	got := currentStorePath()
	if filepath.Base(got) == "data.db" || !strings.HasPrefix(filepath.Base(got), "data-") {
		t.Fatalf("store path = %s, want the token's own data-<hash>.db", got)
	}
	if err := requireBoundStoreScope(); err != nil {
		t.Fatalf("an environment token run should pass: %v", err)
	}
	unscoped := filepath.Join(filepath.Dir(got), "data.db")
	if _, err := os.Stat(unscoped + ".owner"); !os.IsNotExist(err) {
		t.Fatalf("a marker was written although there is no data.db: %v", err)
	}
	if mcpPath, err := MCPStorePath(); err != nil || mcpPath != got {
		t.Fatalf("MCPStorePath = %q, %v; want %q", mcpPath, err, got)
	}

	// A data.db this credential claimed earlier stays its own.
	hash, _, _ := currentDefaultDBScope()
	if err := os.MkdirAll(filepath.Dir(unscoped), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unscoped, []byte("tenant B's own rows"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unscoped+".owner", []byte(hash+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := currentStorePath(); got != unscoped {
		t.Fatalf("a data.db owned by this credential was not used: %s", filepath.Base(got))
	}
}

// When no marker can be published (here an empty owner file left by an
// interrupted claim, which the generated code treats as undecided and then
// shares data.db), the generated resolver still never hands data.db over,
// and the ledger commands fail closed.
func TestUndecidedOwnerNeverHandsOverDataDB(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", scopeAPI)
	unscoped := seedTenantADataDB(t)
	if err := os.WriteFile(unscoped+".owner", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLOUDALLY_ACCESS_TOKEN", "tenant-b-token")

	if got := currentStorePath(); got == unscoped {
		t.Fatal("the CLI store is tenant A's data.db")
	}
	if mcpPath, err := MCPStorePath(); err != nil || mcpPath == unscoped {
		t.Fatalf("MCPStorePath = %q, %v: MCP sql/search would open tenant A's data.db", mcpPath, err)
	}
	if err := requireBoundStoreScope(); err == nil {
		t.Fatal("an unmarkable unowned data.db should fail the ledger commands closed")
	}
	if _, _, err := runAuthCmd(t, "", "restore", "history", "--json"); err == nil {
		t.Fatal("restore history ran against an unmarkable unowned data.db")
	}
}

// Round four: a run with no credential at all (here an MCPB host whose
// ${user_config.*} placeholders were never filled) never opens an older
// tenant's data.db, marked unclaimed or not yet marked.
func TestCredentiallessRunNeverReadsAnOlderTenantsDataDB(t *testing.T) {
	for _, marked := range []bool{true, false} {
		name := "not yet marked"
		if marked {
			name = "marked unclaimed"
		}
		t.Run(name, func(t *testing.T) {
			isolateAuthState(t)
			resetStoreScope(t)
			for _, v := range []string{"CLOUDALLY_CLIENT_ID", "CLOUDALLY_CLIENT_SECRET", "CLOUDALLY_ACCESS_TOKEN", "CLOUDALLY_EMAIL", "CLOUDALLY_PASSWORD"} {
				t.Setenv(v, "${user_config."+strings.ToLower(v)+"}")
			}
			unscoped := seedTenantADataDB(t)
			if marked {
				if err := markUnscopedStoreUnclaimed(); err != nil {
					t.Fatal(err)
				}
			}

			stdout, _, err := runAuthCmd(t, "", "restore", "history", "--json")
			if err != nil {
				t.Fatalf("restore history: %v", err)
			}
			if strings.Contains(stdout, "tenant-a-batch") {
				t.Fatalf("a credential-less run shows the older tenant's batch: %s", stdout)
			}
			if got := currentStorePath(); got == unscoped {
				t.Fatal("the credential-less CLI store is the older tenant's data.db")
			}
			if mcpPath, err := MCPStorePath(); err != nil || mcpPath == unscoped {
				t.Fatalf("MCPStorePath = %q, %v: MCP sql/search would open the older tenant's data.db", mcpPath, err)
			}
		})
	}
}

// The other direction: credential-less runs on a never-used data directory
// still work and keep what they write from one run to the next.
func TestCredentiallessRunOnAFreshDataDirKeepsItsStore(t *testing.T) {
	isolateAuthState(t)
	resetStoreScope(t)

	stdout, _, err := runAuthCmd(t, "", "restore", "history", "--json")
	if err != nil || strings.TrimSpace(stdout) != "[]" {
		t.Fatalf("restore history = %q, %v; want []", stdout, err)
	}
	path := currentStorePath()
	if filepath.Base(path) != credentiallessStoreName {
		t.Fatalf("credential-less store = %s, want %s", filepath.Base(path), credentiallessStoreName)
	}
	if mcpPath, err := MCPStorePath(); err != nil || mcpPath != path {
		t.Fatalf("MCPStorePath = %q, %v; want %q", mcpPath, err, path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledgerInsertBatch(db.DB(), restoreBatch{
		BatchID: "own-batch", BatchKey: "k", ResumeKey: "r", CreatedAt: time.Now(),
		Scope: scopeAccount, TaskID: "t", Source: "MS365", Dates: []string{"2025-12-31"}, Options: map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	stdout, _, err = runAuthCmd(t, "", "restore", "history", "--json")
	if err != nil || !strings.Contains(stdout, "own-batch") {
		t.Fatalf("the next credential-less run lost its own store: %q, %v", stdout, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "data.db")); !os.IsNotExist(err) {
		t.Fatalf("a credential-less run created data.db: %v", err)
	}
}
