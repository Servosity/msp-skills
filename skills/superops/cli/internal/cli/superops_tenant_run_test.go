// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/config"
	"superops-pp-cli/internal/store"
)

func writeSubdomainConfig(t *testing.T, path, subdomain string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("subdomain = \""+subdomain+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A run loads its configuration once: every caller gets the same
// *config.Config, and a store opened with the run's context is checked
// against and recorded for that configuration's tenant, whatever the file
// says by then.
func TestRunSnapshotLoadsTheConfigOnce(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_SUBDOMAIN", "")
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	writeSubdomainConfig(t, cfgPath, "acme-a")

	snapshot := newRunSnapshot(cfgPath)
	first, err := snapshot.config()
	if err != nil {
		t.Fatal(err)
	}
	writeSubdomainConfig(t, cfgPath, "acme-b")
	second, err := snapshot.config()
	if err != nil {
		t.Fatal(err)
	}
	if first != second || second.Subdomain != "acme-a" {
		t.Fatalf("the run loaded its config again: %p %q then %p %q", first, first.Subdomain, second, second.Subdomain)
	}
	if tenant, ok := snapshot.tenant(); !ok || tenant != first.SuperOpsTenant() {
		t.Fatalf("run tenant %q (ok %v), want %q", tenant, ok, first.SuperOpsTenant())
	}

	path := filepath.Join(t.TempDir(), "x.db")
	s, err := store.OpenWithContext(snapshot.storeContext(context.Background()), path)
	if err != nil {
		t.Fatalf("open with the run's context: %v", err)
	}
	_ = s.Close()
	if stamp, ok := readStamp(t, path); !ok || stamp != first.SuperOpsTenant() {
		t.Fatalf("store records %q (recorded %v), want the run's %q", stamp, ok, first.SuperOpsTenant())
	}
}

// One MCP tool call takes one snapshot for the store path and the tenant
// check, and the path is the one MCPStorePath gives.
func TestMCPStoreRunUsesOneSnapshot(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_SUBDOMAIN", "")
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	probe, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	writeSubdomainConfig(t, probe.Path, "acme-a")
	wantPath, err := MCPStorePath()
	if err != nil {
		t.Fatal(err)
	}

	ctx, path, err := MCPStoreRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if path != wantPath {
		t.Fatalf("MCPStoreRun path %s, MCPStorePath %s", path, wantPath)
	}
	writeSubdomainConfig(t, probe.Path, "acme-b")
	s, err := store.OpenWithContext(ctx, path)
	if err != nil {
		t.Fatalf("open with the tool call's context: %v", err)
	}
	_ = s.Close()
	if stamp, ok := readStamp(t, path); !ok || !strings.Contains(stamp, "subdomain=acme-a ") {
		t.Fatalf("store records %q (recorded %v), want acme-a's", stamp, ok)
	}
	// The next tool call takes a fresh snapshot: acme-b, refused.
	ctx, _, err = MCPStoreRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s, err := store.OpenReadOnlyContext(ctx, path); err == nil {
		_ = s.Close()
		t.Fatal("the next tool call, now acme-b, read acme-a's store")
	}
}

// Each execution of a command starts a fresh snapshot, even on a command
// tree that ran before in the same process.
func TestEachExecutionTakesItsOwnSnapshot(t *testing.T) {
	twoTenantFakes(t)
	dir := t.TempDir()
	cmd := RootCmd()
	for _, sub := range []string{"acme-a", "acme-b"} {
		t.Setenv("SUPEROPS_SUBDOMAIN", sub)
		db := filepath.Join(dir, sub+".db")
		cmd.SetArgs([]string{"sync", "--resources", "tickets", "--db", db})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s sync: %v\n%s", sub, err, out.String())
		}
		if stamp, ok := readStamp(t, db); !ok || !strings.Contains(stamp, "subdomain="+sub+" ") {
			t.Fatalf("%s's store records %q (recorded %v)", sub, stamp, ok)
		}
	}
}
