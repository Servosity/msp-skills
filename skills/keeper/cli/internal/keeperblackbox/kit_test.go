// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeperblackbox

// The credential kit on the built binaries, beyond the named classes: MCP HTTP, the
// whole-unit rule, what must still print, a proxy, stores and the plain-http guard.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// serveMCPHTTP starts keeper-mcp over HTTP on a loopback port and returns its address
// once it accepts connections.
func serveMCPHTTP(t *testing.T, e *bbEnv) (addr string, stderr *lockedBuffer) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr = l.Addr().String()
	_ = l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bbMCP, "--transport", "http", "--addr", addr)
	cmd.Env, cmd.Dir = e.env, e.home
	stderr = &lockedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			_ = c.Close()
			return addr, stderr
		}
	}
	t.Fatalf("the MCP HTTP server never listened on %s:\n%s", addr, stderr.String())
	return "", nil
}

// rawExchange writes request to addr as is and returns every byte of the answer.
func rawExchange(t *testing.T, addr, request string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err := io.WriteString(c, request); err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(c)
	return string(b)
}

func httpPost(token, session, body string) string {
	head := "POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\n"
	if token != "" {
		head += "Authorization: Bearer " + token + "\r\n"
	}
	if session != "" {
		head += "Mcp-Session-Id: " + session + "\r\n"
	}
	return head + fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body)) + body
}

const mcpInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

// The server's refusals answer with fixed text, and a bearer token that carries that
// text holds it, so a refusal passes the kit like every other response. A refused
// request carries the text only as the token's piece, so the absence of the piece says
// the refusal went through the kit, and the 4xx says it was still refused.
func TestKitMCPHTTPRefusalsPassTheKit(t *testing.T) {
	for _, tc := range []struct{ name, token, piece, request string }{
		{"no bearer", "Qz7unauthorizedWm4kP9xL2vB8nR5tY3", "unauthorized",
			"GET /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAccept: text/event-stream\r\n\r\n"},
		{"foreign Origin", "Hd3forbidden originJw6Ns1Xc4Gt7Yp2", "forbidden",
			"POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nOrigin: https://evil.example\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}"},
		{"declared oversized body", "Fm2request body too largeKa8Lq5Zr1", "too large",
			"POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAuthorization: Bearer Fm2request body too largeKa8Lq5Zr1\r\nContent-Type: application/json\r\nContent-Length: 104857600\r\n\r\n"},
		{"a body that cannot be read", "Tb6reading request bodyMe9Vx3Cu7Sw4", "reading request",
			"POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAuthorization: Bearer Tb6reading request bodyMe9Vx3Cu7Sw4\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\n{}\r\n0\r\n\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sv := echoServer(t)
			addr, stderr := serveMCPHTTP(t, newEnv(t, sv, true).with("KEEPER_MCP_HTTP_TOKEN="+tc.token))
			resp := rawExchange(t, addr, tc.request)
			if strings.Contains(strings.ToLower(resp), tc.piece) {
				t.Errorf("the refusal carries %q, a piece of the held bearer token:\n%q", tc.piece, resp)
			}
			if !strings.HasPrefix(resp, "HTTP/1.1 4") {
				t.Errorf("the request was not refused:\n%q\nstderr:\n%s", resp, stderr.String())
			}
			if strings.Contains(stderr.String(), tc.token) {
				t.Errorf("stderr carries the bearer token:\n%s", stderr.String())
			}
		})
	}
}

// A refused request reaches the kit too, and must not silence the server for the next,
// ordinary one.
func TestKitMCPHTTPRefusalsLeaveTheServerAnswering(t *testing.T) {
	sv := echoServer(t)
	addr, stderr := serveMCPHTTP(t, newEnv(t, sv, true))
	for _, req := range []string{
		"POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nContent-Type: text/plain; charset=latin1\r\nContent-Encoding: gzip\r\nContent-Length: 4\r\n\r\n\xef\xbb\xbfx",
		"POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAuthorization: Bearer " + bbHTTPToken + "\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\n{}\r\n0\r\n\r\n",
	} {
		if resp := rawExchange(t, addr, req); !strings.HasPrefix(resp, "HTTP/1.1 4") {
			t.Fatalf("the request was not refused:\n%q\nstderr:\n%s", resp, stderr.String())
		}
	}
	resp := rawExchange(t, addr, httpPost(bbHTTPToken, "", mcpInitialize))
	if !strings.HasPrefix(resp, "HTTP/1.1 200") || !strings.Contains(resp, `"protocolVersion"`) {
		t.Fatalf("an ordinary initialize after the refusals was not answered:\n%q\nstderr:\n%s", resp, stderr.String())
	}
}

