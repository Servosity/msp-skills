// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package jamfblackbox

// The credential classes, each from its failing input, driven through the built
// binaries wired to the shared credential kit (handfixes.json credkit-wiring).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const bbWithheld = "credkit: withheld"

// honestEnv is a server that echoes nothing but what routes names.
func honestEnv(t *testing.T, routes map[string]string, extra ...string) (*jamfServer, *bbEnv) {
	t.Helper()
	js := echoServer(t)
	js.routes = routes
	e := newEnv(t, js, true).with(extra...)
	js.honest.Store(true)
	return js, e
}

func without(env []string, prefixes ...string) []string {
	var out []string
next:
	for _, kv := range env {
		for _, p := range prefixes {
			if strings.HasPrefix(kv, p) {
				continue next
			}
		}
		out = append(out, kv)
	}
	return out
}

// A 4-character password under its own name, repeated split across adjacent
// values or in any encoding: no length floor, and the kit takes the unit.
func TestShortPasswordSplitOrEncoded(t *testing.T) {
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
			t.Parallel()
			_, e := honestEnv(t, map[string]string{"/api/v1/buildings/": `{"id":"1","password":"` + pw + `","notes":[` + form + `]}`})
			out := e.run(t, "buildings", "get", "1", "--json", "--data-source", "live")
			if !strings.Contains(out, bbWithheld) {
				t.Fatalf("%s: want the kit's notice, got:\n%.1000s", name, out)
			}
		})
	}
}

// The control: the same password repeated nowhere prints the record with the
// member replaced by name, so the notice above is the echo's.
func TestShortPasswordAloneKeepsTheRecord(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{"/api/v1/buildings/": `{"id":"1","password":"Qz7k","notes":["an ordinary note"]}`})
	out := e.run(t, "buildings", "get", "1", "--json", "--data-source", "live")
	if strings.Contains(out, bbWithheld) || !strings.Contains(out, "an ordinary note") || !strings.Contains(out, "[redacted]") {
		t.Fatalf("want the record with its password replaced, got:\n%.1000s", out)
	}
}

// A rejected mint echoing the client secret returns instead of hanging, and the
// echo never prints.
func TestRejectedMintEchoNeverPrints(t *testing.T) {
	t.Parallel()
	js, e := honestEnv(t, nil)
	js.rejectMint.Store(true)
	assertNoEcho(t, "rejected mint", e.run(t, "jamf-pro-version", "--json", "--data-source", "live"))
}

// A password spelled like a mask word is a password.
func TestPasswordThatReadsLikeAMask(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{"/api/v1/buildings/": `{"id":"1","password":"hidden","notes":"the word hidden"}`})
	if out := e.run(t, "buildings", "get", "1", "--json", "--data-source", "live"); strings.Contains(out, "the word hidden") {
		t.Fatalf("a password spelled hidden was taken for a mask:\n%.1000s", out)
	}
}

// A secret in the environment is held before a malformed config is parsed, and
// so is what the malformed file carries.
func TestStartupHeldBeforeAMalformedConfig(t *testing.T) {
	t.Parallel()
	e := newEnv(t, echoServer(t), true)
	for name, body := range map[string]string{
		"environment secret": "client_secret = \"" + bbSecret + "\nbroken =",
		"the file's own":     "client_secret = 123456789012345678901234567890\nbroken =",
	} {
		cfg := filepath.Join(e.home, "bad.toml")
		if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		out := e.run(t, "--config", cfg, "jamf-pro-version")
		assertNoEcho(t, name, out)
		if strings.Contains(out, "123456789012345678901234567890") {
			t.Errorf("%s: a malformed config's value printed:\n%.800s", name, out)
		}
	}
}

