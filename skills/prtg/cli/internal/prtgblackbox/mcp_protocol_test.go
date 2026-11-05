// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package prtgblackbox

// prtg handfix (output-chokepoint, protocol frames): a message mcp-go writes
// itself, before any tool handler runs (an unknown tool, a method it does not
// know, a request id it echoes), passes the same held-value withholding as a
// tool result, on stdio and on streamable HTTP, and still parses as JSON-RPC.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	protoKey   = "prtgRe" + "viewKey-7f319a"                // PRTG_API_KEY
	protoToken = "protoHTTP" + "Token-0c4f8a2e6b1d9f3a7c5e" // PRTG_MCP_HTTP_TOKEN
	protoNote  = "credkit: withheld"                        // the kit's notice
)

func protoEnv(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	return []string{
		"PATH=" + os.Getenv("PATH"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"PRTG_SERVER=127.0.0.1:9", // never contacted: no probe reaches a tool handler that calls PRTG
		"PRTG_API_KEY=" + protoKey,
		"PRTG_MCP_HTTP_TOKEN=" + protoToken,
	}
}

// protoProbe is one request mcp-go answers itself, and whether its answer
// must carry the withheld notice (the unwithheld answer echoes a held value).
type protoProbe struct {
	line    string
	id      string // the response id as raw JSON; "" for the request whose id is the key
	leaking bool
}

func protoProbes() []protoProbe {
	a, b := halves(protoKey)
	call := func(id int, name string) protoProbe {
		return protoProbe{fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":{}}}`, id, name), fmt.Sprint(id), true}
	}
	return []protoProbe{
		{`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "1", false},
		call(2, protoKey), // the Codex P1 input
		call(3, base64.StdEncoding.EncodeToString([]byte(protoKey))),
		{fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"%s","arguments":{}}}`, jsonUnicode(protoKey)), "4", true},
		call(5, a+" "+b),
		call(6, protoToken), // the HTTP bearer token, held from startup
		{fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":%q,"params":{}}`, protoKey), "7", true},      // malformed: an unknown method
		{fmt.Sprintf(`{"jsonrpc":"2.0","id":8,"method":"%s"}`, jsonUnicode(protoToken)), "8", true}, // malformed, escaped
		{`{"jsonrpc":"2.0","id":9,"method":"ping"}`, "9", false},
	}
}

// assertNoProtoEcho fails when text holds the key or the token in any form
// the probes sent, or both halves of one.
func assertNoProtoEcho(t *testing.T, where, text string) {
	t.Helper()
	for _, s := range []string{protoKey, protoToken} {
		forms := append(echoForms(s), jsonUnicode(s))
		for _, f := range forms {
			if strings.Contains(text, f) {
				t.Errorf("%s holds a credential as %q", where, f)
			}
		}
		if a, b := halves(s); strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
	}
}

type rpcReply struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	JSONRPC string `json:"jsonrpc"`
}

// checkProtoReplies parses every reply (each must be valid JSON-RPC) and
// checks each probe was answered, the leaking ones with the notice.
func checkProtoReplies(t *testing.T, where string, frames []string) {
	t.Helper()
	byID := map[string]rpcReply{}
	for _, f := range frames {
		var r rpcReply
		if err := json.Unmarshal([]byte(f), &r); err != nil || r.JSONRPC != "2.0" {
			t.Errorf("%s: a reply is not valid JSON-RPC (%v): %.300s", where, err, f)
			continue
		}
		byID[string(r.ID)] = r
	}
	if r, ok := byID["0"]; !ok || !bytes.Contains(r.Result, []byte("PRTG Network Monitor")) {
		t.Errorf("%s: initialize was not answered with the server info", where)
	}
	for _, p := range protoProbes() {
		if p.id == "" {
			continue
		}
		r, ok := byID[p.id]
		if !ok {
			t.Errorf("%s: request id %s got no reply", where, p.id)
			continue
		}
		text := string(r.Result)
		if r.Error != nil {
			text = r.Error.Message
		}
		if p.leaking && !strings.Contains(text, protoNote) {
			t.Errorf("%s: request id %s: want the withheld notice, got %.300s", where, p.id, text)
		}
	}
	if r := byID["1"]; !bytes.Contains(r.Result, []byte(`"prtg_read"`)) || bytes.Contains(r.Result, []byte(protoNote)) {
		t.Errorf("%s: tools/list lost a tool or was withheld: %.300s", where, r.Result)
	}
}

const protoInitialize = `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

// TestMCPProtocolFramesWithholdHeldValuesStdio: the stdio channel.
func TestMCPProtocolFramesWithholdHeldValuesStdio(t *testing.T) {
	var in strings.Builder
	in.WriteString(protoInitialize + "\n")
	in.WriteString(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	for _, p := range protoProbes() {
		in.WriteString(p.line + "\n")
	}
	cmd := exec.Command(bbMCP)
	cmd.Env = protoEnv(t)
	stdin, _ := cmd.StdinPipe()
	var out, errb lockedBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(stdin, in.String())
	want := len(protoProbes()) + 1
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) && strings.Count(out.String(), "\n") < want {
		time.Sleep(50 * time.Millisecond)
	}
	_ = stdin.Close()
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill() // the process this test started, by PID
		<-waitDone
	}
	stdout := out.String()
	assertNoProtoEcho(t, "MCP stdio stdout", stdout)
	assertNoProtoEcho(t, "MCP stdio stderr", errb.String())
	var frames []string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.TrimSpace(line) != "" {
			frames = append(frames, line)
		}
	}
	if len(frames) != want {
		t.Errorf("stdio: want %d replies, got %d:\n%.3000s", want, len(frames), stdout)
	}
	checkProtoReplies(t, "stdio", frames)
	if t.Failed() {
		t.Logf("stdio stdout:\n%.4000s", stdout)
	}
}

// TestMCPProtocolFramesWithholdHeldValuesHTTP: the streamable HTTP channel,
// with the bearer token the caller authenticates with held as well.
func TestMCPProtocolFramesWithholdHeldValuesHTTP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	cmd := exec.Command(bbMCP, "--transport", "http", "--addr", addr)
	cmd.Env = protoEnv(t)
	var errb lockedBuffer
	cmd.Stdout, cmd.Stderr = &errb, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill() // the process this test started, by PID
		_ = cmd.Wait()
	})
	deadline := time.Now().Add(30 * time.Second)
	for {
		c, derr := net.DialTimeout("tcp", addr, time.Second)
		if derr == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("MCP HTTP server never listened on %s:\n%s", addr, errb.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	var transcript strings.Builder
	var frames []string
	post := func(session, body string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+protoToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST %.80s: %v", body, err)
		}
		payload, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		fmt.Fprintf(&transcript, "%d %v\n%s\n", resp.StatusCode, resp.Header, payload)
		ct := resp.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(ct, "text/event-stream"):
			for _, line := range strings.Split(string(payload), "\n") {
				if d, ok := strings.CutPrefix(line, "data:"); ok {
					frames = append(frames, strings.TrimSpace(d))
				}
			}
		case strings.HasPrefix(ct, "application/json"):
			frames = append(frames, strings.TrimSpace(string(payload)))
		case len(bytes.TrimSpace(payload)) > 0:
			t.Errorf("POST %.80s: status %d, Content-Type %q, body %.300s", body, resp.StatusCode, ct, payload)
		}
		return resp
	}
	initResp := post("", protoInitialize)
	session := initResp.Header.Get("Mcp-Session-Id")
	if session == "" {
		t.Fatalf("initialize returned no session id:\n%s", transcript.String())
	}
	post(session, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for _, p := range protoProbes() {
		post(session, p.line)
	}
	assertNoProtoEcho(t, "MCP HTTP responses", transcript.String())
	assertNoProtoEcho(t, "MCP HTTP stderr", errb.String())
	if len(frames) != len(protoProbes())+1 {
		t.Errorf("http: want %d replies, got %d", len(protoProbes())+1, len(frames))
	}
	checkProtoReplies(t, "http", frames)
	if t.Failed() {
		t.Logf("http transcript:\n%.4000s", transcript.String())
	}
}

// TestMCPRequestIDThatIsTheKeyEndsTheSession: a reply must repeat its request's id,
// so no reply to this request can leave without the key. The kit fails closed: the
// session ends and the id is never written.
func TestMCPRequestIDThatIsTheKeyEndsTheSession(t *testing.T) {
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"method":"ping"}`, protoKey)
	cmd := exec.Command(bbMCP)
	cmd.Env = protoEnv(t)
	stdin, _ := cmd.StdinPipe()
	var out, errb lockedBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(stdin, protoInitialize+"\n"+`{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"+line+"\n")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(out.String(), "PRTG Network Monitor") {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	_ = stdin.Close()
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill() // the process this test started, by PID
		<-waitDone
	}
	assertNoProtoEcho(t, "MCP stdio stdout", out.String())
	assertNoProtoEcho(t, "MCP stdio stderr", errb.String())
}

// lockedBuffer is a bytes.Buffer safe for the process's writer goroutines.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
