// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package threecxblackbox

// Codex round 1's classes, each from its failing input, through the built
// binaries.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Every member of a duplicate key is held: encoding/json keeps only the last, so
// a Concealed record under a name no declaration can see lost the value the echo
// repeats.
func TestDuplicateMemberEchoNeverPrints(t *testing.T) {
	t.Parallel()
	const value = "DupVal-Zq41Wx93"
	for _, first := range []string{
		`"Mystery":{"Concealed":false,"Value":"` + value + `"},"Mystery":null`,
		`"ServiceAccountCredentials":{"SecretKey":{"Concealed":false,"Value":"` + value + `"}},"ServiceAccountCredentials":null`,
	} {
		_, e := honestEnv(t, map[string]string{"/SystemStatus": `{` + first + `,"Echo":"` + value + `"}`})
		if out := e.run(t, "system-status", "get", "--json", "--data-source", "live"); strings.Contains(out, value) {
			t.Fatalf("a duplicate member's value printed for %s:\n%.800s", first, out)
		}
	}
}

// A PIN the operator supplies is one value they chose to send, so its preview
// shows nothing of it, as a string, a number or a flag.
func TestOperatorPINIsHeld(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	for _, pin := range []any{"7391", 7391} {
		out := e.mcpCalls(t, []map[string]any{{"name": "threecx_execute", "arguments": map[string]any{
			"endpoint_id": "users-id.update-user", "params": map[string]any{"Id": "2", "VMPIN": pin}}}})
		if strings.Contains(out, "7391") {
			t.Fatalf("an MCP preview printed the PIN (%T):\n%.800s", pin, out)
		}
	}
	// A PIN under a name no declaration lists is the shared walker's.
	out := e.mcpCalls(t, []map[string]any{{"name": "threecx_execute", "arguments": map[string]any{
		"endpoint_id": "users-id.update-user", "params": map[string]any{"Id": "2", "Settings": map[string]any{"CustomPin": "8214"}}}}})
	if strings.Contains(out, "8214") {
		t.Fatalf("an MCP preview printed a PIN no declaration names:\n%.800s", out)
	}
	if out := e.run(t, "users-id", "update-user", "2", "--vmpin", "7391", "--dry-run"); strings.Contains(out, "7391") {
		t.Fatalf("a CLI preview printed the PIN:\n%.800s", out)
	}
}

// A listing's own PINs stay out by name and do not take the listing with them.
func TestListingPINsAreReplacedNotHeld(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{"/Users": `{"value":[{"Number":"1001","VMPIN":"1001","PinNumber":7391,"FirstName":"Rec"}]}`})
	out := e.run(t, "users", "list", "--json", "--data-source", "live")
	if strings.Contains(out, "7391") || strings.Contains(out, bbWithheld) || !strings.Contains(out, "Rec") {
		t.Fatalf("want the listing with its PINs replaced by name:\n%.800s", out)
	}
}

// 3CX exposes credential grants as GETs: neither a read tool nor the read-only
// switch may run them, and the execute tool previews until confirmed.
func TestMCPCredentialGrantsNeedConfirm(t *testing.T) {
	t.Parallel()
	for _, ep := range []struct {
		id     string
		params map[string]any
	}{
		{"system-status.get-request-help-link", map[string]any{"grantPeriodDays": 1}},
		{"conference-settings.generate-api-key", map[string]any{}},
		{"trunks.init-master-bridge", map[string]any{}},
	} {
		for _, readOnly := range []bool{false, true} {
			ps, e := honestEnv(t, nil)
			if readOnly {
				e = e.with("THREECX_MCP_READ_ONLY=1")
			}
			before := ps.hits.Load()
			out := e.mcpCalls(t, []map[string]any{
				{"name": "threecx_read", "arguments": map[string]any{"endpoint_id": ep.id, "params": ep.params}},
				{"name": "threecx_execute", "arguments": map[string]any{"endpoint_id": ep.id, "params": ep.params}},
			})
			if n := ps.hits.Load() - before; n != 0 {
				t.Fatalf("%s (read-only %v) reached the phone system %d time(s) without confirm:\n%.1200s", ep.id, readOnly, n, out)
			}
		}
	}
}