// A credential reaches the Jamf Pro server each way the program can be pointed
// at one, and never another host.
func TestCredentialReachesTheConfiguredServerOnly(t *testing.T) {
	t.Parallel()
	js, e := honestEnv(t, nil)
	cfg := filepath.Join(e.home, "pinned.toml")
	if err := os.WriteFile(cfg, []byte("base_url = \"https://{server}\"\nclient_id = \"client-single-0001\"\n[template_vars]\nserver = \""+js.host+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	get := []string{"jamf-pro-version", "--json", "--data-source", "live"}
	for name, run := range map[string]func() string{
		"JAMF_PRO_SERVER":   func() string { return e.run(t, get...) },
		"--instance":        func() string { return e.run(t, append([]string{"--instance", "acme"}, get...)...) },
		"JAMF_PRO_INSTANCE": func() string { return e.with("JAMF_PRO_INSTANCE=globex").run(t, get...) },
		"config file": func() string {
			return (&bbEnv{home: e.home, env: without(e.env, "JAMF_PRO_SERVER=")}).run(t, append([]string{"--config", cfg}, get...)...)
		},
	} {
		before := js.hits.Load()
		out := run()
		if js.hits.Load() == before || !strings.Contains(out, "11.12.0") {
			t.Errorf("%s: want the server read, got:\n%.1000s", name, out)
		}
		if js.offOrigin.Load() != 0 {
			t.Errorf("%s: another host received a request", name)
		}
	}
}

// Each fleet child gets its own JAMF_PRO_FLEET_SECRETS entry and mints with it.
func TestFleetChildGetsItsSecret(t *testing.T) {
	t.Parallel()
	js, e := honestEnv(t, nil)
	e.env = append(without(e.env, "JAMF_PRO_FLEET_SECRETS="), "JAMF_PRO_FLEET_SECRETS=acme="+bbSecretAcme+",globex="+bbSecretGlobex+",custom="+bbFlagSecret)
	e.run(t, "instance", "add", "custom", "--server", js.host, "--client-id", "client-custom-0001")
	out := e.run(t, "fleet", "exec", "/api/v1/sites", "--only", "acme,custom", "--json")
	if strings.Count(out, "Clean-Site") < 2 {
		t.Fatalf("want both instances read, got:\n%.1500s", out)
	}
	assertNoEcho(t, "fleet exec", out)
}

// The API client's own transport is the kit's: a saved token is never sent in
// the clear.
func TestSavedTokenIsNotSentOverPlainHTTP(t *testing.T) {
	t.Parallel()
	var carried atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Authorization"), bbFlagSecret) {
			carried.Add(1)
		}
		fmt.Fprint(w, `{"version":"11.12.0"}`)
	}))
	t.Cleanup(plain.Close)
	_, e := honestEnv(t, nil)
	e.runIn(t, bbFlagSecret+"\n", "auth", "set-token")
	over := &bbEnv{home: e.home, env: append(without(e.env, "JAMF_PRO_SERVER=", "JAMF_PRO_CLIENT_SECRET="), "JAMF_PRO_BASE_URL="+plain.URL)}
	out := over.run(t, "jamf-pro-version", "--json", "--data-source", "live")
	if carried.Load() != 0 {
		t.Fatalf("the saved token went over plain http %d time(s):\n%.800s", carried.Load(), out)
	}
}

// An MCP write's preview shows its arguments, so a nested credential is held
// before it runs.
func TestMCPPreviewHoldsANestedCredential(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	out := e.mcpCalls(t, []map[string]any{{"name": "jamf-pro_execute", "arguments": map[string]any{
		"endpoint_id": "buildings.update",
		"params":      map[string]any{"id": "1", "name": "HQ", "settings": map[string]any{"bindPassword": bbMCPPass}},
	}}})
	assertNoEcho(t, "MCP preview", out)
}

// Whatever token URL the configuration names, 'auth login' sends the client
// secret only to the configured server's own origin.
func TestAuthLoginNeverLeavesTheConfiguredServer(t *testing.T) {
	t.Parallel()
	js := echoServer(t)
	e := newEnv(t, js, true)
	for _, c := range []struct {
		env  []string
		args []string
	}{
		{[]string{"JAMF_PRO_TOKEN_URL=" + js.redirectTo + "/api/v1/oauth/token"}, []string{"auth", "login"}},
		{[]string{"JAMF_PRO_TOKEN_URL=" + js.redirectTo + "/api/v1/oauth/token"}, []string{"auth", "login", "--client-id", "client-flag-0001", "--client-secret", bbFlagSecret}},
	} {
		out := e.with(c.env...).run(t, c.args...)
		if n := js.offOrigin.Load(); n != 0 {
			t.Fatalf("%v with %v: another host received %d request(s):\n%.1500s", c.args, c.env, n, out)
		}
		assertNoEcho(t, "auth login output", out)
	}
}

