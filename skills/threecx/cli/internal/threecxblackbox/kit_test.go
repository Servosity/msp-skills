// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package threecxblackbox

// The credential classes the review named, each from its failing input, driven
// through the built binaries wired to the shared credential kit
// (handfixes.json credkit-wiring).

import (
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const bbWithheld = "credkit: withheld"

// honestEnv is a phone system that echoes nothing but what routes names.
func honestEnv(t *testing.T, routes map[string]string, extra ...string) (*phoneSystem, *bbEnv) {
	t.Helper()
	ps := echoServer(t)
	ps.honest.Store(true)
	ps.routes = routes
	return ps, newEnv(t, ps, true).with(extra...)
}

// Review round 2's input: the base32 of an 8-byte password beside its redacted
// member. The member is replaced by name, the encoded echo withholds the unit.
func TestEncodedPasswordBesideItsMember(t *testing.T) {
	t.Parallel()
	const encoded = "OBQXG43XN5ZGI==="
	_, e := honestEnv(t, map[string]string{"/Users(": `{"AuthPassword":"password","FirstName":"` + encoded + `"}`})
	out := e.run(t, "users-id", "get-user", "2", "--json", "--data-source", "live")
	if strings.Contains(out, encoded) {
		t.Fatalf("the base32 of a held password printed:\n%.1000s", out)
	}
	if !strings.Contains(out, bbWithheld) {
		t.Fatalf("want the kit's notice, got:\n%.1000s", out)
	}
}

// Review round 2's input: a 4-character password split across adjacent values
// (no length floor), and the same value in every encoding.
func TestShortPasswordSplitOrEncoded(t *testing.T) {
	t.Parallel()
	const pw = "Qz7k"
	a85 := make([]byte, ascii85.MaxEncodedLen(len(pw)))
	a85 = a85[:ascii85.Encode(a85, []byte(pw))]
	var dec, uesc, pct strings.Builder
	for _, c := range []byte(pw) {
		fmt.Fprintf(&dec, "&#%d;", c)
		fmt.Fprintf(&uesc, "\\u%04x", c)
		fmt.Fprintf(&pct, "%%%02X", c)
	}
	forms := map[string]string{
		"split":   `"Qz","7k"`,
		"plain":   `"x ` + pw + ` y"`,
		"base64":  `"` + base64.StdEncoding.EncodeToString([]byte(pw)) + `"`,
		"hex":     `"` + hex.EncodeToString([]byte(pw)) + `"`,
		"base32":  `"` + base32.StdEncoding.EncodeToString([]byte(pw)) + `"`,
		"ascii85": strconv.Quote(string(a85)),
		"percent": `"` + pct.String() + `"`,
		"html":    `"` + dec.String() + `"`,
		"json-u":  `"` + uesc.String() + `"`,
	}
	for name, form := range forms {
		t.Run(name, func(t *testing.T) {
			// A Concealed object under a name nobody declared: only the connector's
			// own walker can find this password, so it has no length floor either.
			_, e := honestEnv(t, map[string]string{"/Users(": `{"Mystery":{"Concealed":true,"Value":"` + pw + `"},"Notes":[` + form + `]}`})
			out := e.run(t, "users-id", "get-user", "2", "--json", "--data-source", "live")
			if !strings.Contains(out, bbWithheld) {
				t.Fatalf("%s: want the kit's notice, got:\n%.1000s", name, out)
			}
		})
	}
}

// The control for the class above: the same password with nothing repeating it
// prints the record, the member replaced by name, so the notice above is the echo's.
func TestShortPasswordAloneKeepsTheRecord(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{"/Users(": `{"AuthPassword":"Qz7k","Notes":["an ordinary note"]}`})
	out := e.run(t, "users-id", "get-user", "2", "--json", "--data-source", "live")
	if strings.Contains(out, bbWithheld) || !strings.Contains(out, "an ordinary note") || !strings.Contains(out, "[redacted]") {
		t.Fatalf("want the record with its password replaced, got:\n%.1000s", out)
	}
}

// A rejected mint echoing the client secret returns instead of hanging, and
// the echo never prints.
func TestRejectedMintEchoNeverPrints(t *testing.T) {
	t.Parallel()
	ps, e := honestEnv(t, nil)
	ps.rejectMint.Store(true)
	assertNoEcho(t, "rejected mint", e.run(t, "system-status", "get", "--json"))
}

// Review round 2's input: a password that is literally "hidden" is not a mask.
func TestPasswordThatReadsLikeAMask(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{"/Users(": `{"AuthPassword":"hidden","Notes":"the word hidden"}`})
	if out := e.run(t, "users-id", "get-user", "2", "--json", "--data-source", "live"); strings.Contains(out, "the word hidden") {
		t.Fatalf("a password spelled hidden was taken for a mask:\n%.1000s", out)
	}
}

