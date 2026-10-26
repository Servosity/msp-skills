// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
)

// Class ninja-paging-every-reader, extended to the endpoint `--all` paginator
// (helpers.go paginatedGet) and `tail`: the same keyset rule (a bare-array page
// continues with ?after=<last id> until a page is empty or shorter than the
// size asked for) and object-cursor rule (the report token repeats while the
// offset advances; a short or empty page ends the walk) that sync and export
// apply. spec.yaml: /v2/devices `after` "Last Node ID from previous page";
// /v2/queries/* Cursor {name, offset, count, expires}.

func runNinjaCLI(t *testing.T, srvURL string, args ...string) (string, string) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.toml")
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
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v failed: %v\nstdout: %s\nstderr: %s", args, err, out.String(), errBuf.String())
	}
	return out.String(), errBuf.String()
}

func TestAllWalksBareArrayListsByAfterID(t *testing.T) {
	for _, tc := range []struct {
		name  string
		total int
		extra []string
	}{
		{"250-page-size-100", 250, []string{"--page-size", "100"}},
		{"200-page-size-100", 200, []string{"--page-size", "100"}},
		{"250-server-default", 250, nil},
		{"40-page-size-100", 40, []string{"--page-size", "100"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := &ninjaFake{}
			srv := httptest.NewServer(f.devices(tc.total))
			defer srv.Close()
			args := append([]string{"devices", "get", "--all", "--data-source", "live", "--json"}, tc.extra...)
			out, _ := runNinjaCLI(t, srv.URL, args...)
			if n := strings.Count(out, `"systemName"`); n != tc.total {
				t.Fatalf("--all returned %d devices, want %d; requests: %v", n, tc.total, f.requests)
			}
			if tc.total > 100 && !strings.Contains(f.requests[1], "after=100") {
				t.Fatalf("second request %q does not resume after the last id of page one", f.requests[1])
			}
		})
	}
}

func TestAllWalksObjectCursorReportsToTheEnd(t *testing.T) {
	for _, tc := range []struct {
		name  string
		total int
		extra []string
	}{
		{"25-page-size-10", 25, []string{"--page-size", "10"}},
		{"250-server-default", 250, nil},
		{"40-one-page", 40, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			f := &ninjaFake{}
			srv := httptest.NewServer(f.volumes(tc.total))
			defer srv.Close()
			args := append([]string{"queries", "get-volumes", "--all", "--data-source", "live", "--json"}, tc.extra...)
			out, stderr := runNinjaCLI(t, srv.URL, args...)
			if n := strings.Count(out, `"capacity"`); n != tc.total {
				t.Fatalf("--all returned %d volumes, want %d; requests: %v\nstderr: %s", n, tc.total, f.requests, stderr)
			}
			if strings.Contains(stderr, "truncated") {
				t.Fatalf("--all reported a truncated walk:\n%s", stderr)
			}
		})
	}
}

func TestTailEmitsEveryPageOfAList(t *testing.T) {
	for _, total := range []int{250, 40} {
		t.Run(strconv.Itoa(total), func(t *testing.T) {
			testenv.Isolate(t)
			f := &ninjaFake{}
			srv := httptest.NewServer(f.devices(total))
			defer srv.Close()
			// tail writes NDJSON to os.Stdout itself.
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			saved := os.Stdout
			os.Stdout = w
			done := make(chan string)
			go func() {
				b, _ := io.ReadAll(r)
				done <- string(b)
			}()
			runNinjaCLI(t, srv.URL, "tail", "--resource", "devices", "--follow=false")
			os.Stdout = saved
			w.Close()
			out := <-done
			if n := strings.Count(out, `"systemName"`); n != total {
				t.Fatalf("tail emitted %d devices, want %d; requests: %v", n, total, f.requests)
			}
		})
	}
}

