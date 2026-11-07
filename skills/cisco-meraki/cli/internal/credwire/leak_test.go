// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired. Set
// CISCO_MERAKI_LEAK_BIN_DIR to run them against binaries built elsewhere.

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

	"cisco-meraki-pp-cli/internal/credkit"
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
		if d := os.Getenv("CISCO_MERAKI_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "cisco-meraki-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("cisco-meraki-cli"), "../../cmd/cisco-meraki-cli"},
			{exe("cisco-meraki-mcp"), "../../cmd/cisco-meraki-mcp"},
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
	return filepath.Join(binDir, exe("cisco-meraki-cli")), filepath.Join(binDir, exe("cisco-meraki-mcp"))
}

// vendor is a TLS stub of the Dashboard API: the kit never sends a credential
// over plain http, loopback included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	conns       atomic.Int64
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
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
	v.url = srv.URL
	v.caFile = filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(v.caFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v *vendor) env(key string) []string {
	return []string{"CISCO_MERAKI_BASE_URL=" + v.url + "/api/v1", "CISCO_MERAKI_CA_CERT=" + v.caFile, "MERAKI_DASHBOARD_API_KEY=" + key}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "CISCO_MERAKI_HOME=" + home,
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
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

// TestAPIErrorEchoOfKeyIsWithheld (class key-echo): an API error body that
// echoes part of MERAKI_DASHBOARD_API_KEY. Any 8-character piece of a held
// value withholds the output, on the CLI, --agent and MCP.
func TestAPIErrorEchoOfKeyIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "7d3f0c9a" + "51e24b86a0f7c3d9e1b2a4c6d8f0e2a4b6c8d0e2"
	piece := key[:12]
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"errors":["Invalid API key %s..."]}`, piece)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(key)...)
	assertAbsent(t, "organizations get", run(t, cliBin, env, "organizations", "get"), piece)
	assertAbsent(t, "organizations get --agent", run(t, cliBin, env, "organizations", "get", "--agent"), piece)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"cisco-meraki_read","arguments":{"endpoint_id":"organizations.get"}}`), piece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece)
}