// A tool result over streamable HTTP: an ordinary read prints whole, and one that
// repeats the key is replaced by the notice.
func TestMCPHTTPToolResultWithheld(t *testing.T) {
	sv := echoServer(t)
	sv.honest.Store(true)
	addr, stderr := serveMCPHTTP(t, newEnv(t, sv, true))
	head := rawExchange(t, addr, httpPost(bbHTTPToken, "", mcpInitialize))
	session := ""
	for _, line := range strings.Split(head, "\r\n") {
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(k, "Mcp-Session-Id") {
			session = strings.TrimSpace(v)
		}
	}
	call := func() string {
		return rawExchange(t, addr, httpPost(bbHTTPToken, session, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"companies_list","arguments":{}}}`))
	}
	if out := call(); !strings.Contains(out, "HQ-Record") || strings.Contains(out, bbWithheld) {
		t.Errorf("an ordinary read over MCP HTTP did not print:\n%.1500s\nstderr:\n%s", out, stderr.String())
	}
	sv.show(bbToken[4:20])
	out := call()
	noPiece(t, "MCP HTTP result", out, bbToken)
	if !strings.Contains(out, bbWithheld) || strings.Contains(out, "HQ-Record") {
		t.Errorf("a result that repeats the key was not withheld whole:\n%.1500s", out)
	}
}

// A refused HTTP start exits non-zero and its reason reaches the operator: the program
// leaves through credkit.Exit, which drains the kit's stderr.
func TestMCPRefusedHTTPStartPrintsItsReason(t *testing.T) {
	e := newEnv(t, echoServer(t), true)
	for _, tc := range []struct {
		name string
		env  *bbEnv
		args []string
		want string
	}{
		{"no token", e.without("KEEPER_MCP_HTTP_TOKEN"), nil, "must be set to a bearer token"},
		{"short token", e.with("KEEPER_MCP_HTTP_TOKEN=short"), nil, "must be set to a bearer token"},
		{"non-loopback without TLS", e, []string{"--addr", "203.0.113.9:18080"}, "requires --tls-cert and --tls-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bbMCP, append([]string{"--transport", "http"}, tc.args...)...)
			cmd.Env, cmd.Dir = tc.env.env, tc.env.home
			b, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("a refused start exited 0:\n%s", b)
			}
			if !strings.Contains(string(b), tc.want) {
				t.Errorf("want %q, got:\n%s", tc.want, b)
			}
		})
	}
}

// Ordinary reports survive whole on both binaries: company and user names, addresses,
// node paths and numeric ids are not held. A kit that withheld them would pass every
// leak test in this package.
func TestRealisticListingsSurviveUnwithheld(t *testing.T) {
	t.Parallel()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cmd := commandOf(body)
		w.Header().Set("Content-Type", "application/json")
		switch strings.Fields(cmd)[0] {
		case "switch-to-msp":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"status":"error","error":"Unexpected error: switch-to-msp: Already MSP"}`)
		case "msp-info":
			fmt.Fprint(w, report(cmd, `{"company_id":887412,"company_name":"Contoso Dental Group, LLC","node":"Contoso Dental Group, LLC\\Clients\\Northeast","plan":"Enterprise Plus","storage":"1TB","addons":["compliance_report","enterprise_audit_and_reporting"],"allocated":45,"active":38}`))
		default:
			fmt.Fprint(w, report(cmd, `{"user_id":7340032991,"email":"rosa.diaz@contoso-dental.example","name":"Rosa Diaz","status":"Active","node":"Contoso Dental Group, LLC\\Front Office","roles":["Keeper Administrator"],"2fa_enabled":true}`))
		}
	}))
	e := newEnv(t, &server{url: srv.URL, caFile: pemFile(t, srv)}, true)
	cli := e.run(t, "companies", "list", "--json") + e.run(t, "users", "list", "--json")
	mcp := e.mcpCalls(t, []map[string]any{
		{"name": "companies_list", "arguments": map[string]any{}},
		{"name": "users_list", "arguments": map[string]any{}},
	})
	for where, text := range map[string]string{"CLI": cli, "MCP": mcp} {
		if strings.Contains(text, bbWithheld) {
			t.Errorf("%s: an ordinary listing was withheld:\n%.1500s", where, text)
		}
		for _, want := range []string{"Contoso Dental Group", "887412", "rosa.diaz@contoso-dental.example", "7340032991", "Keeper Administrator"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s lost %q:\n%.1500s", where, want, text)
			}
		}
	}
}

