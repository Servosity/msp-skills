// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/cliutil/testenv"
	"prtg-pp-cli/internal/fleet"
)

// fakeCore is a TLS PRTG list endpoint with `total` rows; it records the
// Authorization headers it saw.
func fakeCore(t *testing.T, total int) (*httptest.Server, *[]string) {
	t.Helper()
	var auths []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer core-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"AUTH_FAILED","message":"The authentication has failed."}`))
			return
		}
		off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items := []map[string]any{}
		for i := off; i < off+lim && i < total; i++ {
			items = append(items, map[string]any{"id": strconv.Itoa(i), "windowsloginpassword": "s3cret"})
		}
		_ = json.NewEncoder(w).Encode(items)
	}))
	t.Cleanup(srv.Close)
	return srv, &auths
}

// pinCore registers core "t1" for srv and returns a reset func for the
// process-wide selector state.
func pinCore(t *testing.T, srv *httptest.Server) {
	t.Helper()
	dir := testenv.Isolate(t)
	t.Setenv(fleet.DirEnv, dir)
	t.Setenv("PRTG_FLEET_KEYS", "t1=core-key,t2=another-cores-key-that-must-not-be-sent")
	t.Setenv("PRTG_API_KEY", "a-different-key-that-must-not-be-sent")
	if err := fleet.Update(func(r *fleet.Registry) error {
		r.Upsert(fleet.Core{Name: "t1", Server: strings.TrimPrefix(srv.URL, "https://"), InsecureTLS: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Core pinning sets process env and the process-wide home override; let
	// t.Setenv restore every variable it may touch, and clear the override.
	for _, k := range []string{"PRTG_SERVER", "PRTG_BASE_URL", "PRTG_CORE", "PRTG_CONFIG", "PRTG_SKIP_TLS_VERIFY", fleet.ChildEnv} {
		t.Setenv(k, "")
	}
	t.Cleanup(func() {
		coreSelection, pinnedCore, insecureTLS = "", "", false
		_, _ = cliutil.SetHomeOverride("")
	})
}

func runRaw(t *testing.T, args ...string) ([]json.RawMessage, error) {
	t.Helper()
	out, errb, err := runRootArgs(t, append([]string{"--core", "t1", "fleet", "exec", "--raw"}, append(args, "--json")...)...)
	if err != nil {
		return nil, err
	}
	var bodies []json.RawMessage
	if jerr := json.Unmarshal(bytes.TrimSpace([]byte(out)), &bodies); jerr != nil {
		t.Fatalf("raw output is not a JSON array: %v\n%s\n%s", jerr, out, errb)
	}
	return bodies, nil
}

func TestRawChildPagesWithThePinnedCoreKey(t *testing.T) {
	srv, auths := fakeCore(t, 5)
	pinCore(t, srv)
	bodies, err := runRaw(t, "--req", "GETALL /sensors?limit=2")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if len(bodies) != 1 || json.Unmarshal(bodies[0], &rows) != nil || len(rows) != 5 {
		t.Fatalf("paged rows = %s", bodies)
	}
	if strings.Contains(string(bodies[0]), "s3cret") {
		t.Fatalf("credential field not redacted: %s", bodies[0])
	}
	for _, a := range *auths {
		if a != "Bearer core-key" {
			t.Fatalf("a request carried %q instead of the pinned core's key", a)
		}
	}
}

func TestRawChildRefusesToTruncateAtThePageCap(t *testing.T) {
	srv, _ := fakeCore(t, 100000) // more rows than MaxListPages pages of 2
	pinCore(t, srv)
	bodies, err := runRaw(t, "--req", "GETALL /sensors?limit=2")
	if err != nil {
		t.Fatal(err)
	}
	e, ok := fleet.RequestError(bodies[0])
	if !ok || !strings.Contains(e.Message, "more than") {
		t.Fatalf("a list past the page cap must be an error, got %s", bodies[0])
	}
}

func TestRawChildRefusesWritesWithoutTheRunnerMarker(t *testing.T) {
	srv, auths := fakeCore(t, 1)
	pinCore(t, srv)
	if _, err := runRaw(t, "--req", "POST /sensors/acknowledge"); err == nil {
		t.Fatal("a raw POST without the runner marker must be refused")
	}
	if len(*auths) != 0 {
		t.Fatalf("a refused write still reached the core: %v", *auths)
	}
}
