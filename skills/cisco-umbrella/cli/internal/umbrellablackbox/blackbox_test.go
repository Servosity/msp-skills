// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package umbrellablackbox runs the built CLI and MCP binaries against an
// Umbrella API that echoes every credential it knows back in every encoding
// it can, whole and split across lines and adjacent values, and checks what a
// reviewer cannot read off the code (umbrella handfixes credkit-wiring,
// store-never-keeps-secrets, no-response-cache, response-credentials-held,
// api-origin-pinned). The server speaks TLS: the credential kit refuses a
// credential over plain http. It stops echoing on request (server.honest), so
// a "the data still lands" assertion tests the record and not the notice.
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the API key and secret, the access token and the
//     child-org token they mint, a password, a PSK and a client secret given
//     as flags or on stdin, and the API key secret, S3 secret key and tunnel
//     PSK the API returns;
//   - no file the program writes, except the 0600 credential store, holds
//     one either;
//   - no credential leaves for a host other than the configured one.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder. No fixture credential shares an 8-character
// run with a field name: the kit matches a held value's pieces, so a value spelled
// "listedClientId-..." would withhold every record with a clientId member.
package umbrellablackbox

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
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
	"sync/atomic"
	"testing"
	"time"
)

const (
	bbKey       = "umbKEY7fb3189b" + "Q6u3sZxW4o1n7R8v52" // CISCO_UMBRELLA_API_KEY
	bbSecret    = "umbSEC" + "globexA9d2c7e41f0b63X5w8Lq" // CISCO_UMBRELLA_API_SECRET
	bbMinted    = "mintedAccess-71c3e9b05d2a4f68"         // POST /auth/v2/token access_token
	bbChild     = "mintedChildOrg-2e8a4c6f1b9d3e57"       // the child-org token (X-Umbrella-OrgId)
	bbPassword  = "UmbPassw0rd-" + "5c1e9a7d3b"           // users create --password
	bbPsk       = "TunnelPsk-8e2d4f6a1c9b7e3d"            // tunnels update-credentials --psk-secret
	bbNewSecret = "newMintedValue-4d8b2e6f1a9c3e70"       // POST /admin/v2/apiKeys clientSecret
	bbS3Secret  = "s3BucketValue-9a7c5e3b1d2f4a6c"        // POST /admin/v2/iam/rotateKey secretAccessKey
	bbShortPass = "Ab1c2D"                                // a 6-character password flag
	bbDashPass  = "-dashPass-91c3e7a5"                    // a password flag value that starts with "-"
	bbStdinPass = "StdinPass-4f2a9c1e"                    // a password in a --stdin request body
	bbShortResp = "Sh0rtPw"                               // a 7-character credential a response carries
	bbBOMResp   = "BomRespSecret-6a1c"                    // a credential in a BOM-prefixed response
	bbLoginKey  = "loginKEY-c4e1a9d7b3f5"                 // auth login --client-id
	bbLoginSec  = "loginSECRET-7b2d9e4f1a6c8e30"          // auth login --client-secret
	bbHTTPToken = "mcpHttpToken-0123456789abcdefABCDEF"   // CISCO_UMBRELLA_MCP_HTTP_TOKEN
	bbListedKey = "listedPublicHalf-5e7a1c9d3b2f4a60"     // GET /admin/v2/apiKeys keys[].clientId (KeyForList)
	bbS3KeyID   = "AKIA" + "REVIEW7Q2X9K4M1Z"             // POST /admin/v2/iam/rotateKey currentKeyId (KeyRotation)
	bbS3OldKey  = "AKIA" + "OLDKEY3H8V5N2B6C"             // POST /admin/v2/iam/rotateKey oldKeyId (KeyRotation)
	bbMCPPass   = "McpParamPw-9e3a7c1d5b"                 // a password nested in cisco-umbrella_execute params
	bbWithheld  = "credkit: withheld"                     // the kit's notice in place of a whole unit
)