// Under --dry-run nothing reaches the server and no credential is saved.
func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	js := echoServer(t)
	e := newEnv(t, js, true)
	before := js.hits.Load()
	for _, args := range [][]string{
		{"auth", "login", "--dry-run"},
		{"auth", "login", "--client-id", "client-flag-0001", "--client-secret", bbFlagSecret, "--dry-run"},
		{"buildings", "list", "--json", "--dry-run", "--data-source", "live"},
		{"computer-prestages", "update", "1", "--account-settings-admin-password", bbPatchPass, "--json", "--dry-run"},
		{"sync", "--resources", "sites", "--dry-run"},
		{"export", "sites", "-o", filepath.Join(e.home, "dry.jsonl"), "--dry-run"},
		{"doctor", "--dry-run"},
		{"fleet", "health", "--dry-run"},
		{"fleet", "exec", "/api/v1/sites", "--dry-run"},
		{"--instance", "acme", "buildings", "list", "--dry-run", "--data-source", "live"},
	} {
		out := e.run(t, args...)
		if n := js.hits.Load() - before; n != 0 {
			t.Errorf("%v sent %d request(s) under --dry-run:\n%.600s", args, n, out)
		}
		before = js.hits.Load()
	}
	e.runIn(t, bbFlagSecret+"\n", "auth", "set-token", "--dry-run")
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasPrefix(d.Name(), "credentials") {
			return nil
		}
		if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte(bbFlagSecret)) {
			t.Errorf("a --dry-run saved the secret in %s", path)
		}
		return nil
	})
}

// A feedback upload passes the kit: one carrying a held credential never leaves,
// and an ordinary one does.
func TestFeedbackUploadCarriesNoCredential(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var got bytes.Buffer
	posts := 0
	fb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got.Write(b)
		posts++
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(fb.Close)
	e := newEnv(t, echoServer(t), true)
	send := e.with("JAMF_PRO_FEEDBACK_ENDPOINT=" + fb.URL + "/fb")
	out := send.run(t, "feedback", "--send", "noted the key "+bbSecret+" in a ticket")
	out2 := send.run(t, "feedback", "--send", "an ordinary note about buildings")
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 || !strings.Contains(got.String(), "ordinary note") {
		t.Fatalf("want exactly the ordinary upload, got %d: %q\n%.600s\n%.600s", posts, got.String(), out, out2)
	}
	assertNoEcho(t, "feedback upload body", got.String())
	assertNoEcho(t, "feedback output", out)
}

// Every member of a duplicate key is held: a decoder keeps only the last, so the
// value an echo repeats would be lost to the holder.
func TestDuplicateMemberEchoNeverPrints(t *testing.T) {
	t.Parallel()
	const value = "DupVal-Zq41Wx93"
	_, e := honestEnv(t, map[string]string{"/api/v1/buildings/": `{"id":"1","password":"` + value + `","password":null,"info":"` + value + `"}`})
	if out := e.run(t, "buildings", "get", "1", "--json", "--data-source", "live"); strings.Contains(out, value) {
		t.Fatalf("a duplicate member's value printed:\n%.800s", out)
	}
	_, e = honestEnv(t, map[string]string{"/api/v1/buildings/": `{"id":"1","pin":"7391","pin":null,"name":"Rec"}`})
	if out := e.run(t, "buildings", "get", "1", "--json", "--data-source", "live"); strings.Contains(out, "7391") || !strings.Contains(out, "Rec") {
		t.Fatalf("a duplicate PIN printed or the record was lost:\n%.800s", out)
	}
}