// The notice replaces a whole unit: stdout is never a document with the key cut out.
func TestWithheldUnitIsWholeNotCutUp(t *testing.T) {
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, true)
	sv.show(bbToken)
	var stdout bytes.Buffer
	cmd := exec.Command(bbCLI, "companies", "list", "--json")
	cmd.Env, cmd.Dir, cmd.Stdout = e.env, e.home, &stdout
	_ = cmd.Run()
	if out := stdout.String(); strings.Contains(out, bbToken) || strings.Contains(out, "HQ-Record") {
		t.Errorf("a unit repeating a held value was not withheld whole:\n%.800s", out)
	}
}

// The binary as shipped has no plain-http seam: the key is refused over plain http even
// to loopback, and the operator is told.
func TestCredentialIsNotSentOverPlainHTTP(t *testing.T) {
	var keyed atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "" {
			keyed.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","command":"msp-info","data":[]}`))
	}))
	t.Cleanup(srv.Close)
	e := newEnv(t, &server{url: srv.URL}, true)
	cli := e.run(t, "companies", "list", "--json")
	mcp := e.mcpCalls(t, []map[string]any{{"name": "companies_list", "arguments": map[string]any{}}})
	if n := keyed.Load(); n != 0 {
		t.Fatalf("the key went over plain http %d time(s):\n%.1000s\n%.1000s", n, cli, mcp)
	}
	for where, out := range map[string]string{"CLI": cli, "MCP": mcp} {
		noPiece(t, where, out, bbToken)
		if !strings.Contains(out, "plain") && !strings.Contains(out, "https") && !strings.Contains(out, bbWithheld) {
			t.Errorf("%s does not say why nothing was read:\n%.1000s", where, out)
		}
	}
}

// fakeProxy records what each connection sends first, and answers nothing useful.
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

// A proxy is a second recipient. One whose own URL carries the held key is never handed
// it; one with no credential still gets the tunnel request, so the first half is not
// passing because nothing was sent.
func TestProxyNeverReceivesAHeldCredential(t *testing.T) {
	const host = "keeper.example.test"
	sv := echoServer(t)
	sv.honest.Store(true)
	run := func(proxyAuth string) string {
		t.Helper()
		addr, heads := fakeProxy(t)
		over := newEnv(t, sv, true).without("KEEPER_BASE_URL").with(
			"KEEPER_BASE_URL=https://"+host, "NO_PROXY=", "no_proxy=", "HTTPS_PROXY=http://"+proxyAuth+addr)
		noPiece(t, "proxy run output", over.run(t, "companies", "list", "--json"), bbToken)
		return heads()
	}
	if got := run("u:" + bbToken + "@"); strings.Contains(strings.ToLower(got), "proxy-authorization") {
		t.Errorf("the proxy received a held credential:\n%s", got)
	}
	if got := run(""); !strings.HasPrefix(got, "CONNECT "+host+":443 ") {
		t.Errorf("a proxy with no credential did not get the tunnel request:\n%q", got)
	}
}

