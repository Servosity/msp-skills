// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package prtgblackbox runs the built CLI and MCP binaries against a PRTG
// core that echoes every credential it knows back in every encoding it can,
// whole and split across lines and adjacent values, and checks what a
// reviewer cannot read off the code (prtg handfixes credkit-wiring,
// store-never-keeps-secrets, no-response-cache, response-credentials-held):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the API keys (single-core, and two registered
//     cores whose keys arrive in PRTG_FLEET_KEYS, well-formed or not), a
//     password and a token given as flags, the session token a login mints
//     and the API key a POST mints;
//   - no file the program writes, except the 0600 credential store, holds
//     one either.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package prtgblackbox

import (
	"bytes"
	"compress/zlib"
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
	"sync"
	"testing"
	"time"
)

const (
	bbKey        = "prtgKE" + "Y7fb3189bQ6u3sZxW4o1n7R8v52" // PRTG_API_KEY / core acme
	bbKey2       = "prtgKE" + "YglobexA9d2c7e41f0b63X5w8Lq" // core globex
	bbPassword   = "PrtgPa" + "ssw0rd-5c1e9a7d3b"           // session login --password
	bbFlagToken  = "resetT" + "ok-8e2d4f6a1c9b7e3d"         // users reset-password --token
	bbMinted     = "mintedSession-71c3e9b05d2a4f68"         // POST /session LoginResult.token
	bbMintedKey  = "minted" + "ApiKey-4d8b2e6f1a9c3e70"     // POST /users/{id}/api-keys key
	bbTenantPass = "WinLogin-41c7e2f9a6"                    // a device credential setting
	bbShortPass  = "Ab1c2D"                                 // a 6-character password flag
	bbDashPass   = "-dashPass-91c3e7a5"                     // a password flag value that starts with "-"
	bbStdinPass  = "StdinPass-4f2a9c1e"                     // a password in a --stdin request body
	bbLicense    = "LIC-7f3e-91a2-c4d8-0b6e"                // LicenseInfo.key the core returns
	bbSameUser   = "prtgadminSame7f3"                       // a --stdin login whose username is its password
	bbShortResp  = "Sh0rtPw"                                // a 7-character credential a response carries
	bbBOMResp    = "BomRespSecret-6a1c"                     // a credential in a BOM-prefixed response
	bbLargePass  = "LargeBodyPass-3d9e"                     // a password in a request body over 4 MiB
	bbPasshash   = "8417305926"                             // a PRTG passhash a response carries
)

func secrets() []string {
	return []string{bbKey, bbKey2, bbPassword, bbFlagToken, bbMinted, bbMintedKey, bbShortPass, bbDashPass, bbStdinPass, bbLicense,
		bbSameUser, bbShortResp, bbBOMResp, bbLargePass, bbPasshash}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "prtg-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "prtg-cli"+exe)
	bbMCP = filepath.Join(dir, "prtg-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/prtg-cli", bbMCP: "../../cmd/prtg-mcp"} {
		if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, b)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// echoForms is every encoding of s the server sends back whole.
func echoForms(s string) []string {
	var a85 bytes.Buffer
	w := ascii85.NewEncoder(&a85)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return []string{
		s,
		"x" + s + "x",
		"Bearer " + s,
		base64.StdEncoding.EncodeToString([]byte(s)),
		base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)),
		base32.StdEncoding.EncodeToString([]byte(s)),
		a85.String(),
		url.QueryEscape(s),
	}
}

func halves(s string) (string, string) { return s[:len(s)/2], s[len(s)/2:] }

// jsonUnicode spells s as \uXXXX escapes, so the raw value appears only once
// a JSON decoder has read it.
func jsonUnicode(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, `\u%04x`, r)
	}
	return b.String()
}