// A PIN the operator supplies is one value they chose to send: a preview shows
// nothing of it, as a string, a number or a flag.
func TestOperatorPINIsHeld(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	for _, pin := range []any{"7391", 7391} {
		out := e.mcpCalls(t, []map[string]any{{"name": "jamf-pro_execute", "arguments": map[string]any{
			"endpoint_id": "buildings.update", "params": map[string]any{"id": "1", "name": "HQ", "pin": pin}}}})
		if strings.Contains(out, "7391") {
			t.Fatalf("an MCP preview printed the PIN (%T):\n%.800s", pin, out)
		}
	}
	if out := e.run(t, "computer-inventory", "erase", "1", "--pin", "739104", "--dry-run"); strings.Contains(out, "739104") {
		t.Fatalf("a CLI preview printed the PIN:\n%.800s", out)
	}
}

// A listing's own PINs stay out by name and do not take the listing with them.
func TestListingPINsAreReplacedNotHeld(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	out := e.run(t, "sites", "list", "--json", "--data-source", "live")
	if strings.Contains(out, bbWithheld) || strings.Contains(out, `"lockPin": "`+bbPIN) || !strings.Contains(out, "Clean-Site") {
		t.Fatalf("want the listing with its PINs replaced by name:\n%.800s", out)
	}
}

// A credential on disk belongs to the server it was saved for: naming another
// server sends it nothing, and its own server still works.
func TestSavedCredentialStaysWithItsServer(t *testing.T) {
	t.Parallel()
	a, b := echoServer(t), echoServer(t)
	a.honest.Store(true)
	b.honest.Store(true)
	both := filepath.Join(t.TempDir(), "both.pem")
	ca, _ := os.ReadFile(a.caFile)
	cb, _ := os.ReadFile(b.caFile)
	if err := os.WriteFile(both, append(ca, cb...), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, a, false)
	pointed := func(extra ...string) *bbEnv {
		return &bbEnv{home: e.home, env: append(without(e.env, "JAMF_PRO_SERVER=", "JAMF_PRO_CA_CERT="), append(extra, "JAMF_PRO_CA_CERT="+both)...)}
	}
	get := []string{"jamf-pro-version", "--json", "--data-source", "live"}
	e.run(t, "auth", "login", "--client-id", "client-flag-0001", "--client-secret", bbFlagSecret)
	if out := e.run(t, get...); !strings.Contains(out, "11.12.0") {
		t.Fatalf("the credential stopped working on its own server:\n%.800s", out)
	}
	for _, env := range [][]string{
		{"JAMF_PRO_SERVER=" + b.host},
		{"JAMF_PRO_SERVER=" + a.host, "JAMF_PRO_BASE_URL=https://" + b.host},
	} {
		out := pointed(env...).run(t, get...)
		if n := b.hits.Load(); n != 0 && strings.Contains(out, bbFlagSecret) {
			t.Fatalf("%v: printed the credential:\n%.800s", env, out)
		}
		if b.saw(bbFlagSecret) || b.saw(bbMinted) {
			t.Fatalf("%v: another server received the saved credential:\n%.800s", env, out)
		}
	}
	// A config file's secret, token and credential header, under --config.
	cfg := filepath.Join(e.home, "profile.toml")
	body := "base_url = \"https://" + a.host + "\"\nclient_id = \"client-flag-0001\"\nclient_secret = \"" + bbFlagSecret + "\"\naccess_token = \"" + bbMinted + "\"\n[headers]\nX-Api-Key = \"" + bbTenantPass + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := pointed("JAMF_PRO_SERVER="+b.host).run(t, append(get, "--config", cfg)...)
	if b.saw(bbFlagSecret) || b.saw(bbMinted) || b.saw(bbTenantPass) {
		t.Fatalf("a config file's credential reached another server:\n%.800s", out)
	}
}

// One run's path context never chooses what the config file holds.
func TestPathContextStaysOutOfTheConfigFile(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	e.mcpCalls(t, []map[string]any{{"name": "sync", "arguments": map[string]any{"resources": "sites", "path-context": "server=attacker.example"}}})
	if out := e.run(t, "jamf-pro-version", "--json", "--data-source", "live"); !strings.Contains(out, "11.12.0") {
		t.Fatalf("the next start did not work:\n%.800s", out)
	}
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".toml") {
			if data, _ := os.ReadFile(path); strings.Contains(string(data), "attacker.example") {
				t.Errorf("%s holds the path context:\n%s", path, data)
			}
		}
		return nil
	})
}

