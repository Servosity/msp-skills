// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package kaseyablackbox runs the built CLI and MCP binaries against a VSA
// server that echoes the operator's credential back in every encoding it can,
// then checks the two things a reviewer cannot read off the code (review
// round 2, classes credential-persisted-in-files and
// secret-in-response-derived-text):
//
//   - no file the program writes, except the 0600 credential store, holds the
//     credential in any of those encodings;
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds it either.
//
// The oracle is the exact set of encoded strings the server sent, so a pass
// does not depend on the program's own decoder.
package kaseyablackbox

import (
	"bytes"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	bbTokenID     = "kvsaid" + "7fb3189b" // split: a synthetic fixture, not a credential for secret scanners
	bbTokenSecret = "kvsa+Secret/R8v52W4o1n7Q6u3sZx"
	bbTenantValue = "AdminPass-41c7e2f9"
	bbDevice      = "11110000-0000-4000-8000-000000000000"
)

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "kvsa-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "kaseya-vsa-cli"+exe)
	bbMCP = filepath.Join(dir, "kaseya-vsa-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/kaseya-vsa-cli", bbMCP: "../../cmd/kaseya-vsa-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// echoForms is every encoding of the token secret the server sends back.
func echoForms() []string {
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(bbTokenSecret))
	_ = w.Close()
	return []string{
		bbTokenSecret,
		base64.StdEncoding.EncodeToString([]byte(bbTokenSecret)),
		base64.StdEncoding.EncodeToString([]byte(bbTokenID + ":" + bbTokenSecret)),
		hex.EncodeToString([]byte(bbTokenSecret)),
		base32.StdEncoding.EncodeToString([]byte(bbTokenSecret)),
		a85.String(),
		url.QueryEscape(bbTokenSecret),
	}
}