// echoObject repeats each of sent whole, in every form.
func echoObject(sent []string) string {
	parts := []string{`"note":"echo"`}
	for i, s := range sent {
		for j, f := range echoForms(s) {
			v, _ := json.Marshal(f)
			parts = append(parts, fmt.Sprintf(`"e%d_%d":%s`, i, j, v))
		}
		parts = append(parts, fmt.Sprintf(`"u%d":"%s"`, i, jsonUnicode(s)))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// splitObject repeats each of sent split: across two adjacent values, across
// an embedded newline, and across a space.
func splitObject(sent []string) string {
	parts := []string{`"note":"split"`}
	for i, s := range sent {
		a, b := halves(s)
		parts = append(parts, fmt.Sprintf(`"s%d_a":%q,"s%d_b":%q,"s%d_nl":%q,"s%d_sp":%q`, i, a, i, b, i, a+"\n"+b, i, a+" "+b))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// sentSecrets is every known secret the request carried (its Authorization
// header, URL or body): a real core can echo only what it was sent. Each is
// recorded in sentLog, so a test can prove a key really reached the core.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	seen := r.Header.Get("Authorization") + "\n" + r.URL.String() + "\n" + string(body)
	var out []string
	sentLog.Lock()
	defer sentLog.Unlock()
	for _, s := range secrets() {
		if strings.Contains(seen, s) {
			out = append(out, s)
			sentLog.seen[s] = true
			if sentLog.byHost[r.Host] == nil {
				sentLog.byHost[r.Host] = map[string]bool{}
			}
			sentLog.byHost[r.Host][s] = true
		}
	}
	return out
}

// sentLog is every secret the current echoServer was sent (seen), and every
// secret each server was sent (byHost), which tests running in parallel read
// through bbEnv.sent. honest names the servers that repeat nothing.
var sentLog = struct {
	sync.Mutex
	seen   map[string]bool
	byHost map[string]map[string]bool
	honest map[string]bool
}{seen: map[string]bool{}, byHost: map[string]map[string]bool{}, honest: map[string]bool{}}

// sent reports whether this environment's own server was sent s.
func (e *bbEnv) sent(s string) bool {
	sentLog.Lock()
	defer sentLog.Unlock()
	return sentLog.byHost[e.host][s]
}

// setHonest makes the server at host answer as a real core does, repeating
// nothing it was sent: a kit sink withholds a whole unit that repeats a held
// value, so this is what proves ordinary data still reaches each sink.
func setHonest(host string, honest bool) {
	sentLog.Lock()
	defer sentLog.Unlock()
	sentLog.honest[host] = honest
}

func isHonest(host string) bool {
	sentLog.Lock()
	defer sentLog.Unlock()
	return sentLog.honest[host]
}

func wasSent(s string) bool {
	sentLog.Lock()
	defer sentLog.Unlock()
	return sentLog.seen[s]
}

// honestServer is echoServer with no echoes (setHonest).
func honestServer(t *testing.T) string {
	t.Helper()
	host := echoServer(t)
	setHonest(host, true)
	return host
}

func echoServer(t *testing.T) string {
	t.Helper()
	sentLog.Lock()
	sentLog.seen = map[string]bool{}
	sentLog.Unlock()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimPrefix(r.URL.Path, "/api/v2")
		sent := sentSecrets(r)
		if isHonest(r.Host) {
			sent = nil
		}
		echo := echoObject(sent)
		switch {
		case r.Method == http.MethodPost && p == "/session":
			fmt.Fprintf(w, `{"token":%q,"expires_at":"2026-10-28T00:00:00Z","echo":%s}`, bbMinted, echoObject(append(sent, bbMinted)))
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/api-keys"):
			// Deflate-encoded, as a core behind a compressing proxy answers.
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			fmt.Fprintf(zw, `{"id":"k1","key":%q,"echo":%s}`, bbMintedKey, echoObject(append(sent, bbMintedKey)))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(z.Bytes())
		case r.Method == http.MethodPost && p == "/session" && r.ContentLength > 4<<20:
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"code":"BAD_REQUEST","message":"rejected %s","echo":%s}`, strings.Join(sent, " "), echo)
		case r.Method == http.MethodPost && p == "/sensors/resume":
			// PRTG's documented "200: Success with one failure" (prtg.api.yaml).
			fmt.Fprint(w, `[{"node":{"id":"2322","type":"REFERENCED_SENSOR","name":"Ping","href":"/api/v2/sensors/2322"},"failure":{"code":"BAD_REQUEST","message":"The request could not be processed.","request_id":"amAUOX"}},{"node":{"id":"2323","type":"REFERENCED_SENSOR","name":"HTTP","href":"/api/v2/sensors/2323"},"failure":null}]`)
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/ai/analyze"):
			id := "req-1"
			if len(sent) > 0 {
				id = sent[0] // a job id that repeats the key it was sent
			}
			fmt.Fprintf(w, `{"request_id":%q,"status":"pending"}`, id)
		case strings.HasSuffix(p, "/devices/40"):
			fmt.Fprintf(w, `{"id":"40","name":%q,"windowsconnection":{"windowsloginpassword":%q}}`, bbShortResp, bbShortResp)
		case strings.HasSuffix(p, "/devices/41"):
			fmt.Fprintf(w, "\xef\xbb\xbf"+`{"id":"41","name":%q,"windowsconnection":{"windowsloginpassword":%q}}`, bbBOMResp, bbBOMResp)
		case strings.HasSuffix(p, "/users/100"):
			fmt.Fprintf(w, `{"id":"100","name":"rosa","login":"rosa","passhash":%q}`, bbPasshash)
		case strings.HasSuffix(p, "/license"):
			fmt.Fprintf(w, `{"key":%q,"edition":"PRTG 1000","license_type":"Commercial","echo":%s}`, bbLicense, echo)
		case strings.Contains(p, "probes"):
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"code":"UNAUTHORIZED","message":"invalid key %s","echo":%s}`, bbKey, echo)
		case strings.Contains(p, "groups"):
			fmt.Fprintf(w, `[{"id":"50","name":"Servers","split":%s}]`, splitObject(sent))
		case strings.Contains(p, "version"):
			fmt.Fprintf(w, `{"api":"2.0","echo":%s}`, echo)
		case r.Method == http.MethodPost:
			fmt.Fprintf(w, `{"echo":%s}`, echo)
		default:
			extra := ""
			if len(sent) > 0 {
				// A record whose identifier repeats the key it was sent.
				extra = fmt.Sprintf(`,{"id":%q,"name":"Echo-ID","status":"UP"}`, sent[0])
			}
			// VPN went down three hours ago and is up since 20 minutes ago,
			// scanned every minute: a recovery after a gap (paused,
			// acknowledged or rescheduled in between).
			vpn := fmt.Sprintf(`,{"id":"2077","name":"VPN","status":"UP","status_since":%q,"last_down":%q,"scanning_interval":60}`,
				time.Now().UTC().Add(-20*time.Minute).Format(time.RFC3339), time.Now().UTC().Add(-3*time.Hour).Format(time.RFC3339))
			fmt.Fprintf(w, `[{"id":"2045","name":"Ping-1","status":"DOWN","message":"timeout","windowsloginpassword":%q,"echo":%s}%s%s]`, bbTenantPass, echo, extra, vpn)
		}
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "https://")
}

