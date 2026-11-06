// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (maxio; handfixes.json credkit-wiring): one test per
// credential class the factory's Codex round 2 left open on the connector's
// old hand-built layer, each driven from its recorded failing input against
// the built binaries. The shared credential kit closes each class; the
// declarations each depends on are in internal/credwire/declarations.go.

package maxioblackbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Class holder-incomplete-on-outbound-refusal. Factory r2 failing input: with
// empty configuration, `version --deliver
// 'webhook:http://127.0.0.1:18080/?token=NewQuerySecretq7'` printed the token
// in the delivery warning. The token is a declared request credential field
// (CredentialFields: token), held as the request passes the outbound guard,
// before the refusal and before the warning: the warning is withheld, and no
// request leaves for a plain-http or foreign target.
func TestKitClassHolderIncompleteOnOutboundRefusal(t *testing.T) {
	const secret = "NewQuerySecretq7"
	f := newFake(t)
	home := t.TempDir()
	for _, target := range []string{
		"http://" + freeAddr(t) + "/?token=" + secret, // the recorded input: nothing listens
		f.plain.URL + "/?token=" + secret,             // plain http, a listener
		f.evil.URL + "/hook?token=" + secret,          // https, another origin than the API
	} {
		out, _ := runWith(t, baseEnv(home), "", "version", "--deliver", "webhook:"+target)
		assertNoneOf(t, "version --deliver webhook:"+target, out, secret)
	}
	if n := f.plainHits.Load() + f.evilHits.Load(); n != 0 {
		t.Errorf("%d webhook request(s) carrying the token were sent; the guard must refuse them first", n)
	}
	f.assertOthersSawNothing(t)
}

// Class credential-sent-over-plaintext-http. Factory r2 failing input:
// `version --deliver 'webhook:http://redacted:redacted@127.0.0.1:18080/'`
// with a listener sent Basic authorization over plain http (r1:
// operator:NewHookSecretq7). Also the API key itself, when the base URL is
// plain http. No credential may cross plain http at all.
func TestKitClassCredentialSentOverPlaintextHTTP(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	hostPort := strings.TrimPrefix(f.plain.URL, "http://")
	for _, target := range []string{
		"http://redacted:redacted@" + hostPort + "/",
		"http://operator:NewHookSecretq7@" + hostPort + "/",
	} {
		out, _ := runWith(t, baseEnv(home), "", "version", "--deliver", "webhook:"+target)
		assertNoneOf(t, "version --deliver webhook:"+target, out, "NewHookSecretq7")
	}
	plainAPI := append(baseEnv(home), "MAXIO_API_KEY="+apiKey, "MAXIO_BASE_URL="+f.plain.URL)
	out, _ := runWith(t, plainAPI, "", "customers", "read", plainID, "--json")
	assertNoneOf(t, "customers read over a plain-http base URL", out, apiKey)
	f.assertOthersSawNothing(t)
}

