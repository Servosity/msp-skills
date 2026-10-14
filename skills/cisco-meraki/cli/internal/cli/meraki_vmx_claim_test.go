// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-vmx-claim-is-a-write".

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"cisco-meraki-pp-cli/internal/cliutil/testenv"
)

// TestVmxClaimIsNotReplayed: claiming a vMX uses a license, so an ambiguous
// 502 must not be resubmitted the way an idempotent read would be.
func TestVmxClaimIsNotReplayed(t *testing.T) {
	testenv.Isolate(t)
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"networks", "devices", "vmx-network-claim", "L_100", "--size", "small", "--json"})
	_ = cmd.Execute()
	if n := posts.Load(); n != 1 {
		t.Fatalf("vmx claim POSTed %d times after a 502, want 1\n%s", n, out.String())
	}
	if cmd, _, _ := RootCmd().Find([]string{"networks", "devices", "vmx-network-claim"}); cmd.Annotations["mcp:read-only"] == "true" {
		t.Fatal("vmx-network-claim is a write and must not be annotated mcp:read-only")
	}
}