type bbEnv struct {
	home string
	env  []string
	host string // the server newEnv pointed it at
}

func newEnv(t *testing.T, host string, withEnvKey bool) *bbEnv {
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
		"PRTG_SERVER=" + host, "PRTG_SKIP_TLS_VERIFY=true",
		"PRTG_CLI_PATH=" + bbCLI,
		// Both registered cores' keys, comma- and newline-separated.
		"PRTG_FLEET_KEYS= acme=" + bbKey + ",\n globex=" + bbKey2 + "\n",
	}
	if withEnvKey {
		env = append(env, "PRTG_API_KEY="+bbKey)
	}
	e := &bbEnv{home: home, env: env, host: host}
	e.run(t, "core", "add", "acme", "--server", host, "--insecure-tls")
	e.run(t, "core", "add", "globex", "--server", host, "--insecure-tls")
	return e
}

func (e *bbEnv) run(t *testing.T, args ...string) string {
	t.Helper()
	return e.runIn(t, "", args...)
}

func (e *bbEnv) runIn(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env = e.env
	cmd.Dir = e.home
	cmd.Stdin = strings.NewReader(stdin)
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

// mcpCalls runs one MCP stdio session and returns everything the server
// wrote: every tools/call result and its stderr.
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

// assertNoEcho fails when text holds a secret in any form the server sent,
// or both halves of one (a split the reader could rejoin).
func assertNoEcho(t *testing.T, where, text string) {
	t.Helper()
	for _, s := range secrets() {
		for _, f := range echoForms(s) {
			if strings.Contains(text, f) {
				t.Errorf("%s holds a credential as %q", where, f)
			}
		}
		if a, b := halves(s); strings.Contains(text, a) && strings.Contains(text, b) {
			t.Errorf("%s holds both halves of %q", where, s)
		}
	}
}

func battery(e *bbEnv, exportFile, deliverFile string) [][]string {
	return [][]string{
		{"sensors", "list-experimental", "--json"},
		{"sensors", "list-experimental"},
		{"sensors", "list-experimental", "--csv"},
		{"groups", "list-experimental", "--json"},
		{"groups", "list-experimental"},
		{"probes", "list-experimental", "--json"},
		{"health", "--json", "--receipt"},
		{"devices", "list-experimental", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"devices", "list-experimental", "--json", "--deliver", "file:" + deliverFile},
		{"sync", "--resources", "sensors,devices,groups"},
		{"--core", "acme", "sync", "--resources", "sensors"},
		{"export", "sensors", "-o", exportFile},
		{"session", "login", "--username", "prtgadmin", "--password", bbPassword, "--json"},
		{"session", "login", "--username", "prtgadmin", "--password=" + bbPassword, "--bogus-flag"},
		{"session", "login", "--username", "prtgadmin", "--password", bbShortPass, "--dry-run", "--json"},
		{"session", "login", "--username", "prtgadmin", "--password", bbDashPass, "--dry-run", "--json"},
		{"session", "login", "--username", "prtgadmin", "--password", bbShortPass, "--json"},
		{"system", "license-info", "--json"},
		{"system", "license-info", "--csv"},
		{"system", "license-info"},
		{"system", "license-info", "--json", "--deliver", "file:" + filepath.Join(e.home, "license.json")},
		{"--core", "acme", "system", "license-info", "--json"},
		{"devices", "get", "40", "--json", "--data-source", "live"},
		{"devices", "get", "40"},
		{"devices", "get", "41", "--json"},
		{"users", "get", "100", "--json"},
		{"users", "get", "100"},
		{"ai", "analyze-aiincidents", "--alarms", "[]", "--json"},
		{"jobs", "list", "--json"},
		{"feedback", "noted the key " + bbKey + " in a ticket"},
		{"sensors", "list-experimental", "--" + bbKey},
		{"users", "reset-password", "100", "--password", bbPassword, "--token", bbFlagToken, "--dry-run"},
		{"users", "api-keys", "create", "100", "--name", "bb", "--access-level", "READ", "--json"},
		{"fleet", "alarms", "--json"},
		{"fleet", "status", "--json"},
		{"fleet", "exec", "/version", "--json"},
		{"fleet", "exec", "/experimental/groups", "--json"},
		{"fleet", "ack", "--filter", `name = "Ping-1"`, "--message", "TKT-1", "--json"},
		{"fleet", "sync", "--resources", "sensors", "--json"},
		{"--core", "globex", "sensors", "list-experimental", "--json"},
		{"search", "Ping", "--json"},
		{"doctor"},
		{"auth", "status"},
		{"teach", "--query", "sensor uptime", "--resource-type", "sensors", "--resource", "2322", "--notes", "Authentication failed for " + bbKey, "--no-validate"},
	}
}

func mcpBattery() []map[string]any {
	return []map[string]any{
		{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "sensors.list-experimental"}},
		{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "groups.list-experimental"}},
		{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "probes.list-experimental"}},
		{"name": "prtg_execute", "arguments": map[string]any{"endpoint_id": "session.login", "params": map[string]any{"username": "u", "password": bbPassword}, "confirm": true}},
		{"name": "fleet_exec", "arguments": map[string]any{"path": "/version"}},
		{"name": "fleet_ack", "arguments": map[string]any{"filter": `name = "Ping-1"`, "message": "TKT-1"}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
		{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "system.license-info"}},
		{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "devices.get", "params": map[string]any{"id": "40"}}},
		{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "devices.get", "params": map[string]any{"id": "41"}}},
		{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "users.get", "params": map[string]any{"id": "100"}}},
		{"name": "prtg_execute", "arguments": map[string]any{"endpoint_id": "users.api-keys.create", "params": map[string]any{"id": "100", "name": "bb", "access_level": "READ"}, "confirm": true}},
	}
}