// jsonUnicode spells s as \uXXXX escapes, so the raw secret appears only once
// a JSON decoder has read it.
func jsonUnicode(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

func echoObject() string {
	f := echoForms()
	keys := []string{"raw", "b64", "basic", "hex", "b32", "a85", "url"}
	var parts []string
	for i, k := range keys {
		v, _ := json.Marshal(f[i])
		parts = append(parts, fmt.Sprintf("%q:%s", k, v))
	}
	parts = append(parts, fmt.Sprintf(`"uesc":"%s"`, jsonUnicode(bbTokenSecret)))
	return "{" + strings.Join(parts, ",") + "}"
}

func echoServer(t *testing.T) string {
	t.Helper()
	echo := echoObject()
	item := fmt.Sprintf(`{"Id":1,"Identifier":%q,"AuditLogRecordId":1,"Name":"PC-1","Description":"note","value":%q,"GlobalValue":%q,"DefaultValue":%q,"Echo":%s}`,
		bbDevice, bbTenantValue, bbTenantValue, bbTenantValue, echo)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case strings.Contains(p, "/scopes"):
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"Data":null,"Meta":{"ResponseCode":401,"ErrorMessage":"invalid token %s"},"Echo":%s}`, bbTokenSecret, echo)
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/run"):
			fmt.Fprintf(w, `{"Data":{"ExecutionId":"exec-1","Echo":%s},"Meta":{"ResponseCode":200}}`, echo)
		case strings.Contains(p, "/executions/"):
			fmt.Fprintf(w, `{"Data":{"State":%q,"ExitCode":1,"EndTime":"2026-10-27T12:00:00Z","Output":%q,"Echo":%s},"Meta":{"ResponseCode":200}}`,
				echoForms()[5], echoForms()[1], echo)
		default:
			fmt.Fprintf(w, `{"Data":[%s],"Meta":{"ResponseCode":200,"TotalCount":1}}`, item)
		}
	}))
	t.Cleanup(srv.Close)
	return trustTestServer(t, srv)
}

type bbEnv struct {
	home string
	env  []string
}

func newEnv(t *testing.T, host string, withEnvCreds bool) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"KASEYA_VSA_SERVER=" + host, "KASEYA_VSA_CA_CERT=" + testCAFile(t, host),
		"KASEYA_VSA_CLI_PATH=" + bbCLI,
	}
	if withEnvCreds {
		env = append(env, "KASEYA_VSA_TOKEN_ID="+bbTokenID, "KASEYA_VSA_TOKEN_SECRET="+bbTokenSecret)
	}
	return &bbEnv{home: home, env: env}
}

func (e *bbEnv) run(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env = e.env
	cmd.Dir = e.home
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%v: timed out", args)
	}
	return out.String() + "\n" + errb.String()
}

// mcpCalls runs one MCP stdio session and returns every tools/call result
// line as the server wrote it.
func (e *bbEnv) mcpCalls(t *testing.T, calls []map[string]any) string {
	t.Helper()
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	for i, c := range calls {
		_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": c})
	}
	cmd := exec.Command(bbMCP)
	cmd.Env = e.env
	cmd.Dir = e.home
	stdin, _ := cmd.StdinPipe()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(stdin, &in)
	// Keep stdin open until every call has answered, then close it.
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(out.String(), `"id":`) >= len(calls)+1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = stdin.Close()
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
	}
	return out.String() + "\n" + errb.String()
}

func assertNoEcho(t *testing.T, where, text string) {
	t.Helper()
	for _, f := range echoForms() {
		if strings.Contains(text, f) {
			t.Errorf("%s holds the credential as %q", where, f)
		}
	}
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	host := echoServer(t)
	e := newEnv(t, host, false)
	e.run(t, "auth", "set-credentials", bbTokenID, bbTokenSecret)
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range [][]string{
		{"auth", "status"},
		{"doctor"},
		{"sync"},
		{"organizations", "list", "--json", "--receipt"},
		{"custom-fields", "list", "--json"},
		{"webhooks", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"assets", "list", "--json", "--deliver", "file:" + deliverFile},
		{"scopes", "list", "--json"},
		{"health", "--json"},
		{"audit-logs", "digest", "--json"},
		{"devices", "brief", bbDevice, "--json"},
		{"scripts", "run-wait", "77", "--device", bbDevice, "--attach", "exec-1", "--json"},
		{"export", "organizations", "-o", exportFile},
	} {
		e.run(t, args...)
	}
	e.mcpCalls(t, []map[string]any{
		{"name": "kaseya-vsa_read", "arguments": map[string]any{"endpoint_id": "organizations.list"}},
		{"name": "scripts_run_wait", "arguments": map[string]any{"script-id": "77", "device": bbDevice, "attach": "exec-1", "confirm": true}},
	})
	var scanned int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		scanned++
		rel, _ := filepath.Rel(e.home, path)
		if d.Name() == "credentials.toml" {
			if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
			}
			return nil
		}
		assertNoEcho(t, "file "+rel, string(data))
		if path != exportFile && path != deliverFile && bytes.Contains(data, []byte(bbTenantValue)) {
			t.Errorf("file %s holds a custom-field value", rel)
		}
		return nil
	})
	if scanned < 3 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	// The battery really wrote data, so the scan above is not vacuous.
	for _, path := range []string{exportFile, deliverFile} {
		if b, err := os.ReadFile(path); err != nil || !bytes.Contains(b, []byte("PC-1")) {
			t.Errorf("%s: want the exported record (err %v)", filepath.Base(path), err)
		}
	}
	var sawStore, sawCreds bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if d.Name() == "credentials.toml" {
				sawCreds = true
			}
			if strings.HasSuffix(d.Name(), ".db") {
				if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte("PC-1")) {
					sawStore = true
				}
			}
		}
		return nil
	})
	if !sawStore || !sawCreds {
		t.Errorf("store written with data: %v; credential store written: %v", sawStore, sawCreds)
	}
}

// Class secret-in-response-derived-text.
func TestNoCredentialInAnyOutput(t *testing.T) {
	host := echoServer(t)
	e := newEnv(t, host, true)
	for _, args := range [][]string{
		{"organizations", "list", "--json"},
		{"organizations", "list"},
		{"scopes", "list", "--json"},
		{"sync", "--resources", "organizations,custom_fields"},
		{"devices", "brief", bbDevice, "--json"},
		{"scripts", "run-wait", "77", "--device", bbDevice, "--attach", "exec-1", "--json"},
		{"scripts", "run-wait", "77", "--device", bbDevice, "--json"},
		{"tasks", "run-wait", "5", "--task-scope", "--json"},
		{"doctor"},
		{"auth", "status"},
	} {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	// The data still prints, the echo is withheld in place, and JSON output
	// stays valid JSON.
	cmd := exec.Command(bbCLI, "organizations", "list", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("organizations list --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("PC-1")) || !bytes.Contains(stdout, []byte("text withheld")) {
		t.Fatalf("want valid JSON with the record and the withheld notice, got:\n%.1500s", stdout)
	}
	out := e.mcpCalls(t, []map[string]any{
		{"name": "kaseya-vsa_read", "arguments": map[string]any{"endpoint_id": "organizations.list"}},
		{"name": "kaseya-vsa_read", "arguments": map[string]any{"endpoint_id": "scopes.list"}},
		{"name": "scripts_run_wait", "arguments": map[string]any{"script-id": "77", "device": bbDevice, "attach": "exec-1", "confirm": true}},
		{"name": "organizations_list", "arguments": map[string]any{}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	})
	if !strings.Contains(out, `"id":5`) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	if !strings.Contains(out, "PC-1") {
		t.Fatalf("MCP results lost the record itself:\n%.2000s", out)
	}
}
