// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired (or, for the token
// id, on a build that held whole tokens). Set PROXMOX_LEAK_BIN_DIR to run them
// against binaries built elsewhere.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"proxmox-pp-cli/internal/credkit"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("drives Unix config-file modes; the Windows build is vetted and compiled by the gates")
	}
	binOnce.Do(func() {
		if d := os.Getenv("PROXMOX_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "proxmox-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("proxmox-cli"), "../../cmd/proxmox-cli"},
			{exe("proxmox-mcp"), "../../cmd/proxmox-mcp"},
		} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, b.out), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("proxmox-cli")), filepath.Join(binDir, exe("proxmox-mcp"))
}

// vendor is a TLS stub of a Proxmox VE or Backup Server API: the kit never
// sends a credential over plain http, loopback included.
type vendor struct {
	srv   *httptest.Server
	url   string
	hits  atomic.Int64
	conns atomic.Int64
	mu    sync.Mutex
	auths []string
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		v.mu.Lock()
		v.auths = append(v.auths, r.Header.Get("Authorization"))
		v.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	// A connection is counted before TLS: a refused request never dials.
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			v.conns.Add(1)
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	v.srv, v.url = srv, srv.URL
	return v
}

// hostPort is the stub's host:port, the form PROXMOX_PBS_HOST and
// clusters.toml take.
func (v *vendor) hostPort() string {
	u, _ := url.Parse(v.url)
	return u.Host
}

// sawSecret reports whether any request to the stub carried secret.
func (v *vendor) sawSecret(secret string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return slices.ContainsFunc(v.auths, func(a string) bool { return strings.Contains(a, secret) })
}

// caFile trusts every stub given.
func caFile(t *testing.T, vs ...*vendor) string {
	t.Helper()
	var data []byte
	for _, v := range vs {
		data = append(data, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: v.srv.Certificate().Raw})...)
	}
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// pveEnv points the CLI at a PVE stub with token.
func pveEnv(t *testing.T, v *vendor, token string) []string {
	return []string{"PROXMOX_BASE_URL=" + v.url + "/api2/json", "PROXMOX_CA_CERT=" + caFile(t, v), "PROXMOX_API_TOKEN=" + token}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "PROXMOX_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func configDir(home string) string { return filepath.Join(home, "config") }

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	out, code := runCode(t, bin, env, args...)
	// A refused start prints nothing, which every absence check would pass on.
	if code == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused, so this run proves nothing:\n%s", filepath.Base(bin), strings.Join(args[:min(3, len(args))], " "), out)
	}
	return out
}

func runCode(t *testing.T, bin string, env []string, args ...string) (string, int) {
	t.Helper()
	return runStdinCode(t, bin, env, "", args...)
}

func runStdinCode(t *testing.T, bin string, env []string, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run() // exit codes vary by case; the output is judged
	return out.String(), cmd.ProcessState.ExitCode()
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertAbsent(t *testing.T, label, out string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("%s shows %q:\n%s", label, s, out)
		}
	}
}

// assertNoFileCarries walks home; skip names the files the test itself wrote.
func assertNoFileCarries(t *testing.T, home string, skip []string, secrets ...string) {
	t.Helper()
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || slices.Contains(skip, p) {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		for _, s := range secrets {
			if bytes.Contains(data, []byte(s)) {
				t.Errorf("%s holds %q", p, s)
			}
		}
		return nil
	})
}

// mcpCall runs one MCP stdio session of tools/call params and returns stdout
// plus stderr, with every call answered.
func mcpCall(t *testing.T, bin string, env []string, calls ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = env
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for i, c := range calls {
		fmt.Fprintf(stdin, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":%s}`+"\n", i+2, c)
	}
	var out strings.Builder
	answered := 0
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for answered < len(calls) && sc.Scan() {
		out.WriteString(sc.Text() + "\n")
		var msg struct {
			ID *int `json:"id"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) == nil && msg.ID != nil && *msg.ID >= 2 {
			answered++
		}
	}
	_ = stdin.Close()
	_, _ = io.Copy(&out, stdout)
	_ = cmd.Wait()
	if answered != len(calls) {
		t.Errorf("MCP answered %d of %d calls:\n%s\n%s", answered, len(calls), out.String(), stderr.String())
	}
	return out.String() + stderr.String()
}

// Fixture token secrets: UUIDs written in two pieces, so no 8-character piece
// of one occurs in ordinary output.
const (
	pveSecret = "0f3c9a51-8d2e-" + "4b7a-9c61-5e2d8f4a7b13"
	pbsSecret = "7d1e4c2a-5b9f-" + "4e8d-a3c6-2f8b1d9e0a74"
	pveToken  = "root@pam!monitor=" + pveSecret
	pbsToken  = "backup@pbs!audit:" + pbsSecret
)

// TestTokenIDInOrdinaryReadIsShown (class public-token-id): USER@REALM!TOKENID
// is a token's public id, and Proxmox lists it in ordinary reads (the user of
// every task the token started). Only the SECRET is held, so a task list
// naming root@pam!monitor prints in full, on the CLI and over MCP. Seen failing
// on a build that held the whole token: every such read printed only the
// kit's withheld notice.
func TestTokenIDInOrdinaryReadIsShown(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"upid":"UPID:pve1:0001A2B3:00C4D5E6:6720F1A0:vzdump::root@pam!monitor:","node":"pve1","type":"vzdump","user":"root@pam!monitor","status":"OK","starttime":1730212256,"endtime":1730212356}]}`)
	})
	home := t.TempDir()
	env := closedEnv(home, pveEnv(t, v, pveToken)...)
	out := run(t, cliBin, env, "tasks", "cluster", "--json")
	if !strings.Contains(out, "root@pam!monitor") || !strings.Contains(out, "vzdump") {
		t.Errorf("tasks cluster lost an ordinary read naming the token id:\n%s", out)
	}
	assertAbsent(t, "tasks cluster", out, pveSecret[:12])
	mout := mcpCall(t, mcpBin, env, `{"name":"proxmox_read","arguments":{"endpoint_id":"tasks.cluster"}}`)
	if !strings.Contains(mout, "root@pam!monitor") {
		t.Errorf("MCP read lost an ordinary read naming the token id:\n%s", mout)
	}
	if !v.sawSecret(pveSecret) {
		t.Error("the PVE stub never received the token, so nothing was read")
	}
}

