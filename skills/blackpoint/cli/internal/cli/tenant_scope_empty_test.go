// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (Codex r2 P1 "tenant scope crossed", empty selector): an
// explicitly supplied but empty x-tenant-id still names the tenant scope, so
// it must never be answered from the mixed-tenant local store.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blackpoint-pp-cli/internal/cliutil/testenv"
)

func TestEmptyTenantSelectorNeverServesTheFleet(t *testing.T) {
	testenv.Isolate(t)
	fake := &fakeCompassOne{detections: map[string][]map[string]any{
		"t1": {{"id": "ag-t1-only", "status": "OPEN", "type": "MDR", "alertCount": 1, "created": time.Now().UTC().Format(time.RFC3339)}},
		"t2": {{"id": "ag-t2-only", "status": "OPEN", "type": "MDR", "alertCount": 1, "created": time.Now().UTC().Format(time.RFC3339)}},
	}}
	srv := httptest.NewServer(http.HandlerFunc(fake.handler))
	defer srv.Close()
	t.Setenv("BLACKPOINT_BASE_URL", srv.URL)
	t.Setenv("BLACKPOINT_API_TOKEN", "test-token")
	if _, err := runFleetCmd(t, "fleet-sync", "--kinds", "detections", "--json"); err != nil {
		t.Fatalf("fleet-sync: %v", err)
	}
	out, err := runFleetCmd(t, "detections", "list", "--x-tenant-id=", "--data-source", "local", "--json")
	if strings.Contains(out, "ag-t1-only") || strings.Contains(out, "ag-t2-only") {
		t.Fatalf("an empty --x-tenant-id with --data-source local returned fleet rows: %s", out)
	}
	if err == nil {
		t.Fatalf("an empty tenant selector read locally must be refused, got: %s", out)
	}
}