// A config the operator rewrites while the MCP server runs is read again at the first
// call, so its values are held before the parser can quote them.
func TestMCPConfigRewrittenWhileRunningIsHeldBeforeParse(t *testing.T) {
	const secret = "Gn6Wz1X" + "k9Bt4Rd7Jq3Lv"
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, true)
	cfg := filepath.Join(e.home, "profile.toml")
	writeFile(t, cfg, "# nothing yet\n")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bbMCP)
	cmd.Env, cmd.Dir = append(append([]string{}, e.env...), "KEEPER_CONFIG="+cfg), e.home
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
	send(mcpInitialize)
	await("1")
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	writeFile(t, cfg, "api_key = "+secret+"\n") // a bare value: the file no longer parses
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"companies_list","arguments":{}}}`)
	await("2")
	if !strings.Contains(frames.String(), `"isError":true`) {
		t.Fatalf("companies_list did not fail on the unparseable config, so this test proved nothing:\n%s", frames.String())
	}
	noPiece(t, "the MCP frames and stderr", frames.String()+errb.String(), secret)
}

// A store records the base URL its key was issued for. It must not become the base the
// key may go to: a run's own base still receives it.
func TestAStoreRecordingABaseURLDoesNotMoveTheOrigin(t *testing.T) {
	const stored = "Sk8Mf3Wq6Ty1Hz4Lb9Nc"
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, false)
	e.runIn(t, stored+"\n", "auth", "set-token")
	store := filepath.Join(e.home, ".local", "share", "keeper-cli", "credentials.toml")
	data, err := os.ReadFile(store)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, store, "base_url = \"https://issued.example\"\n"+strings.ReplaceAll(string(data), "base_url", "# base_url"))
	out := e.run(t, "companies", "list", "--json")
	if !strings.Contains(out, "HQ-Record") || !sv.sawKey(stored) {
		t.Errorf("a stored key was refused at the run's own base:\n%.1000s", out)
	}
}

// A key named on this run wins over a stored one, on both binaries, including a stale
// auth_header in the config file.
func TestNamedKeyWinsOverAStoredOne(t *testing.T) {
	t.Parallel()
	const stored = "Vy5Dp2Kr8Jc4Tn7Gs1Xm"
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, false)
	e.runIn(t, stored+"\n", "auth", "set-token")
	cfg := filepath.Join(e.home, "stale.toml")
	writeFile(t, cfg, fmt.Sprintf("auth_header = %q\n", stored))
	named := e.with("KEEPER_API_KEY=" + bbToken)
	named.run(t, "companies", "list", "--json")
	named.run(t, "companies", "list", "--json", "--config", cfg)
	named.mcpCalls(t, []map[string]any{{"name": "companies_list", "arguments": map[string]any{}}})
	if !sv.sawKey(bbToken) {
		t.Errorf("the named key never arrived")
	}
	if sv.sawKey(stored) {
		t.Errorf("a stored key was sent though one was named on the run")
	}
}

// A key pasted on the command line is refused, and neither the error nor a parse error
// repeats it; the error prints once. A dashed word (auth set-token --<key>) is not here:
// credwire does not hold it, a positional is not a contract section 1 source, and the
// parser repeats it (reported with this change).
func TestErrorsPrintOnceAndNeverShowTheKey(t *testing.T) {
	const key = "PastedKeyOnTheLine8Zr3Yw"
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, true)
	for i, args := range [][]string{
		{"auth", "set-token", key},
		{"auth", "set-token", "--x=" + key},
		{"auth", "set-token", key, "extra"},
		{"companies", "list", "--" + bbToken},
		{"companies", "list", "--limit=" + bbToken},
		{bbToken},
		{"companies", bbToken, "--json"},
	} {
		out := e.runIn(t, "", args...)
		where := fmt.Sprintf("command line %d", i)
		noPiece(t, where, out, key)
		noPiece(t, where, out, bbToken)
		if n := strings.Count(out, "Error:"); n > 1 {
			t.Errorf("%s printed its error %d times:\n%.800s", where, n, out)
		}
	}
}

// doctor is the operator's first look. Its verdict stays readable with a key held, and
// the report does not repeat the key.
func TestDoctorShowsVerdictInClear(t *testing.T) {
	sv := echoServer(t)
	sv.honest.Store(true)
	e := newEnv(t, sv, true)
	cmd := exec.Command(bbCLI, "doctor", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, _ := cmd.Output()
	var report map[string]any
	if err := json.Unmarshal(stdout, &report); err != nil {
		t.Fatalf("doctor --json is not JSON: %v\n%s", err, stdout)
	}
	noPiece(t, "doctor --json", string(stdout), bbToken)
	if strings.Contains(string(stdout), bbWithheld) {
		t.Errorf("doctor was withheld:\n%s", stdout)
	}
}
