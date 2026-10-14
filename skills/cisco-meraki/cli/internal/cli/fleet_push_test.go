// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cisco-meraki-pp-cli/internal/cliutil/testenv"
)

// TestNovelFleetPushHelpWires smoke-tests that the fleet push command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelFleetPushHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"fleet", "push", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fleet push --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "push"} {
		if !strings.Contains(help, want) {
			t.Fatalf("fleet push --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestFleetPushApplyExitsPartialOnRejectedBatch: a rejected action batch must
// not read as success. `fleet push --apply` exits 6 (partial failure) so
// automation and the MCP tool see that the change did not land.
func TestFleetPushApplyExitsPartialOnRejectedBatch(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations":
			_, _ = w.Write([]byte(`[{"id":"1","name":"Acme","api":{"enabled":true}}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations/1/networks":
			_, _ = w.Write([]byte(`[{"id":"N_1","name":"HQ","organizationId":"1","productTypes":["appliance"],"tags":[]}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/networks/N_1/snmp":
			_, _ = w.Write([]byte(`{"access":"none"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/organizations/1/actionBatches":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":["Action batch rejected"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"fleet", "push", "--path", "/networks/{networkId}/snmp", "--body", `{"access":"community"}`, "--all-networks", "--apply", "--json"})
	err := cmd.Execute()
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 6 {
		t.Fatalf("fleet push --apply with a rejected batch: err=%v, want exit 6\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "rejected") {
		t.Fatalf("output does not name the rejected batch:\n%s", out.String())
	}
}