// Class startup-credential-before-parse: a secret in the environment is held
// before a malformed config is parsed, so the parser's error text cannot carry it.
func TestStartupHeldBeforeAMalformedConfig(t *testing.T) {
	t.Parallel()
	ps := echoServer(t)
	e := newEnv(t, ps, true)
	cfg := filepath.Join(e.home, "bad.toml")
	if err := os.WriteFile(cfg, []byte("client_secret = \""+bbSecret+"\nbroken ="), 0o600); err != nil {
		t.Fatal(err)
	}
	out := e.run(t, "--config", cfg, "system-status", "get")
	assertNoEcho(t, "malformed config", out)
}

// Class credential-sent-to-another-host, and the base URL the kit is shown: a
// credential reaches the phone system each way the program can be pointed at
// one, and never another host.
func TestCredentialReachesTheConfiguredPhoneSystemOnly(t *testing.T) {
	t.Parallel()
	ps, e := honestEnv(t, nil)
	cfg := filepath.Join(e.home, "pinned.toml")
	if err := os.WriteFile(cfg, []byte("base_url = \"https://{fqdn}/xapi/v1\"\nclient_id = \"900\"\n[template_vars]\nfqdn = \""+ps.host+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]func() string{
		"THREECX_FQDN": func() string { return e.run(t, "system-status", "get", "--json") },
		"--pbx":        func() string { return e.run(t, "--pbx", "acme", "system-status", "get", "--json") },
		"THREECX_PBX":  func() string { return e.with("THREECX_PBX=globex").run(t, "system-status", "get", "--json") },
		"config file": func() string {
			var env []string
			for _, kv := range e.env {
				if !strings.HasPrefix(kv, "THREECX_FQDN=") {
					env = append(env, kv)
				}
			}
			return (&bbEnv{home: e.home, env: env}).run(t, "--config", cfg, "system-status", "get", "--json")
		},
	} {
		before := ps.hits.Load()
		out := run()
		if ps.hits.Load() == before || !strings.Contains(out, "20.0.10.1479") {
			t.Errorf("%s: want the phone system read, got:\n%.1000s", name, out)
		}
		if ps.offOrigin.Load() != 0 {
			t.Errorf("%s: another host received a request", name)
		}
	}
}

// Each PBX's secret reaches its child down the kit's pipe, under whatever name
// the registry gave it.
func TestFleetChildGetsItsSecret(t *testing.T) {
	t.Parallel()
	ps, e := honestEnv(t, nil, "CUSTOM_PBX_CLIENT_SECRET="+bbFlagSecret)
	e.run(t, "fleet", "add", "custom", "--fqdn", ps.host, "--client-id", "904", "--env-prefix", "CUSTOM_PBX")
	out := e.run(t, "fleet", "versions", "--only", "acme,custom", "--json")
	if strings.Count(out, "20.0.10.1479") < 2 {
		t.Fatalf("want both PBXes read, got:\n%.1500s", out)
	}
	assertNoEcho(t, "fleet versions", out)
}

// An OAuth exchange's --code is a credential beside its companions.
func TestArgvOAuthCodeIsHeldBesideItsCompanions(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{"/McpServers": `{"note":"got argvCode-5a1c9e3b back"}`})
	out := e.run(t, "mcp-servers", "exchange", "--code", "argvCode-5a1c9e3b", "--verifier=argvVer-9e3b7d1f", "--token-endpoint", "https://idp.example/t", "--json", "--yes")
	if strings.Contains(out, "argvCode-5a1c9e3b") || strings.Contains(out, "argvVer-9e3b7d1f") {
		t.Errorf("the OAuth exchange printed its code or verifier:\n%.1000s", out)
	}
}

// Class credential-sent-to-another-host, plain http: the API client's own
// transport is the kit's, so a saved token is never sent in the clear.
func TestSavedTokenIsNotSentOverPlainHTTP(t *testing.T) {
	t.Parallel()
	var carried atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), bbFlagSecret) {
			carried.Add(1)
		}
		fmt.Fprint(w, `{"Version":"20.0.10.1479"}`)
	}))
	t.Cleanup(plain.Close)
	ps, e := honestEnv(t, nil)
	e.runIn(t, bbFlagSecret+"\n", "auth", "set-token")
	var env []string
	for _, kv := range e.env {
		if !strings.HasPrefix(kv, "THREECX_FQDN=") && !strings.HasPrefix(kv, "THREECX_CLIENT_SECRET=") {
			env = append(env, kv)
		}
	}
	over := &bbEnv{home: e.home, env: append(env, "THREECX_BASE_URL="+plain.URL+"/xapi/v1")}
	out := over.run(t, "system-status", "get", "--json", "--data-source", "live")
	if carried.Load() != 0 {
		t.Fatalf("the saved token went over plain http %d time(s):\n%.800s", carried.Load(), out)
	}
	_ = ps
}

// An MCP write's preview shows its arguments, so a password in a shape no
// declared name can see (a Concealed object at any depth) is held before it runs.
func TestMCPPreviewHoldsANestedCredential(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	out := e.mcpCalls(t, []map[string]any{{"name": "threecx_execute", "arguments": map[string]any{
		"endpoint_id": "users-id.update-user",
		"params":      map[string]any{"Id": "2", "Settings": map[string]any{"Mystery": map[string]any{"Concealed": true, "Value": bbMCPPass}}},
	}}})
	assertNoEcho(t, "MCP preview", out)
}