// /v2/ticketing/app-user-contact: a bare array keyset-paged by
// ?anchorNaturalId=<last naturalId> (spec.yaml "Last User Identifier from
// previous page"). naturalId differs from id, and an unknown parameter is
// ignored, so a reader that sends `after` or the id gets page one again.
func (f *ninjaFake) appUsers(total int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		if r.URL.Path != "/v2/ticketing/app-user-contact" {
			http.NotFound(w, r)
			return
		}
		anchor, _ := strconv.Atoi(r.URL.Query().Get("anchorNaturalId"))
		var rows []string
		for n := anchor + 1; n <= total && len(rows) < pageSizeOf(r); n++ {
			rows = append(rows, fmt.Sprintf(`{"uid":"u-%d","id":%d,"naturalId":%d,"firstName":"User%d","userType":"END_USER"}`, n, 5000+n, n, n))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "[%s]", strings.Join(rows, ","))
	}
}

func TestEveryReaderWalksTheAnchorNaturalIdKeyset(t *testing.T) {
	const total = 230
	t.Run("sync", func(t *testing.T) {
		testenv.Isolate(t)
		f := &ninjaFake{}
		srv := httptest.NewServer(f.appUsers(total))
		defer srv.Close()
		dbPath, _ := runNinjaSync(t, srv.URL, "ticketing-app-user-contact")
		if n := storedRows(t, dbPath, "ticketing-app-user-contact"); n != total {
			t.Fatalf("sync stored %d users, want %d; requests: %v", n, total, f.requests)
		}
	})
	t.Run("all", func(t *testing.T) {
		testenv.Isolate(t)
		f := &ninjaFake{}
		srv := httptest.NewServer(f.appUsers(total))
		defer srv.Close()
		out, _ := runNinjaCLI(t, srv.URL, "ticketing", "get-all-user-and-contacts", "--all", "--data-source", "live", "--json")
		if n := strings.Count(out, `"naturalId"`); n != total {
			t.Fatalf("--all returned %d users, want %d; requests: %v", n, total, f.requests)
		}
	})
	t.Run("export", func(t *testing.T) {
		testenv.Isolate(t)
		f := &ninjaFake{}
		srv := httptest.NewServer(f.appUsers(total))
		defer srv.Close()
		out, _ := runNinjaCLI(t, srv.URL, "export", "ticketing", "--format", "jsonl")
		if n := strings.Count(out, `"naturalId"`); n != total {
			t.Fatalf("export returned %d users, want %d; %d requests: %v", n, total, f.count(), f.requests[:min(5, len(f.requests))])
		}
	})
}

// The rendered paginator's 100-page safety cap is not a NinjaOne limit: a keyset
// or report walk carries its own progress guard, so `--all` walks to the end,
// and a walk that cannot finish never reports itself complete.
func TestAllWalksPastTheGeneratedPageCap(t *testing.T) {
	testenv.Isolate(t)
	f := &ninjaFake{}
	srv := httptest.NewServer(f.devices(150))
	defer srv.Close()
	var out string
	captured := captureProcessOutput(t, func() {
		out, _ = runNinjaCLI(t, srv.URL, "devices", "get", "--all", "--page-size", "1", "--data-source", "live", "--json", "--rate-limit", "0")
	})
	if n := strings.Count(out, `"systemName"`); n != 150 {
		t.Fatalf("--all returned %d devices, want 150 (%d requests)", n, f.count())
	}
	if strings.Contains(captured, "truncated") {
		t.Fatalf("a finished walk reported truncation:\n%s", captured)
	}
}

func TestAllNeverReportsATruncatedWalkComplete(t *testing.T) {
	testenv.Isolate(t)
	// A server that ignores `after` serves page one forever.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"systemName":"PC-1"},{"id":2,"systemName":"PC-2"}]`)
	}))
	defer srv.Close()
	captured := captureProcessOutput(t, func() {
		runNinjaCLI(t, srv.URL, "devices", "get", "--all", "--page-size", "2", "--data-source", "live", "--json")
	})
	if !strings.Contains(captured, "truncated") {
		t.Fatalf("a walk that could not advance was not reported truncated:\n%s", captured)
	}
	if strings.Contains(captured, `"event":"complete"`) {
		t.Fatalf("a truncated walk was reported complete:\n%s", captured)
	}
}
