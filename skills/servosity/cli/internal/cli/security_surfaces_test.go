// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"servosity-msp-pp-cli/internal/store"
)

func TestAPIDiscoveryUsesShippedBinaryName(t *testing.T) {
	stdout, _, err := runRootArgs(t, "api", "issues")
	if err != nil {
		t.Fatalf("api issues: %v", err)
	}
	if strings.Contains(stdout, "servosity-msp-cli") {
		t.Fatalf("api discovery leaked internal mint name: %q", stdout)
	}
	if !strings.Contains(stdout, "Use 'servosity-cli issues <method> --help'") {
		t.Fatalf("api discovery omitted shipped binary name: %q", stdout)
	}

	stdout, _, err = runRootArgs(t, "api")
	if err != nil {
		t.Fatalf("api: %v", err)
	}
	if !strings.Contains(stdout, "Use 'servosity-cli api <interface>'") {
		t.Fatalf("api list omitted shipped binary name: %q", stdout)
	}

	_, _, err = runRootArgs(t, "api", "does-not-exist")
	if err == nil || !strings.Contains(err.Error(), "Run 'servosity-cli api'") {
		t.Fatalf("api invalid-interface error = %v, want shipped binary guidance", err)
	}
}

func TestAPITokenReadRequiresExplicitHumanConsent(t *testing.T) {
	withTempLearnHome(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/current-user/api-token/" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"token":"fixture-live-token"}`)
	}))
	defer server.Close()
	t.Setenv("SERVOSITY_MSP_BASE_URL", server.URL)
	t.Setenv("SERVOSITY_MSP_TOKEN", "fixture-auth-token")

	stdout, _, err := runRootArgs(t, "current-user", "api-token-list", "--json")
	if err == nil || !strings.Contains(err.Error(), "--reveal-token") {
		t.Fatalf("default token read error = %v, want explicit-consent refusal", err)
	}
	if stdout != "" || calls.Load() != 0 {
		t.Fatalf("default token read contacted API or wrote output: calls=%d stdout=%q", calls.Load(), stdout)
	}

	stdout, _, err = runRootArgs(t, "current-user", "api-token-list", "--json", "--reveal-token")
	if err != nil {
		t.Fatalf("explicit token read: %v", err)
	}
	if calls.Load() != 1 || !strings.Contains(stdout, "fixture-live-token") {
		t.Fatalf("explicit token read calls=%d stdout=%q", calls.Load(), stdout)
	}
}

func TestAPITokenReadIsExcludedFromMCPAndSync(t *testing.T) {
	root := RootCmd()
	cmd, _, err := root.Find([]string{"current-user", "api-token-list"})
	if err != nil {
		t.Fatalf("find token command: %v", err)
	}
	if cmd.Annotations["mcp:hidden"] != "true" {
		t.Fatalf("token command annotations = %#v, want mcp:hidden=true", cmd.Annotations)
	}
	if _, ok := cmd.Annotations["mcp:read-only"]; ok {
		t.Fatalf("secret-returning command must not be annotated read-only: %#v", cmd.Annotations)
	}

	for label, resources := range map[string][]string{
		"default": defaultSyncResources(),
		"known":   knownSyncResourceNames(),
	} {
		for _, resource := range resources {
			if resource == "current-user-api-token" {
				t.Fatalf("%s sync resources still include sensitive resource", label)
			}
		}
	}
	if _, err := syncResourcePath("current-user-api-token"); err == nil {
		t.Fatal("syncResourcePath accepted sensitive resource")
	}
}

func TestCLIReadOnlyStoreRefusesPrePurgeSchema(t *testing.T) {
	withTempLearnHome(t)
	t.Setenv("SERVOSITY_MSP_TOKEN", "fixture-scope-token")
	configureDefaultDBScope("")
	t.Cleanup(func() { setDefaultDBScopeCredential("") })
	dbPath := defaultDBPath("servosity-cli")
	writable, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw store: %v", err)
	}
	const fixtureSecret = "fixture-cli-pre-migration-token"
	if _, err := raw.Exec(`INSERT INTO resources (id, resource_type, data) VALUES ('token-user', 'current-user', ?)`, fmt.Sprintf(`{"id":"token-user","token":%q}`, fixtureSecret)); err != nil {
		raw.Close()
		t.Fatalf("seed sensitive current-user row: %v", err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 9`); err != nil {
		raw.Close()
		t.Fatalf("stamp old schema: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw store: %v", err)
	}

	stdout, _, err := runRootArgs(t, "current-user", "list", "--data-source", "local", "--json")
	if err == nil || !strings.Contains(err.Error(), "security migration") || strings.Contains(err.Error(), fixtureSecret) {
		t.Fatalf("pre-purge local read error = %v", err)
	}
	if strings.Contains(stdout, fixtureSecret) {
		t.Fatalf("pre-purge local read exposed secret: %q", stdout)
	}

	raw, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("reopen raw store: %v", err)
	}
	defer raw.Close()
	var version, rows int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'current-user'`).Scan(&rows); err != nil {
		t.Fatalf("count sensitive rows: %v", err)
	}
	if version != 9 || rows != 1 {
		t.Fatalf("read-only CLI mutated refused store: version=%d rows=%d", version, rows)
	}
}
