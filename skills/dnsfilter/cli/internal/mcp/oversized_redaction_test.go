// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-redacts-before-bounding" entry.

package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/store"
)

// fakeCLIModeEnv turns this test binary into a stand-in companion CLI: the
// mirror tools exec DNSFILTER_CLI_PATH (pointed at os.Args[0]) and the child
// prints an oversized body carrying network secrets instead of running tests.
const fakeCLIModeEnv = "DNSFILTER_MCP_TEST_FAKE_CLI_MODE"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeCLIModeEnv); mode != "" {
		os.Exit(runFakeCLI(mode))
	}
	os.Exit(m.Run())
}

// bigNetworkRow is a 19-attribute networks row with a unique secret_key.
func bigNetworkRow(i int) map[string]any {
	return map[string]any{
		"id": i, "name": fmt.Sprintf("Site %04d", i), "organization_id": 3, "secret_key": fmt.Sprintf("NETSECRET-%04d-a8f3c2d1e9b7", i),
		"block_page_id": 11, "external_id": fmt.Sprintf("ext-%d", i), "is_legacy_vpn_active": false, "local_domains": []any{"corp.local"},
		"local_resolvers": []any{"10.0.0.1"}, "physical_address": "1 Main Street, Springfield", "policy_ids": []any{1, 2},
		"scheduled_policy_id": 4, "protected": true, "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-10-01T00:00:00Z",
		"ip_addresses_count": 2, "latitude": 40.1, "longitude": -74.2, "has_secret_key": true,
	}
}

// bigNetworksBody is a networks list over the 60000-byte MCP budget with two
// top-level arrays (data and included), the shape that falls back to the
// raw-byte preview.
func bigNetworksBody() []byte {
	rows := make([]any, 0, 200)
	included := make([]any, 0, 200)
	for i := 0; i < 200; i++ {
		rows = append(rows, bigNetworkRow(i))
		included = append(included, map[string]any{"type": "network_secret", "id": i, "attributes": map[string]any{"secret_key": fmt.Sprintf("NETSECRET-INC-%04d-77aa", i)}})
	}
	b, _ := json.Marshal(map[string]any{"data": rows, "included": included})
	return b
}