// Class credential-persisted-outside-the-store. Factory r2 failing input: a
// 0600 config with base_url "https://operator:Qf8Lr3Nw6Bt9Zs2C@acme.chargify.com"
// and [headers] X-Trace = "Qf8Lr3Nw6Bt9Zs2C", then `auth set-token --config
// <file>` with stdin Zx7Qm2Vp9Kw4Rt6Y. The config file and the 0600
// credential store are the connector's declared credential sources (the kit
// holds both at every start, and their own writes are its one exemption):
// the run must leave both 0600 and put no credential in any other file. The
// same class on the program's other files: an export --out file, a --deliver
// file, the response cache, the snapshot store and the learn journal must
// never hold the key an answer repeats.
func TestKitClassCredentialPersistedOutsideTheStore(t *testing.T) {
	const urlPassword, newToken = "Qf8Lr3Nw" + "6Bt9Zs2C", "Zx7Qm2Vp" + "9Kw4Rt6Y" // split so the literal is no scanner hit
	f := newFake(t)
	home := t.TempDir()

	cfg := filepath.Join(home, "profile", "config.toml")
	writePrivate(t, cfg, "base_url = \"https://operator:"+urlPassword+"@acme.chargify.com\"\n[headers]\nX-Trace = \""+urlPassword+"\"\n")
	out, _ := runWith(t, baseEnv(home), newToken+"\n", "auth", "set-token", "--config", cfg)
	assertNoneOf(t, "auth set-token --config", out, urlPassword, newToken)

	exportOut := filepath.Join(home, "exports", "rows.jsonl")
	csvOut := filepath.Join(home, "exports", "rows.csv")
	delivered := filepath.Join(home, "exports", "delivered.json")
	if err := os.MkdirAll(filepath.Dir(exportOut), 0o700); err != nil {
		t.Fatal(err)
	}
	f.listEcho.Store(true)
	for _, args := range [][]string{
		{"sync", "--resources", "customers-json", "--json"},
		{"export", "customers-json", "--format", "jsonl", "--output", exportOut},
		{"export", "customers-json", "--format", "csv", "--output", csvOut},
		{"customers", "read", echoID, "--json", "--deliver", "file:" + delivered},
		{"customers", "read", echoID, "--json"},
	} {
		out, _ := runWith(t, env(home, f), "", args...)
		assertNoneOf(t, fmt.Sprint(args), out, apiKey)
	}
	if f.apiHits.Load() == 0 {
		t.Fatal("no request reached the API, so the file sinks were never offered the echoed key")
	}
	scanFiles(t, home, apiKey, urlPassword, newToken)
}