// stdinRuns feed a request body on stdin (session login --stdin): a plain
// one, one whose username is its password (previewed), and one over 4 MiB.
func stdinRuns(e *bbEnv, t *testing.T) string {
	body := fmt.Sprintf(`{"username":"admin","password":%q}`, bbStdinPass)
	same := fmt.Sprintf(`{"username":%q,"password":%q}`, bbSameUser, bbSameUser)
	// 64 KiB: the kit holds a declared stdin line whole and its cost grows faster
	// than the line (.review/kit-findings.md); PRTG takes no body near this size.
	large := fmt.Sprintf(`{"a":%q,"password":%q,"username":"admin"}`, strings.Repeat("x", 64<<10), bbLargePass)
	return e.runIn(t, body, "session", "login", "--stdin", "--dry-run", "--json") + e.runIn(t, body, "session", "login", "--stdin", "--json") +
		e.runIn(t, same, "session", "login", "--stdin", "--dry-run", "--json") + e.runIn(t, same, "session", "login", "--stdin", "--dry-run") +
		e.runIn(t, large, "session", "login", "--stdin", "--json")
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	host := echoServer(t)
	e := newEnv(t, host, false)
	e.runIn(t, bbKey+"\n", "auth", "set-token")
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, exportFile, deliverFile) {
		e.run(t, args...)
	}
	stdinRuns(e, t)
	e.mcpCalls(t, mcpBattery())
	var scanned int
	var sawCreds bool
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
		if bytes.Contains(data, []byte(bbKey)) && !bytes.Contains(data, []byte(bbKey2)) && strings.HasPrefix(d.Name(), "credentials") {
			// The credential store: the one file allowed to hold the key.
			sawCreds = true
			if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
			}
			return nil
		}
		if !synccpuStoreFile(path) { // synccpu: the local store is not checked
			assertNoEcho(t, "file "+rel, string(data))
		}
		if bytes.Contains(data, []byte(bbTenantPass)) && path != exportFile && path != deliverFile {
			t.Errorf("file %s holds a device credential setting", rel)
		}
		return nil
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-token wrote no credential store")
	}
	if !e.sent(bbKey2) {
		t.Errorf("globex's key (PRTG_FLEET_KEYS only) never reached the core; the battery did not use it")
	}
}

