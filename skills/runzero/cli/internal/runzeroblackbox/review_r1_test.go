// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

// Codex review round 1 (liongard 4.34.2 reprint, the shared credential layer)
// class holder-incomplete, end to end on runZero: a credential that is a JSON
// number under a credential name (the liongard input was
// {"password":12345678}) was printed by a preview. The runZero forms of the
// class are a credential-named query value the user sends as digits (sync
// --param password=48261937, previewed and sent), a number under a credential
// name in an answer (a user's password, api_key, token and secret; an
// API key's token; an organization's export_token and download_token; an
// asset's api_key), in every output format and over MCP, and the API key
// itself as digits (RUNZERO_API_KEY), echoed by the agents answer, which the
// credential kit withholds whole. A number under a count-like name
// (usage_today, counter, service_count) still prints.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestNumericCredentialValuesNeverPrint(t *testing.T) {
	t.Parallel() // its own fake runZero and home

	const named = "48261937"      // under a credential name (password, api_key, token, secret, export_token, download_token)
	const envKey = "731904625518" // RUNZERO_API_KEY made of digits, echoed as a number
	user := fmt.Sprintf(`{"id":"user-num","email":"num@hq-record.example.com","first_name":"HQ numeric User",`+
		`"login_failures":3,"password":%s,"api_key":%s,"token":%s,"secret":%s}`, named, named, named, named)
	sv := shapeServer(t, func(p string, r *http.Request) (string, bool) {
		switch p {
		case "/account/users":
			return `[` + user + `]`, true
		case "/account/keys":
			return fmt.Sprintf(`[{"id":%q,"comment":"HQ numeric key","usage_today":4242,"counter":7,"token":%s}]`, bbKeyID, named), true
		case "/account/orgs":
			return fmt.Sprintf(`[{"id":%q,"name":"HQ numeric org","export_token":%s,"download_token":%s,"export_token_counter":3}]`, bbOrgID, named, named), true
		case "/org/assets":
			return fmt.Sprintf(`[{"id":%q,"names":["HQ numeric asset"],"addresses":[%q],"service_count":2,"api_key":%s}]`, bbAssetID, bbAddr, named), true
		case "/org/agents":
			return fmt.Sprintf(`[{"id":%q,"name":"HQ numeric agent","password":%s,"echoed":%s}]`, bbAgentID, named, envKey), true
		}
		return "", false
	})
	e := newEnv(t, sv, credEnv)
	e.env = append(withoutEnv(e.env, "RUNZERO_API_KEY"), "RUNZERO_API_KEY="+envKey)
	var out strings.Builder
	for _, args := range [][]string{
		{"account", "get-users", "--json"},
		{"account", "get-users", "--csv"},
		{"account", "get-users", "--plain"},
		{"account", "get-users"},
		{"account", "get-keys", "--json"},
		{"account", "get-keys", "--csv"},
		{"account", "get-keys"},
		{"account", "get-organizations", "--json"},
		{"org", "get-assets", "--json"},
		{"org", "get-agents", "--json"},
		{"sync", "--resources", "org-assets", "--param", "password=" + named, "--dry-run"},
		{"sync", "--resources", "org-assets", "--param", "password=" + named, "--json"},
		{"export", "org", "--format", "json"},
	} {
		out.WriteString(e.run(t, args...))
	}
	out.WriteString(e.mcpCalls(t, []map[string]any{
		read("account.get-users", nil),
		read("account.get-keys", nil),
		read("account.get-organizations", nil),
		tool("sync", map[string]any{"resources": []any{"org-assets"}, "param": []any{"password=" + named}, "dry-run": true}),
	}))
	got := out.String()
	if !strings.Contains(got, "HQ numeric") || !strings.Contains(got, "dry run") || !publicField("usage_today", "4242").MatchString(got) ||
		!publicField("service_count", "2").MatchString(got) || !strings.Contains(got, kitNotice("stdout")) {
		t.Fatalf("the reads and previews did not run (or a count was withheld):\n%.3000s", got)
	}
	for _, v := range []string{named, envKey} {
		if strings.Contains(got, v) {
			t.Errorf("output repeats the numeric credential %s:\n%.4000s", v, got)
		}
	}
	var sent bool
	for _, m := range sv.allMethods() {
		sent = sent || strings.HasPrefix(m, "GET /org/assets")
	}
	if !sent {
		t.Errorf("the sync never reached the fake; requests %v", sv.allMethods())
	}
	if n, _ := sv.sawKey(envKey); n == 0 {
		t.Errorf("no request carried the numeric key; the case did not run")
	}
}
