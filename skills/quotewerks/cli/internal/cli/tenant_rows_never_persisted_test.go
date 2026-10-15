// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: tenant-rows-never-persisted).

package cli

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quotewerks-pp-cli/internal/cliutil/testenv"
)

const tenantSentinel = "SENTINEL-SECRET-7731"

// TestTenantRowsNeverPersisted reads and writes a secret-bearing table row
// through the generic tables commands (without --no-cache, so the response
// cache is exercised) and fails if the secret lands anywhere under the
// sandboxed home: response cache, local store, receipts or journal.
func TestTenantRowsNeverPersisted(t *testing.T) {
	home := testenv.Isolate(t)
	fake := newFakeQW()
	fake.tables["SecurityAccounts"] = append(fake.tables["SecurityAccounts"], map[string]any{
		"RecGUID": guid(501), "ID": 1.0, "UserName": "Ann", "ApiSecret": tenantSentinel,
	})
	fake.columns["SecurityAccounts"] = map[string]bool{"RecGUID": true, "ID": true, "UserName": true, "ApiSecret": true}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	t.Setenv("QUOTEWERKS_BASE_URL", srv.URL)
	t.Setenv("QUOTEWERKS_API_KEY", "qww_test_key")

	run := func(stdin string, args ...string) string {
		t.Helper()
		cmd := RootCmd()
		var out, errb bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errb)
		in, err := os.CreateTemp(t.TempDir(), "stdin")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = in.WriteString(stdin)
		_, _ = in.Seek(0, 0)
		saved := os.Stdin
		os.Stdin = in // tables update reads os.Stdin directly
		defer func() { os.Stdin = saved; _ = in.Close() }()
		cmd.SetArgs(append(args, "--home", home)) // cache, data and state all under home
		_ = cmd.Execute()
		return out.String() + errb.String()
	}
	for i := 0; i < 2; i++ { // a second read would be served from a cache if one were written
		if out := run("", "tables", "row", "SecurityAccounts", guid(501), "--json"); !strings.Contains(out, tenantSentinel) {
			t.Fatalf("tables row did not return the stub row: %s", out)
		}
	}
	run("", "tables", "query", "SecurityAccounts", "--page-number", "1", "--page-size", "10", "--json")
	if out := run(`{"data":{"type":"SecurityAccounts","id":"`+guid(501)+`","attributes":{"UserName":"Ann B"}}}`, "tables", "update", "SecurityAccounts", guid(501), "--stdin", "--json"); !strings.Contains(out, tenantSentinel) {
		t.Fatalf("tables update did not return the patched row: %s", out)
	}

	err := filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, rerr := os.ReadFile(path)
		if rerr == nil && bytes.Contains(data, []byte(tenantSentinel)) {
			return errors.New(path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tenant secret persisted to disk at %v", err)
	}
}

// TestSyncRefusesCloudSettings: CloudSystemSettings rows are read live only.
func TestSyncRefusesCloudSettings(t *testing.T) {
	h := newQWHarness(t)
	out, code := h.run("sync", "--resources", "cloud_settings")
	if code == 0 {
		t.Fatalf("sync --resources cloud_settings succeeded: %s", out)
	}
	if res := syncResource(t.Context(), nil, nil, "cloud_settings", "", false, 1, false, false, nil, nil); !errors.Is(res.Err, errCloudSettingsNeverSynced) {
		t.Fatalf("syncResource(cloud_settings) err = %v", res.Err)
	}
	for _, r := range defaultSyncResources() {
		if r == "cloud_settings" {
			t.Fatal("cloud_settings is in the default sync set")
		}
	}
}