// TestOrdinaryDataReachesEverySink is the other direction of the two tests around
// it: from a core that echoes nothing, the record reaches stdout as valid JSON, the
// export file, the delivered file, the local store and the MCP result, and a
// device's credential setting reaches none of them.
func TestOrdinaryDataReachesEverySink(t *testing.T) {
	t.Parallel()
	host := honestServer(t)
	e := newEnv(t, host, true)
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	e.run(t, "sync", "--resources", "sensors,devices,groups")
	e.run(t, "export", "sensors", "-o", exportFile)
	e.run(t, "devices", "list-experimental", "--json", "--deliver", "file:"+deliverFile)
	cmd := exec.Command(bbCLI, "sensors", "list-experimental", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("sensors list-experimental --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("Ping-1")) {
		t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
	}
	if bytes.Contains(stdout, []byte(bbTenantPass)) {
		t.Fatalf("a device credential setting reached stdout:\n%.1500s", stdout)
	}
	for _, path := range []string{exportFile, deliverFile} {
		b, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(b, []byte("Ping-1")) || bytes.Contains(b, []byte(bbTenantPass)) {
			t.Errorf("%s: want the record without the credential setting (err %v):\n%.500s", filepath.Base(path), err, b)
		}
	}
	var sawStore bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte("Ping-1")) {
				sawStore = true
			}
		}
		return nil
	})
	if !sawStore {
		t.Errorf("no local store holds the synced record")
	}
	out := e.mcpCalls(t, []map[string]any{{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "sensors.list-experimental"}}})
	if !strings.Contains(out, "Ping-1") || strings.Contains(out, bbTenantPass) {
		t.Fatalf("MCP: want the record without the credential setting, got:\n%.2000s", out)
	}
}

// Class secret-in-output.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	host := echoServer(t)
	e := newEnv(t, host, true)
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, exportFile, deliverFile) {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	assertNoEcho(t, "output of session login --stdin", stdinRuns(e, t))
	if !e.sent(bbKey2) {
		t.Fatal("globex's key (PRTG_FLEET_KEYS only) never reached the core, so its echoes were never tested")
	}
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
}

