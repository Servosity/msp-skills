// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"microsoft-graph-pp-cli/internal/cliutil/testenv"
)

// TestNovelAppsConsentHelpWires smoke-tests that the apps consent command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelAppsConsentHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"apps", "consent", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("apps consent --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "consent"} {
		if !strings.Contains(help, want) {
			t.Fatalf("apps consent --help missing %q in output:\n%s", want, help)
		}
	}
}

// skiptokenPrefix makes the fakes' paging cursors opaque, as Graph's are: a
// $skiptoken value is held like any credential-named parameter, so a bare
// page number would be withheld wherever it later appears as a word.
const skiptokenPrefix = "RFNwdAIAAQAAAB8xOk-page"

// pagedGraph serves fixed collections one object per page through
// @odata.nextLink, as Graph pages any collection, and answers 403 for refused paths.
type pagedGraph struct {
	collections map[string][]any
	refuse      map[string]bool
}

func (g pagedGraph) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1.0")
	if g.refuse[path] {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"Authorization_RequestDenied","message":"Insufficient privileges"}}`))
		return
	}
	items := g.collections[path]
	q := r.URL.Query()
	page, _ := strconv.Atoi(strings.TrimPrefix(q.Get("$skiptoken"), skiptokenPrefix))
	body := map[string]any{"value": []any{}}
	if page < len(items) {
		body["value"] = items[page : page+1]
	}
	if page+1 < len(items) {
		q.Set("$skiptoken", skiptokenPrefix+strconv.Itoa(page+1))
		body["@odata.nextLink"] = "http://" + r.Host + r.URL.Path + "?" + q.Encode()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// Codex r1 class incomplete-read-as-complete: apps consent reads every page of
// every collection ("List appRoleAssignments granted for a service principal"
// pages with @odata.nextLink like any Graph collection), so an escalation
// permission on a later page is flagged, and an app whose permissions could
// not be read is named in the result instead of reading as having none.
func TestAppsConsentReadsEveryPage(t *testing.T) {
	testenv.Isolate(t)
	sp := func(id, owner string) map[string]any {
		return map[string]any{"id": id, "appId": "app-" + id, "displayName": "App " + id, "appOwnerOrganizationId": owner, "accountEnabled": true}
	}
	role := func(principal, appRole string) map[string]any {
		return map[string]any{"id": principal + appRole[:4], "principalId": principal, "resourceDisplayName": "Microsoft Graph", "appRoleId": appRole}
	}
	srv := httptest.NewServer(pagedGraph{
		collections: map[string][]any{
			"/servicePrincipals": {sp("ms1", "f8cdef31-a31e-4b4a-93e4-5f571e91255a"), sp("sp1", "vendor-tenant"), sp("sp2", "vendor-tenant")},
			"/servicePrincipals/sp1/appRoleAssignments": {
				role("sp1", "df021288-bdef-4463-88db-98f22de89214"), // User.Read.All
				role("sp1", "9e3f62cf-ca93-4989-b6ce-bf83c28f9fe8"), // RoleManagement.ReadWrite.Directory, page 2
			},
			"/organization": {map[string]any{"id": "home-tenant"}},
		},
		refuse: map[string]bool{"/servicePrincipals/sp2/appRoleAssignments": true},
	})
	t.Cleanup(srv.Close)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "appsConsentToken-7b2e")
	cmd := RootCmd()
	cmd.SetArgs([]string{"apps", "consent", "--json"})
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("apps consent: %v\n%s%s", err, out.String(), errb.String())
	}
	var res struct {
		Apps []struct {
			ID        string   `json:"id"`
			RiskFlags []string `json:"riskFlags"`
		} `json:"apps"`
		Note string `json:"note"`
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	flags := map[string]string{}
	for _, a := range res.Apps {
		flags[a.ID] = strings.Join(a.RiskFlags, ",")
	}
	if !strings.Contains(flags["sp1"], "privilege-escalation") {
		t.Errorf("sp1 flags %q (apps %v): the escalation permission on page 2 was missed", flags["sp1"], flags)
	}
	if !strings.Contains(res.Note, "sp2") {
		t.Errorf("note %q does not name sp2, whose application permissions could not be read", res.Note)
	}
}
