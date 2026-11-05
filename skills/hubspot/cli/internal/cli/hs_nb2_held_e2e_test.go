// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Nextfix2 (handfixes.json credkit-wiring): every credential the
// program holds, from every source, is withheld from every output, whatever
// its length. Each source below is driven end to end through the built CLI
// and the built MCP server (stdio) against a portal that echoes the value as
// written, ROT13'd and base64'd in a successful answer. The record survives;
// only the echoes are withheld. A response cache written before the fix is
// purged on first open.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func nb2ROT13(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return 'a' + (r-'a'+13)%26
		case r >= 'A' && r <= 'Z':
			return 'A' + (r-'A'+13)%26
		}
		return r
	}, s)
}

func nb2Forms(v string) map[string]string {
	return map[string]string{"raw": v, "rot13": nb2ROT13(v), "base64": base64.StdEncoding.EncodeToString([]byte(v))}
}

// nb2Build builds both binaries once per test run.
var nb2Build = struct {
	sync.Once
	cli, mcp string
	err      error
	out      []byte
}{}

func nb2Binaries(t *testing.T) (string, string) {
	t.Helper()
	nb2Build.Do(func() {
		dir, err := os.MkdirTemp("", "hs-nb2-bin-")
		if err != nil {
			nb2Build.err = err
			return
		}
		exe := ""
		if runtime.GOOS == "windows" {
			exe = ".exe"
		}
		nb2Build.cli = filepath.Join(dir, "hubspot-cli"+exe)
		nb2Build.mcp = filepath.Join(dir, "hubspot-mcp"+exe)
		for bin, pkg := range map[string]string{nb2Build.cli: "../../cmd/hubspot-cli", nb2Build.mcp: "../../cmd/hubspot-mcp"} {
			if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
				nb2Build.err, nb2Build.out = err, out
				return
			}
		}
	})
	if nb2Build.err != nil {
		t.Fatalf("build: %v\n%s", nb2Build.err, nb2Build.out)
	}
	return nb2Build.cli, nb2Build.mcp
}

// nb2Env is a clean environment: no inherited HubSpot or XDG settings, and
// the config directory pinned to nb2ConfigDir on every OS.
func nb2Env(home, baseURL, cliBin string, extra ...string) []string {
	_ = os.MkdirAll(nb2ConfigDir(home), 0o700)
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "HUBSPOT_") || strings.HasPrefix(name, "XDG_") || name == "HOME" || name == "USERPROFILE" || name == "PP_MCP_TRANSPORT" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home, "HUBSPOT_HOME="+home, "HUBSPOT_CONFIG_DIR="+nb2ConfigDir(home), "HUBSPOT_BASE_URL="+baseURL, "HUBSPOT_CLI_PATH="+cliBin, "HUBSPOT_NO_LEARN=1")
	if ca, ok := nb2PortalCAs.Load(baseURL); ok {
		env = append(env, "HUBSPOT_CA_CERT="+ca.(string))
	}
	return append(env, extra...)
}

