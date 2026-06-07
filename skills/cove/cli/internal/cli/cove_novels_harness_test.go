// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Shared harness for behavioral tests of the hand-built Cove commands: a
// mock JSON-RPC server speaking the Login/EnumerateAccountStatistics
// contract, and an in-process command runner.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockFleet builds a JSON-RPC server that answers Login and
// EnumerateAccountStatistics with the given device rows.
func mockFleet(t *testing.T, devices []map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("bad request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch req.Method {
		case "Login":
			fmt.Fprint(w, `{"id":"jsonrpc","jsonrpc":"2.0","visa":"test-visa","result":{"result":{"Id":1,"PartnerId":9000,"EmailAddress":"api@test"}}}`)
		case "EnumerateAccountStatistics":
			// Single page: callers stop when len(rows) < page size.
			payload := map[string]any{
				"id": "jsonrpc", "jsonrpc": "2.0", "visa": "test-visa",
				"result": map[string]any{"result": devices},
			}
			_ = json.NewEncoder(w).Encode(payload)
		default:
			t.Errorf("unexpected JSON-RPC method %q", req.Method)
			fmt.Fprint(w, `{"id":"jsonrpc","jsonrpc":"2.0","error":{"code":-32601,"data":0,"message":"method not found"}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// settingsList renders column→value pairs in the wire shape Cove uses:
// a list of single-key objects.
func settingsList(kv map[string]string) []any {
	out := make([]any, 0, len(kv))
	for k, v := range kv {
		out = append(out, map[string]any{k: v})
	}
	return out
}

// runCoveCmd executes the full root command in-process with a clean flag set
// and returns stdout.
func runCoveCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	flags := &rootFlags{}
	root := newRootCmd(flags)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// fleetTestEnv points the CLI at the mock server with creds present and an
// isolated HOME so session caching cannot leak between tests.
func fleetTestEnv(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("COVE_BASE_URL", srv.URL)
	t.Setenv("COVE_USERNAME", "api@test")
	t.Setenv("COVE_PASSWORD", "secret")
	t.Setenv("COVE_PARTNER", "TestRoot")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
}
