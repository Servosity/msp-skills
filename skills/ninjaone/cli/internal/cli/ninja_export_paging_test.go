// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
)

// export walks NinjaOne's keyset lists and object-cursor reports to the end, the
// same as sync (ninja-paging-every-reader).

func runNinjaExport(t *testing.T, srvURL, resource string) []string {
	t.Helper()
	home := t.TempDir()
	cfg := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfg, []byte("access_token = \"fixture-token\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NINJAONE_CONFIG", cfg)
	t.Setenv("NINJAONE_BASE_URL", srvURL)
	t.Setenv("NINJAONE_CLIENT_ID", "")
	t.Setenv("NINJAONE_CLIENT_SECRET", "")
	root := newRootCmd(&rootFlags{})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"export", resource, "--format", "jsonl"})
	if err := root.Execute(); err != nil {
		t.Fatalf("export %s failed: %v\nstdout: %s\nstderr: %s", resource, err, out.String(), errBuf.String())
	}
	return strings.Fields(strings.TrimSpace(out.String()))
}

func TestNinjaExportWalksBareArrayListsByAfterID(t *testing.T) {
	for _, tc := range []struct{ total, requests int }{{250, 3}, {200, 3}, {40, 1}} {
		t.Run(strconv.Itoa(tc.total), func(t *testing.T) {
			testenv.Isolate(t)
			f := &ninjaFake{}
			srv := httptest.NewServer(f.devices(tc.total))
			defer srv.Close()
			rows := runNinjaExport(t, srv.URL, "devices")
			if len(rows) != tc.total {
				t.Fatalf("export devices wrote %d rows, want %d; requests: %v", len(rows), tc.total, f.requests)
			}
			if f.count() != tc.requests {
				t.Fatalf("export made %d requests, want %d: %v", f.count(), tc.requests, f.requests)
			}
		})
	}
}

func TestNinjaExportWalksObjectCursorReportsToTheEnd(t *testing.T) {
	for _, tc := range []struct{ total, requests int }{{250, 3}, {40, 1}} {
		t.Run(strconv.Itoa(tc.total), func(t *testing.T) {
			testenv.Isolate(t)
			f := &ninjaFake{}
			srv := httptest.NewServer(f.volumes(tc.total))
			defer srv.Close()
			rows := runNinjaExport(t, srv.URL, "queries")
			if len(rows) != tc.total {
				t.Fatalf("export queries wrote %d rows, want %d; requests: %v", len(rows), tc.total, f.requests)
			}
			if f.count() != tc.requests {
				t.Fatalf("export made %d requests, want %d: %v", f.count(), tc.requests, f.requests)
			}
			for _, r := range f.requests[1:] {
				if strings.Contains(r, "%00") || !strings.Contains(r, "cursor=SESSION_TOKEN&") {
					t.Fatalf("request %q must send only the cursor token", r)
				}
			}
		})
	}
}