func nb2Run(t *testing.T, bin string, env []string, stdin string, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

const nb2ReadContact = `{"name":"hubspot_read","arguments":{"endpoint_id":"hubspot-contacts-crm.get-v3-objects-contacts-contact-id","params":{"contactId":"1"}}}`

// nb2MCPRead calls hubspot_read for contact 1 over stdio and returns the
// tools/call response line.
func nb2MCPRead(t *testing.T, mcpBin string, env []string, args ...string) string {
	t.Helper()
	return nb2MCPCall(t, mcpBin, env, nb2ReadContact, args...)
}

// nb2MCPCall makes one tools/call (call is its params) over stdio.
func nb2MCPCall(t *testing.T, mcpBin string, env []string, call string, args ...string) string {
	t.Helper()
	msgs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"e2e","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":` + call + `}`,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, args...)
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close(); _ = cmd.Wait() }()
	_, _ = fmt.Fprintln(stdin, strings.Join(msgs, "\n"))
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var msg struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) == nil && string(msg.ID) == "2" {
			return sc.Text() + errb.String()
		}
	}
	t.Fatalf("MCP server returned no tools/call answer; stderr:\n%s", errb.String())
	return ""
}

// nb2PortalCAs is the certificate file of each fake portal, by its URL: the
// credential kit never sends a credential over plain http, so the portal is
// https and the binaries trust it through HUBSPOT_CA_CERT.
var nb2PortalCAs sync.Map

// nb2Portal answers contact 1 with a record whose properties echo each of
// echoes, and contact 2 with a 400 whose message repeats them.
func nb2Portal(t *testing.T, echoes map[string]string) (*httptest.Server, *int) {
	t.Helper()
	props := map[string]any{"firstname": "Ada", "lastname": "Lovelace"}
	for k, v := range echoes {
		props["echo_"+k] = v
	}
	rec, _ := json.Marshal(map[string]any{"id": "1", "properties": props, "createdAt": "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z", "archived": false})
	hits := new(int)
	var mu sync.Mutex
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/crm/v3/objects/contacts/1":
			_, _ = w.Write(rec)
		case r.URL.Path == "/crm/v3/objects/contacts/2":
			var vals []string
			for _, v := range echoes {
				vals = append(vals, v)
			}
			w.WriteHeader(http.StatusBadRequest)
			msg, _ := json.Marshal("Property values were not valid: " + strings.Join(vals, " "))
			_, _ = fmt.Fprintf(w, `{"status":"error","category":"VALIDATION_ERROR","message":%s}`, msg)
		case r.URL.Path == "/crm/v3/objects/contacts":
			_, _ = fmt.Fprintf(w, `{"results":[%s]}`, rec)
		default:
			_, _ = fmt.Fprint(w, `{"results":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "portal-ca.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(ca, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	nb2PortalCAs.Store(srv.URL, ca)
	return srv, hits
}

func nb2ConfigDir(home string) string { return filepath.Join(home, ".config", "hubspot-cli") }

func nb2WriteConfig(t *testing.T, home, body string) {
	t.Helper()
	dir := nb2ConfigDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func nb2AssertNoForms(t *testing.T, where, out string, forms map[string]string) {
	t.Helper()
	for name, f := range forms {
		if strings.Contains(out, f) {
			t.Errorf("%s printed the held value (%s %q):\n%s", where, name, f, out)
		}
	}
}

func nb2AssertWithheld(t *testing.T, where, out string, forms map[string]string) {
	t.Helper()
	nb2AssertNoForms(t, where, out, forms)
	if !strings.Contains(out, "credkit: withheld") {
		t.Errorf("%s: want the unit that carried the value replaced by the kit's notice:\n%s", where, out)
	}
}

func TestNB2EveryHeldSourceIsWithheldEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	const base = "pat-na1-" + "e2e7f3c0-9a6b-4c8e-8d1f-2b5e7c1a9d30"
	pemLine := "MIIBVQIBADANBgkqhkiG9w0BAQEFAASCAT8wggE7AgEAAkEAnb2HeldKeyLine"
	sources := []struct {
		name  string
		value string
		cli   bool
		setup func(t *testing.T, home, cliBin string, env []string) (cliEnv []string, mcpArgs []string)
	}{
		{"env-access-token", "pat-na1-" + "e2e7f3c1-9a6b-4c8e-8d1f-2b5e7c1a9d31", true, func(t *testing.T, home, cliBin string, env []string) ([]string, []string) {
			const held = "pat-na1-" + "e2e7f3c1-9a6b-4c8e-8d1f-2b5e7c1a9d31"
			return append(env, "HUBSPOT_ACCESS_TOKEN="+held), nil
		}},
		{"config-access-token", "pat-na1-" + "e2e7f3c2-9a6b-4c8e-8d1f-2b5e7c1a9d32", true, func(t *testing.T, home, cliBin string, env []string) ([]string, []string) {
			nb2WriteConfig(t, home, "access_token = \"pat-na1-"+"e2e7f3c2-9a6b-4c8e-8d1f-2b5e7c1a9d32\"\n")
			return env, nil
		}},
		{"config-header-short", "Zq7", true, func(t *testing.T, home, cliBin string, env []string) ([]string, []string) {
			nb2WriteConfig(t, home, "access_token = \""+base+"\"\n\n[headers]\nX-Api-Key = \"Zq7\"\n")
			return env, nil
		}},
		{"credential-store", "pat-na1-" + "e2e7f3c4-9a6b-4c8e-8d1f-2b5e7c1a9d34", true, func(t *testing.T, home, cliBin string, env []string) ([]string, []string) {
			if _, stderr, err := nb2Run(t, cliBin, env, "pat-na1-"+"e2e7f3c4-9a6b-4c8e-8d1f-2b5e7c1a9d34\n", "auth", "set-token"); err != nil {
				t.Fatalf("auth set-token: %v\n%s", err, stderr)
			}
			return env, nil
		}},
		{"env-mcp-http-token-short", "Hq4", true, func(t *testing.T, home, cliBin string, env []string) ([]string, []string) {
			return append(env, "HUBSPOT_ACCESS_TOKEN="+base, "HUBSPOT_MCP_HTTP_TOKEN=Hq4"), nil
		}},
		{"mcp-tls-key-flag", pemLine, false, func(t *testing.T, home, cliBin string, env []string) ([]string, []string) {
			key := filepath.Join(home, "server.key")
			// The armor is assembled so secret scanners do not read the fixture as a key.
			armor := "PRIVATE " + "KEY-----"
			keyPEM := "-----BEGIN " + armor + "\n" + pemLine + "\nAbCdEfGhIjKlMnOpQrStUvWxYz0123456789\n-----END " + armor + "\n"
			if err := os.WriteFile(key, []byte(keyPEM), 0o600); err != nil {
				t.Fatal(err)
			}
			return append(env, "HUBSPOT_ACCESS_TOKEN="+base), []string{"--transport", "stdio", "--tls-key", key}
		}},
	}
	for _, src := range sources {
		t.Run(src.name, func(t *testing.T) {
			forms := nb2Forms(src.value)
			srv, _ := nb2Portal(t, forms)
			home := t.TempDir()
			env, mcpArgs := src.setup(t, home, cliBin, nb2Env(home, srv.URL, cliBin))
			if src.cli {
				out, stderr, err := nb2Run(t, cliBin, env, "", "hubspot-contacts-crm", "get-v3-objects-contacts-contact-id", "1", "--json")
				if err != nil {
					t.Fatalf("CLI read: %v\n%s\n%s", err, out, stderr)
				}
				nb2AssertWithheld(t, "CLI", out+stderr, forms)
			}
			nb2AssertWithheld(t, "MCP", nb2MCPRead(t, mcpBin, env, mcpArgs...), forms)
			if src.cli {
				out, stderr, _ := nb2Run(t, cliBin, env, "", "hubspot-contacts-crm", "get-v3-objects-contacts-contact-id", "2", "--json")
				nb2AssertNoForms(t, "CLI error", out+stderr, forms)
			}
			nb2AssertNoForms(t, "MCP error", nb2MCPCall(t, mcpBin, env, strings.Replace(nb2ReadContact, `"contactId":"1"`, `"contactId":"2"`, 1), mcpArgs...), forms)
		})
	}
}

// A value that becomes held after it was cached or stored is withheld on the
// way out: the cached read, the local read and the MCP sql tool all pass the
// kit at their sink.
func TestNB2PreFixCacheAndStoreAreWithheldOnTheWayOut(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	const held0 = "pat-na1-" + "e2e7f3c5-9a6b-4c8e-8d1f-2b5e7c1a9d35"
	forms := nb2Forms("Kx9vQ2mZ")
	srv, hits := nb2Portal(t, forms)
	home := t.TempDir()
	cache := filepath.Join(home, "cache")
	env := nb2Env(home, srv.URL, cliBin, "HUBSPOT_ACCESS_TOKEN="+held0, "HUBSPOT_CACHE_DIR="+cache)
	read := []string{"hubspot-contacts-crm", "get-v3-objects-contacts-contact-id", "1", "--json", "--data-source", "live"}
	// Kx9vQ2mZ is not held yet: it is answered, cached and stored as is.
	if out, stderr, err := nb2Run(t, cliBin, env, "", read...); err != nil || !strings.Contains(out, "Kx9vQ2mZ") {
		t.Fatalf("first read: %v\n%s\n%s", err, out, stderr)
	}
	if _, stderr, err := nb2Run(t, cliBin, env, "", "sync", "--resources", "hubspot-contacts-crm", "--json"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	before := *hits
	out, stderr, err := nb2Run(t, cliBin, append(env, "HUBSPOT_MCP_HTTP_TOKEN=Kx9vQ2mZ"), "", read...)
	if err != nil {
		t.Fatalf("cached read: %v\n%s\n%s", err, out, stderr)
	}
	if *hits != before {
		t.Errorf("the second read went to the portal, so the cache read was not exercised")
	}
	nb2AssertWithheld(t, "cached read", out+stderr, forms)

	held := append(env, "HUBSPOT_MCP_HTTP_TOKEN=Kx9vQ2mZ")
	local := []string{"hubspot-contacts-crm", "get-v3-objects-contacts-contact-id", "1", "--json", "--data-source", "local"}
	out, stderr, err = nb2Run(t, cliBin, held, "", local...)
	if err != nil {
		t.Fatalf("local read: %v\n%s\n%s", err, out, stderr)
	}
	nb2AssertWithheld(t, "local read", out+stderr, forms)
	got := nb2MCPCall(t, mcpBin, held, `{"name":"sql","arguments":{"query":"SELECT id, data FROM hubspot_contacts_crm"}}`)
	for name, f := range forms {
		if strings.Contains(got, f) {
			t.Errorf("MCP sql printed the stored value (%s %q):\n%s", name, f, got)
		}
	}
	if !strings.Contains(got, "credkit: withheld") {
		t.Errorf("MCP sql: want the stored record withheld, not missing:\n%s", got)
	}
}
