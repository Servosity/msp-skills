// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"nerdio-pp-cli/internal/store"
)

// Codex r1 (action1 reprint on 4.34.2) class incomplete list read as
// complete, on Nerdio's one documented list shape: every NMM list is a bare
// JSON array with no paging. Each list here holds more records than any
// default page size the generator knows (100), and the fake answers the whole
// array whatever paging parameter it is sent, so sync must store every record
// of the one answer and report the sync complete; the read carries the token
// minted at the Entra tenant.
func TestSyncReadsEveryPage(t *testing.T) {
	const accounts, scripts = 131, 105
	var pagingParams []string
	m := newFakeInstance(t, "paging-client-id", "paging-client-secret-3b7d", "paging-minted-token-6c1e", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer paging-minted-token-6c1e" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		for k := range r.URL.Query() {
			pagingParams = append(pagingParams, r.URL.Path+"?"+k)
		}
		var items []map[string]any
		switch r.URL.Path {
		case "/rest-api/v1/accounts":
			for i := 0; i < accounts; i++ {
				items = append(items, map[string]any{"id": 1000 + i, "name": fmt.Sprintf("Account %d", i)})
			}
		case "/rest-api/v1/scripted-actions":
			for i := 0; i < scripts; i++ {
				items = append(items, map[string]any{"id": 5000 + i, "name": fmt.Sprintf("Script %d", i)})
			}
		default:
			_, _ = io.WriteString(w, `[]`)
			return
		}
		_ = json.NewEncoder(w).Encode(items)
	})
	dir := t.TempDir()
	for k, v := range map[string]string{"NERDIO_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"),
		"NERDIO_CONFIG": filepath.Join(dir, "config.toml"), "NERDIO_NO_CONFIG_WRITE": "1"} {
		t.Setenv(k, v)
	}
	m.useAs(t)
	db := filepath.Join(dir, "nmm.db")
	var out, errb bytes.Buffer
	cmd := newRootCmd(&rootFlags{})
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"sync", "--db", db, "--full", "--resources", "accounts,scripted-actions", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync: %v\nstdout: %s\nstderr: %s", err, out.String(), errb.String())
	}
	if m.mints.Load() == 0 {
		t.Fatalf("sync never minted; the fake answered nothing:\n%s%s", out.String(), errb.String())
	}
	if !strings.Contains(out.String(), `"errored": 0`) || !strings.Contains(out.String(), `"warned": 0`) {
		t.Errorf("sync did not report a complete sync:\n%s", out.String())
	}
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for rt, want := range map[string]int{"accounts": accounts, "scripted-actions": scripts} {
		var n int
		if err := s.DB().QueryRow(`SELECT count(*) FROM resources WHERE resource_type = ?`, rt).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("%s: sync stored %d of %d records (paging params sent: %v):\n%s", rt, n, want, pagingParams, strings.TrimSpace(errb.String()))
		}
	}
}

// Codex r1 (action1 reprint on 4.34.2) class rotating token changes offline
// store identity, as it reaches Nerdio: each MSP's NMM instance is its own
// tenant. Instance A was used with a pasted token (auth set-token) and
// synced; the operator then points the CLI at instance B with B's client pair
// (and B's Entra tenant). The default store is named by the token in effect
// when no client is configured and adopted into the client's store on the
// next run, so B's commands read A's records. B's local reads must show only
// what B synced.
func TestStoreOfAnotherInstanceIsNeverAdopted(t *testing.T) {
	instance := func(tag string) *fakeInstance {
		return newFakeInstance(t, "scope-client-"+tag, "scope-secret-"+tag+"-7c3e", "scope-minted-"+tag+"-4d2a", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/rest-api/v1/accounts" {
				_, _ = io.WriteString(w, `[{"id":101,"name":"Account-`+tag+`"}]`)
				return
			}
			_, _ = io.WriteString(w, `[]`)
		})
	}
	a, b := instance("InstanceA"), instance("InstanceB")
	dir := t.TempDir()
	for k, v := range map[string]string{"NERDIO_HOME": dir, "HOME": dir, "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_CACHE_HOME": filepath.Join(dir, "cache"), "NERDIO_CONFIG": "",
		"NERDIO_TOKEN_URL": "", "NERDIO_CLIENT_ID": "", "NERDIO_CLIENT_SECRET": "", "NERDIO_NO_CONFIG_WRITE": ""} {
		t.Setenv(k, v)
	}
	runErr := func(stdin string, args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		cmd := newRootCmd(&rootFlags{})
		cmd.SetIn(strings.NewReader(stdin))
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}
	run := func(stdin string, args ...string) string {
		t.Helper()
		out, err := runErr(stdin, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	t.Setenv("NERDIO_BASE_URL", a.srv.URL)
	run("scope-pasted-token-InstanceA-0123456789\n", "auth", "set-token")
	if out := run("", "sync", "--resources", "accounts", "--json"); !strings.Contains(out, `"total_records": 1`) {
		t.Fatalf("instance A's sync stored nothing; the test did not run:\n%s", out)
	}
	b.useAs(t)
	// Before B syncs, B's store is empty (a "no local data" refusal is the
	// right answer); it must never be A's.
	if out, err := runErr("", "accounts", "--data-source", "local", "--json"); strings.Contains(out, "Account-InstanceA") {
		t.Errorf("instance B's local read shows instance A's synced account (A's store was adopted into B's; err %v):\n%s", err, out)
	}
	if out := run("", "sync", "--resources", "accounts", "--json"); !strings.Contains(out, `"errored": 0`) {
		t.Fatalf("instance B's sync failed:\n%s", out)
	}
	if b.mints.Load() == 0 {
		t.Fatalf("instance B's sync never minted at B's tenant; the control did not run")
	}
	out := run("", "accounts", "--data-source", "local", "--json")
	if !strings.Contains(out, "Account-InstanceB") {
		t.Fatalf("instance B's local read lost its own account; the control did not run:\n%s", out)
	}
	if strings.Contains(out, "Account-InstanceA") {
		t.Errorf("instance B's local read shows instance A's synced account:\n%s", out)
	}
}
