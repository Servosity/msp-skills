// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"domain-cert-tracker-pp-cli/internal/client"
	"domain-cert-tracker-pp-cli/internal/cliutil"
	"domain-cert-tracker-pp-cli/internal/config"
	"domain-cert-tracker-pp-cli/internal/netguard"
)

func trackerHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("DOMAIN_CERT_TRACKER_HOME", home)
	return home
}

func TestMCPChildCannotUseCLIOnlyGates(t *testing.T) {
	home := trackerHome(t)
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	csv := filepath.Join(home, "clients.csv")
	if err := os.WriteFile(csv, []byte("client,domain,host\nAcme,example.com,www.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--home", home, "tls", "probe", "example.com:8443", "--allow-port", "8443"},
		{"--home", home, "sweep", "--allow-port", "8443", "--confirm"}, // past the confirm gate, to the port refusal
		{"--home", home, "clients", "import", csv},
	} {
		_, _, err := runRootArgs(t, args...)
		if err == nil || !strings.Contains(err.Error(), "command line only") {
			t.Errorf("%v under MCP: err = %v, want a command-line-only refusal", args, err)
		}
	}
}

func TestTLSProbeRefusesBeforeConnecting(t *testing.T) {
	home := trackerHome(t)
	for _, target := range []string{"example.com:8443", "10.1.2.3", "169.254.169.254", "[::1]", "100.64.0.1"} {
		_, _, err := runRootArgs(t, "--home", home, "tls", "probe", target)
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Errorf("tls probe %s: err = %v, want refused", target, err)
		}
	}
}

func TestClientHookGuardsGeneratedClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"objectClassName":"domain","ldhName":"example.com"}`))
	}))
	defer srv.Close()
	// On the operator's own command line the config-file base_url is
	// reached even on loopback.
	t.Setenv("DOMAIN_CERT_TRACKER_BASE_URL", "")
	c := client.New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.HTTPClient.Get(srv.URL + "/domain/example.com"); err != nil {
		t.Fatalf("command line refused its own config-file base URL: %v", err)
	}
	// Under MCP (a mirrored child or the server process itself) a config-file
	// base_url grants nothing; only DOMAIN_CERT_TRACKER_BASE_URL is reached.
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.HTTPClient.Get(srv.URL + "/domain/example.com"); !errors.Is(err, netguard.ErrRefused) {
		t.Fatalf("a config-file base_url reached a loopback origin under MCP: %v", err)
	}
	t.Setenv("DOMAIN_CERT_TRACKER_BASE_URL", srv.URL)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.HTTPClient.Get(srv.URL + "/domain/example.com"); err != nil {
		t.Fatalf("generated client refused the operator's environment base URL: %v", err)
	}
	// Any other origin, such as a link a response points at, is still refused.
	c = client.New(&config.Config{BaseURL: "https://rdap.example.test"}, 5*time.Second, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.HTTPClient.Get(srv.URL + "/domain/example.com"); !errors.Is(err, netguard.ErrRefused) {
		t.Fatalf("generated client reached a loopback origin it was not configured with: %v", err)
	}
	// End to end: the operator's own override is reached as configured.
	home := trackerHome(t)
	t.Setenv("DOMAIN_CERT_TRACKER_BASE_URL", srv.URL)
	if _, _, err := runRootArgs(t, "--home", home, "--no-cache", "domain", "example.com", "--json"); err != nil {
		t.Fatalf("domain lookup against the operator's base URL: %v", err)
	}
}

func TestClientsInventoryLifecycle(t *testing.T) {
	home := trackerHome(t)
	run := func(args ...string) (string, error) {
		out, _, err := runRootArgs(t, append([]string{"--home", home, "--json"}, args...)...)
		return out, err
	}
	if _, err := run("clients", "set", "Acme Dental", "--domain", "Example.COM", "--host", "www.example.com,api.example.com:8443", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("clients", "set", "Bad", "--host", "192.168.1.10", "--yes"); err == nil {
		t.Fatal("a private IP literal was accepted into the inventory")
	}
	out, err := run("clients", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"example.com"`) || !strings.Contains(out, `api.example.com:8443`) {
		t.Fatalf("list: %s", out)
	}
	if _, err := run("clients", "remove", "Acme Dental"); err == nil {
		t.Fatal("remove without --yes/--confirm deleted inventory")
	}
	if out, _ := run("clients", "list"); !strings.Contains(out, "Acme Dental") {
		t.Fatal("unconfirmed remove changed the inventory")
	}
	if _, err := run("clients", "remove", "Acme Dental", "--host", "api.example.com:8443", "--confirm"); err != nil {
		t.Fatal(err)
	}
	if out, _ := run("clients", "list"); strings.Contains(out, "api.example.com") || !strings.Contains(out, "www.example.com") {
		t.Fatalf("target remove: %s", out)
	}
	if _, err := run("clients", "remove", "Acme Dental", "--whole-client", "--yes"); err != nil {
		t.Fatal(err)
	}
	if out, _ := run("clients", "list"); strings.Contains(out, "Acme Dental") {
		t.Fatalf("client remove: %s", out)
	}
}

