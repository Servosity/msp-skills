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
	if !strings.Contains(err.Error(), "Acme / HQ: rejected") {
		t.Fatalf("the error must name the network and why (MCP returns only stderr): %v", err)
	}
}

// TestFleetPushApplyExitsPartialOnUnreadableTarget: a target whose current
// settings cannot be read gets no change, so --apply must not exit 0.
func TestFleetPushApplyExitsPartialOnUnreadableTarget(t *testing.T) {
	testenv.Isolate(t)
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations":
			_, _ = w.Write([]byte(`[{"id":"1","name":"Acme","api":{"enabled":true}}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations/1/networks":
			_, _ = w.Write([]byte(`[{"id":"N_1","name":"HQ","organizationId":"1","productTypes":["appliance"],"tags":[]}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/networks/N_1/snmp":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errors":["forbidden"]}`))
		case r.Method == http.MethodPost:
			posts++
			_, _ = w.Write([]byte(`{"id":"b1"}`))
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
	if !errors.As(err, &ce) || ce.code != 6 || posts != 0 {
		t.Fatalf("unreadable target: err=%v posts=%d, want exit 6 and no batch\n%s", err, posts, out.String())
	}
}

// TestFleetPushApplyExitsPartialWhenOrgUnreadable: an org whose network list
// cannot be read had none of its networks evaluated, so --apply must not exit
// 0 with an empty result.
func TestFleetPushApplyExitsPartialWhenOrgUnreadable(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/organizations" {
			_, _ = w.Write([]byte(`[{"id":"1","name":"Acme","api":{"enabled":true}}]`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["forbidden"]}`))
	}))
	defer srv.Close()
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"fleet", "push", "--path", "/networks/{networkId}/snmp", "--body", `{"access":"none"}`, "--all-networks", "--apply", "--json"})
	err := cmd.Execute()
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 6 || !strings.Contains(err.Error(), "Acme:") {
		t.Fatalf("unreadable org under --apply: err=%v, want exit 6 naming the org\n%s", err, out.String())
	}
}

// TestPushNotCompletedCountsPastTheNameCap: the count is the true total even
// when only the first 20 targets are named.
func TestPushNotCompletedCountsPastTheNameCap(t *testing.T) {
	rows := make([]pushRow, 50)
	for i := range rows {
		rows[i] = pushRow{OrganizationName: "Acme", NetworkName: "Site", Action: "error", Error: "403"}
	}
	named, n := pushNotCompleted(rows, nil)
	if n != 50 || len(named) != 21 || named[20] != "..." {
		t.Fatalf("got n=%d named=%d last=%q, want 50, 21, ...", n, len(named), named[len(named)-1])
	}
}

// TestFleetPushRefusesBlankTargets: a blank --org is dropped by the org
// filter, so it must not satisfy the targeting guard and push to every org.
func TestFleetPushRefusesBlankTargets(t *testing.T) {
	testenv.Isolate(t)
	var batches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/organizations":
			_, _ = w.Write([]byte(`[{"id":"1","name":"Acme","api":{"enabled":true}}]`))
		case r.URL.Path == "/api/v1/organizations/1/networks":
			_, _ = w.Write([]byte(`[{"id":"N_1","name":"HQ","organizationId":"1","productTypes":["appliance"],"tags":[]}]`))
		case r.URL.Path == "/api/v1/networks/N_1/snmp":
			_, _ = w.Write([]byte(`{"access":"none"}`))
		case r.Method == http.MethodPost:
			batches++
			_, _ = w.Write([]byte(`{"id":"b1","status":{"completed":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("CISCO_MERAKI_BASE_URL", srv.URL+"/api/v1")
	t.Setenv("MERAKI_DASHBOARD_API_KEY", "test-key")
	for _, blank := range [][]string{{"--org", " "}, {"--org", " , "}, {"--network-tag", " "}} {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"fleet", "push", "--path", "/networks/{networkId}/snmp", "--body", `{"access":"community"}`, "--apply", "--json"}, blank...))
		err := cmd.Execute()
		var ce *cliError
		if !errors.As(err, &ce) || ce.code != 2 {
			t.Fatalf("fleet push %v: err=%v, want a usage error\n%s", blank, err, out.String())
		}
	}
	if batches != 0 {
		t.Fatalf("a blank target sent %d action batch(es)", batches)
	}
}

// TestFleetPushPreviewRedactsCurrentSecrets: a preview of an SSID PSK change
// printed the live PSK under current (Codex round 12).
func TestFleetPushPreviewRedactsCurrentSecrets(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/organizations":
			_, _ = w.Write([]byte(`[{"id":"1","name":"Acme","api":{"enabled":true}}]`))
		case "/api/v1/organizations/1/networks":
			_, _ = w.Write([]byte(`[{"id":"N_1","name":"HQ","organizationId":"1","productTypes":["wireless"],"tags":[]}]`))
		case "/api/v1/networks/N_1/wireless/ssids/0":
			_, _ = w.Write([]byte(`{"number":0,"name":"Staff","psk":"LIVE-PSK-7731","radiusServers":[{"host":"10.0.0.1","secret":"LIVE-XXXXXXXX"}]}`))
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
	cmd.SetArgs([]string{"fleet", "push", "--path", "/networks/{networkId}/wireless/ssids/0", "--body", `{"psk":"new-psk","radiusServers":[]}`, "--all-networks", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("preview: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "LIVE-") || !strings.Contains(out.String(), `"update"`) {
		t.Fatalf("preview leaked a live secret or did not diff:\n%s", out.String())
	}
}

// TestFleetPushPreviewRedactsWebhookURL: an HTTP server's credential is its
// plain "url" field, which no key-name rule catches (Codex round 13).
func TestFleetPushPreviewRedactsWebhookURL(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/organizations":
			_, _ = w.Write([]byte(`[{"id":"1","name":"Acme","api":{"enabled":true}}]`))
		case "/api/v1/organizations/1/networks":
			_, _ = w.Write([]byte(`[{"id":"N_1","name":"HQ","organizationId":"1","productTypes":["appliance"],"tags":[]}]`))
		case "/api/v1/networks/N_1/webhooks/httpServers/aHR0cHM6Ly9ob29rcw==":
			_, _ = w.Write([]byte(`{"id":"aHR0cHM6Ly9ob29rcw==","name":"Slack","url":"https://hooks.slack.com/services/LIVE-HOOK-7731"}`))
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
	cmd.SetArgs([]string{"fleet", "push", "--path", "/networks/{networkId}/webhooks/httpServers/aHR0cHM6Ly9ob29rcw==", "--body", `{"url":"https://hooks.slack.com/services/new"}`, "--all-networks", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("preview: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "LIVE-HOOK") || !strings.Contains(out.String(), `"update"`) {
		t.Fatalf("preview leaked the receiver URL or did not diff:\n%s", out.String())
	}
}
