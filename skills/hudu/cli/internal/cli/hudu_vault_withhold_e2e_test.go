// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): end to end through the
// real client, the real `sync` walk and the real SQLite store. Before the
// filter, a synced vault entry put its password and TOTP seed in the store,
// where `search` and MCP `sql` returned them.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hudu-pp-cli/internal/client"
	"hudu-pp-cli/internal/config"
	"hudu-pp-cli/internal/store"
)

const (
	e2eVaultSentinelPW  = "SENTINEL-PW-1"
	e2eVaultSentinelOTP = "SENTINEL-OTP-1"
)

func TestVaultSecretsWithheldFromSyncStore(t *testing.T) {
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_VERIFY_LIVE_HTTP", "")
	t.Setenv("HUDU_HOME", t.TempDir())
	entry := func(id, name string) string {
		return `{"id":` + id + `,"company_id":3,"name":"` + name + `","username":"administrator","password":"` + e2eVaultSentinelPW + `","otp_secret":"` + e2eVaultSentinelOTP + `","updated_at":"2026-02-03T04:05:06Z"}`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/asset_passwords" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		if p := r.URL.Query().Get("page"); p != "" && p != "1" {
			_, _ = w.Write([]byte(`{"asset_passwords":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"asset_passwords":[` + entry("1", "DC admin") + `,` + entry("2", "Firewall admin") + `]}`))
	}))
	t.Cleanup(srv.Close)

	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	c := client.New(&config.Config{BaseURL: srv.URL}, 10*time.Second, 0)
	c.NoCache = true
	res := syncResource(context.Background(), c, db, "asset-passwords", "", true, 0, false, false, nil, nil)
	if res.Err != nil {
		t.Fatalf("sync asset-passwords: %v", res.Err)
	}
	if res.Count != 2 {
		t.Fatalf("sync stored %d entries, want 2 (the filter must not break sync)", res.Count)
	}

	// `search` over the mirror finds the entry by name and returns no secret.
	hits, err := db.Search("admin", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("search found no synced vault entry; metadata did not reach the store")
	}
	for _, h := range hits {
		if strings.Contains(string(h), e2eVaultSentinelPW) || strings.Contains(string(h), e2eVaultSentinelOTP) {
			t.Fatalf("search returned a vault secret: %s", h)
		}
	}
	if !strings.Contains(vaultJoinRaw(hits), "administrator") {
		t.Fatalf("search lost the username metadata: %s", vaultJoinRaw(hits))
	}

	// What MCP `sql` would read: every stored JSON document.
	rows, err := db.DB().Query(`SELECT data FROM resources UNION ALL SELECT data FROM asset_passwords`)
	if err != nil {
		t.Fatalf("query store: %v", err)
	}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if strings.Contains(data, e2eVaultSentinelPW) || strings.Contains(data, e2eVaultSentinelOTP) {
			t.Fatalf("store row holds a vault secret: %s", data)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	_ = rows.Close()
	if err := db.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	// No byte of the secret anywhere in the database files (WAL included).
	matches, _ := filepath.Glob(dbPath + "*")
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if strings.Contains(string(b), e2eVaultSentinelPW) || strings.Contains(string(b), e2eVaultSentinelOTP) {
			t.Fatalf("database file %s holds a vault secret", m)
		}
	}
}

func vaultJoinRaw(items []json.RawMessage) string {
	var b strings.Builder
	for _, it := range items {
		b.Write(it)
	}
	return b.String()
}
