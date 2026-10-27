// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ninjaone-pp-cli/internal/cliutil/testenv"
	"ninjaone-pp-cli/internal/store"
)

// spec.yaml /v2/queries/volumes: `include` "Additional information to include
// (bl - BitLocker status)". Without include=bl NinjaOne returns no
// bitLockerStatus, and bitlocker-gaps reads every Windows volume as NOT_REPORTED.

func TestSyncRequestsBitLockerStatusForVolumes(t *testing.T) {
	testenv.Isolate(t)
	f := &ninjaFake{}
	srv := httptest.NewServer(f.volumes(5))
	defer srv.Close()
	runNinjaSync(t, srv.URL, "queries-volumes")
	if len(f.requests) == 0 || !strings.Contains(f.requests[0], "include=bl") {
		t.Fatalf("sync of queries-volumes did not request BitLocker status: %v", f.requests)
	}
	// An operator override still wins.
	f2 := &ninjaFake{}
	srv2 := httptest.NewServer(f2.volumes(5))
	defer srv2.Close()
	runNinjaSyncArgs(t, srv2.URL, "--resources", "queries-volumes", "--resource-param", "queries-volumes:include=none")
	if !strings.Contains(f2.requests[0], "include=none") {
		t.Fatalf("--resource-param include did not win: %v", f2.requests)
	}
}

func TestBitlockerGapsSaysWhenTheStoreCarriesNoBitLockerStatus(t *testing.T) {
	for _, tc := range []struct {
		name  string
		vols  []string
		warns bool
	}{
		{"synced-without-include", []string{
			`{"deviceId":7,"name":"C:","capacity":100,"freeSpace":40}`,
			`{"deviceId":7,"name":"D:","capacity":100,"freeSpace":40}`,
		}, true},
		{"synced-with-include", []string{
			`{"deviceId":7,"name":"C:","capacity":100,"freeSpace":40,"bitLockerStatus":{"protectionStatus":"UNPROTECTED"}}`,
			`{"deviceId":7,"name":"D:","capacity":100,"freeSpace":40}`,
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			dbPath := filepath.Join(t.TempDir(), "data.db")
			db, err := store.OpenWithContext(context.Background(), dbPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := db.UpsertBatch(rtDevices, []json.RawMessage{json.RawMessage(`{"id":7,"systemName":"PC-7","organizationId":2,"nodeClass":"WINDOWS_WORKSTATION"}`)}); err != nil {
				t.Fatal(err)
			}
			items := make([]json.RawMessage, len(tc.vols))
			for i, v := range tc.vols {
				items[i] = json.RawMessage(v)
			}
			if _, _, err := db.UpsertBatch(rtVolumes, items); err != nil {
				t.Fatal(err)
			}
			db.Close()
			root := newRootCmd(&rootFlags{})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			root.SetArgs([]string{"bitlocker-gaps", "--db", dbPath, "--json"})
			if err := root.Execute(); err != nil {
				t.Fatalf("bitlocker-gaps: %v\n%s", err, errBuf.String())
			}
			warned := strings.Contains(errBuf.String(), "include=bl")
			if warned != tc.warns {
				t.Fatalf("warning shown = %v, want %v; stderr:\n%s", warned, tc.warns, errBuf.String())
			}
		})
	}
}