func runFakeCLI(mode string) int {
	switch mode {
	case "json":
		_, _ = os.Stdout.Write(bigNetworksBody())
	case "ndjson":
		for i := 0; i < 300; i++ {
			line, _ := json.Marshal(map[string]any{"event": "data", "data": bigNetworkRow(i)})
			fmt.Println(string(line))
		}
	case "csv":
		fmt.Println("id,name,organization_id,secret_key,physical_address")
		for i := 0; i < 2000; i++ {
			fmt.Printf("%d,Site %04d,3,NETSECRET-%04d-a8f3c2d1e9b7,1 Main Street\n", i, i, i)
		}
	case "fail-stdout":
		_, _ = os.Stdout.Write(bigNetworksBody())
		return 1
	case "fail-stderr":
		fmt.Fprint(os.Stderr, "Error: upstream returned ")
		_, _ = os.Stderr.Write(bigNetworksBody())
		return 1
	case "auth401", "auth403", "auth-real-key":
		// The companion CLI's own 401/403 output (captured from dnsfilter-cli
		// 2026-10-20), and the same hint carrying a real-looking key
		// (mcp-auth-errors-keep-remedy).
		key := "your-token-here"
		if mode == "auth-real-key" {
			key = "9f8e7d6c5b4a3f2e1d0c"
		}
		if mode == "auth403" {
			fmt.Fprintf(os.Stderr, "Error: GET /v1/networks/all returned HTTP 403: {\"errors\": [{\"detail\": \"Forbidden\"}]}\nhint: permission denied. Your credentials are valid but lack access to this resource.\n      Check that your credentials have the required permissions and match the API's expected auth scheme.\n      Set your API key with: export DNSFILTER_API_KEY=\"%s\" (PowerShell: $env:DNSFILTER_API_KEY = \"%s\")\n      Get a key at: https://help.dnsfilter.com/hc/en-us/articles/21169189058323-API-Tokens\n", key, key)
		} else {
			fmt.Fprintf(os.Stderr, "Error: GET /v1/networks/all returned HTTP 401: {\"errors\": [{\"detail\": \"Unauthorized\"}]}\nhint: check your API key. Set your API key with: export DNSFILTER_API_KEY=\"%s\" (PowerShell: $env:DNSFILTER_API_KEY = \"%s\")\n      Get a key at: https://help.dnsfilter.com/hc/en-us/articles/21169189058323-API-Tokens\n      Run 'dnsfilter-cli doctor' to check auth status.\n", key, key)
		}
		return 4
	case "marker":
		// Proves a mirror tool ran: the argument-gate test asserts this file
		// never appears (mcp-tools-refuse-undeclared-arguments).
		_ = os.WriteFile(os.Getenv("DNSFILTER_MCP_TEST_MARKER"), []byte("ran"), 0o600)
		return 0
	case "sync-resumed":
		// sync on a walk resumed from a saved cursor: the remedy is on
		// stdout, stderr only reports the warned run (mcp-failure-keeps-stdout-remedy).
		fmt.Println(`{"event":"sync_start","resource":"user-agents"}`)
		fmt.Println(`{"event":"sync_warning","resource":"user-agents","reason":"resumed_from_cursor","message":"the walk resumed from a saved cursor; run 'dnsfilter-cli sync --full --resources user-agents' to complete it"}`)
		fmt.Println(`{"event":"sync_summary","warned":1}`)
		fmt.Fprintln(os.Stderr, "Error: 1 resource(s) completed with warnings but no successful syncs")
		return 1
	}
	return 0
}

// TestSyncToolFailureShowsStdoutRemedy: over MCP the sync tool's error carries
// the child's sync_warning remedy, through the result funnel.
func TestSyncToolFailureShowsStdoutRemedy(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	t.Setenv("DNSFILTER_CLI_PATH", os.Args[0])
	t.Setenv(fakeCLIModeEnv, "sync-resumed")
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	frame := callToolWire(t, newRegisteredServer(t), "sync", map[string]any{"resources": "user-agents"})
	if !strings.Contains(frame, "completed with warnings") || !strings.Contains(frame, "sync --full --resources user-agents") {
		t.Fatalf("the sync tool error lost the --full remedy: %s", frame)
	}
}

func setIsolatedHome(t *testing.T, baseURL string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("DNSFILTER_HOME", home)
	t.Setenv("DNSFILTER_CONFIG", "")
	t.Setenv("DNSFILTER_BASE_URL", baseURL)
	t.Setenv("DNSFILTER_API_KEY", "test-key")
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		t.Fatalf("home override: %v", err)
	}
	t.Cleanup(restore)
}

// TestReadOnlyOversizedEndpointBodyNeverLeaksSecrets is adversarial round 1's
// P1: a read-only GET whose body exceeds the budget used to return a preview
// of the RAW body, secret_key included.
func TestReadOnlyOversizedEndpointBodyNeverLeaksSecrets(t *testing.T) {
	body := bigNetworksBody()
	if len(body) <= 60000 {
		t.Fatalf("fixture is %d bytes; it must exceed the 60000-byte budget", len(body))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)

	ids := []string{"networks.index", "networks.all", "networks.msp", "networks.msp-all"}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	s := newRegisteredServer(t)
	frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "networks.index", "params": map[string]any{"page[size]": 100}})
	if !strings.Contains(frame, "_pp_truncated") || !strings.Contains(frame, "NETSECRET") {
		t.Fatalf("default mode should reach the bounded preview with the raw body; the fixture does not exercise the leak path: %.300s", frame)
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	s = newRegisteredServer(t)
	for _, id := range ids {
		frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": id, "params": map[string]any{"page[size]": 100}})
		if strings.Contains(frame, "NETSECRET") {
			t.Fatalf("%s leaked a secret_key from an oversized body under read-only: %.600s", id, frame)
		}
		if !strings.Contains(frame, "_pp_truncated") || !strings.Contains(frame, "Site 0000") {
			t.Fatalf("%s: expected a bounded preview that still shows site names: %.600s", id, frame)
		}
	}
}