// TestAPIErrorEchoOfSecretIsWithheld (class secret-echo): an API error body
// that echoes part of the token's SECRET is withheld, on the CLI, --agent and
// MCP, and no file keeps it.
func TestAPIErrorEchoOfSecretIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	piece := pveSecret[:13]
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"data":null,"message":"invalid token value %s..."}`, piece)
	})
	home := t.TempDir()
	env := closedEnv(home, pveEnv(t, v, pveToken)...)
	assertAbsent(t, "cluster api-version", run(t, cliBin, env, "cluster", "api-version"), piece)
	assertAbsent(t, "cluster api-version --agent", run(t, cliBin, env, "cluster", "api-version", "--agent"), piece)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"proxmox_read","arguments":{"endpoint_id":"cluster.api-version"}}`), piece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece)
}

// TestFileTokensHeldAtStartup (class config-source): a token in config.toml
// (api_token, pbs_api_token), in the 0600 credentials store and in a config
// that does not parse is held before anything prints, so a startup
// diagnostic that echoes its secret (a flag error, in both binaries) is
// withheld.
func TestFileTokensHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct {
		name, file, body, secret string
	}{
		{"config-pve", "config.toml", "api_token = \"root@pam!cfg=cfgSecret-" + "9a8b7c6d5e4f\"\n", "cfgSecret-9a8b7c6d5e4f"},
		{"config-pbs", "config.toml", "pbs_api_token = \"backup@pbs!cfg:pbsCfgSecret-" + "1a2b3c4d\"\n", "pbsCfgSecret-1a2b3c4d"},
		{"malformed-config", "config.toml", "api_token = \"root@pam!m=malSecret-" + "1f2e3d4c5b6a\" broken\n", "malSecret-1f2e3d4c5b6a"},
		{"credentials-store", "../data/credentials.toml", "api_token = \"root@pam!s=stoSecret-" + "0a9b8c7d6e5f\"\n", "stoSecret-0a9b8c7d6e5f"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(configDir(home), c.file), c.body)
			env := closedEnv(home)
			assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", c.secret, "version"), c.secret)
			assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", c.secret), c.secret)
		})
	}
}

// TestEnvTokensHeldAtStartup (class env-source): PROXMOX_API_TOKEN's and
// PROXMOX_PBS_API_TOKEN's secrets are held before anything prints.
func TestEnvTokensHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	env := closedEnv(t.TempDir(), "PROXMOX_API_TOKEN="+pveToken, "PROXMOX_PBS_API_TOKEN="+pbsToken)
	for _, s := range []string{pveSecret, pbsSecret} {
		assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", s, "version"), s)
		assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", s), s)
	}
}

