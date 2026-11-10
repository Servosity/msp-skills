// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package vsphereblackbox runs the built CLI and MCP binaries against a fake
// vCenter that echoes every credential it was sent back in every encoding it
// can, whole and split across lines and adjacent values, and checks what a
// reviewer cannot read off the code (vsphere handfixes credkit-wiring,
// store-never-keeps-secrets, no-response-cache, no-credential-in-files,
// response-credentials-held, api-origin-pinned):
//
//   - nothing the program prints (stdout, stderr, MCP results and errors)
//     holds a credential: the password from the environment or the
//     credential store, the session ids vCenter mints at login (Automation
//     and VI/JSON), a credential-named flag value, a password a login fault
//     repeats, and the credentials a response carries;
//   - no file the program writes, except the 0600 credential store, holds
//     one either (session ids live in memory only);
//   - no credential leaves for a host other than the configured vCenter;
//   - the MCP server sends nothing but reads and the two logins.
//
// The oracle is the exact set of strings the server sent, so a pass does not
// depend on the program's own decoder.
package vsphereblackbox

import (
	"bytes"
	"compress/gzip"
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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	bbUser      = "ro@vsphere.local"
	bbBadUser   = "locked@vsphere.local"            // a login vCenter refuses with a fault that repeats the password
	bbPassword  = "vcPassw0rd-7fb3189bQ6u3sZxW4o1n" // VSPHERE_PASSWORD (gitleaks:allow: synthetic)
	bbStored    = "storedPassw0rd-71c3e9b05d2a4f68" // auth set-credentials (argv), the credential store
	bbBadPass   = "refusedPassw0rd-3b5d7f9a1c"      // the refused user's password
	bbAPISess   = "apiSess1on-5c1e9a7d3b4f8e21a6c"  // minted by POST /api/session (gitleaks:allow: synthetic)
	bbVISess    = "viSess1on-2e8a4c6f1b9d7a35e0c"   // minted by the VI/JSON SessionManager Login
	bbArgvSec   = "-argvSecret-91c3e7a5"            // --password on the command line (starts with "-")
	bbShortResp = "Sh0rtPw"                         // a 7-character credential a response carries
	bbBOMResp   = "BomRespSecret-6a1c"              // a credential in a BOM-prefixed response
	bbGzipResp  = "GzipRespToken-4f2a9c1e"          // a credential in a gzip-encoded response
	viRoot      = "/sdk/vim25/8.0.1.0"
)

// withheldNotice is the start of what the credential kit prints in place of a
// unit that carried a credential.
const withheldNotice = "[credkit: withheld"

func secrets() []string {
	return []string{bbPassword, bbStored, bbBadPass, bbAPISess, bbVISess, bbArgvSec, bbShortResp, bbBOMResp, bbGzipResp}
}

var bbCLI, bbMCP string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "vsphere-blackbox-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	bbCLI = filepath.Join(dir, "vsphere-cli"+exe)
	bbMCP = filepath.Join(dir, "vsphere-mcp"+exe)
	for out, pkg := range map[string]string{bbCLI: "../../cmd/vsphere-cli", bbMCP: "../../cmd/vsphere-mcp"} {
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
		"Basic " + base64.StdEncoding.EncodeToString([]byte(bbUser+":"+s)),
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
	url     string
	caFile  string
	quiet   bool // repeats nothing it was sent: the control for what reaches a sink
	mu      sync.Mutex
	offHost int
	sent    []string // "METHOD path" of every request the fake vCenter answered
}

func (sv *server) requests() []string {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return append([]string(nil), sv.sent...)
}

// tlsServer starts a TLS fake; the client trusts it through VSPHERE_CA_CERT.
func tlsServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// isList reports a collection path (/api/vcenter/vm, /api/cis/tagging/tag),
// which vCenter answers with an array.
func isList(p string) bool {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	return len(parts) == 3 && parts[1] == "vcenter" || len(parts) == 4 && parts[1] == "cis"
}

// quietServer is echoServer without the echoes: a real vCenter's answer, whose
// records must reach every sink the echoing server's are withheld from.
func quietServer(t *testing.T) *server {
	t.Helper()
	return newServer(t, true)
}

func echoServer(t *testing.T) *server {
	t.Helper()
	return newServer(t, false)
}