// TestResponseSecretFieldIsWithheld (class tenant-secret): a read whose answer
// carries a declared Dashboard secret field (an SSID psk, a VPN peer secret)
// never prints the value, on the CLI and over MCP, and the local mirror never
// stores it. --select without the field still prints the rest.
func TestResponseSecretFieldIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "4c8e2a6f" + "0b1d3f5a7c9e1b3d5f7a9c1e3b5d7f9a1c3e5b7d"
	const psk = "Gu3stW1fi" + "Passphrase!2026"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"number":0,"name":"Guest","enabled":true,"authMode":"psk","psk":%q}]`, psk)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(key)...)
	assertAbsent(t, "get-network-ssids", run(t, cliBin, env, "networks", "wireless", "get-network-ssids", "N_1001", "--json"), psk)
	assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, `{"name":"cisco-meraki_read","arguments":{"endpoint_id":"networks.wireless.get-network-ssids","params":{"networkId":"N_1001"}}}`), psk)
	if out := run(t, cliBin, env, "networks", "wireless", "get-network-ssids", "N_1001", "--json", "--select", "number,name"); !strings.Contains(out, "Guest") {
		t.Errorf("--select without the secret field lost the rest of the answer:\n%s", out)
	}
	if v.hits.Load() == 0 {
		t.Error("no request reached the API stub")
	}
	assertNoFileCarries(t, home, nil, psk)
}

// TestConfigKeyHeldAtStartup (class config-source): the key in config.toml
// (dashboard_api_key), in the 0600 credentials store and in a config that does
// not parse is held before anything prints, so a startup diagnostic that echoes
// it (a flag error, in both binaries) is withheld.
func TestConfigKeyHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct {
		name, file, body, secret string
	}{
		{"config", "config.toml", "dashboard_api_key = \"cfgKey" + "9a8b7c6d5e4f3a2b\"\n", "cfgKey9a8b7c6d5e4f3a2b"},
		{"malformed-config", "config.toml", "dashboard_api_key = \"malKey" + "1f2e3d4c5b6a7980\" broken\n", "malKey1f2e3d4c5b6a7980"},
		{"credentials-store", "../data/credentials.toml", "dashboard_api_key = \"stoKey" + "0a9b8c7d6e5f4a3b\"\n", "stoKey0a9b8c7d6e5f4a3b"},
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

// TestProfileBaseURLSecretHeldAtStartup (class profile-value): a password
// inside a base-url saved in profiles.json is held at startup.
func TestProfileBaseURLSecretHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const secret = "profPw" + "8c7b6a5f4e3d"
	home := t.TempDir()
	writeFile(t, filepath.Join(configDir(home), "profiles.json"),
		`{"profiles":{"acme":{"name":"acme","values":{"base-url":"https://ops:`+secret+`@api.meraki.com/api/v1"}}}}`)
	env := closedEnv(home)
	assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "--timeout", secret, "version"), secret)
	assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "--transport", secret), secret)
	assertAbsent(t, "profile show", run(t, cliBin, env, "profile", "show", "acme"), secret)
}

// TestKeyRefusedOffOriginAndOverPlainHTTP (class origin): the key goes only to
// the configured Dashboard API, and never over plain http. A --deliver webhook
// whose URL carries the key is refused before it dials.
func TestKeyRefusedOffOriginAndOverPlainHTTP(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "9e1c3a5b" + "7d9f1b3c5e7a9c1d3f5b7e9a1c3d5f7b9e1a3c5d"
	api := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `[{"id":"1001","name":"Acme MSP Client"}]`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	env := closedEnv(home, api.env(key)...)
	run(t, cliBin, env, "organizations", "get", "--json", "--deliver", "webhook:"+other.url+"/hook/"+key)
	if n := other.conns.Load(); n != 0 {
		t.Errorf("webhook to another https origin: %d connection(s) opened for a URL that carries the key", n)
	}
	if api.hits.Load() == 0 {
		t.Error("the API stub was never read, so no webhook was attempted")
	}

	var keyHits atomic.Int64
	plainAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("X-Cisco-Meraki-API-Key"), key) || strings.Contains(r.Header.Get("Authorization"), key) {
			keyHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[]`)
	}))
	t.Cleanup(plainAPI.Close)
	env = closedEnv(t.TempDir(), "CISCO_MERAKI_BASE_URL="+plainAPI.URL+"/api/v1", "MERAKI_DASHBOARD_API_KEY="+key)
	assertAbsent(t, "plain http API", run(t, cliBin, env, "organizations", "get"), key)
	if n := keyHits.Load(); n != 0 {
		t.Errorf("plain http API: %d request(s) carried the key", n)
	}
}

// TestSetTokenStdinKeyHeld (class stdin): auth set-token reads the key from
// stdin; it is held where it is read, so nothing it prints carries it, and
// only the 0600 credentials store keeps it.
func TestSetTokenStdinKeyHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "5a7c9e1b" + "3d5f7a9c1e3b5d7f9a1c3e5b7d9f1a3c5e7b9d1f"
	home := t.TempDir()
	env := closedEnv(home)
	assertAbsent(t, "auth set-token", runStdin(t, cliBin, env, key+"\n", "auth", "set-token"), key)
	assertAbsent(t, "auth status", run(t, cliBin, env, "auth", "status"), key)
	assertAbsent(t, "doctor", run(t, cliBin, env, "doctor"), key)
	store, _ := filepath.Glob(filepath.Join(home, "*", "credentials.toml"))
	assertNoFileCarries(t, home, store, key)
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

// TestJournalNeverRecordsAHeldKey (class journal): the learn journal records a
// mistyped flag's name. A key pasted where a flag name goes (--<key>) must not
// reach state/learn/journal-*.jsonl; the journal appends through the kit
// (credkit.AppendFile), which withholds an entry carrying a held value.
func TestJournalNeverRecordsAHeldKey(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "c3e5a7b9" + "d1f3a5c7e9b1d3f5a7c9e1b3d5f7a9c1e3b5d7f9"
	home := t.TempDir()
	env := closedEnv(home, "MERAKI_DASHBOARD_API_KEY="+key)
	run(t, cliBin, env, "organizations", "get", "--"+key)
	run(t, cliBin, env, "organizations", "get", "--orgnization", key)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, key)
}

// runStdin is run with stdin.
func runStdin(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	if cmd.ProcessState.ExitCode() == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused:\n%s", filepath.Base(bin), strings.Join(args, " "), out.String())
	}
	return out.String()
}
