// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rewst-pp-cli/internal/cliutil/testenv"
)

func TestOrgsFromAnswerListsManagedOrgsSorted(t *testing.T) {
	data := json.RawMessage(`{"userOrganization":{"id":"msp-1","name":"Acme MSP","isMsp":true,"managingOrgId":null,
	  "managedAndSubOrgs":[{"id":"o-2","name":"zeta","managingOrgId":"msp-1","isEnabled":true,"isDeleted":false},
	  {"id":"o-1","name":"Alpha","managingOrgId":"msp-1","isEnabled":false,"isDeleted":false},
	  {"id":"o-3","name":"Gone","managingOrgId":"msp-1","isEnabled":true,"isDeleted":true},
	  {"id":"msp-1","name":"Acme MSP","managingOrgId":null,"isEnabled":true,"isDeleted":false},
	  {"id":42,"name":"numeric","managingOrgId":"msp-1"}]}}`)
	v, err := orgsFromAnswer(data, false)
	if err != nil {
		t.Fatal(err)
	}
	if v.Org.ID != "msp-1" || v.IsMSP == nil || !*v.IsMSP {
		t.Fatalf("org = %+v", v.Org)
	}
	var ids []string
	for _, o := range v.ManagedOrgs {
		ids = append(ids, o.ID)
	}
	if got := strings.Join(ids, ","); got != "o-1,42,o-2" || v.ManagedCount != 3 || v.DisabledCount != 1 {
		t.Fatalf("managed = %s (count %d, disabled %d), want o-1,42,o-2 / 3 / 1 (self and deleted excluded)", got, v.ManagedCount, v.DisabledCount)
	}
	if v, err = orgsFromAnswer(data, true); err != nil || v.ManagedCount != 4 {
		t.Fatalf("--include-deleted: count %d, err %v", v.ManagedCount, err)
	}
}

// Missing input is an error, never an empty directory: no organization, no
// managed list, an empty or undecodable answer.
func TestOrgsFromAnswerRefusesMissingInput(t *testing.T) {
	for name, body := range map[string]string{
		"no organization":         `{"userOrganization":null}`,
		"organization without id": `{"userOrganization":{"name":"x","managedAndSubOrgs":[]}}`,
		"no managed list":         `{"userOrganization":{"id":"msp-1","name":"x"}}`,
		"empty":                   ``,
		"not json":                `<html>`,
		"wrong type":              `{"userOrganization":{"id":"msp-1","managedAndSubOrgs":{"id":"o-1"}}}`,
	} {
		if _, err := orgsFromAnswer(json.RawMessage(body), false); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if v, err := orgsFromAnswer(json.RawMessage(`{"userOrganization":{"id":"msp-1","name":"x","managedAndSubOrgs":[]}}`), false); err != nil || v.ManagedCount != 0 || v.ManagedOrgs == nil {
		t.Fatalf("an org that manages none must answer an empty (non-null) list: %+v %v", v, err)
	}
}

// The command sends one GraphQL query with the Bearer token to POST /graphql
// and fails (non-zero) on Rewst's AUTH_ERR answer instead of listing nothing.
func TestOrgsCommandEndToEnd(t *testing.T) {
	const token = "rwst-orgs-token-3f9c1a7e"
	for _, tc := range []struct {
		name, body string
		fail       bool
	}{
		{"healthy", `{"data":{"userOrganization":{"id":"msp-1","name":"Acme MSP","isMsp":true,"managedAndSubOrgs":[{"id":"o-1","name":"Alpha","managingOrgId":"msp-1","isEnabled":true,"isDeleted":false}]}}}`, false},
		{"not authorized", `{"errors":[{"message":"Not Authorized - User not found - userOrganization","extensions":{"code":"AUTH_ERR"}}],"data":{"userOrganization":null}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sawQuery bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				if r.Method == http.MethodPost && r.URL.Path == "/graphql" && r.Header.Get("Authorization") == "Bearer "+token &&
					strings.Contains(string(b), "managedAndSubOrgs") && !strings.Contains(string(b), "mutation") {
					sawQuery = true
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			testenv.Isolate(t)
			t.Setenv("REWST_BASE_URL", srv.URL)
			t.Setenv("REWST_API_TOKEN", token)
			out, errOut, err := runRootArgs(t, "orgs", "--json")
			if !sawQuery {
				t.Fatalf("orgs never sent the GraphQL query with the Bearer token to POST /graphql")
			}
			if tc.fail {
				if err == nil {
					t.Fatalf("an AUTH_ERR answer exited 0:\n%s", out)
				}
				return
			}
			if err != nil || !strings.Contains(out, `"o-1"`) || !strings.Contains(out, `"managed_count": 1`) {
				t.Fatalf("err %v\n%s\n%s", err, out, errOut)
			}
		})
	}
}