func newServer(t *testing.T, quiet bool) *server {
	t.Helper()
	sv := &server{quiet: quiet}
	// A second host the vCenter redirects to: a credential must never reach it.
	other := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sv.mu.Lock()
		sv.offHost++
		sv.mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		sent := sentSecrets(r)
		if sv.quiet {
			sent = nil
		}
		echo := echoObject(sent)
		p := r.URL.Path
		sv.mu.Lock()
		sv.sent = append(sv.sent, r.Method+" "+p)
		sv.mu.Unlock()
		// A fake vCenter has no budget to protect: without these the client's
		// starting pace was half of this package's run time.
		w.Header().Set("X-Ratelimit-Remaining", "1000000")
		w.Header().Set("X-Ratelimit-Reset", strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && p == "/api/session":
			if u, pw, ok := r.BasicAuth(); !ok || u == bbBadUser {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, `{"error_type":"UNAUTHENTICATED","messages":[{"default_message":"login refused for %s"}],"echo":%s}`, pw, echo)
				return
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, "%q", bbAPISess)
			return
		case p == viRoot+"/ServiceInstance/ServiceInstance/content":
			_, _ = w.Write([]byte(`{"about":{"name":"VMware VirtualCenter Server","version":"8.0.2","apiVersion":"8.0.2.0","instanceUuid":"vc-uuid"},"sessionManager":{"value":"SessionManager"},"licenseManager":{"value":"LicenseManager"}}`))
			return
		case r.Method == http.MethodPost && p == viRoot+"/SessionManager/SessionManager/Login":
			var login map[string]string
			_ = json.Unmarshal(body, &login)
			if login["userName"] == bbBadUser {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"_typeName":"InvalidLogin","faultstring":"Cannot complete login due to an incorrect user name or password %s","echo":%s}`, login["password"], echo)
				return
			}
			w.Header().Set("vmware-api-session-id", bbVISess)
			fmt.Fprintf(w, `{"key":"k1","userName":%q}`, login["userName"])
			return
		}
		if strings.HasPrefix(p, "/api/") && r.Header.Get("vmware-api-session-id") != bbAPISess {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error_type":"UNAUTHENTICATED"}`))
			return
		}
		if strings.HasPrefix(p, "/sdk/") && r.Header.Get("vmware-api-session-id") != bbVISess {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"_typeName":"NotAuthenticated"}`))
			return
		}
		extra := ""
		if len(sent) > 0 {
			// A record whose identifier repeats the session id it was sent.
			extra = fmt.Sprintf(`,{"vm":%q,"name":"Echo-ID"}`, sent[0])
		}
		switch {
		case sv.quiet && isList(p) && p != "/api/vcenter/vm":
			_, _ = w.Write([]byte(`[{"name":"HQ-Record"}]`))
		case p == "/api/vcenter/folder":
			http.Redirect(w, r, other.URL+p, http.StatusTemporaryRedirect)
		case p == "/api/vcenter/network":
			fmt.Fprintf(w, `[{"network":"network-1","name":"HQ-Net","type":"STANDARD_PORTGROUP","split":%s}]`, splitObject(sent))
		case p == "/api/vcenter/datastore":
			fmt.Fprintf(w, "\xef\xbb\xbf"+`[{"datastore":"datastore-11","name":%q,"type":"VMFS","password":%q}]`, bbBOMResp, bbBOMResp)
		case p == "/api/vcenter/resource-pool":
			fmt.Fprintf(w, `[{"resource_pool":"resgroup-9","name":%q,"password":%q}]`, bbShortResp, bbShortResp)
		case p == "/api/vcenter/cluster":
			var z bytes.Buffer
			zw := gzip.NewWriter(&z)
			fmt.Fprintf(zw, `[{"cluster":"domain-c8","name":"HQ-Cluster","AccessToken":%q,"echo":%s}]`, bbGzipResp, echoObject(append(sent, bbGzipResp)))
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(z.Bytes())
		case p == "/api/vcenter/vm":
			if !sv.quiet {
				// What the MCP server itself holds from a response (vm_list runs in
				// the server; most tools run in a CLI child).
				extra += fmt.Sprintf(`,{"vm":"vm-1022","name":"Held","AccessToken":%q,"password":%q}`, bbGzipResp, bbShortResp)
			}
			fmt.Fprintf(w, `[{"vm":"vm-1021","name":"HQ-Record","power_state":"POWERED_ON","cpu_count":2,"memory_size_MiB":4096,"echo":%s}%s]`, echo, extra)
		case isList(p):
			fmt.Fprintf(w, `[{"name":"HQ-Record","echo":%s}]`, echo)
		default:
			fmt.Fprintf(w, `{"name":"HQ-Record","echo":%s}`, echo)
		}
	}))
	sv.url = srv.URL
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	sv.caFile = ca
	return sv
}

type bbEnv struct {
	home string
	env  []string
	mcp  string // the MCP server to run, when not bbMCP
}

func newEnv(t *testing.T, sv *server, withEnvPassword bool) *bbEnv {
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
		"VSPHERE_BASE_URL=" + sv.url, "VSPHERE_CA_CERT=" + sv.caFile,
		"VSPHERE_CLI_PATH=" + bbCLI,
	}
	if withEnvPassword {
		env = append(env, "VSPHERE_USERNAME="+bbUser, "VSPHERE_PASSWORD="+bbPassword)
	}
	return &bbEnv{home: home, env: env}
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

// lockedBuffer is a bytes.Buffer a test can poll while a child writes to it.
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

// mcpCalls runs one MCP stdio session and returns everything the server
// wrote: every tools/call result and its stderr.
func (e *bbEnv) mcpCalls(t *testing.T, calls []map[string]any) string {
	t.Helper()
	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	bin := bbMCP
	if e.mcp != "" {
		bin = e.mcp
	}
	cmd := exec.Command(bin)
	cmd.Env = e.env
	cmd.Dir = e.home
	stdin, _ := cmd.StdinPipe()
	var out lockedBuffer
	var errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(stdin, &in)
	waitDone := make(chan struct{})
	go func() { _ = cmd.Wait(); close(waitDone) }()
	exited := func() bool {
		select {
		case <-waitDone:
			return true
		default:
			return false
		}
	}
	// One call at a time: the server answers concurrently, and what a later call
	// is judged against is what the earlier answers made it hold. A server that
	// exited (a refused start) answers nothing more.
	deadline := time.Now().Add(120 * time.Second)
	await := func(id int) {
		for time.Now().Before(deadline) && !exited() && !strings.Contains(out.String(), fmt.Sprintf(`"id":%d,`, id)) {
			time.Sleep(50 * time.Millisecond)
		}
	}
	await(0)
	for i, c := range calls {
		_ = json.NewEncoder(stdin).Encode(map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "tools/call", "params": c})
		await(i + 1)
	}
	_ = stdin.Close()
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

// battery runs every output path; pw is the password the process holds
// (the environment one, or the stored one when the environment has none).
func battery(e *bbEnv, deliverFile, pw string) [][]string {
	return [][]string{
		{"vm", "list", "--json"},
		{"vm", "list"},
		{"vm", "list", "--csv"},
		{"vm", "list", "--plain"},
		{"vm", "list", "--json", "--receipt"},
		{"vm", "list", "--json", "--audit-dir", filepath.Join(e.home, "audit")},
		{"vm", "list", "--json", "--deliver", "file:" + deliverFile},
		{"vm", "get", "vm-1021", "--json"},
		{"vm", "get", "vm-1021", "--dry-run", "--json"},
		{"host", "--json"},
		{"network", "--json"},
		{"datastore", "list", "--json"},
		{"resource-pool", "list", "--json"},
		{"cluster", "list", "--json"},
		{"folder", "--json"},
		{"session", "--json"},
		{"vi-vm", "snapshot", "vm-1021", "--json"},
		{"vi-vm", "summary", "vm-1021"},
		{"vi-host", "host-10", "--json"},
		{"vm", "list", "--password", bbArgvSec},
		{"vm", "list", "--json", "--" + pw},
		{"migration", "blockers", "--json"},
		{"snapshots", "audit", "--json"},
		{"licence", "exposure", "--json"},
		{"migration", "capacity", "--json"},
		{"migration", "proxmox-map", "--json"},
		{"rvtools", "export", "--json"},
		{"rvtools", "export", "--out", filepath.Join(e.home, "rvtools")},
		{"sync", "--json"},
		{"export", "vm", "--format", "json", "--output", filepath.Join(e.home, "export.json")},
		{"feedback", "noted the password " + pw + " in a ticket"},
		// Learnings, playbooks and learn events land in the local store's own tables.
		{"teach", "--query", "find vms " + pw, "--resource", "123", "--resource-type", "vm",
			"--notes", "rotated " + pw, "--playbook-notes", "use " + pw, "--no-validate"},
		{"doctor"},
		{"auth", "status"},
	}
}

func mcpBattery() []map[string]any {
	call := func(name string, args map[string]any) map[string]any {
		return map[string]any{"name": name, "arguments": args}
	}
	none := map[string]any{}
	return []map[string]any{
		call("vm_list", none),
		call("vm_get", map[string]any{"vm": "vm-1021"}),
		call("network_list", none),
		call("datastore_list", none),
		call("resource_pool_list", none),
		call("cluster_list", none),
		call("session_get", none),
		call("vi_vm_snapshot", map[string]any{"moId": "vm-1021"}),
		call("migration_blockers", none),
		call("snapshots_audit", none),
		call("licence_exposure", none),
		call("rvtools_export", none),
		call("sql", map[string]any{"query": "SELECT data FROM resources"}),
	}
}

// refusedLogin runs a CLI read and an MCP read as a user vCenter refuses: both
// login faults repeat the password they were sent.
func refusedLogin(t *testing.T, sv *server) string {
	e := newEnv(t, sv, false)
	e.env = append(e.env, "VSPHERE_USERNAME="+bbBadUser, "VSPHERE_PASSWORD="+bbBadPass)
	return e.run(t, "vm", "list", "--json") + e.run(t, "vi-vm", "snapshot", "vm-1021", "--json") +
		e.mcpCalls(t, []map[string]any{{"name": "vm_list", "arguments": map[string]any{}}})
}

// Class credential-persisted-in-files.
func TestNoCredentialInAnyWrittenFile(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, false)
	e.run(t, "auth", "set-credentials", bbUser, bbStored)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, deliverFile, bbStored) {
		e.run(t, args...)
	}
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
		if strings.HasPrefix(d.Name(), "credentials") {
			// The credential store: the one file allowed to hold a credential.
			sawCreds = sawCreds || bytes.Contains(data, []byte(bbStored))
			if info, err := d.Info(); err == nil && runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
			}
			for _, sess := range []string{bbAPISess, bbVISess} {
				if bytes.Contains(data, []byte(sess)) {
					t.Errorf("%s holds a session id; sessions live in memory only", rel)
				}
			}
			return nil
		}
		if !synccpuStoreFile(path) { // synccpu: the local store is not checked
			assertNoEcho(t, "file "+rel, string(data))
		}
		return nil
	})
	if scanned < 5 {
		t.Fatalf("only %d files written; the battery did not run", scanned)
	}
	if !sawCreds {
		t.Errorf("auth set-credentials wrote no credential store")
	}
	// The battery really reached the vCenter, so the scan above is not vacuous.
	if got := strings.Join(sv.requests(), "\n"); !strings.Contains(got, "POST /api/session") || !strings.Contains(got, "POST "+viRoot+"/SessionManager/SessionManager/Login") {
		t.Fatalf("the stored credentials never logged in:\n%.1500s", got)
	}
}

// The control for TestNoCredentialInAnyWrittenFile: a vCenter that repeats no
// credential has its records reach every file sink, so a write withheld there is
// the credential and not a broken sink.
func TestOrdinaryRecordsReachEveryFile(t *testing.T) {
	t.Parallel()
	e := newEnv(t, quietServer(t), true)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range [][]string{
		{"vm", "list", "--json", "--deliver", "file:" + deliverFile},
		{"export", "vm", "--format", "json", "--output", filepath.Join(e.home, "export.json")},
		{"sync", "--json"},
		{"rvtools", "export", "--out", filepath.Join(e.home, "rvtools")},
		{"teach", "--query", "find vms", "--resource", "123", "--resource-type", "vm", "--notes", "rotated", "--no-validate"},
		{"feedback", "an ordinary note"},
	} {
		if out := e.run(t, args...); strings.Contains(out, withheldNotice) {
			t.Errorf("%v withheld an ordinary record:\n%.1500s", args, out)
		}
	}
	if b, err := os.ReadFile(deliverFile); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("%s: want the delivered record (err %v)", filepath.Base(deliverFile), err)
	}
	if b, err := os.ReadFile(filepath.Join(e.home, "export.json")); err != nil || !bytes.Contains(b, []byte("HQ-Record")) {
		t.Errorf("export --output: want the exported record (err %v)", err)
	}
	var sawStore, sawLearning, sawRVTools bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.Contains(d.Name(), ".db") {
			sawStore = sawStore || bytes.Contains(b, []byte("HQ-Record"))
			sawLearning = sawLearning || bytes.Contains(b, []byte("rotated"))
		}
		sawRVTools = sawRVTools || strings.HasSuffix(d.Name(), ".csv") && bytes.Contains(b, []byte("HQ-Record"))
		return nil
	})
	if !sawStore {
		t.Errorf("no local store holds a fetched record")
	}
	if !sawLearning {
		t.Errorf("no local store holds the taught learning; the teach run wrote nothing")
	}
	if !sawRVTools {
		t.Errorf("rvtools export --out wrote no CSV tab holding a fetched record")
	}
}

// Class secret-in-output.
func TestNoCredentialInAnyOutput(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	deliverFile := filepath.Join(e.home, "deliver.json")
	for _, args := range battery(e, deliverFile, bbPassword) {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	refused := refusedLogin(t, sv)
	assertNoEcho(t, "output of a refused login", refused)
	if !strings.Contains(refused, "login") {
		t.Fatalf("the refused login printed no login error:\n%.1500s", refused)
	}
	// The unit that repeats a credential is withheld whole, with the kit's notice.
	if vm := e.run(t, "vm", "list", "--json"); !strings.Contains(vm, withheldNotice) {
		t.Fatalf("vm list --json repeats a credential and printed no notice:\n%.1500s", vm)
	}
	out := e.mcpCalls(t, mcpBattery())
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(mcpBattery()))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	assertJSONLines(t, out)
}

// The control for TestNoCredentialInAnyOutput: a vCenter that repeats no
// credential has its records printed, on the CLI and through the MCP server, and
// JSON output stays JSON.
func TestOrdinaryRecordsStillPrint(t *testing.T) {
	t.Parallel()
	e := newEnv(t, quietServer(t), true)
	cmd := exec.Command(bbCLI, "vm", "list", "--json")
	cmd.Env, cmd.Dir = e.env, e.home
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("vm list --json: %v", err)
	}
	var doc any
	if json.Unmarshal(stdout, &doc) != nil || !bytes.Contains(stdout, []byte("HQ-Record")) {
		t.Fatalf("want valid JSON with the record, got:\n%.1500s", stdout)
	}
	// vm_list runs in the server, migration_blockers in a CLI child that the kit
	// hands the server's credential (credkit.RunCommand, then credkit.Start).
	out := e.mcpCalls(t, []map[string]any{
		{"name": "vm_list", "arguments": map[string]any{}},
		{"name": "migration_blockers", "arguments": map[string]any{}},
	})
	if strings.Contains(out, withheldNotice) || strings.Count(out, "HQ-Record") < 2 {
		t.Fatalf("MCP results lost the record:\n%.3000s", out)
	}
}

// Class credential-sent-to-another-host: a redirect off the configured
// vCenter is refused before it is followed, from the CLI and the MCP server.
func TestRedirectNeverReachesAnotherHost(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	redirect := e.run(t, "folder", "--json")
	e.mcpCalls(t, []map[string]any{{"name": "folder_list", "arguments": map[string]any{}}})
	sv.mu.Lock()
	n := sv.offHost
	sv.mu.Unlock()
	if n != 0 {
		t.Fatalf("the other host was contacted %d time(s)", n)
	}
	if !strings.Contains(redirect, "leave the configured vCenter") {
		t.Fatalf("redirect: want the pin refusal, got:\n%.1500s", redirect)
	}
}

// The MCP server changes nothing in vCenter: every request a full MCP battery
// sends is a GET, apart from the two session logins.
func TestMCPSendsOnlyReadsAndLogins(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	e.mcpCalls(t, mcpBattery())
	reqs := sv.requests()
	if len(reqs) < len(mcpBattery()) {
		t.Fatalf("the MCP battery reached the vCenter only %d time(s)", len(reqs))
	}
	for _, r := range reqs {
		if strings.HasPrefix(r, "GET ") || r == "POST /api/session" || r == "POST "+viRoot+"/SessionManager/SessionManager/Login" {
			continue
		}
		t.Errorf("an MCP call sent %s", r)
	}
}
