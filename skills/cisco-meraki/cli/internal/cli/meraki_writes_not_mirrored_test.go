// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-writes-not-mirrored" and "meraki-sync-skips-webhook-logs".

package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cisco-meraki-pp-cli/internal/cliutil/testenv"
	"cisco-meraki-pp-cli/internal/store"
)

// TestWriteResponseIsNotMirrored: an SSID update echoes its PSK and webhook
// template updates echo Authorization headers, so a write's response must
// never land on disk where search, analytics and MCP sql can read it. The
// sentinel sits in the plain "name" field, so the test fails whenever a row
// is stored, not only when redaction misses a key.
func TestWriteResponseIsNotMirrored(t *testing.T) {
	home := testenv.Isolate(t)
	const sentinel = "SENTINEL-WRITE-7c41"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"number":0,"name":"Guest ` + sentinel + `","authMode":"psk","psk":"psk-value"}`))
	}))
	defer srv.Close()
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
	// Write-through only runs into an existing mirror, so create it first.
	db, err := store.OpenWithContext(context.Background(), defaultDBPath("cisco-meraki-cli"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"networks", "wireless", "update-network-ssid", "N_1", "0", "--name", "Guest", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("update-network-ssid: %v\n%s", err, out.String())
	}
	_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, _ := os.ReadFile(path); bytes.Contains(b, []byte(sentinel)) {
			t.Fatalf("write response stored on disk: %s", path)
		}
		return nil
	})
}

// TestSyncSkipsWebhookLogs: every webhook delivery log row carries the
// receiver URL, which for Slack, Teams or PagerDuty is a credential.
func TestSyncSkipsWebhookLogs(t *testing.T) {
	for _, d := range dependentResourceDefs() {
		if strings.Contains(d.PathTemplate, "/webhooks/") {
			t.Fatalf("sync mirrors %s (%s)", d.Name, d.PathTemplate)
		}
	}
}
