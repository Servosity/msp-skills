// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"microsoft-graph-pp-cli/internal/cliutil/testenv"
)

// microsoft-graph handfix (teams-phone-agent-fields): --agent output of the
// Teams Phone reads keeps the number and the voice settings, list and get.
func TestTeamsPhoneAgentOutputKeepsTheirFields(t *testing.T) {
	testenv.Isolate(t)
	number := `{"id":"n1","telephoneNumber":"+12025550100","numberType":"callingPlan","activationState":"activated","assignmentStatus":"unassigned","assignmentCategory":"primary"}`
	user := `{"id":"u1","userPrincipalName":"a@contoso.example","isEnterpriseVoiceEnabled":true,"accountType":"user","telephoneNumbers":[{"telephoneNumber":"+12025550101"}],"effectivePolicyAssignments":[{"policyType":"teamsCallingPolicy"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/numberAssignments") && r.URL.Query().Get("$skip") != "":
			_, _ = w.Write([]byte(`{"value":[]}`)) // past the one number: $skip paging ends
		case strings.HasSuffix(r.URL.Path, "/numberAssignments"):
			_, _ = w.Write([]byte(`{"value":[` + number + `]}`))
		case strings.Contains(r.URL.Path, "/numberAssignments/"):
			_, _ = w.Write([]byte(number))
		case strings.HasSuffix(r.URL.Path, "/userConfigurations"):
			_, _ = w.Write([]byte(`{"value":[` + user + `]}`))
		case strings.Contains(r.URL.Path, "/userConfigurations/"):
			_, _ = w.Write([]byte(user))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "test-token-0000")
	t.Setenv("MICROSOFT_GRAPH_NO_LOCAL_DATA", "1")
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"phone-numbers", "list", "--agent"}, []string{"+12025550100", `"numberType"`, `"assignmentStatus"`}},
		{[]string{"phone-numbers", "get", "n1", "--agent"}, []string{"+12025550100", `"numberType"`}},
		{[]string{"teams-users", "list", "--agent"}, []string{"+12025550101", `"isEnterpriseVoiceEnabled"`, "teamsCallingPolicy"}},
		{[]string{"teams-users", "get", "u1", "--agent"}, []string{"+12025550101", `"isEnterpriseVoiceEnabled"`, "teamsCallingPolicy"}},
	} {
		out, err := runTranscripts(t, c.args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", c.args, err, out)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v dropped %s:\n%s", c.args, w, out)
			}
		}
	}
}
