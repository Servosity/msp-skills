// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
	"ninjaone-pp-cli/internal/store"
)

// End-to-end pins for after-id-pagination and queries-object-cursor-pagination
// on the press 4.34.0 sync engine: a fake NinjaOne answers the way the public API
// does, and `sync` must store every row and finish without an incomplete-walk
// warning (cursor_unavailable or stuck_pagination).

type ninjaFake struct {
	mu       sync.Mutex
	requests []string
}

func (f *ninjaFake) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.Path+"?"+r.URL.RawQuery)
}

func (f *ninjaFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func pageSizeOf(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if err != nil || n <= 0 {
		return 100
	}
	return n
}

// /v2/devices: a bare JSON array, keyset-paged by ?after=<last device id>.
func (f *ninjaFake) devices(total int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		after, _ := strconv.Atoi(r.URL.Query().Get("after"))
		var rows []string
		for id := after + 1; id <= total && len(rows) < pageSizeOf(r); id++ {
			rows = append(rows, fmt.Sprintf(`{"id":%d,"organizationId":1,"systemName":"PC-%d","nodeClass":"WINDOWS_WORKSTATION"}`, id, id))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", strings.Join(rows, ","))
	}
}

// /v2/queries/volumes: results under an object cursor whose name stays the same
// while offset advances; the last page still carries the cursor object.
func (f *ninjaFake) volumes(total int) http.HandlerFunc {
	served := 0
	return func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		if r.URL.Query().Get("cursor") == "" {
			served = 0
		}
		offset := served
		var rows []string
		for i := offset; i < total && len(rows) < pageSizeOf(r); i++ {
			rows = append(rows, fmt.Sprintf(`{"deviceId":%d,"name":"C:","capacity":100,"freeSpace":40}`, i+1))
		}
		served += len(rows)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"cursor":{"name":"SESSION_TOKEN","offset":%d,"count":"%d","expires":"2026-01-01T00:00:00Z"},"results":[%s]}`,
			offset, len(rows), strings.Join(rows, ","))
	}
}

func runNinjaSync(t *testing.T, srvURL, resource string) (string, string) {
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
	dbPath := filepath.Join(t.TempDir(), "data.db")
	root := newRootCmd(&rootFlags{})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"sync", "--resources", resource, "--full", "--db", dbPath, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync %s failed: %v\nstdout: %s\nstderr: %s", resource, err, out.String(), errBuf.String())
	}
	for _, bad := range []string{"cursor_unavailable", "stuck_pagination", "pagination_cursor_missing", "max_pages_cap_hit"} {
		if strings.Contains(out.String()+errBuf.String(), bad) {
			t.Fatalf("sync %s reported %s (an incomplete walk):\n%s\n%s", resource, bad, out.String(), errBuf.String())
		}
	}
	return dbPath, out.String()
}

func storedRows(t *testing.T, dbPath, resource string) int {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	n, err := db.Count(resource)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNinjaSyncWalksBareArrayListsByAfterID(t *testing.T) {
	for _, tc := range []struct{ total, requests int }{{250, 3}, {200, 3}, {40, 1}} {
		t.Run(strconv.Itoa(tc.total), func(t *testing.T) {
			testenv.Isolate(t)
			f := &ninjaFake{}
			srv := httptest.NewServer(f.devices(tc.total))
			defer srv.Close()
			dbPath, _ := runNinjaSync(t, srv.URL, "devices")
			if n := storedRows(t, dbPath, "devices"); n != tc.total {
				t.Fatalf("mirror holds %d devices, want %d; requests: %v", n, tc.total, f.requests)
			}
			if f.count() != tc.requests {
				t.Fatalf("sync made %d requests, want %d: %v", f.count(), tc.requests, f.requests)
			}
			if tc.total > 100 && !strings.Contains(f.requests[1], "after=100") {
				t.Fatalf("second request %q does not resume after the last id of page one", f.requests[1])
			}
		})
	}
}

func TestNinjaSyncWalksObjectCursorReportsToTheEnd(t *testing.T) {
	for _, tc := range []struct{ total, requests int }{{250, 3}, {40, 1}} {
		t.Run(strconv.Itoa(tc.total), func(t *testing.T) {
			testenv.Isolate(t)
			f := &ninjaFake{}
			srv := httptest.NewServer(f.volumes(tc.total))
			defer srv.Close()
			dbPath, _ := runNinjaSync(t, srv.URL, "queries-volumes")
			if n := storedRows(t, dbPath, "queries-volumes"); n != tc.total {
				t.Fatalf("mirror holds %d volumes, want %d; requests: %v", n, tc.total, f.requests)
			}
			if f.count() != tc.requests {
				t.Fatalf("sync made %d requests, want %d: %v", f.count(), tc.requests, f.requests)
			}
		})
	}
}