// An unparseable PRTG_FLEET_KEYS sends no key anywhere and is never repeated:
// core list still lists every core and names the bad entry, a pinned or fleet
// command fails, and nothing printed or written holds either key.
func TestUnparseableFleetKeysNeverEcho(t *testing.T) {
	t.Parallel()
	host := echoServer(t)
	e := newEnv(t, host, false)
	for i, kv := range e.env {
		if strings.HasPrefix(kv, "PRTG_FLEET_KEYS=") {
			// Entry 2 is a bare key: its name was forgotten.
			e.env[i] = "PRTG_FLEET_KEYS=acme=" + bbKey + "\n" + bbKey2
		}
	}
	list, code := e.runCode(t, "core", "list", "--json")
	if code != 0 || !strings.Contains(list, `PRTG_FLEET_KEYS entry 2 has no "="`) {
		t.Fatalf("core list: want exit 0 and the bad entry named, got exit %d:\n%s", code, list)
	}
	if strings.Count(list, `"key_set": false`) != 2 {
		t.Fatalf("core list: want both cores listed without a key:\n%s", list)
	}
	outputs := list
	for _, args := range [][]string{
		{"core", "list"},
		{"--core", "acme", "sensors", "list-experimental", "--json"},
		{"--core", "globex", "health", "--json"},
		{"fleet", "status", "--json"},
		{"fleet", "exec", "/version", "--json"},
		{"core", "add", "initech", "--server", host},
		{"doctor"},
	} {
		out := e.run(t, args...)
		assertNoEcho(t, fmt.Sprintf("output of %v", args), out)
		outputs += out
	}
	mcp := e.mcpCalls(t, []map[string]any{
		{"name": "fleet_exec", "arguments": map[string]any{"path": "/version"}},
		{"name": "core_list", "arguments": map[string]any{}},
	})
	assertNoEcho(t, "MCP session", mcp)
	if !strings.Contains(outputs, "PRTG_FLEET_KEYS") {
		t.Fatalf("no command told the operator what is wrong:\n%.2000s", outputs)
	}
	if e.sent(bbKey) || e.sent(bbKey2) {
		t.Fatal("a key from an unparseable PRTG_FLEET_KEYS reached the core")
	}
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if data, rerr := os.ReadFile(path); rerr == nil {
				rel, _ := filepath.Rel(e.home, path)
				assertNoEcho(t, "file "+rel, string(data))
			}
		}
		return nil
	})
}

// runCode runs the CLI and returns its combined output and exit code.
func (e *bbEnv) runCode(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env, cmd.Dir = e.env, e.home
	out, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode()
	}
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return string(out), 0
}

// Class failed-bulk-action-reported-success: a bulk action whose result has
// one failed item fails, with per-item detail, on the CLI and over MCP.
func TestBulkActionFailureFailsCLIAndMCP(t *testing.T) {
	t.Parallel()
	host := honestServer(t)
	e := newEnv(t, host, true)
	out, code := e.runCode(t, "sensors", "multi-resume", "--filter", `name = "Ping"`, "--json")
	if code == 0 || !strings.Contains(out, `"success": false`) || !strings.Contains(out, "2322") || !strings.Contains(out, "BAD_REQUEST") {
		t.Fatalf("CLI: want a non-zero exit, success false and the failed item, got exit %d:\n%.1500s", code, out)
	}
	if _, code := e.runCode(t, "sensors", "multi-resume", "--filter", `name = "Ping"`); code == 0 {
		t.Fatal("CLI table output: a failed item exited 0")
	}
	// --allow-partial-failure only changes the exit code; the reported outcome
	// stays false (Codex r1: success:true with every item failed).
	if out, code := e.runCode(t, "sensors", "multi-resume", "--filter", `name = "Ping"`, "--allow-partial-failure", "--json"); code != 0 || !strings.Contains(out, `"success": false`) || strings.Contains(out, `"success": true`) {
		t.Fatalf("CLI --allow-partial-failure: want exit 0 with success false, got exit %d:\n%.1500s", code, out)
	}
	mcp := e.mcpCalls(t, []map[string]any{
		{"name": "prtg_execute", "arguments": map[string]any{"endpoint_id": "sensors.multi-resume", "params": map[string]any{"filter": `name = "Ping"`}, "confirm": true}},
	})
	if !strings.Contains(mcp, `"isError":true`) || !strings.Contains(mcp, "bulk action failed") || !strings.Contains(mcp, "BAD_REQUEST") {
		t.Fatalf("MCP: want an error result with the failed item, got:\n%.2000s", mcp)
	}
}

// Class handoff-recovery-omitted: a recovery whose way back was interrupted
// is reported, with its path, by the CLI and by the MCP tool.
func TestHandoffRecoveryOnCLIAndMCP(t *testing.T) {
	t.Parallel()
	host := honestServer(t)
	e := newEnv(t, host, true)
	out := e.run(t, "fleet", "handoff", "--since", "1h", "--json")
	if !strings.Contains(out, `"name": "VPN"`) || !strings.Contains(out, `"via": "after_gap"`) || !strings.Contains(out, `"event": "recovered"`) {
		t.Fatalf("CLI fleet handoff: want VPN recovered via after_gap, got:\n%.2000s", out)
	}
	mcp := e.mcpCalls(t, []map[string]any{{"name": "fleet_handoff", "arguments": map[string]any{"since": "1h"}}})
	if !strings.Contains(mcp, "VPN") || !strings.Contains(mcp, "after_gap") || !strings.Contains(mcp, "recovered") {
		t.Fatalf("MCP fleet_handoff: want VPN recovered via after_gap, got:\n%.2000s", mcp)
	}
}