// TestReadOnlyOversizedMirrorOutputNeverLeaksSecrets covers the shell-out
// mirror (tail and every novel command): oversized JSON, NDJSON and CSV
// stdout, and a failing command whose stdout or stderr carries the body.
func TestReadOnlyOversizedMirrorOutputNeverLeaksSecrets(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	t.Setenv("DNSFILTER_CLI_PATH", os.Args[0])

	for _, mode := range []string{"json", "ndjson", "csv", "fail-stdout", "fail-stderr"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(fakeCLIModeEnv, mode)
			t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
			s := newRegisteredServer(t)
			frame := callToolWire(t, s, "tail", map[string]any{"resource": "networks", "follow": false})
			if !strings.Contains(frame, "NETSECRET") {
				t.Fatalf("default mode should return the raw body; the fixture does not exercise the path: %.300s", frame)
			}

			t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
			s = newRegisteredServer(t)
			frame = callToolWire(t, s, "tail", map[string]any{"resource": "networks", "follow": false})
			if strings.Contains(frame, "NETSECRET") {
				t.Fatalf("%s: mirrored output leaked a secret_key under read-only: %.600s", mode, frame)
			}
			// JSON output is redacted before the preview is cut, so the
			// preview still shows sites; CSV cannot be redacted by key and
			// is withheld by the funnel instead.
			if mode != "csv" && !strings.Contains(frame, "Site 0000") {
				t.Fatalf("%s: the preview lost the non-secret data (redaction ran after bounding?): %.600s", mode, frame)
			}
		})
	}
}

// TestReadOnlyOversizedStoreOutputNeverLeaksSecrets covers sql and search over
// a store written before secrets-never-persisted (rows inserted raw), with
// results near and over the budget.
func TestReadOnlyOversizedStoreOutputNeverLeaksSecrets(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	path, err := mcpDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for i := 0; i < 300; i++ {
		row, _ := json.Marshal(bigNetworkRow(i))
		id := fmt.Sprint(i)
		if _, err := db.DB().Exec(`INSERT INTO resources (id, resource_type, data, synced_at) VALUES (?, 'networks', ?, CURRENT_TIMESTAMP)`, id, string(row)); err != nil {
			t.Fatalf("seed resources: %v", err)
		}
		if _, err := db.DB().Exec(`INSERT INTO resources_fts (id, resource_type, content) VALUES (?, 'networks', ?)`, id, fmt.Sprintf("Site %04d Springfield", i)); err != nil {
			t.Fatalf("seed fts: %v", err)
		}
	}
	_ = db.Close()

	calls := []struct {
		tool string
		args map[string]any
	}{
		{"sql", map[string]any{"query": "SELECT data FROM resources WHERE resource_type = 'networks'"}},
		{"search", map[string]any{"query": "Springfield", "limit": 300}},
	}
	s := newRegisteredServer(t)
	for _, c := range calls {
		if frame := callToolWire(t, s, c.tool, c.args); !strings.Contains(frame, "NETSECRET") {
			t.Fatalf("default mode %s should see the seeded secrets; the fixture does not exercise the path: %.300s", c.tool, frame)
		}
	}
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	s = newRegisteredServer(t)
	for _, c := range calls {
		frame := callToolWire(t, s, c.tool, c.args)
		if strings.Contains(frame, "NETSECRET") {
			t.Fatalf("%s leaked a stored secret_key under read-only: %.600s", c.tool, frame)
		}
		if !strings.Contains(frame, "Site 0") {
			t.Fatalf("%s lost the non-secret data: %.300s", c.tool, frame)
		}
	}
}
