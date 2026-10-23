// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

// TestDefaultSyncSkipsMFASecrets pins that a default sync (and workflow
// archive, which reuses the list) never requests the MFA-secret endpoints.
// Two of them are GETs that can regenerate backup codes or start an
// enrollment on the server, and the store refuses all three.
func TestDefaultSyncSkipsMFASecrets(t *testing.T) {
	defaults := defaultSyncResources()
	for _, name := range []string{"backups-mfa-codes", "current-user-mfa-backup-codes", "current-user-start-mfa"} {
		if slices.Contains(defaults, name) {
			t.Errorf("default sync includes %s", name)
		}
	}
	if !slices.Contains(defaults, "backups") || !slices.Contains(defaults, "companies") {
		t.Fatal("default sync lost its ordinary resources")
	}
}

// TestSyncRefusesExplicitMFASecretResources pins that naming an MFA-secret
// resource is refused before any request, so the side-effecting GET never runs.
func TestSyncRefusesExplicitMFASecretResources(t *testing.T) {
	isolateNovelStore(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	t.Setenv("SERVOSITY_MSP_BASE_URL", srv.URL)
	t.Setenv("SERVOSITY_MSP_TOKEN", "fixture-token")
	for _, name := range []string{"current-user-mfa-backup-codes", "current-user-start-mfa", "backups-mfa-codes"} {
		cmd := RootCmd()
		cmd.SetArgs([]string{"sync", "--resources", name})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "is not synced") {
			t.Fatalf("sync --resources %s: err = %v, want the refusal", name, err)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("%d request(s) reached the API for a refused MFA resource", n)
	}
}