// runMCPOnce starts the MCP server over stdio, writes the frames, closes
// stdin and returns stdout and stderr once the server ends.
func runMCPOnce(t *testing.T, environ []string, frames ...string) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe("maxio-mcp"))
	cmd.Env = environ
	cmd.Stdin = strings.NewReader(strings.Join(frames, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("the MCP server did not end within %v:\n%s\n%s", runWait, stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

const initializeFrame = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"blackbox","version":"1"}}}`

// Class replacement-protocol-frame-leaks-credentials. Factory r2 failing
// input: stdio MCP with MAXIO_API_KEY=jsonrpc MAXIO_MCP_HTTP_TOKEN=withheld
// answered initialize with frames that carry the key ("jsonrpc"); HTTP mode
// with MAXIO_API_KEY=unauthorized answered an unauthenticated GET /mcp with
// the key as its 401 body. A startup credential equal to public text (the
// JSON-RPC envelope, an HTTP reason phrase, the kit's own notice) refuses to
// start, naming only its source, so no frame or body can carry it.
func TestKitClassReplacementProtocolFrameLeaksCredentials(t *testing.T) {
	home := t.TempDir()
	stdout, stderr := runMCPOnce(t, append(baseEnv(home), "MAXIO_API_KEY=jsonrpc", "MAXIO_MCP_HTTP_TOKEN=withheld"), initializeFrame)
	for _, v := range []string{"jsonrpc", "withheld"} {
		if containsToken(stdout, v) || containsToken(stderr, v) {
			t.Errorf("stdio MCP with a credential equal to %q printed it:\nstdout:\n%.1500s\nstderr:\n%.1500s", v, stdout, stderr)
		}
	}

	addr := freeAddr(t)
	const bearer = "HttpBearer-5Lp9Qx2Vm7-Rt3Wn8Ky4Bd6Zh1"
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, exe("maxio-mcp"), "--transport", "http", "--addr", addr)
	cmd.Env = append(baseEnv(home), "MAXIO_API_KEY=unauthorized", "MAXIO_MCP_HTTP_TOKEN="+bearer)
	var serverOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &serverOut, &serverOut
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	// The process this test started, by PID (CommandContext kills it).
	t.Cleanup(func() { cancel(); <-exited })
	var bodies strings.Builder
	listening := false
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline) && !listening; time.Sleep(50 * time.Millisecond) {
		select {
		case <-exited:
			deadline = time.Now() // refused to start: nothing to ask
		default:
		}
		if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			_ = c.Close()
			listening = true
		}
	}
	if listening {
		hc := &http.Client{Timeout: 30 * time.Second}
		for _, auth := range []string{"", "Bearer wrong-token-of-enough-length-000000"} {
			req, _ := http.NewRequest(http.MethodGet, "http://"+addr+"/mcp", nil)
			if auth != "" {
				req.Header.Set("Authorization", auth)
			}
			resp, err := hc.Do(req)
			if err != nil {
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			var head strings.Builder
			_ = resp.Header.Write(&head)
			fmt.Fprintf(&bodies, "%s\n%s\n%s\n", resp.Status, head.String(), body)
		}
	}
	cancel()
	<-exited
	text := bodies.String() + "\n" + serverOut.String()
	if strings.Contains(strings.ToLower(text), "unauthorized") {
		t.Errorf("HTTP MCP with MAXIO_API_KEY=unauthorized sent the key:\n%.2000s", text)
	}
}

// Class holder-incomplete-for-malformed-toml. Factory r1 failing input: a
// 0600 config holding `api_key = """\nLeakProbeKeyq7xyz"""\nbad = [\n`, then
// `MAXIO_CONFIG=<file> maxio-cli LeakProbeKeyq7xyz` printed the key in
// the unknown-command error. The kit reads a config that does not parse
// token by token. The r2 shapes (an unterminated array BEFORE the key,
// kit-findings row 69) are read afresh by the merged kit's
// credentialAssignmentsLocked because api_key is a declared ConfigFields key.
func TestKitClassHolderIncompleteForMalformedTOML(t *testing.T) {
	const key = "LeakProbe" + "Keyq7xyz" // split so the literal is no scanner hit
	home := t.TempDir()
	for i, content := range []string{
		"api_key = \"\"\"\n" + key + "\"\"\"\nbad = [\n",
		"bad = [\napi_key = \"\"\"\n" + key + "\"\"\"\n",
		"bad = [\napi_key = \"" + key + "\"\n",
		"api_key = '''\n" + key + "\n'''\nbroken = [\n",
		"api_key = \"" + key + "\"#c\nbroken = \"\n",
		"api_key=\"" + key + "\"\nbase_url=42\n",
		"[headers]\nX-Api = \"" + key + "\"\nbroken = =\n",
	} {
		cfg := filepath.Join(home, fmt.Sprintf("bad-%d.toml", i))
		writePrivate(t, cfg, content)
		// The recorded input names the file in MAXIO_CONFIG; --config is
		// the other way to select it.
		out, _ := runWith(t, append(baseEnv(home), "MAXIO_CONFIG="+cfg), "", key)
		assertNoneOf(t, fmt.Sprintf("MAXIO_CONFIG with %q", content), out, key)
		out, _ = runWith(t, baseEnv(home), "", "--config", cfg, key)
		assertNoneOf(t, fmt.Sprintf("--config with %q", content), out, key)
	}
}

// Class one-character-credential-leaks. Factory r2 failing input:
// `MAXIO_API_KEY=x maxio-cli '@x@'` printed `@x@`, the whole credential
// between two characters that are not letters or digits (contract 2(a)).
func TestKitClassOneCharacterCredentialLeaks(t *testing.T) {
	home := t.TempDir()
	for _, arg := range []string{"@x@", "x", "-x-"} {
		out, _ := runWith(t, append(baseEnv(home), "MAXIO_API_KEY=x"), "", arg)
		if containsToken(out, "x") {
			t.Errorf("MAXIO_API_KEY=x maxio-cli %q printed the key as a whole token:\n%s", arg, out)
		}
	}
}

// Class real-credential-mistaken-for-an-encoded-needle. Factory r2 failing
// input: a 0600 config with access_token = "MFRGGZDFMZTW" (the base32 of
// another held value) and MAXIO_API_KEY=abcdefgh, then `maxio-cli
// MFRGGZDF` printed an 8-character piece of the config token. Every held
// value is matched as itself, whatever else it also spells.
func TestKitClassRealCredentialMistakenForAnEncodedNeedle(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "r2.toml")
	writePrivate(t, cfg, "access_token = \"MFRGGZDFMZTW\"\n")
	out, _ := runWith(t, append(baseEnv(home), "MAXIO_API_KEY=abcdefgh", "MAXIO_CONFIG="+cfg), "", "MFRGGZDF")
	if strings.Contains(out, "MFRGGZDF") {
		t.Errorf("an 8-character piece of the config token printed:\n%s", out)
	}
}

