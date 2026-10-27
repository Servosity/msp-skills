// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
	"ninjaone-pp-cli/internal/store"
)

// Class sync-effective-page-size: every continuation decision in both sync
// loops (after-id on a full bare-array page, a short page under an
// object-offset cursor, the rendered short-page check) compares the page
// against the page size actually requested, which --param / --resource-param
// can override, not the spec default.

func runNinjaSyncArgs(t *testing.T, srvURL string, args ...string) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	runNinjaSyncArgsAt(t, srvURL, dbPath, args...)
	return dbPath
}

func runNinjaSyncArgsAt(t *testing.T, srvURL, dbPath string, args ...string) {
	t.Helper()
	runNinjaSyncRaw(t, srvURL, dbPath, append([]string{"--full"}, args...)...)
}

// runNinjaSyncRaw runs `sync --db dbPath --json args...` against srvURL.
func runNinjaSyncRaw(t *testing.T, srvURL, dbPath string, args ...string) {
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
	root.SetArgs(append([]string{"sync", "--db", dbPath, "--json"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("sync %v failed: %v\nstdout: %s\nstderr: %s", args, err, out.String(), errBuf.String())
	}
}

func TestNinjaSyncHonoursAnOverriddenPageSize(t *testing.T) {
	for _, tc := range []struct {
		name, resource, flag, size string
		total                      int
		volumes                    bool
	}{
		{"report-resource-param-10", "queries-volumes", "--resource-param", "queries-volumes:pageSize=10", 25, true},
		{"report-param-7", "queries-volumes", "--param", "pageSize=7", 30, true},
		{"keyset-resource-param-50", "devices", "--resource-param", "devices:pageSize=50", 120, false},
		{"keyset-param-1000", "devices", "--param", "pageSize=1000", 1500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := &ninjaFake{}
			var srv *httptest.Server
			if tc.volumes {
				srv = httptest.NewServer(f.volumes(tc.total))
			} else {
				srv = httptest.NewServer(f.devices(tc.total))
			}
			defer srv.Close()
			dbPath := runNinjaSyncArgs(t, srv.URL, "--resources", tc.resource, tc.flag, tc.size)
			if n := storedRows(t, dbPath, tc.resource); n != tc.total {
				t.Fatalf("mirror holds %d %s rows, want %d; requests: %v", n, tc.resource, tc.total, f.requests)
			}
			want := strings.SplitN(tc.size, "=", 2)[1]
			if !strings.Contains(f.requests[0], "pageSize="+want) {
				t.Fatalf("first request %q did not carry the overridden pageSize=%s", f.requests[0], want)
			}
			if n, _ := strconv.Atoi(want); n > 0 && f.count() < (tc.total+n-1)/n {
				t.Fatalf("sync made %d requests for %d rows at pageSize %d", f.count(), tc.total, n)
			}
		})
	}
}

// /v2/organization/{id}/devices: a bare JSON array, keyset-paged by ?after=<last node id>
// (spec.yaml), fetched by the dependent loop once per synced organization.
func (f *ninjaFake) orgDevices(total int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		after, _ := strconv.Atoi(r.URL.Query().Get("after"))
		var rows []string
		for id := after + 1; id <= total && len(rows) < pageSizeOf(r); id++ {
			rows = append(rows, fmt.Sprintf(`{"id":%d,"organizationId":3,"systemName":"PC-%d"}`, id, id))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", strings.Join(rows, ","))
	}
}

func TestNinjaSyncHonoursAnOverriddenPageSizeInTheDependentLoop(t *testing.T) {
	testenv.Isolate(t)
	f := &ninjaFake{}
	srv := httptest.NewServer(f.orgDevices(25))
	defer srv.Close()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert("organizations", "3", json.RawMessage(`{"id":3,"name":"Acme"}`)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	runNinjaSyncArgsAt(t, srv.URL, dbPath, "--resources", "organization_devices", "--resource-param", "organization_devices:pageSize=10")
	if n := storedRows(t, dbPath, "organization_devices"); n != 25 {
		t.Fatalf("mirror holds %d organization_devices rows, want 25; requests: %v", n, f.requests)
	}
	if f.count() < 3 || !strings.Contains(f.requests[1], "after=10") {
		t.Fatalf("dependent loop stopped after a full overridden page: %v", f.requests)
	}
}

// An out-of-range pageSize (spec.yaml documents maximum 1000 on /v2/devices) that
// NinjaOne clamps must not end the walk on the first clamped page.
func TestNinjaSyncKeepsWalkingWhenAnOversizedPageSizeIsClamped(t *testing.T) {
	testenv.Isolate(t)
	f := &ninjaFake{}
	devices := f.devices(2500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if n, _ := strconv.Atoi(q.Get("pageSize")); n > 1000 {
			q.Set("pageSize", "1000")
			r.URL.RawQuery = q.Encode()
		}
		devices(w, r)
	}))
	defer srv.Close()
	dbPath := runNinjaSyncArgs(t, srv.URL, "--resources", "devices", "--param", "pageSize=20000")
	if n := storedRows(t, dbPath, "devices"); n != 2500 {
		t.Fatalf("mirror holds %d devices, want 2500; requests: %v", n, f.requests)
	}
}
