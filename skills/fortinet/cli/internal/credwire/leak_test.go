// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each input is a credential class's failing input, seen
// failing on the binaries built before the kit was wired
// (.review/kit-failing-first.log in the publish worktree). Set
// FORTINET_LEAK_BIN_DIR to run them against binaries built elsewhere.

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

	"fortinet-pp-cli/internal/cliutil"
	"fortinet-pp-cli/internal/credkit"
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
		if d := os.Getenv("FORTINET_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "fortinet-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("fortinet-cli"), "../../cmd/fortinet-cli"},
			{exe("fortinet-mcp"), "../../cmd/fortinet-mcp"},
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
	return filepath.Join(binDir, exe("fortinet-cli")), filepath.Join(binDir, exe("fortinet-mcp"))
}

// vendor is a TLS stub of a FortiOS REST API: the kit never sends a credential
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

func (v *vendor) env(token string) []string {
	return []string{"FORTINET_BASE_URL=" + v.url, "FORTINET_CA_CERT=" + v.caFile, "FORTINET_API_TOKEN=" + token}
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "FORTINET_HOME=" + home,
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

// TestAPIErrorEchoOfTokenIsWithheld (class token-echo): a FortiOS error body
// that echoes a piece of FORTINET_API_TOKEN. Any 8-character piece of a held
// value withholds the output, on the CLI and the MCP server.
func TestAPIErrorEchoOfTokenIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "fgtok-" + "Zq81LmP0xV7c"
	piece := token[:12]
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"http_status":401,"status":"error","error":"token %s... rejected"}`, piece)
	})
	home := t.TempDir()
	env := closedEnv(home, v.env(token)...)
	assertAbsent(t, "monitor status", run(t, cliBin, env, "monitor", "status"), piece)
	assertAbsent(t, "monitor status --agent", run(t, cliBin, env, "monitor", "status", "--agent"), piece)
	assertAbsent(t, "MCP fortinet_read", mcpCall(t, mcpBin, env, `{"name":"fortinet_read","arguments":{"endpoint_id":"monitor.status"}}`), piece)
	if v.hits.Load() == 0 {
		t.Error("no request reached the FortiOS stub, so nothing was echoed")
	}
	assertNoFileCarries(t, home, nil, piece)
}

// TestFMGStatusEchoOfKeyIsWithheld (kit finding: FortiManager JSON-RPC status
// text echoes FORTINET_FMG_API_KEY into errors and MCP results). The key is a
// startup credential, held before anything prints.
func TestFMGStatusEchoOfKeyIsWithheld(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const key = "fmgkey-" + "Hq72WnB4cR9s"
	fmg := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"id":1,"result":[{"status":{"code":-11,"message":"No permission for the request with key %s"},"url":"/dvmdb/adom"}]}`, key)
	})
	home := t.TempDir()
	env := closedEnv(home, "FORTINET_FMG_URL="+fmg.url, "FORTINET_FMG_API_KEY="+key, "FORTINET_FMG_INSECURE=true")
	out := run(t, cliBin, env, "fmg", "adoms")
	assertAbsent(t, "fmg adoms", out, key, key[:12])
	assertAbsent(t, "MCP fmg_adoms", mcpCall(t, mcpBin, env, `{"name":"fmg_adoms","arguments":{}}`), key, key[:12])
	if fmg.hits.Load() == 0 {
		t.Fatalf("no request reached the FortiManager stub:\n%s", out)
	}
}

// TestCloudTokenCacheHeldAtStartup (kit finding: the FortiGate Cloud token
// cache file). The cache is a credential source: its minted tokens are held at
// the next start, so a FortiOS response that carries one is withheld.
func TestCloudTokenCacheHeldAtStartup(t *testing.T) {
	cliBin, _ := binaries(t)
	const access, refresh = "fcacc-" + "Lk47PqZ2xW8m", "fcref-" + "Tn58RsY3vB1d"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"success","serial":"FGT60F","version":"v7.4.4","results":{"hostname":"%s","alias":"%s"}}`, access, refresh)
	})
	home := t.TempDir()
	t.Setenv("FORTINET_HOME", home)
	state, err := cliutil.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(state, "fortigate-cloud-token.json")
	writeFile(t, cache, fmt.Sprintf(`{"user":"api-user","access_token":%q,"refresh_token":%q,"expires_at":"2099-01-01T00:00:00Z"}`, access, refresh))
	env := closedEnv(home, v.env("fgtok-"+"Mx63JkD9pQ2w")...)
	out := run(t, cliBin, env, "monitor", "status", "--json")
	assertAbsent(t, "monitor status", out, access, refresh)
	if v.hits.Load() == 0 {
		t.Fatalf("no request reached the FortiOS stub:\n%s", out)
	}
	assertNoFileCarries(t, home, []string{cache}, access, refresh)
}

// TestDeviceTokenEchoIsWithheldAndSentOnlyToItsDevice (class token-echo on a
// fleet device): a device that echoes its own token_file token. The token is
// held the moment it is read, so the echo is withheld; the device still gets
// its token, from a client outside the kit's transport (the device URL comes
// from the operator's devices file, which no build-time origin can list).
func TestDeviceTokenEchoIsWithheldAndSentOnlyToItsDevice(t *testing.T) {
	cliBin, _ := binaries(t)
	const devTok = "devtok-" + "Rb29XcV6nM4k"
	var sawToken atomic.Bool
	dev := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+devTok {
			sawToken.Store(true)
		}
		fmt.Fprintf(w, `{"status":"success","serial":"FGT60F","version":"v7.4.4","results":{"hostname":"echo %s","version":"v7.4.4"}}`, devTok)
	})
	home := t.TempDir()
	tokFile := filepath.Join(home, "tokens", "hq")
	writeFile(t, tokFile, devTok+"\n")
	devices := filepath.Join(home, "devices.toml")
	writeFile(t, devices, fmt.Sprintf("[[device]]\nname = \"hq\"\nurl = %q\ntoken_file = %q\ninsecure = true\n", dev.url, tokFile))
	env := closedEnv(home, "FORTINET_DEVICES="+devices)
	for _, args := range [][]string{{"fleet", "health", "--json"}, {"fleet", "audit", "--json"}} {
		assertAbsent(t, strings.Join(args, " "), run(t, cliBin, env, args...), devTok, devTok[:12])
	}
	if !sawToken.Load() {
		t.Fatal("the device never received its own token: the fleet client did not send it")
	}
	assertNoFileCarries(t, home, []string{tokFile, devices}, devTok)
}
