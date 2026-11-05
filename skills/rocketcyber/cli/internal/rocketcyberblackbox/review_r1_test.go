// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rocketcyberblackbox

// Codex review round 1 (liongard 4.34.2 reprint, the shared credential layer)
// class holder-incomplete, end to end on rocketcyber: a credential that is a
// JSON number (the liongard input was {"password":12345678}) was printed by a
// preview. Credential leak contract section 1: no exemption for type, numbers
// count. The held credential here is an all-digit API token
// (ROCKETCYBER_API_TOKEN=48261937); the fake repeats it as a JSON number
// under credential names in its answers (an agent's password, api_key, token
// and secret; a suppression rule's secret and api_key; an app's password),
// and the user types it as a query value (sync --param password=48261937,
// previewed and sent; MCP too). It reaches no output, in any format or over
// MCP.

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestNumericCredentialValuesNeverPrint(t *testing.T) {
	const named = "48261937" // under a credential name (password, api_key, token, secret)
	agent := fmt.Sprintf(`{"id":"asda12341aasdafg1ag1llk412a","hostname":"HQ numeric agent","connectivity":"online","token_count":3,`+
		`"password":%s,"api_key":%s,"token":%s,"secret":%s}`, named, named, named, named)
	rule := fmt.Sprintf(`{"ruleId":42,"ruleName":"HQ numeric rule","status":"active","secret":%s,"api_key":%s}`, named, named)
	list := func(items ...string) string {
		return fmt.Sprintf(`{"totalCount":%d,"currentPage":1,"totalPages":1,"data":[%s]}`, len(items), strings.Join(items, ","))
	}
	sv := shapeServer(t, func(p string, r *http.Request) (string, bool) {
		switch p {
		case "/agents":
			return list(agent), true
		case "/suppression/rules":
			return list(rule), true
		case "/suppression/rule/42":
			return rule, true
		case "/apps":
			return list(fmt.Sprintf(`{"id":60,"name":"HQ numeric app","status":"active","password":%s}`, named)), true
		}
		return "", false
	})
	e := newEnv(t, sv, credEnv)
	e.env = append(withoutEnv(e.env, "ROCKETCYBER_API_TOKEN"), "ROCKETCYBER_API_TOKEN="+named)
	var out strings.Builder
	for _, args := range [][]string{
		{"suppression", "rule", "42", "--json"},
		{"suppression", "rule", "42", "--csv"},
		{"suppression", "rule", "42", "--plain"},
		{"suppression", "rule", "42"},
		{"suppression", "rules", "--json"},
		{"agents", "--json"},
		{"agents", "--csv"},
		{"agents"},
		{"sync", "--resources", "apps", "--param", "password=" + named, "--dry-run"},
		{"sync", "--resources", "apps", "--param", "password=" + named, "--json"},
		{"export", "apps", "--format", "json"},
	} {
		out.WriteString(e.run(t, args...))
	}
	out.WriteString(e.mcpCalls(t, []map[string]any{
		tool("suppression_rule", map[string]any{"ruleId": 42}),
		tool("agents_list", map[string]any{}),
		tool("suppression_rules", map[string]any{}),
		tool("sync", map[string]any{"resources": []any{"apps"}, "param": []any{"password=" + named}, "dry-run": true}),
	}))
	got := out.String()
	if n, _ := sv.sawKey(named); n == 0 || !strings.Contains(got, kitNotice("stdout")) {
		t.Fatalf("the reads did not run with the numeric token (or the kit withheld nothing):\n%.3000s", got)
	}
	if strings.Contains(got, named) {
		t.Errorf("output repeats the numeric credential %s:\n%.4000s", named, got)
	}
	var sent bool
	for _, m := range sv.allMethods() {
		sent = sent || strings.HasPrefix(m, "GET /v3/apps")
	}
	if !sent {
		t.Errorf("the sync never reached the fake; requests %v", sv.allMethods())
	}
}