// THREECX_PBX picks the phone system the kit is shown; the native tools must use
// it too, not the global configuration, and its store rather than the global one.
func TestMCPNativeToolsFollowThePBXSelection(t *testing.T) {
	t.Parallel()
	ps, e := honestEnv(t, nil)
	var env []string
	for _, kv := range e.env {
		if !strings.HasPrefix(kv, "THREECX_FQDN=") {
			env = append(env, kv)
		}
	}
	over := &bbEnv{home: e.home, env: append(env, "THREECX_FQDN=other.example.test", "THREECX_PBX=acme")}
	before := ps.hits.Load()
	global := filepath.Join(e.home, ".local", "share", "threecx-cli", "data*.db")
	stores, _ := filepath.Glob(global)
	out := over.mcpCalls(t, []map[string]any{
		{"name": "threecx_read", "arguments": map[string]any{"endpoint_id": "system-status.get"}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT 1"}},
	})
	if ps.hits.Load() == before || !strings.Contains(out, "20.0.10.1479") {
		t.Fatalf("the native tool did not reach the selected phone system:\n%.1500s", out)
	}
	if after, _ := filepath.Glob(global); len(after) != len(stores) {
		t.Fatalf("the global store was used under a selection: %v", after)
	}
	if !strings.Contains(out, "acme") {
		t.Fatalf("the sql tool did not name the selected phone system's store:\n%.1500s", out)
	}
}

// A selection naming no registered phone system stops the server with the reason.
func TestMCPUnknownPBXSelectionIsRefused(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	cmd := exec.Command(bbMCP)
	cmd.Env = e.with("THREECX_PBX=nosuch").env
	cmd.Dir = e.home
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "nosuch") {
		t.Fatalf("want a non-zero exit naming the phone system, got %v:\n%.800s", err, out)
	}
}

// A credential on disk belongs to the phone system it was saved for: naming another
// client's phone system sends it nothing, and its own phone system still works.
func TestSavedCredentialStaysWithItsPhoneSystem(t *testing.T) {
	t.Parallel()
	a, b := echoServer(t), echoServer(t)
	a.honest.Store(true)
	b.honest.Store(true)
	e := newEnv(t, a, false)
	pointed := func(extra ...string) *bbEnv {
		var env []string
		for _, kv := range e.env {
			if !strings.HasPrefix(kv, "THREECX_FQDN=") {
				env = append(env, kv)
			}
		}
		return &bbEnv{home: e.home, env: append(env, extra...)}
	}
	get := []string{"system-status", "get", "--json", "--data-source", "live"}
	e.run(t, "auth", "login", "--client-id", "900", "--client-secret", bbFlagSecret)
	if out := e.run(t, get...); !strings.Contains(out, "20.0.10.1479") {
		t.Fatalf("the credential stopped working on its own phone system:\n%.800s", out)
	}
	for _, env := range [][]string{
		{"THREECX_FQDN=" + b.host},
		{"THREECX_FQDN=" + a.host, "THREECX_BASE_URL=https://" + b.host + "/xapi/v1"},
	} {
		out := pointed(env...).run(t, get...)
		if n := b.hits.Load(); n != 0 {
			t.Fatalf("%v: another phone system received %d request(s):\n%.800s", env, n, out)
		}
	}
	// The same for a credential a config file holds.
	cfg := filepath.Join(e.home, "profile.toml")
	body := "client_id = \"900\"\nclient_secret = \"" + bbFlagSecret + "\"\n[template_vars]\nfqdn = \"" + a.host + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := pointed("THREECX_FQDN="+b.host).run(t, append(get, "--config", cfg)...)
	if n := b.hits.Load(); n != 0 {
		t.Fatalf("a config file's credential reached another phone system %d time(s):\n%.800s", n, out)
	}
	if out := pointed("THREECX_FQDN="+a.host).run(t, append(get, "--config", cfg)...); !strings.Contains(out, "20.0.10.1479") {
		t.Fatalf("a config file's credential stopped working on its own phone system:\n%.800s", out)
	}
}

// One run's path context never chooses what config.toml holds: Id=1 used to be
// saved, then held at the next start, which refused to start.
func TestPathContextStaysOutOfTheConfigFile(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	e.mcpCalls(t, []map[string]any{{"name": "sync", "arguments": map[string]any{"resources": "users", "path-context": "Id=1"}}})
	if out := e.run(t, "system-status", "get", "--json", "--data-source", "live"); !strings.Contains(out, "20.0.10.1479") {
		t.Fatalf("the next start did not work:\n%.800s", out)
	}
	files, _ := filepath.Glob(filepath.Join(e.home, ".config", "threecx-cli", "*.toml"))
	for _, f := range files {
		if data, _ := os.ReadFile(f); strings.Contains(string(data), "Id =") {
			t.Fatalf("%s holds the path context:\n%s", f, data)
		}
	}
}

// A number under a credential name is a credential, and every member of a
// duplicate key on stdin is held before the command parses the body.
func TestStdinBodyHeldWhole(t *testing.T) {
	t.Parallel()
	const first, last = "Fa71cQ93xZ", "Lb20dW84yU"
	_, e := honestEnv(t, map[string]string{"/Trunks": `{"Echo":"` + first + `"}`})
	if out := e.runIn(t, `{"AuthPassword":12345678901}`, "trunks", "create", "--stdin", "--dry-run"); strings.Contains(out, "12345678901") {
		t.Fatalf("a numeric password printed:\n%.800s", out)
	}
	body := `{"AuthPassword":"` + first + `","AuthPassword":"` + last + `"}`
	if out := e.runIn(t, body, "trunks", "create", "--stdin", "--yes", "--json"); strings.Contains(out, first) {
		t.Fatalf("the first member of a duplicate key printed:\n%.800s", out)
	}
}
