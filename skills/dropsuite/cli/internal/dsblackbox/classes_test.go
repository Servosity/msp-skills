// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package dsblackbox

// The three classes the factory print left open, each closed through the
// shared credential kit (handfixes.json credkit-wiring) and proven here on
// the built binaries from its failing input. Each was seen failing on the
// pre-kit tree first.

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Class credential-source-not-held: a config file named by DROPSUITE_CONFIG
// that does not parse (a multi-line literal string, then a broken line) still
// has its token held before Cobra repeats the token in its unknown-command
// error; so does one whose token sits after its first MiB.
func TestCredentialSourceNotHeld(t *testing.T) {
	sv := echoServer(t)
	for name, pad := range map[string]string{
		"multi-line literal before a broken line": "",
		"token after the first MiB":               strings.Repeat("#\n", 1<<19+1),
	} {
		const tok = "mlTok-Zq7Xw3Rv9Kp2Ty5Ub8"
		e := newEnv(t, sv, false)
		cfg := filepath.Join(e.home, "ml", "config.toml")
		writePrivate(t, cfg, pad+"access_token = '''\n"+tok+"\n'''\nbroken = [\n", 0o600)
		e.env = append(e.env, "DROPSUITE_CONFIG="+cfg)
		out := e.run(t, tok)
		assertNotEchoed(t, name+": output of dropsuite-cli <token>", out, tok)
		if !strings.Contains(out, "unknown command") && !strings.Contains(out, "withheld") {
			t.Fatalf("%s: dropsuite-cli <token> answered neither Cobra's error nor a withheld notice; the test did not run:\n%.1500s", name, out)
		}
	}
}

// Class secret-in-startup-diagnostics: the credential-store refusal warning
// names the store's path, and that path is under a home directory whose name
// holds the environment token. It prints while the configuration loads, before
// any command runs, from the CLI (--home and DROPSUITE_HOME) and the MCP server.
func TestSecretInStartupDiagnostics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the store refusal is driven by Unix mode bits")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	home := filepath.Join(e.home, "home-"+bbAccess)
	writePrivate(t, filepath.Join(home, "data", "credentials.toml"), "access_token = 'looseStoreTk-8Hn4Jm6Kp2Lq'\n", 0o644)
	refused := func(where, out string) {
		t.Helper()
		assertNoEcho(t, where, out)
		if !strings.Contains(out, "refused") && !strings.Contains(out, "withheld") {
			t.Fatalf("%s: no refusal warning and no withheld notice; the test did not run:\n%.1500s", where, out)
		}
	}
	refused("CLI with --home", e.run(t, "--home", home, "organizations", "list", "--json"))
	base := e.env
	e.env = append(append([]string{}, base...), "DROPSUITE_HOME="+home)
	refused("CLI with DROPSUITE_HOME", e.run(t, "organizations", "list", "--json"))
	refused("MCP server with DROPSUITE_HOME", e.mcpCalls(t, []map[string]any{{"name": "organizations_list", "arguments": map[string]any{}}}))
}

// nestedQP is quoted-printable over base64 over quoted-printable of s.
func nestedQP(s string) string {
	return qpForm(base64.StdEncoding.EncodeToString([]byte(qpForm(s, false))), false)
}

// Class held-value-in-an-undecoded-encoding: an answer the CLI prints carries
// the environment token as quoted-printable over base64 over quoted-printable.
func TestHeldValueInAnUndecodedEncoding(t *testing.T) {
	enc := nestedQP(bbAccess)
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{"id":"100","email":"hq@example.com","display_name":"HQ-Record","note":%q}]`, enc)
	}))
	e := newEnv(t, &server{url: srv.URL + "/api", caFile: writeCA(t, srv)}, true)
	out := e.run(t, "reference", "list-countries", "--json")
	if strings.Contains(out, enc) {
		t.Errorf("output repeats the token as quoted-printable over base64 over quoted-printable:\n%.1500s", out)
	}
	assertNoEcho(t, "reference list-countries --json", out)
	if !strings.Contains(out, "HQ-Record") && !strings.Contains(out, "withheld") {
		t.Fatalf("the answer was neither printed nor withheld; the test did not run:\n%.1500s", out)
	}
}