// TestEachTokenGoesOnlyToItsService (class origin): the PVE token reaches the
// PVE host only and the PBS token the PBS host only; a --deliver webhook whose
// URL carries the PVE secret is refused before it dials.
func TestEachTokenGoesOnlyToItsService(t *testing.T) {
	cliBin, _ := binaries(t)
	pve := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":{"version":"8.2.4","release":"8.2","repoid":"faa83925c9641325"}}`)
	})
	pbs := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":[{"userid":"backup@pbs","enable":true}]}`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	env := closedEnv(home, "PROXMOX_BASE_URL="+pve.url+"/api2/json", "PROXMOX_CA_CERT="+caFile(t, pve, pbs, other),
		"PROXMOX_API_TOKEN="+pveToken, "PROXMOX_PBS_HOST="+pbs.hostPort(), "PROXMOX_PBS_API_TOKEN="+pbsToken)
	run(t, cliBin, env, "cluster", "api-version", "--json")
	run(t, cliBin, env, "pbs-access", "users", "--json")
	if !pve.sawSecret(pveSecret) || !pbs.sawSecret(pbsSecret) {
		t.Fatalf("each service must receive its own token (pve %v, pbs %v)", pve.sawSecret(pveSecret), pbs.sawSecret(pbsSecret))
	}
	if pve.sawSecret(pbsSecret) || pbs.sawSecret(pveSecret) {
		t.Error("a token reached the other service")
	}
	run(t, cliBin, env, "cluster", "api-version", "--json", "--deliver", "webhook:"+other.url+"/hook/"+pveSecret)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the PVE secret", n)
	}
}

// TestClusterTokensHeldAndBound (class cluster-source): a clusters.toml
// entry's token, read from the environment variable the entry names, is held
// from startup, reaches that entry's host, and is refused toward another
// client's host.
func TestClusterTokensHeldAndBound(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "acmeSecret-" + "5c6d7e8f9a0b"
	acme := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":{"version":"8.2.4","release":"8.2","repoid":"faa83925c9641325"}}`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "clusters.toml"),
		"[clusters.acme]\nclient = \"Acme\"\npve_host = \""+acme.hostPort()+"\"\npve_token_env = \"ACME_PVE_TOKEN\"\n")
	env := closedEnv(home, "ACME_PVE_TOKEN=root@pam!acme="+secret, "PROXMOX_CA_CERT="+caFile(t, acme, other))
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	out := run(t, cliBin, env, "--cluster", "acme", "cluster", "api-version", "--json")
	if !acme.sawSecret(secret) {
		t.Fatalf("the acme cluster's host never received its token:\n%s", out)
	}
	run(t, cliBin, env, "--cluster", "acme", "cluster", "api-version", "--json", "--deliver", "webhook:"+other.url+"/hook/"+secret)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another client's host: %d connection(s) opened for a URL that carries the acme secret", n)
	}
	assertNoFileCarries(t, home, nil, secret)
}

// TestSetTokenStdinSecretHeld (class stdin): auth set-token reads the token
// from stdin; its secret is held where it is read, so nothing it prints
// carries it, and only the 0600 credentials store keeps it.
func TestSetTokenStdinSecretHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	env := closedEnv(home)
	out, code := runStdinCode(t, cliBin, env, pveToken+"\n", "auth", "set-token")
	if code == credkit.ExitRefused {
		t.Fatalf("auth set-token: the start was refused:\n%s", out)
	}
	assertAbsent(t, "auth set-token", out, pveSecret)
	assertAbsent(t, "auth status", run(t, cliBin, env, "auth", "status"), pveSecret)
	assertAbsent(t, "doctor", run(t, cliBin, env, "doctor"), pveSecret)
	var stores []string
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (d.Name() == "credentials.toml" || d.Name() == "config.toml") {
			stores = append(stores, p)
		}
		return nil
	})
	assertNoFileCarries(t, home, stores, pveSecret)

	// The saved token still works: a later run sends it to the PVE host.
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"data":{"version":"8.2.4","release":"8.2","repoid":"faa83925c9641325"}}`)
	})
	saved := closedEnv(home, "PROXMOX_BASE_URL="+v.url+"/api2/json", "PROXMOX_CA_CERT="+caFile(t, v))
	if out := run(t, cliBin, saved, "cluster", "api-version", "--json"); !v.sawSecret(pveSecret) || !strings.Contains(out, "8.2.4") {
		t.Errorf("the saved token was not sent to the PVE host, or its answer was lost:\n%s", out)
	}
}

// TestSpacefastTokenHeld (class spacefast-token): SPACEFAST_TOKEN, the
// credential of the --deliver spacefast: sink, is held from startup in both
// binaries.
func TestSpacefastTokenHeld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "sft_" + "live_4b7c9d1e2f3a"
	env := closedEnv(t.TempDir(), "SPACEFAST_TOKEN="+secret)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "CLI deliver target error", run(t, cliBin, env, "version", "--deliver", "spacefast:"+secret+"/x"), secret)
}

// TestJournalNeverRecordsAHeldSecret (class journal): the learn journal
// records a mistyped flag's name. A secret pasted where a flag name goes
// (--<secret>) must not reach state/learn/journal-*.jsonl; the journal appends
// through the kit (credkit.AppendFile), which withholds an entry carrying a
// held value.
func TestJournalNeverRecordsAHeldSecret(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	env := closedEnv(home, "PROXMOX_API_TOKEN="+pveToken)
	run(t, cliBin, env, "cluster", "api-version", "--"+pveSecret)
	run(t, cliBin, env, "cluster", "api-version", "--formt", pveSecret)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, pveSecret)
}