// Class credential-provenance-never-promoted. Factory r2 failing input: in one
// MCP process, maxio_read of a customer answering {"password":<value>},
// then tools/call of the unknown tool "Dm7Jz4Eg9Xi2Kc6A" (the ROT13 of that
// value) with arguments {"api_key":<value>}: the unknown-tool
// error printed the ROT13 of the now-credential value. A declared credential
// argument (MCPArguments: api_key) is held before the server reads the frame,
// whatever answer carried the same value first.
func TestKitClassCredentialProvenanceNeverPromoted(t *testing.T) {
	const value, rotated = "Qz7Wm4Rt" + "9Kv2Xp6N", "Dm7Jz4Eg" + "9Xi2Kc6A"
	f := newFake(t)
	s := startMCP(t, env(t.TempDir(), f))
	s.call("maxio_read", map[string]any{"endpoint_id": "customers.read", "params": map[string]any{"id": provID}})
	answer := s.send("tools/call", map[string]any{"name": rotated, "arguments": map[string]any{"api_key": value}})
	if strings.Contains(answer, rotated) || strings.Contains(answer, value) {
		t.Errorf("the unknown-tool answer repeats the credential argument: %s", answer)
	}
	if !strings.Contains(answer, `"error"`) && !strings.Contains(answer, kitNotice) {
		t.Errorf("an unknown tool must answer with an error or be withheld: %s", answer)
	}
	// The server keeps answering after a withheld frame.
	if res := s.call("maxio_read", map[string]any{"endpoint_id": "customers.read", "params": map[string]any{"id": plainID}}); !strings.Contains(res, "Second row") {
		t.Errorf("the server stopped answering after the probe: %s", res)
	}
}

// Kit item 8(c): a saved profile's value under a credential flag's name or
// under base-url is a credential source, held at startup, every member of a
// duplicate key included, wherever the program reads profiles.json from.
func TestKitSavedProfileCredentialsAreHeld(t *testing.T) {
	const first, last = "ProfileFirst-7c2dQ", "ProfileLast-91abW"
	const base = "https://profile-base.example"
	body := `{"profiles":{"p":{"name":"p","values":{"api-key":"` + first + `","api-key":"` + last +
		`","base-url":"` + base + `","select":"KeptColumn-Number"}}}}`
	f := newFake(t)
	home := t.TempDir()
	other := filepath.Join(home, "elsewhere")
	for name, tc := range map[string]struct {
		path string
		args []string
	}{
		"config dir": {filepath.Join(home, "mh", "config", "profiles.json"), nil},
		"legacy":     {filepath.Join(home, ".maxio-cli", "profiles.json"), nil},
		"--home":     {filepath.Join(other, "config", "profiles.json"), []string{"--home", other}},
	} {
		writePrivate(t, tc.path, body)
		out, _ := runWith(t, env(home, f), "", append(tc.args, "profile", "show", "p", "--json")...)
		assertNoneOf(t, name+": profile show", out, first, last, base)
		if err := os.Remove(tc.path); err != nil {
			t.Fatal(err)
		}
	}
	// A value under no credential name is not a credential.
	writePrivate(t, filepath.Join(home, "mh", "config", "profiles.json"), `{"profiles":{"p":{"name":"p","values":{"select":"KeptColumn-Number"}}}}`)
	if out, _ := runWith(t, env(home, f), "", "profile", "show", "p", "--json"); !strings.Contains(out, "KeptColumn-Number") {
		t.Errorf("a saved profile's plain value was held:\n%.800s", out)
	}
}