func secrets() []string {
	return []string{bbKey, bbSecret, bbMinted, bbChild, bbPassword, bbPsk, bbNewSecret, bbS3Secret, bbShortPass, bbDashPass,
		bbStdinPass, bbShortResp, bbBOMResp, bbLoginKey, bbLoginSec, bbHTTPToken, bbListedKey, bbS3KeyID, bbS3OldKey, bbMCPPass}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	// Short skips the builds; Windows skips the POSIX pipes. The kit's Windows
	// stdio path is covered by the kit's own CI, and the Windows build of both
	// binaries by the matrix.
	if testing.Short() || runtime.GOOS == "windows" {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "umbrella-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "cisco-umbrella-cli"+exe)
	bbMCP = filepath.Join(dir, "cisco-umbrella-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/cisco-umbrella-cli", bbMCP: "../../cmd/cisco-umbrella-mcp"} {
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

// sentSecrets is every known secret the request carried (its headers, a
// decoded Basic credential, the URL or the body): a real API can echo only
// what it was sent.
func sentSecrets(r *http.Request) []string {
	body, _ := io.ReadAll(r.Body)
	seen := fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
	if u, p, ok := r.BasicAuth(); ok {
		seen += "\n" + u + "\n" + p
	}
	var out []string
	for _, s := range secrets() {
		if strings.Contains(seen, s) {
			out = append(out, s)
		}
	}
	return out
}

type server struct {
	url         string
	caFile      string
	otherURL    string
	hits        atomic.Int32 // requests the API received, the token endpoint's included
	writes      atomic.Int32 // non-GET requests that were not a token exchange
	honest      atomic.Bool  // true: echo nothing, the records only
	rejectMint  atomic.Bool  // true: the token endpoint answers 401, echoing what it was sent
	routes      map[string]string
	routeStatus atomic.Int32 // the status routes answer with; 0 is 200
	mu          sync.Mutex
	offHost     int
	mintUser    string // the last token exchange's Basic credentials
	mintPass    string
	lastAuth    string // the last API request's Authorization header
}

func (sv *server) off() int {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.offHost
}

func (sv *server) auth() string {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.lastAuth
}

func (sv *server) mintBasic() (string, string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.mintUser, sv.mintPass
}

func echoServer(t *testing.T) *server {
	t.Helper()
	sv := &server{}
	// A second host the API redirects to: a credential must never reach it.
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(other.Close)
	sv.otherURL = other.URL
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		sent := sentSecrets(r)
		honest := sv.honest.Load()
		echoOf := func(extra ...string) string {
			if honest {
				return "{}"
			}
			return echoObject(append(append([]string{}, sent...), extra...))
		}
		if p == "/auth/v2/token" {
			if u, pw, ok := r.BasicAuth(); ok {
				sv.mu.Lock()
				sv.mintUser, sv.mintPass = u, pw
				sv.mu.Unlock()
			}
		} else {
			sv.mu.Lock()
			sv.lastAuth = r.Header.Get("Authorization")
			sv.mu.Unlock()
			if r.Method != http.MethodGet {
				sv.writes.Add(1)
			}
		}
		for prefix, body := range sv.routes {
			if strings.HasPrefix(p, prefix) {
				if st := sv.routeStatus.Load(); st != 0 {
					w.WriteHeader(int(st))
				}
				fmt.Fprint(w, body)
				return
			}
		}
		switch {
		case r.Method == http.MethodPost && p == "/auth/v2/token" && sv.rejectMint.Load():
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid_client","echo":%s}`, echoObject(sent))
		case r.Method == http.MethodPost && p == "/auth/v2/token" && strings.Contains(fmt.Sprint(sent), bbLoginKey):
			// An OAuth error that names the client it refused: in that
			// process only the argv hold knows --client-id's value.
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid_client","error_description":"unknown client %s"}`, bbLoginKey)
		case r.Method == http.MethodPost && p == "/auth/v2/token":
			tok := bbMinted
			if r.Header.Get("X-Umbrella-OrgId") != "" {
				tok = bbChild
			}
			fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer","expires_in":3600,"echo":%s}`, tok, echoOf(tok))
		case r.Method == http.MethodPost && p == "/admin/v2/apiKeys":
			// Deflate-encoded, as an API behind a compressing proxy answers.
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			fmt.Fprintf(zw, `{"id":"k1","name":"bb","key":"newKeyIdPublic","clientSecret":%q,"echo":%s}`, bbNewSecret, echoOf(bbNewSecret))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(z.Bytes())
		case r.Method == http.MethodPost && strings.HasSuffix(p, "/refresh"):
			fmt.Fprintf(w, `{"id":"k1","clientSecret":%q,"echo":%s}`, bbNewSecret, echoOf(bbNewSecret))
		case r.Method == http.MethodPost && p == "/admin/v2/iam/rotateKey":
			// KeyRotation (vendor-docs/cisco_umbrella_s_3_bucket_key_rotation_api_1_0_0.yaml):
			// the AWS access key IDs pair with the secret access key.
			fmt.Fprintf(w, `{"oldKeyId":%q,"currentKeyId":%q,"secretAccessKey":%q,"keyCreationDate":"2025-03-12T22:00:00","echo":%s}`,
				bbS3OldKey, bbS3KeyID, bbS3Secret, echoOf(bbS3Secret))
		case r.Method == http.MethodGet && p == "/admin/v2/apiKeys":
			// KeysResponseList (vendor-docs/cisco_umbrella_key_admin_api_2_0_0.yaml): every
			// key's clientId, the half of a credential sent with its secret for a token.
			fmt.Fprintf(w, `{"message":"success","offset":0,"limit":10,"total":1,"keys":[{"name":"Networks RW","id":"00000000-0000-0000-0000-000000000001","clientId":%q,"creatorKeyId":"130eec4c3825`+`463e8d06f1dd9c0b2c19","creatorName":"Rosa","createdAt":"2022-10-02T20:00:00Z","scopes":["deployments.networks:read"]}]}`, bbListedKey)
		case strings.HasSuffix(p, "/admin/v2/users/100"):
			fmt.Fprintf(w, `{"id":100,"firstname":%q,"email":"rosa@example.com","password":%q}`, bbShortResp, bbShortResp)
		case strings.HasSuffix(p, "/roamingcomputers/41"):
			fmt.Fprintf(w, "\xef\xbb\xbf"+`{"deviceId":"41","name":%q,"password":%q}`, bbBOMResp, bbBOMResp)
		case strings.HasPrefix(p, "/deployments/v2/sites"):
			w.WriteHeader(http.StatusUnauthorized)
			if honest {
				fmt.Fprint(w, `{"message":"invalid token"}`)
				return
			}
			fmt.Fprintf(w, `{"message":"invalid token %s","echo":%s}`, strings.Join(sent, " "), echoOf())
		case strings.HasPrefix(p, "/deployments/v2/internaldomains"):
			http.Redirect(w, r, other.URL+p, http.StatusFound)
		case strings.HasPrefix(p, "/deployments/v2/tags") && !honest:
			fmt.Fprintf(w, `[{"id":50,"name":"Servers","split":%s}]`, splitObject(sent))
		case strings.HasPrefix(p, "/admin/v2/managed/customers"):
			fmt.Fprintf(w, `[{"customerId":1234567,"customerName":"Acme","seats":10,"echo":%s}]`, echoOf())
		case r.Method != http.MethodGet:
			fmt.Fprintf(w, `{"echo":%s}`, echoOf())
		default:
			extra := ""
			if len(sent) > 0 && !honest {
				// A record whose identifier repeats the token it was sent.
				extra = fmt.Sprintf(`,{"originId":%q,"name":"Echo-ID","status":"open"}`, sent[0])
			}
			fmt.Fprintf(w, `[{"originId":2045,"name":"HQ-Network","status":"open","psk":%q,"echo":%s}%s]`, bbPsk, echoOf(), extra)
		}
	}))
	t.Cleanup(srv.Close)
	sv.url = srv.URL
	sv.caFile = filepath.Join(shortTemp(t), "ca.pem")
	var pemBuf bytes.Buffer
	_ = pem.Encode(&pemBuf, &pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	_ = pem.Encode(&pemBuf, &pem.Block{Type: "CERTIFICATE", Bytes: other.Certificate().Raw})
	if err := os.WriteFile(sv.caFile, pemBuf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return sv
}

// shortTemp is an empty directory. Not t.TempDir: its path carries the test's name,
// and some long runs of letters and digits spend the kit's decoding budget, so the
// kit refuses a store under them (.review/kit-findings.md). No real home does.
func shortTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "umb-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

type bbEnv struct {
	home string
	env  []string
}

func newEnv(t *testing.T, sv *server, withEnvKey bool) *bbEnv {
	t.Helper()
	home := shortTemp(t)
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"CISCO_UMBRELLA_BASE_URL=" + sv.url, "CISCO_UMBRELLA_TOKEN_URL=" + sv.url + "/auth/v2/token", "CISCO_UMBRELLA_CA_CERT=" + sv.caFile,
		"CISCO_UMBRELLA_CLI_PATH=" + bbCLI, "CISCO_UMBRELLA_MCP_HTTP_TOKEN=" + bbHTTPToken,
	}
	if withEnvKey {
		env = append(env, "CISCO_UMBRELLA_API_KEY="+bbKey, "CISCO_UMBRELLA_API_SECRET="+bbSecret)
	}
	return &bbEnv{home: home, env: env}
}

func (e *bbEnv) with(extra ...string) *bbEnv {
	return &bbEnv{home: e.home, env: append(append([]string(nil), e.env...), extra...)}
}

// without is e with the named variables unset.
func (e *bbEnv) without(names ...string) *bbEnv {
	var env []string
	for _, kv := range e.env {
		name, _, _ := strings.Cut(kv, "=")
		keep := true
		for _, n := range names {
			keep = keep && n != name
		}
		if keep {
			env = append(env, kv)
		}
	}
	return &bbEnv{home: e.home, env: env}
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

func battery(e *bbEnv, exportFile, deliverFile, listFile string) [][]string {
	return [][]string{
		{"networks", "list", "--json"},
		{"networks", "list"},
		{"networks", "list", "--csv"},
		{"tags", "list", "--json"},
		{"tags", "list"},
		{"sites", "list", "--json"},
		{"internal-domains", "list", "--json"},
		{"roaming-computers", "list", "--json", "--receipt"},
		{"networks", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"networks", "list", "--json", "--deliver", "file:" + deliverFile},
		{"sync", "--resources", "networks,roaming_computers,tags"},
		{"--org", "1234567", "networks", "list", "--json"},
		{"export", "networks", "-o", exportFile},
		{"api-keys", "list", "--json"},
		{"api-keys", "list"},
		{"users", "create", "--email", "a@example.com", "--password", bbPassword, "--role-id", "2", "--json"},
		{"users", "create", "--email", "a@example.com", "--password=" + bbPassword, "--bogus-flag"},
		{"users", "create", "--email", "a@example.com", "--password", bbShortPass, "--role-id", "2", "--dry-run", "--json"},
		{"users", "create", "--email", "a@example.com", "--password", bbDashPass, "--role-id", "2", "--dry-run", "--json"},
		{"users", "create", "--email", "a@example.com", "--password", bbShortPass, "--role-id", "2", "--json"},
		{"users", "get", "100", "--json"},
		{"users", "get", "100"},
		{"roaming-computers", "get", "41", "--json"},
		{"tunnels", "update-credentials", "7", "--auto-rotate", "false", "--psk-secret", bbPsk, "--psk-id-prefix", "x", "--dry-run", "--json"},
		{"tunnels", "update-credentials", "7", "--auto-rotate", "false", "--psk-secret", bbPsk, "--psk-id-prefix", "x", "--json"},
		{"feedback", "noted the key " + bbSecret + " in a ticket"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find networks " + bbSecret, "--resource", "123", "--resource-type", "networks",
			"--notes", "rotated " + bbSecret, "--playbook-notes", "use " + bbSecret, "--no-validate"},
		{"networks", "list", "--" + bbSecret},
		{"fleet", "health", "--json"},
		{"verdict", "example.com", "--json"},
		{"hygiene", "--json"},
		{"allowlist", "audit", "--json"},
		{"destination-lists", "diff", "11", "--file", listFile, "--json"},
		{"search", "HQ", "--json"},
		{"doctor"},
		{"auth", "status"},
		{"auth", "login", "--client-id", bbLoginKey, "--client-secret", bbLoginSec},
	}
}

func mcpBattery() []map[string]any {
	return []map[string]any{
		{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "networks.list"}},
		{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "tags.list"}},
		{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "sites.list"}},
		{"name": "cisco-umbrella_execute", "arguments": map[string]any{"endpoint_id": "users.create", "params": map[string]any{"email": "u@example.com", "password": bbPassword, "role_id": 2}, "confirm": true}},
		{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "users.get", "params": map[string]any{"userId": "100"}}},
		{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "api_keys.list"}},
		{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "roaming_computers.get", "params": map[string]any{"deviceId": "41"}}},
		{"name": "fleet_health", "arguments": map[string]any{}},
		{"name": "verdict", "arguments": map[string]any{"domain": "example.com"}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// stdinRuns feed a request body on stdin (users create --stdin): previewed
// and sent.
func stdinRuns(e *bbEnv, t *testing.T) string {
	body := fmt.Sprintf(`{"email":"s@example.com","password":%q,"roleId":2}`, bbStdinPass)
	return e.runIn(t, body, "users", "create", "--stdin", "--dry-run", "--json") + e.runIn(t, body, "users", "create", "--stdin", "--json")
}

func writeListFile(t *testing.T, e *bbEnv) string {
	p := filepath.Join(e.home, "blocklist.txt")
	if err := os.WriteFile(p, []byte("bad.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, false)
	e.run(t, "auth", "login", "--client-id", bbKey, "--client-secret", bbSecret)
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	listFile := writeListFile(t, e)
	for _, args := range battery(e, exportFile, deliverFile, listFile) {
		e.run(t, args...)
	}
	stdinRuns(e, t)
	e.mcpCalls(t, mcpBattery())
	var scanned int
	var sawCreds bool
	scan := func() {
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
			if strings.HasPrefix(d.Name(), "credentials") {
				// The credential store: the one file allowed to hold a credential.
				sawCreds = sawCreds || bytes.Contains(data, []byte(bbSecret)) || bytes.Contains(data, []byte(bbLoginSec))
				if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
					t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
				}
				return nil
			}
			assertNoEcho(t, "file "+rel, string(data))
			return nil
		})
	}
	scan()
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth login wrote no credential store")
	}
	// Against an API that echoes nothing the same writes do write data, so the
	// scan is not vacuous; and then it runs again, over what landed.
	sv.honest.Store(true)
	e.run(t, "networks", "list", "--json", "--deliver", "file:"+deliverFile)
	e.run(t, "sync", "--resources", "networks,roaming_computers,tags")
	e.run(t, "export", "networks", "-o", exportFile)
	e.run(t, "teach", "--query", "find networks", "--resource", "123", "--resource-type", "networks", "--notes", "rotated on schedule", "--no-validate")
	scan()
	for _, path := range []string{exportFile, deliverFile} {
		if b, err := os.ReadFile(path); err != nil || !bytes.Contains(b, []byte("HQ-Network")) {
			t.Errorf("%s: want the exported record (err %v)", filepath.Base(path), err)
		}
	}
	var sawStore, sawLearning bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".db") {
			if b, rerr := os.ReadFile(path); rerr == nil {
				sawStore = sawStore || bytes.Contains(b, []byte("HQ-Network"))
				sawLearning = sawLearning || bytes.Contains(b, []byte("rotated on schedule"))
			}
		}
		return nil
	})
	if !sawStore {
		t.Errorf("no local store holds the synced record")
	}
	if !sawLearning {
		t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
	}
}