// A number under a credential name is a credential; every member of a duplicate
// key on stdin is held; a body that does not parse prints no byte of it.
func TestStdinBodyHeldWhole(t *testing.T) {
	t.Parallel()
	const first, last = "Fa71cQ93xZ", "Lb20dW84yU"
	_, e := honestEnv(t, map[string]string{"/api/v3/computer-prestages": `{"id":"1","note":"` + first + `"}`})
	update := []string{"computer-prestages", "update", "1", "--stdin"}
	if out := e.runIn(t, `{"password":12345678901}`, append(update, "--dry-run")...); strings.Contains(out, "12345678901") {
		t.Fatalf("a numeric password printed:\n%.800s", out)
	}
	body := `{"password":"` + first + `","password":"` + last + `"}`
	if out := e.runIn(t, body, append(update, "--yes", "--json")...); strings.Contains(out, first) {
		t.Fatalf("the first member of a duplicate key printed:\n%.800s", out)
	}
	if out := e.runIn(t, `{"password":"Zebra7","bad":Zebra7}`, append(update, "--dry-run")...); strings.Contains(out, "Zebra") || strings.Contains(out, "'Z'") {
		t.Fatalf("a malformed body's byte printed:\n%.800s", out)
	}
}

// fakeProxy records what each connection sends first, and answers nothing.
func fakeProxy(t *testing.T) (addr string, heads func() string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	var seen strings.Builder
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					mu.Lock()
					seen.WriteString(line)
					mu.Unlock()
					if err != nil || line == "\r\n" {
						break
					}
				}
				_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
			}(c)
		}
	}()
	return ln.Addr().String(), func() string {
		mu.Lock()
		defer mu.Unlock()
		return seen.String()
	}
}

// A proxy is a second recipient. One whose own URL carries the client secret is
// never handed it; one with no credential still gets the tunnel request, so the
// first half is not passing because nothing was sent at all.
func TestProxyNeverReceivesAHeldCredential(t *testing.T) {
	t.Parallel()
	const host = "jamf.example.test"
	run := func(proxyAuth string, args ...string) string {
		t.Helper()
		addr, heads := fakeProxy(t)
		_, e := honestEnv(t, nil)
		over := &bbEnv{home: e.home, env: append(without(e.env, "JAMF_PRO_SERVER="), "JAMF_PRO_SERVER="+host, "NO_PROXY=", "no_proxy=",
			"HTTPS_PROXY=http://"+proxyAuth+addr)}
		out := over.run(t, args...)
		assertNoEcho(t, "proxy run output", out)
		return heads()
	}
	for _, args := range [][]string{
		{"jamf-pro-version", "--json", "--data-source", "live"},
		{"auth", "login"},
	} {
		if got := run("u:"+bbSecret+"@", args...); strings.Contains(strings.ToLower(got), "proxy-authorization") {
			t.Errorf("%v: the proxy received a held credential:\n%s", args, got)
		}
		if got := run("", args...); !strings.HasPrefix(got, "CONNECT "+host+":443 ") {
			t.Errorf("%v: a proxy with no credential did not get the tunnel request:\n%q", args, got)
		}
	}
}

