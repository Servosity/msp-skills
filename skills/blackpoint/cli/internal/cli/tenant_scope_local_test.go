// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Codex r1 P1 "tenant scope crossed"): the local store does not
// key rows by x-tenant-id, so a tenant-scoped read must never be answered from
// it, whether the rows came from fleet-sync, a write-through, or a fallback.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blackpoint-pp-cli/internal/cliutil/testenv"
)

func TestTenantScopedReadsNeverServeAnotherTenantsRows(t *testing.T) {
	testenv.Isolate(t)
	fake := &fakeCompassOne{detections: map[string][]map[string]any{
		"t1": {{"id": "ag-t1-only", "status": "OPEN", "type": "MDR", "alertCount": 1, "created": time.Now().UTC().Format(time.RFC3339)}},
		"t2": {},
	}}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	t.Setenv("BLACKPOINT_BASE_URL", srv.URL)
	t.Setenv("BLACKPOINT_API_TOKEN", "test-token")

	// Rows reach the default store two ways: fleet-sync's search index and the
	// auto-mode write-through of a live tenant-scoped read.
	if _, err := runFleetCmd(t, "fleet-sync", "--kinds", "detections", "--json"); err != nil {
		t.Fatalf("fleet-sync: %v", err)
	}
	if out, err := runFleetCmd(t, "detections", "list", "--x-tenant-id", "t1", "--json"); err != nil || !strings.Contains(out, "ag-t1-only") {
		t.Fatalf("live read for t1 should return ag-t1-only (err=%v): %s", err, out)
	}

	out, err := runFleetCmd(t, "detections", "list", "--x-tenant-id", "t2", "--data-source", "local", "--json")
	if strings.Contains(out, "ag-t1-only") {
		t.Fatalf("--data-source local for t2 returned t1's detection: %s", out)
	}
	if err == nil {
		t.Fatalf("a tenant-scoped --data-source local read must be refused, got: %s", out)
	}

	srv.Close() // the network drops; auto must not fall back to unscoped local rows
	out, err = runFleetCmd(t, "detections", "list", "--x-tenant-id", "t2", "--json")
	if strings.Contains(out, "ag-t1-only") {
		t.Fatalf("network fallback for t2 returned t1's detection: %s", out)
	}
	if err == nil {
		t.Fatalf("a tenant-scoped read with the API unreachable must fail, got: %s", out)
	}
}