// Class secret-in-output.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	exportFile := filepath.Join(e.home, "export.jsonl")
	deliverFile := filepath.Join(e.home, "deliver.json")
	listFile := writeListFile(t, e)
	for _, args := range battery(e, exportFile, deliverFile, listFile) {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	assertNoEcho(t, "output of users create --stdin", stdinRuns(e, t))
	// A response that repeats a held value is withheld whole, with the kit's notice.
	if out := e.run(t, "networks", "list", "--json"); !strings.Contains(out, bbWithheld) {
		t.Errorf("want the kit's notice for an echoing response, got:\n%.1500s", out)
	}
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	// An API that echoes nothing: the records print, the credential members are
	// replaced by name, and JSON output stays valid JSON.
	sv.honest.Store(true)
	cmd := exec.Command(bbCLI, "networks", "list", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("networks list --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Network")) || !bytes.Contains(stdout, []byte("[redacted]")) {
		t.Fatalf("want valid JSON with the record and the psk redacted, got:\n%.1500s", stdout)
	}
	assertNoEcho(t, "honest networks list", string(stdout))
	honestMCP := e.mcpCalls(t, []map[string]any{{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "networks.list"}}})
	assertNoEcho(t, "honest MCP session", honestMCP)
	if !strings.Contains(honestMCP, "HQ-Network") {
		t.Fatalf("MCP results lost the record itself:\n%.2000s", honestMCP)
	}
}

// Class credential-sent-to-another-host: a redirect off the configured API
// host is refused before anything is sent there.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	out := e.run(t, "internal-domains", "list", "--json")
	e.mcpCalls(t, []map[string]any{{"name": "cisco-umbrella_read", "arguments": map[string]any{"endpoint_id": "internal_domains.list"}}})
	if n := sv.off(); n != 0 {
		t.Fatalf("the redirect target was contacted %d time(s)", n)
	}
	if !strings.Contains(out, "leave the configured Umbrella API host") {
		t.Fatalf("want the pin refusal, got:\n%.1500s", out)
	}
}
