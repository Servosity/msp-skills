// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cove-pp-cli/internal/cliutil/testenv"
)

// TestNovelFleetActivityHelpWires smoke-tests that the fleet activity command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelFleetActivityHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"fleet", "activity", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fleet activity --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "activity"} {
		if !strings.Contains(help, want) {
			t.Fatalf("fleet activity --help missing %q in output:\n%s", want, help)
		}
	}
}

// mockAudit answers Login and EnumerateAuditActions with the given rows and
// records the params the command sent.
func mockAudit(t *testing.T, rows []map[string]any, sent *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "Login":
			fmt.Fprint(w, `{"id":"jsonrpc","jsonrpc":"2.0","visa":"audit-visa-0123456789","result":{"result":{"PartnerId":9000}}}`)
		case "EnumerateAuditActions":
			*sent = req.Params
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "jsonrpc", "jsonrpc": "2.0", "visa": "audit-visa-0123456789", "result": map[string]any{"result": rows}})
		default:
			t.Errorf("unexpected method %q", req.Method)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFleetActivityDigestsTheAuditTrail(t *testing.T) {
	var sent map[string]any
	srv := mockAudit(t, []map[string]any{
		{"PartnerName": "Acme", "UserName": "alice", "EntityName": "Device", "OperationName": "Remove", "ResultName": "Success"},
		{"PartnerName": "Acme", "UserName": "alice", "EntityName": "Device", "OperationName": "Remove", "ResultName": "Failure"},
		{"PartnerName": "Acme", "UserName": "bob", "EntityName": "Device", "OperationName": "Remove", "ResultName": "Success"},
		{"PartnerName": "Globex", "UserName": "", "EntityName": "Partner", "OperationName": "Modify"},
	}, &sent)
	fleetTestEnv(t, srv)

	out, err := runCoveCmd(t, "fleet", "activity", "--since", "48h", "--json")
	if err != nil {
		t.Fatalf("fleet activity: %v\n%s", err, out)
	}
	var view activityView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if view.TotalActions != 4 || view.Truncated {
		t.Fatalf("total=%d truncated=%v", view.TotalActions, view.Truncated)
	}
	if len(view.Users) != 3 || view.Users[0].User != "alice" || view.Users[0].Actions != 2 || view.Users[0].Results["Failure"] != 1 {
		t.Fatalf("users = %+v", view.Users)
	}
	if view.Users[1].User != activityUnknown || view.Users[1].Partner != "Globex" { // ties sort by name
		t.Fatalf("a row with no user name was not counted under %q: %+v", activityUnknown, view.Users)
	}
	if view.Operations[0].Operation != "Remove" || view.Operations[0].Actions != 3 || view.Operations[0].Users != 2 {
		t.Fatalf("operations = %+v", view.Operations)
	}
	if view.Results["Success"] != 2 || view.Results["Failure"] != 1 || view.Results[activityUnknown] != 1 {
		t.Fatalf("results = %v", view.Results)
	}
	from, _ := sent["from"].(float64)
	to, _ := sent["to"].(float64)
	if to-from != 48*3600 || sent["includeAllSubPartners"] != true || sent["countLimit"] != float64(1000) {
		t.Fatalf("params sent = %v", sent)
	}
}

func TestFleetActivityFlagsATruncatedWindow(t *testing.T) {
	var sent map[string]any
	rows := []map[string]any{
		{"UserName": "a", "EntityName": "Device", "OperationName": "Add", "ResultName": "Success"},
		{"UserName": "b", "EntityName": "Device", "OperationName": "Add", "ResultName": "Success"},
	}
	srv := mockAudit(t, rows, &sent)
	fleetTestEnv(t, srv)
	out, err := runCoveCmd(t, "fleet", "activity", "--limit", "2", "--json")
	if err != nil {
		t.Fatalf("fleet activity: %v\n%s", err, out)
	}
	var view activityView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Truncated || !strings.Contains(view.Note, "limit") {
		t.Fatalf("a window cut at --limit was not flagged: %+v", view)
	}
}

func TestFleetActivityRefusesUnreadableFlags(t *testing.T) {
	testenv.Isolate(t)
	for _, args := range [][]string{
		{"fleet", "activity", "--since", "0"},
		{"fleet", "activity", "--since", "400d"},
		{"fleet", "activity", "--since", "soon"},
		{"fleet", "activity", "--limit", "0"},
	} {
		if out, err := runCoveCmd(t, args...); err == nil {
			t.Errorf("%v was accepted:\n%s", args, out)
		}
	}
}

func TestFleetActivityMissingCredentialsIsAnAuthError(t *testing.T) {
	testenv.Isolate(t)
	for _, k := range []string{"COVE_USERNAME", "COVE_PASSWORD", "COVE_PARTNER"} {
		t.Setenv(k, "")
	}
	_, err := runCoveCmd(t, "fleet", "activity")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 4 {
		t.Fatalf("missing credentials: err=%v, want exit code 4", err)
	}
}