func TestClientsImportCSVAndJSON(t *testing.T) {
	home := trackerHome(t)
	csvPath := filepath.Join(home, "c.csv")
	_ = os.WriteFile(csvPath, []byte("client,domain,host\nAcme,acme.example,www.acme.example\nAcme,,shop.acme.example\nBeta,beta.example,\n"), 0o600)
	jsonPath := filepath.Join(home, "c.json")
	_ = os.WriteFile(jsonPath, []byte(`[{"name":"Gamma","domains":["gamma.example"],"hosts":["gamma.example"]}]`), 0o600)
	for _, p := range []string{csvPath, jsonPath} {
		if _, _, err := runRootArgs(t, "--home", home, "clients", "import", p); err != nil {
			t.Fatalf("import %s: %v", p, err)
		}
	}
	out, _, err := runRootArgs(t, "--home", home, "--json", "clients", "list")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Results struct {
			Count int `json:"count"`
		} `json:"results"`
		Count int `json:"count"`
	}
	_ = json.Unmarshal([]byte(out), &got)
	if got.Count != 3 && got.Results.Count != 3 {
		t.Fatalf("clients after import: %s", out)
	}
	bad := filepath.Join(home, "bad.csv")
	_ = os.WriteFile(bad, []byte("Acme,10.0.0.1/24\n"), 0o600)
	if _, _, err := runRootArgs(t, "--home", home, "clients", "import", bad); err == nil {
		t.Fatal("an invalid domain was imported")
	}
}

func TestEventsListEmptyStore(t *testing.T) {
	home := trackerHome(t)
	out, _, err := runRootArgs(t, "--home", home, "--json", "events", "list", "--since", "0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"events":[]`) && !strings.Contains(out, `"events": []`) {
		t.Fatalf("empty store events: %s", out)
	}
	if _, _, err := runRootArgs(t, "--home", home, "events", "list", "--type", "nope"); err == nil {
		t.Fatal("unknown event type accepted")
	}
}

func TestNoLocalDataRefusesSweep(t *testing.T) {
	home := trackerHome(t)
	t.Setenv("DOMAIN_CERT_TRACKER_NO_LOCAL_DATA", "1")
	_, _, err := runRootArgs(t, "--home", home, "sweep")
	if err == nil || !strings.Contains(err.Error(), "DOMAIN_CERT_TRACKER_NO_LOCAL_DATA") {
		t.Fatalf("sweep under NO_LOCAL_DATA: %v, want a refusal", err)
	}
}

