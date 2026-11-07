// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"cork-pp-cli/internal/store"
)

// Two connected integrations share one vendor key (documented Integration.vendor.key). A client whose only
// tenant is on the healthy one must never be listed as affected by the down one.
func TestIntegrationsHealthAttributesClientsByIntegrationNotVendor(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cork.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	client := `{"uuid":"c-1","name":"Client C","associated_tenants":[{"uuid":"t-1","name":"T","integration":{"uuid":"int-B","vendor":{"key":"NINJA_RMM","name":"NinjaOne"}}}]}`
	if err := db.Upsert("clients", "c-1", json.RawMessage(client)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"uuid":"int-A","display_name":"Ninja A","connection_status":"down","last_synced_at":"2026-11-01T00:00:00Z","vendor":{"key":"NINJA_RMM","name":"NinjaOne"}},
			{"uuid":"int-B","display_name":"Ninja B","connection_status":"down","last_synced_at":"2026-11-01T00:00:00Z","vendor":{"key":"NINJA_RMM","name":"NinjaOne"}}]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORK_BASE_URL", srv.URL)
	t.Setenv("CORK_API_KEY", "dummy")
	cmd := RootCmd()
	cmd.SetArgs([]string{"integrations", "health", "--db", dbPath, "--json"})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v\n%s%s", err, out.String(), errOut.String())
	}
	var v integrationHealthView
	if err := json.Unmarshal([]byte(out.String()[strings.Index(out.String(), "{"):]), &v); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	got := map[string][]string{}
	for _, row := range v.Items {
		got[row.UUID] = row.AffectedClients
	}
	if len(got["int-A"]) != 0 {
		t.Errorf("int-A lists %v, but Client C's only tenant is on int-B", got["int-A"])
	}
	if len(got["int-B"]) != 1 || got["int-B"][0] != "Client C" {
		t.Errorf("int-B must list Client C, got %v", got["int-B"])
	}
}