// Kit item 8(b): the key goes to the base URL of the config the program
// really reads. An explicit --config or MAXIO_CONFIG wins over the
// discovered config.toml, whose base_url names another host here: the
// request reaches the selected base with the key, and the other host
// receives none.
func TestKitCredentialsReachTheSelectedConfigBase(t *testing.T) {
	f := newFake(t)
	home := t.TempDir()
	writePrivate(t, filepath.Join(home, "mh", "config", "config.toml"), "base_url = \""+f.evil.URL+"\"\n")
	explicit := filepath.Join(home, "explicit.toml")
	writePrivate(t, explicit, "base_url = \""+f.srv.URL+"\"\n")
	creds := append(baseEnv(home), "MAXIO_API_KEY="+apiKey)
	for name, run := range map[string]func() (string, int){
		"--config": func() (string, int) {
			return runWith(t, creds, "", "--config", explicit, "customers", "read", plainID, "--json")
		},
		"MAXIO_CONFIG": func() (string, int) {
			return runWith(t, append(creds, "MAXIO_CONFIG="+explicit), "", "customers", "read", plainID, "--json")
		},
	} {
		if out, code := run(); code != 0 || !strings.Contains(out, "Second row") {
			t.Errorf("%s: the selected base did not answer (exit %d):\n%s", name, code, out)
		}
	}
	if n := f.evilHits.Load(); n != 0 {
		t.Errorf("the discovered config's host was reached %d time(s)", n)
	}
	f.assertOthersSawNothing(t)
}

// The MCP HTTP server's own refusals (Origin, bearer, body limit) and the
// protocol layer's unknown-tool error are written through the kit
// (handfixes.json credkit-mcphttp-outermost): an unknown tool named after
// the API key, or after a credential only its own arguments carry, is never
// echoed.
func TestKitMCPUnknownToolNeverEchoesHTTP(t *testing.T) {
	f := newFake(t)
	const token = "HttpBearer-5Lp9Qx2Vm7-Rt3Wn8Ky4Bd6Zh1"
	const fresh = "FreshArg-7Qx2Vb9Lm4"
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, exe("maxio-mcp"), "--transport", "http", "--addr", addr)
	cmd.Env = append(env(t.TempDir(), f), "MAXIO_MCP_HTTP_TOKEN="+token)
	stderr := &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stderr, stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	// The process this test started, by PID (CommandContext kills it).
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if c, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the MCP HTTP server never listened on %s:\n%s", addr, stderr)
		}
	}
	hc := &http.Client{Timeout: 60 * time.Second}
	var transcript strings.Builder
	post := func(session, auth, body string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", auth)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
		resp, err := hc.Do(req)
		if err != nil {
			t.Fatalf("POST %.80s: %v", body, err)
		}
		payload, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var head strings.Builder
		_ = resp.Header.Write(&head)
		fmt.Fprintf(&transcript, "%d\n%s\n%s\n", resp.StatusCode, head.String(), payload)
		return resp
	}
	// A refusal: a bearer that is not the server's.
	if resp := post("", "Bearer "+apiKey, `{}`); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a wrong bearer must be refused with 401, got %d", resp.StatusCode)
	}
	init := post("", "Bearer "+token, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	session := init.Header.Get("Mcp-Session-Id")
	if session == "" {
		t.Fatalf("initialize returned no session id:\n%s", transcript.String())
	}
	post(session, "Bearer "+token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for i, p := range []map[string]any{
		{"name": apiKey, "arguments": map[string]any{}},
		{"name": fresh, "arguments": map[string]any{"api_key": fresh}},
	} {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": p})
		post(session, "Bearer "+token, string(b))
	}
	text := transcript.String()
	assertNoneOf(t, "MCP HTTP responses", text, apiKey, fresh)
	assertNoneOf(t, "MCP HTTP stderr", stderr.String(), apiKey, fresh)
}