func TestClientsRemoveBlankSelectorDoesNotWiden(t *testing.T) {
	home := trackerHome(t)
	if _, _, err := runRootArgs(t, "--home", home, "clients", "set", "Acme", "--domain", "example.com", "--host", "www.example.com", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRootArgs(t, "--home", home, "clients", "remove", "Acme", "--domain", " ", "--confirm"); err == nil {
		t.Fatal("a blank --domain selector removed something")
	}
	// The MCP layer drops an empty selector ("domain": [], "", [null]), so
	// clients_remove {"client":"Acme","domain":[],"confirm":true} arrives as
	// this argv. Only --whole-client may delete the client.
	if _, _, err := runRootArgs(t, "--home", home, "clients", "remove", "Acme", "--confirm"); err == nil {
		t.Fatal("a remove with its selector dropped deleted the whole client")
	}
	if _, _, err := runRootArgs(t, "--home", home, "clients", "remove", "Acme", "--whole-client", "--domain", "example.com", "--confirm"); err == nil {
		t.Fatal("--whole-client combined with a selector was accepted")
	}
	out, _, _ := runRootArgs(t, "--home", home, "--json", "clients", "list")
	if !strings.Contains(out, "www.example.com") || !strings.Contains(out, `"example.com"`) {
		t.Fatalf("blank selector widened the removal: %s", out)
	}
}

// Generated clients reach a non-public address only at the exact origin of
// their base URL, and only when that origin is the operator's
// DOMAIN_CERT_TRACKER_BASE_URL (or, on the operator's own command line, the
// config-file base_url); under MCP a config-file value, and anywhere an empty
// or unparsable one or another client's origin, grants nothing.
func TestTrackerClientGuardHonorsOnlyTheOperatorBaseURL(t *testing.T) {
	ctx := context.Background()
	defer func() { mcpServerProcess = false }()
	for _, c := range []struct {
		env, base  string
		mcp, child bool
		reach      bool
	}{
		{"", "http://127.0.0.1:65530", false, false, true}, // config file, command line
		{"", "http://127.0.0.1:65530", true, false, false}, // config file, MCP server process
		{"", "http://127.0.0.1:65530", false, true, false}, // config file, MCP child
		{"http://127.0.0.1:65530", "http://127.0.0.1:65530", true, false, true},
		{"http://127.0.0.1:65530", "http://127.0.0.1:65530", false, true, true},
	} {
		t.Setenv("DOMAIN_CERT_TRACKER_BASE_URL", c.env)
		t.Setenv(cliutil.MCPChildEnvVar, map[bool]string{true: "1"}[c.child])
		mcpServerProcess = c.mcp
		_, err := trackerClientGuard(c.base).DialContext(ctx, "tcp", "127.0.0.1:65530")
		if refused := errors.Is(err, netguard.ErrRefused); refused == c.reach {
			t.Errorf("env %q base %q mcp %v child %v: refused=%v (err %v), want reachable=%v", c.env, c.base, c.mcp, c.child, refused, err, c.reach)
		}
	}
	mcpServerProcess = false
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	for _, c := range []struct {
		env, base string
		reach     bool
	}{
		{"", "", false},
		{"", "http://127.0.0.1:65530", false}, // config file only, under MCP
		{"http://127.0.0.1:65530", "http://127.0.0.1:65530", true},
		{"http://127.0.0.1:65530/rdap/", "http://127.0.0.1:65530/rdap", true},
		{"http://127.0.0.1:65530", "https://crt.sh", false}, // another client
		{"http://127.0.0.1:65530", "not a url", false},
		{"http://127.0.0.1:65531", "http://127.0.0.1:65531", false},
		{"http://127.0.0.1:65530", "http://127.0.0.1:65531", false},
	} {
		t.Setenv("DOMAIN_CERT_TRACKER_BASE_URL", c.env)
		_, err := trackerClientGuard(c.base).DialContext(ctx, "tcp", "127.0.0.1:65530")
		refused := errors.Is(err, netguard.ErrRefused)
		if refused == c.reach {
			t.Errorf("env %q base %q: dial 127.0.0.1:65530 refused=%v (err %v), want reachable=%v", c.env, c.base, refused, err, c.reach)
		}
	}
}
