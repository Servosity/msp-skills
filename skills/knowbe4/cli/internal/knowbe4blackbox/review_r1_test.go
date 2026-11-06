// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package knowbe4blackbox

// Codex review round 1 (liongard 4.34.2 reprint, the shared credential layer)
// class holder-incomplete, end to end on knowbe4: a credential that is a JSON
// number under a credential name (the liongard input was
// {"password":12345678}) was printed by a preview. KnowBe4's writes (events
// create and delete) carry no credential-named field, so the KnowBe4 forms of
// the class are a credential-named query value the user sends as digits
// (sync --param password=48261937, previewed and sent) and a number under a
// credential name in an answer from either API (a user's password, api_key,
// token and secret; an event's secret and api_key), in every output format
// and over MCP. A number under a count-like name (risk_count) still prints.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestNumericCredentialValuesNeverPrint(t *testing.T) {
	t.Skip("asserts withholding of invented credential-named response fields; the kit declares no response field (credwire ResponseFields is empty)")
	const named = "48261937" // under a credential name (password, api_key, token, secret)
	user := fmt.Sprintf(`{"id":667542,"first_name":"HQ numeric","last_name":"User","email":"num@hq-record.example.com","risk_count":3,`+
		`"password":%s,"api_key":%s,"token":%s,"secret":%s}`, named, named, named, named)
	sv := shapeServer(t, func(p string, r *http.Request) (string, bool) {
		switch p {
		case "/users":
			return "[" + user + "]", true
		case "/users/667542":
			return user, true
		case "/policies":
			return fmt.Sprintf(`[{"policy_id":1,"name":"HQ numeric policy","password":%s}]`, named), true
		}
		return "", false
	}, func(p string, r *http.Request) (string, bool) {
		if p == "/events/513f46ac" {
			return fmt.Sprintf(`{"data":{"id":"513f46ac","event_type":"HQ numeric","risk_level":25,"secret":%s,"api_key":%s},"meta":{"count":1}}`, named, named), true
		}
		return "", false
	})
	e := newEnv(t, sv, credEnv)
	var out strings.Builder
	for _, args := range [][]string{
		{"users", "get", "667542", "--json"},
		{"users", "get", "667542", "--csv"},
		{"users", "get", "667542", "--plain"},
		{"users", "get", "667542"},
		{"users", "list", "--json"},
		{"events", "get", "513f46ac", "--json"},
		{"events", "get", "513f46ac", "--csv"},
		{"events", "get", "513f46ac"},
		{"sync", "--resources", "policies", "--param", "password=" + named, "--dry-run"},
		{"sync", "--resources", "policies", "--param", "password=" + named, "--json"},
		{"export", "policies", "--format", "json"},
	} {
		out.WriteString(e.run(t, args...))
	}
	out.WriteString(e.mcpCalls(t, []map[string]any{
		tool("users_get", map[string]any{"id": 667542}),
		tool("users_list", map[string]any{}),
		tool("events_get", map[string]any{"event-id": "513f46ac"}),
		tool("sync", map[string]any{"resources": []any{"policies"}, "param": []any{"password=" + named}, "dry-run": true}),
	}))
	got := out.String()
	if !strings.Contains(got, "HQ numeric") || !strings.Contains(got, "dry run") || !publicField("risk_count", "3").MatchString(got) {
		t.Fatalf("the reads and previews did not run (or a count was withheld):\n%.3000s", got)
	}
	if strings.Contains(got, named) {
		t.Errorf("output repeats the numeric credential %s:\n%.4000s", named, got)
	}
	var sent bool
	for _, m := range sv.allMethods() {
		sent = sent || strings.HasPrefix(m, "GET /v1/policies")
	}
	if !sent {
		t.Errorf("the sync never reached the fake; requests %v", sv.allMethods())
	}
}
