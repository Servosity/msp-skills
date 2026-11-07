// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated). What this connector's wiring of the shared
// credential kit must do, driven through the built binaries: credentials the
// command line names are held before the command line is parsed, a stdin body's
// are held before its preview, an MCP tool result is checked, and an MCP write
// is sent only with confirm true (handfixes.json credkit-wiring,
// mcp-mirror-write-confirm, mcp-endpoint-write-confirm).

package credwire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"pagerduty-pp-cli/internal/cliutil/testenv"
)

// TestCommandLineConfigIsHeldBeforeParse: a credential in the --config or --home
// the command line names, or in the store a flag's value only looks like it moves
// away from, never prints in the parse errors that repeat a word.
func TestCommandLineConfigIsHeldBeforeParse(t *testing.T) {
	const cfgTok, homeTok, defaultTok = "AltToken-7x9k4m6n2q", "HomeStoreToken-5b8d1e3f", "DefaultStoreToken-9c4a6e2d"
	srv, ca, _ := kitServer(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	home := testenv.Isolate(t)
	cfg := filepath.Join(home, "alt.toml")
	if err := os.WriteFile(cfg, []byte("api_key = '"+cfgTok+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(home, "althome")
	if err := os.MkdirAll(alt, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, code := kitRunIn(t, srv, ca, home, homeTok+"\n", "auth", "set-token", "--home", alt); code != 0 {
		t.Fatalf("auth set-token: exit %d:\n%s", code, out)
	}
	if out, code := kitRunIn(t, srv, ca, home, defaultTok+"\n", "auth", "set-token"); code != 0 {
		t.Fatalf("auth set-token: exit %d:\n%s", code, out)
	}
	for _, c := range []struct {
		tok  string
		args []string
	}{
		// A flag's value that looks like --home is that flag's value to the
		// program, whose own store stays in use: both stores are held.
		{defaultTok, []string{"--select", "--home=" + alt, defaultTok}},
		{homeTok, []string{"--select", "--home=" + alt, homeTok}},
		{cfgTok, []string{cfgTok, "--config", cfg, "--no-learn"}},
		{cfgTok, []string{cfgTok, "--config=" + cfg, "--no-learn"}},
		{cfgTok, []string{"users", "get", "--no-such-flag", cfgTok, "--config", cfg}},
		{cfgTok, []string{"--config", cfg, "users", "list", "--limit", cfgTok}},
		{homeTok, []string{homeTok, "--home", alt, "--no-learn"}},
		{homeTok, []string{"--home=" + alt, "users", "get", "--no-such-flag", homeTok}},
	} {
		if out, _ := kitRun(t, srv, ca, home, c.args...); strings.Contains(out, c.tok) {
			t.Errorf("%v printed the credential its command-line configuration holds:\n%s", c.args, out)
		}
	}
}

// TestStdinBodyCredentialIsHeldBeforeItsPreview: a credential field of a --stdin
// body never prints in the preview (the client holds a body before it previews or
// sends it), and the words of a body that is no document never print in its parse
// error; the rest of an ordinary preview still does.
func TestStdinBodyCredentialIsHeldBeforeItsPreview(t *testing.T) {
	const secret = "Bq5Tn7Yk3H" + "z9Wd2Ls6Fm"
	srv, ca, seen := kitServer(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	home := testenv.Isolate(t)
	body := fmt.Sprintf(`{"service":{"name":"Checkout","description":"%s","secret":"%s"}}`, "HQ-Record", secret)
	out, _ := kitRunIn(t, srv, ca, home, body, "services", "create", "--stdin", "--dry-run", "--yes")
	if strings.Contains(out, secret) {
		t.Errorf("a preview printed a stdin body's credential field:\n%s", out)
	}
	notDoc := "not a document " + secret
	out, code := kitRunIn(t, srv, ca, home, notDoc, "services", "create", "--stdin", "--yes")
	if code == 0 || strings.Contains(out, secret) {
		t.Errorf("a body that is no document: exit %d, printed:\n%s", code, out)
	}
	out, _ = kitRunIn(t, srv, ca, home, `{"service":{"name":"Checkout"}}`, "services", "create", "--stdin", "--dry-run", "--yes")
	if strings.Contains(out, "credkit: withheld") || !strings.Contains(out, "Checkout") {
		t.Errorf("an ordinary write preview did not print:\n%s", out)
	}
	if *seen != 0 {
		t.Errorf("%d requests reached the API", *seen)
	}
}

// mcpReplyBuffer is a buffer safe to poll while the server writes to it.
type mcpReplyBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *mcpReplyBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *mcpReplyBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func mcpReplies(out string) map[int]string {
	replies := map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		var msg struct {
			ID *int `json:"id"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && msg.ID != nil {
			replies[*msg.ID] = line
		}
	}
	return replies
}

// mcpSession runs one MCP stdio session against srv, numbering the calls from 1
// and sending each only after the last was answered, and returns the replies.
func mcpSession(t *testing.T, srv *httptest.Server, ca, home string, calls ...map[string]any) map[int]string {
	t.Helper()
	cli := nestedCLI(t)
	cmd := exec.Command(mcpBinary(t))
	cmd.Env = append(kitEnv(srv, ca, home), "PAGERDUTY_CLI_PATH="+cli)
	cmd.Dir = home
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var out, errb mcpReplyBuffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(stdin)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	deadline := time.Now().Add(3 * time.Minute)
	for i, c := range calls {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": c})
		for time.Now().Before(deadline) && len(mcpReplies(out.String())) < i+2 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	_ = stdin.Close()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	replies := mcpReplies(out.String())
	if len(replies) != len(calls)+1 {
		t.Fatalf("%d replies to %d calls; stdout:\n%.2000s\nstderr:\n%.1000s", len(replies), len(calls)+1, out.String(), errb.String())
	}
	return replies
}

func mcpExecute(endpoint string, params map[string]any, extra map[string]any) map[string]any {
	args := map[string]any{"endpoint_id": endpoint, "params": params}
	for k, v := range extra {
		args[k] = v
	}
	return map[string]any{"name": "pagerduty_execute", "arguments": args}
}

var mcpPreviewed = regexp.MustCompile(`Preview only: nothing was sent|confirmation_required\\*"\s*:\s*true`)

// TestMCPWriteSendsOnlyWithConfirmTrue: every write previews and sends nothing
// until confirm is the boolean true, whatever else a caller tries; then the
// write reaches the API once.
func TestMCPWriteSendsOnlyWithConfirmTrue(t *testing.T) {
	var log requestLog
	srv, ca, _ := kitServer(t, func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		fmt.Fprint(w, `{"incident":{"id":"PINC001","type":"incident"}}`)
	})
	home := testenv.Isolate(t)
	del := map[string]any{"id": "PSVC001"}
	incident := map[string]any{"From": "pat.clicker@hq-record.example.com", "incident": map[string]any{"type": "incident", "title": "Preview",
		"service": map[string]any{"id": "PSVC001", "type": "service_reference"}}}
	previews := []map[string]any{
		mcpExecute("services.delete", del, nil),
		mcpExecute("incidents.create", incident, nil),
		mcpExecute("services.delete", del, map[string]any{"confirm": "true"}),
		mcpExecute("services.delete", del, map[string]any{"confirm": 1}),
		{"name": "import", "arguments": map[string]any{"resource": "incidents"}},
	}
	refusedOrPreviewed := []map[string]any{
		mcpExecute("services.delete", map[string]any{"id": "PSVC001", "confirm": true}, nil),
		{"name": "pagerduty_read", "arguments": map[string]any{"endpoint_id": "services.delete", "params": del}},
		{"name": "pagerduty_get", "arguments": map[string]any{"endpoint_id": "services.delete"}},
	}
	replies := mcpSession(t, srv, ca, home, append(append([]map[string]any(nil), previews...), refusedOrPreviewed...)...)
	for i, call := range previews {
		if r := replies[i+1]; !mcpPreviewed.MatchString(r) {
			t.Errorf("%v did not preview:\n%.1000s", call["arguments"], r)
		}
	}
	for i, call := range refusedOrPreviewed {
		r := replies[len(previews)+i+1]
		if !mcpPreviewed.MatchString(r) && !strings.Contains(r, `"isError":true`) && !strings.Contains(r, `"error":{`) {
			t.Errorf("%v was neither previewed nor refused:\n%.1000s", call["arguments"], r)
		}
	}
	for _, m := range log.all() {
		if !strings.HasPrefix(m, "GET ") {
			t.Errorf("sent %s without confirm true", m)
		}
	}
	mcpSession(t, srv, ca, home, mcpExecute("services.delete", del, map[string]any{"confirm": true}), mcpExecute("incidents.create", incident, map[string]any{"confirm": true}))
	var deletes, creates int
	for _, m := range log.all() {
		switch m {
		case "DELETE /services/PSVC001":
			deletes++
		case "POST /incidents":
			creates++
		}
	}
	if deletes != 1 || creates != 1 {
		t.Errorf("confirm true: want one DELETE and one POST, got %v", log.all())
	}
}

// TestMCPToolResultCarryingTheKeyIsWithheld: an API answer repeating the key
// reaches the MCP client as the kit's notice, through the server's own tool and
// through a mirrored CLI child, and the same read without it prints.
func TestMCPToolResultCarryingTheKeyIsWithheld(t *testing.T) {
	var echo struct {
		sync.Mutex
		key string
	}
	srv, ca, _ := kitServer(t, func(w http.ResponseWriter, r *http.Request) {
		echo.Lock()
		defer echo.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, offsetPage("services", r.URL.Query(), []string{fmt.Sprintf(`{"id":"PSVC001","type":"service","name":"Checkout %s"}`, echo.key)}))
	})
	home := testenv.Isolate(t)
	calls := []map[string]any{
		{"name": "pagerduty_read", "arguments": map[string]any{"endpoint_id": "services.list"}},
		{"name": "export", "arguments": map[string]any{"resource": "services", "format": "json"}},
	}
	echo.key = nestedKey
	for id, r := range mcpSession(t, srv, ca, home, calls...) {
		if id == 0 {
			continue
		}
		if strings.Contains(r, nestedKey) || !strings.Contains(r, "credkit: withheld from") {
			t.Errorf("call %d did not withhold an answer carrying the key:\n%.1000s", id, r)
		}
	}
	echo.key = "operational"
	for id, r := range mcpSession(t, srv, ca, home, calls...) {
		if id != 0 && (!strings.Contains(r, "Checkout operational") || strings.Contains(r, "credkit: withheld")) {
			t.Errorf("call %d did not print an ordinary answer:\n%.1000s", id, r)
		}
	}
}

// TestCredentialFilesOnlyReads: what Start calls before anything is installed
// writes no file and prints nothing, whatever the command line names.
func TestCredentialFilesOnlyReads(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv("PAGERDUTY_HOME", home)
	alt := filepath.Join(home, "absent-home")
	cfg := filepath.Join(home, "absent", "alt.toml")
	stdout, stderr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w
	files := credentialFiles([]string{"users", "list", "--home", alt, "--config", cfg, "--home=relative", "--no-such-flag"})
	os.Stdout, os.Stderr = stdout, stderr
	_ = w.Close()
	var printed bytes.Buffer
	if _, err := printed.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if printed.Len() != 0 {
		t.Errorf("credentialFiles printed:\n%s", printed.String())
	}
	if len(files) == 0 {
		t.Error("credentialFiles named no file")
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("credentialFiles created %s under the home", e.Name())
	}
}