// A config the operator rewrites while the MCP server runs is read again by the
// next call, so its values are held before the parser can quote them.
func TestMCPConfigRewrittenWhileRunningIsHeldBeforeParse(t *testing.T) {
	t.Parallel()
	const secret = "123456789012345678901234567890"
	_, e := honestEnv(t, nil)
	cfg := filepath.Join(e.home, "profile.toml")
	if err := os.WriteFile(cfg, []byte("client_id = \"client-single-0001\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bbMCP)
	cmd.Env = append(append([]string{}, e.env...), "JAMF_PRO_CONFIG="+cfg)
	cmd.Dir = e.home
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_, _ = io.Copy(io.Discard, stdout)
		_ = cmd.Wait()
	}()
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	var frames strings.Builder
	await := func(id string) {
		t.Helper()
		for sc.Scan() {
			frames.WriteString(sc.Text() + "\n")
			var msg struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal(sc.Bytes(), &msg) == nil && string(msg.ID) == id {
				return
			}
		}
		t.Fatalf("the MCP server never answered id %s:\n%s\n%s", id, frames.String(), errb.String())
	}
	send := func(m string) {
		t.Helper()
		if _, err := io.WriteString(stdin, m+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	await("1")
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if err := os.WriteFile(cfg, []byte("client_secret = "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"jamf-pro_read","arguments":{"endpoint_id":"jamf-pro-version.get"}}}`)
	await("2")
	if strings.Contains(frames.String()+errb.String(), secret) {
		t.Errorf("a rewritten config value printed:\n%s\n%s", frames.String(), errb.String())
	}
}

// doctor is the command every guide names. Its "credentials" row is a verdict,
// not a credential: judged as one, the verdict was held and the whole report
// came back as the kit's notice, with or without any credential configured.
func TestDoctorJSONPrintsItsReport(t *testing.T) {
	t.Parallel()
	for name, env := range map[string][]string{
		"nothing configured": nil,
		"environment":        {"JAMF_PRO_SERVER=doctor.example.invalid", "JAMF_PRO_CLIENT_ID=doctor-client", "JAMF_PRO_CLIENT_SECRET=" + bbSecret},
	} {
		cmd := exec.Command(bbCLI, "doctor", "--json")
		cmd.Env = append([]string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH"), "NO_COLOR=1", "SystemRoot=" + os.Getenv("SystemRoot")}, env...)
		cmd.Env = append(cmd.Env, "USERPROFILE="+t.TempDir())
		out, _ := cmd.Output()
		var report map[string]any
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatalf("%s: doctor --json is not its report: %v\n%s", name, err, out)
		}
		if _, ok := report["credentials"]; !ok {
			t.Fatalf("%s: no credentials row: %s", name, out)
		}
		if strings.Contains(string(out), bbSecret) {
			t.Fatalf("%s: the report printed the secret", name)
		}
	}
}

// An auth_header written by hand next to its base_url goes to that server and no
// other. Doctor proves the install with it, and a different server chosen by the
// environment never sees it.
func TestHandWrittenHeaderStaysWithItsBaseURL(t *testing.T) {
	t.Parallel()
	a, b := echoServer(t), echoServer(t)
	a.honest.Store(true)
	b.honest.Store(true)
	both := filepath.Join(t.TempDir(), "both.pem")
	ca, _ := os.ReadFile(a.caFile)
	cb, _ := os.ReadFile(b.caFile)
	if err := os.WriteFile(both, append(ca, cb...), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, a, false)
	base := without(e.env, "JAMF_PRO_SERVER=", "JAMF_PRO_CA_CERT=", "JAMF_PRO_CLIENT_ID=", "JAMF_PRO_CLIENT_SECRET=")
	pointed := func(extra ...string) *bbEnv {
		return &bbEnv{home: e.home, env: append(append([]string{}, base...), append(extra, "JAMF_PRO_CA_CERT="+both)...)}
	}
	const header = "Bearer 5k2w5k2w5k2w5k2w5k2w"
	cfg := filepath.Join(e.home, "pair.toml")
	body := "base_url = \"https://" + a.host + "\"\nauth_header = \"" + header + "\"\n"
	if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out := pointed().run(t, "doctor", "--json", "--config", cfg)
	if !a.saw("5k2w5k2w5k2w5k2w5k2w") {
		t.Fatalf("the hand-written header never reached its own server:\n%.800s", out)
	}
	if !strings.Contains(out, "valid (verified with GET /api/v1/jamf-pro-version)") {
		t.Fatalf("doctor did not prove the header:\n%.800s", out)
	}
	out = pointed("JAMF_PRO_SERVER="+b.host).run(t, "jamf-pro-version", "--json", "--data-source", "live", "--config", cfg)
	if b.saw("5k2w5k2w5k2w5k2w5k2w") {
		t.Fatalf("another server received the hand-written header:\n%.800s", out)
	}
}
