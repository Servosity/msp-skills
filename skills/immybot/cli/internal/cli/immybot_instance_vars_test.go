// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"immybot-pp-cli/internal/store"
)

// The round-2 input: the store is keyed on the configured instance, then
// `sync --path-context subdomain=beta` would point the requests at another
// one. sync refuses it as a usage error before any request or store write.
func TestSyncRefusesAServerURLVariableFromPathContext(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":42,"name":"BETA-PC","tenant":"Beta","tenantId":8,"online":true}]`))
	}))
	defer srv.Close()
	credTierHome(t, srv.URL)
	t.Setenv("IMMYBOT_SUBDOMAIN", "alpha")
	dbPath := filepath.Join(t.TempDir(), "immy.db")

	_, stderr, err := credTierRun(t, "sync", "--resources", "computers", "--path-context", "subdomain=beta", "--db", dbPath)
	if err == nil || !strings.Contains(err.Error()+stderr, "cannot set subdomain") {
		t.Fatalf("sync --path-context subdomain=beta: err %v, stderr %q; want the server-URL variable refused", err, stderr)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("%d request(s) reached the server; want none", n)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources`).Scan(&rows); err != nil && !immyNoSuchTableErr(err) {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("the store holds %d row(s); want none", rows)
	}
}

// Every server-URL variable is refused: the ones buildURL fills from
// Config.TemplateVars and any placeholder the configured base URL carries.
func TestServerURLVariablesAreNeverPathContext(t *testing.T) {
	for _, tc := range []struct {
		base, key string
		refused   bool
	}{
		{"https://{subdomain}.immy.bot", "subdomain", true},
		{"https://acme.immy.bot", "subdomain", true},
		{"https://{region}.example.test/{subdomain}", "region", true},
		{"https://acme.immy.bot", "tenantId", false},
	} {
		err := immyRefuseServerURLVars(map[string]string{tc.key: "x"}, tc.base)
		if (err != nil) != tc.refused {
			t.Errorf("base %q key %q: err %v, want refused = %t", tc.base, tc.key, err, tc.refused)
		}
	}
}
