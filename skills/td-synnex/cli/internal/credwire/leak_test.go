// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Each test is one credential class; its failing inputs were
// seen failing on the binaries built before the kit was wired. Set
// TD_SYNNEX_LEAK_BIN_DIR to run them against binaries built elsewhere.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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

	"td-synnex-pp-cli/internal/credkit"
)

const testAccount = "10505"

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
		if d := os.Getenv("TD_SYNNEX_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "td-synnex-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{
			{exe("td-synnex-cli"), "../../cmd/td-synnex-cli"},
			{exe("td-synnex-mcp"), "../../cmd/td-synnex-mcp"},
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
	return filepath.Join(binDir, exe("td-synnex-cli")), filepath.Join(binDir, exe("td-synnex-mcp"))
}

// vendor is a TLS stub of the ION API and its OAuth endpoints: the kit never
// sends a credential over plain http, loopback included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	conns       atomic.Int64
	mu          sync.Mutex
	auth        []string
	bodies      []string
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		v.mu.Lock()
		v.auth = append(v.auth, r.Header.Get("Authorization"))
		v.bodies = append(v.bodies, string(body))
		v.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if h != nil {
			h(w, r)
		}
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

// carried reports whether any request this stub received carried s in its
// Authorization header or body.
func (v *vendor) carried(s string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range v.auth {
		if strings.Contains(v.auth[i], s) || strings.Contains(v.bodies[i], s) {
			return true
		}
	}
	return false
}

// env points the program at the stub (API under /api/v3, OAuth under
// /oauth on the same origin, as ionOAuthURL derives it).
func (v *vendor) env(token string) []string {
	e := []string{"TD_SYNNEX_BASE_URL=" + v.url + "/api/v3", "TD_SYNNEX_CA_CERT=" + v.caFile, "TD_SYNNEX_ACCOUNT_ID=" + testAccount}
	if token != "" {
		e = append(e, "TD_SYNNEX_ACCESS_TOKEN="+token)
	}
	return e
}

// closedEnv is a closed environment under one sandbox home; every press
// directory resolves under it.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "TD_SYNNEX_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + filepath.Join(home, "tmp"), "TD_SYNNEX_NO_AUTO_REFRESH=1", "NO_COLOR=1"}, extra...)
}

func newHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	return home
}

func run(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	return runStdin(t, bin, env, "", args...)
}

func runStdin(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	out, code := execBin(t, bin, env, stdin, args...)
	// A refused start prints nothing, which every absence check would pass on.
	if code == credkit.ExitRefused {
		t.Errorf("%s %s: the start was refused, so this run proves nothing:\n%s", filepath.Base(bin), strings.Join(args[:min(3, len(args))], " "), out)
	}
	return out
}

// execBin runs bin and returns its combined output and exit code.
func execBin(t *testing.T, bin string, env []string, stdin string, args ...string) (string, int) {
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
	if os.Getenv("TD_SYNNEX_LEAK_DEBUG") != "" {
		t.Logf("%s %v:\n%s", filepath.Base(bin), args, out.String())
	}
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return out.String(), code
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

// assertNoFileCarries walks home; skip names the files the program may keep a
// credential in (the 0600 credential store and the files the test wrote).
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

// credentialsFile is the 0600 store under a sandbox home (TD_SYNNEX_HOME/data).
func credentialsFile(home string) string { return filepath.Join(home, "data", "credentials.toml") }

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
	if os.Getenv("TD_SYNNEX_LEAK_DEBUG") != "" {
		t.Logf("mcp:\n%s\n%s", out.String(), stderr.String())
	}
	if answered != len(calls) {
		t.Errorf("MCP answered %d of %d calls:\n%s\n%s", answered, len(calls), out.String(), stderr.String())
	}
	return out.String() + stderr.String()
}

// echoVendor answers every API call 404 with the token it was sent echoed in
// the error body: the whole Authorization header and a piece of the token, or
// (piece) the piece alone, which nothing that matches only whole values hides.
// The OAuth endpoints answer 400 with the posted form echoed back.
func echoVendor(t *testing.T, piece bool) *vendor {
	return startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/oauth/") {
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":"invalid_grant","error_description":"rejected %s"}`, body)
			return
		}
		auth := r.Header.Get("Authorization")
		tok := strings.TrimPrefix(auth, "Bearer ")
		w.WriteHeader(http.StatusNotFound)
		if piece {
			fmt.Fprintf(w, `{"error":{"code":404,"message":"no account for key %s"}}`, tok[:min(12, len(tok))])
			return
		}
		fmt.Fprintf(w, `{"error":{"code":404,"message":"no account for %s (key %s)"}}`, auth, tok[:min(12, len(tok))])
	})
}

// TestHeldCredentialNeverEchoed (class 1): an ION error body that echoes a held
// credential, whole or as a piece, never reaches stdout, stderr, an MCP result
// or any file the program writes (store, cache, journal). Sources: the access
// token from the environment, the access token stored in the 0600 credentials
// file, the refresh token from the environment (read outside Config), the
// tokens auth mint receives, and a refresh token piped on stdin.
func TestHeldCredentialNeverEchoed(t *testing.T) {
	cliBin, mcpBin := binaries(t)

	t.Run("env access token", func(t *testing.T) {
		const token = "envAcc" + "Q7rZ81mLp0xV"
		piece := token[:12]
		for _, onlyPiece := range []bool{false, true} {
			v := echoVendor(t, onlyPiece)
			home := newHome(t)
			env := closedEnv(home, v.env(token)...)
			assertAbsent(t, "customers list", run(t, cliBin, env, "customers", "list", "--data-source", "live"), piece, token)
			assertAbsent(t, "customers list --agent", run(t, cliBin, env, "customers", "list", "--data-source", "live", "--agent"), piece, token)
			assertAbsent(t, "MCP customers_list", mcpCall(t, mcpBin, env, `{"name":"customers_list","arguments":{"accountId":"`+testAccount+`"}}`), piece, token)
			if v.hits.Load() == 0 {
				t.Error("no request reached the API stub, so nothing was echoed")
			}
			assertNoFileCarries(t, home, nil, piece)
		}
	})

	t.Run("stored access token", func(t *testing.T) {
		const token = "storAcc" + "J4nW2qT9sYb8"
		piece := token[:12]
		for _, onlyPiece := range []bool{false, true} {
			v := echoVendor(t, onlyPiece)
			home := newHome(t)
			writeFile(t, credentialsFile(home), "access_token = \""+token+"\"\n")
			env := closedEnv(home, v.env("")...)
			assertAbsent(t, "customers list", run(t, cliBin, env, "customers", "list", "--data-source", "live"), piece, token)
			assertAbsent(t, "MCP customers_list", mcpCall(t, mcpBin, env, `{"name":"customers_list","arguments":{"accountId":"`+testAccount+`"}}`), piece, token)
			if v.hits.Load() == 0 {
				t.Error("no request reached the API stub, so nothing was echoed")
			}
			assertNoFileCarries(t, home, []string{credentialsFile(home)}, piece)
		}
	})

	t.Run("env refresh token", func(t *testing.T) {
		// auth_ion.go reads TD_SYNNEX_REFRESH_TOKEN outside Config; the kit
		// holds it from SecretEnv. The body echoes it alone (raw, URL-encoded
		// once and twice, base64) and as a piece, so nothing that masks the
		// access token can hide it. sync printed exactly such a 403 body
		// before the ion-mask-env-refresh-token hand-fix.
		const token = "envAcc" + "Hk3Nw8Rt5Zq1"
		const refresh = "envRef" + "Mv6/Cx2+Lb9=Pd4"
		forms := []string{refresh, url.QueryEscape(refresh), url.QueryEscape(url.QueryEscape(refresh)),
			base64.StdEncoding.EncodeToString([]byte(refresh)), refresh[:12]}
		v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `{"error":{"code":403,"message":"grant %s | %s | %s | %s | prefix %s"}}`, forms[0], forms[1], forms[2], forms[3], forms[4])
		})
		home := newHome(t)
		env := closedEnv(home, append(v.env(token), "TD_SYNNEX_REFRESH_TOKEN="+refresh)...)
		assertAbsent(t, "customers list", run(t, cliBin, env, "customers", "list", "--data-source", "live"), forms...)
		assertAbsent(t, "sync", run(t, cliBin, env, "sync", "--resources", "customers"), forms...)
		assertAbsent(t, "MCP customers_list", mcpCall(t, mcpBin, env, `{"name":"customers_list","arguments":{"accountId":"`+testAccount+`"}}`), forms...)
		if v.hits.Load() == 0 {
			t.Error("no request reached the API stub, so nothing was echoed")
		}
		assertNoFileCarries(t, home, nil, forms...)
	})

	t.Run("minted tokens", func(t *testing.T) {
		const stdinRefresh = "stdRef" + "Wq7Ze3Xn6Ka2"
		const minted = "mintAcc" + "Ty5Us8Gh1Jf4"
		const rotated = "rotRef" + "Bn2Mk7Vc4Xs9"
		v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/oauth/token" {
				fmt.Fprintf(w, `{"access_token":%q,"refresh_token":%q,"expires_in":7200,"token_type":"Bearer"}`, minted, rotated)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"code":401,"message":"unauthorized"}}`)
		})
		home := newHome(t)
		env := closedEnv(home, v.env("")...)
		out := runStdin(t, cliBin, env, stdinRefresh+"\n", "auth", "mint", "--refresh-token-stdin", "--print-access-token", "--json")
		assertAbsent(t, "auth mint --print-access-token --json", out, minted, rotated, stdinRefresh)
		out = runStdin(t, cliBin, env, stdinRefresh+"\n", "auth", "mint", "--refresh-token-stdin", "--print-access-token")
		assertAbsent(t, "auth mint --print-access-token", out, minted, rotated, stdinRefresh)
		if !v.carried(stdinRefresh) {
			t.Error("the refresh token never reached the stub's token endpoint, so nothing was minted")
		}
		assertNoFileCarries(t, home, []string{credentialsFile(home)}, minted, rotated, stdinRefresh)
	})

	t.Run("stdin refresh token echoed by the token endpoint", func(t *testing.T) {
		const stdinRefresh = "stdRef" + "Pa4Lo9Ik2Uj7"
		v := echoVendor(t, false)
		home := newHome(t)
		env := closedEnv(home, v.env("")...)
		out := runStdin(t, cliBin, env, stdinRefresh+"\n", "auth", "mint", "--refresh-token-stdin", "--json")
		assertAbsent(t, "auth mint (rejected)", out, stdinRefresh[:12])
		if !v.carried(stdinRefresh) {
			t.Error("the refresh token never reached the stub's token endpoint")
		}
		assertNoFileCarries(t, home, nil, stdinRefresh[:12])
	})
}

// TestMalformedConfigCredentialHeldAtStartup (class 2): a credential in a
// config.toml that does not parse is held before the first byte of output, in
// both mains. A startup flag error echoed the value when it was also pasted
// where a flag value goes; the kit holds it before the flag parser runs and
// withholds that error.
func TestMalformedConfigCredentialHeldAtStartup(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "malCfg" + "Rf8Yt3Wq6Ez1"
	home := newHome(t)
	writeFile(t, filepath.Join(home, "config", "config.toml"), "synnex_access_token = \""+token+"\"\n= broken\n")
	env := closedEnv(home)
	for _, c := range []struct {
		label, bin string
		args       []string
	}{
		{"CLI startup flag error", cliBin, []string{"--timeout", token, "version"}},
		{"MCP startup flag error", mcpBin, []string{"--transport", token}},
		{"MCP unknown flag", mcpBin, []string{"--" + token}},
	} {
		out, code := execBin(t, c.bin, env, "", c.args...)
		assertAbsent(t, c.label, out, token)
		if code == 0 {
			t.Errorf("%s: exit 0 on a bad flag:\n%s", c.label, out)
		}
	}
	// With the value only in the file, the program starts and reads it.
	assertAbsent(t, "CLI config load", run(t, cliBin, env, "customers", "list", testAccount), token)
	assertAbsent(t, "MCP config load", mcpCall(t, mcpBin, env, `{"name":"customers_list","arguments":{"accountId":"`+testAccount+`"}}`), token)
}

// TestCredentialNeverSentOffOrigin (class 3): an ION credential goes only to
// the configured base URL's origin, over https: never over plain http (not
// even to loopback), never across a redirect to another origin, and never to a
// --deliver webhook on another origin.
func TestCredentialNeverSentOffOrigin(t *testing.T) {
	cliBin, _ := binaries(t)

	t.Run("plain http base URL", func(t *testing.T) {
		const token = "plainAcc" + "Dg5Sx8Cv2Bn7"
		var carried atomic.Int64
		plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.Header.Get("Authorization"), token) {
				carried.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"customers":[]}`)
		}))
		t.Cleanup(plain.Close)
		env := closedEnv(newHome(t), "TD_SYNNEX_BASE_URL="+plain.URL+"/api/v3", "TD_SYNNEX_ACCOUNT_ID="+testAccount, "TD_SYNNEX_ACCESS_TOKEN="+token)
		assertAbsent(t, "plain http API", run(t, cliBin, env, "customers", "list", "--data-source", "live"), token)
		if n := carried.Load(); n != 0 {
			t.Errorf("plain http API: %d request(s) carried the token", n)
		}
	})

	t.Run("redirect to another origin", func(t *testing.T) {
		const token = "redirAcc" + "Kj6Hg9Fd3Sa1"
		other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"customers":[]}`) })
		api := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", other.url+r.URL.Path)
			w.WriteHeader(http.StatusTemporaryRedirect)
		})
		// The second stub's certificate is trusted too, so only the kit can refuse it.
		bundle := filepath.Join(t.TempDir(), "both.pem")
		a, _ := os.ReadFile(api.caFile)
		b, _ := os.ReadFile(other.caFile)
		writeFile(t, bundle, string(a)+string(b))
		env := closedEnv(newHome(t), "TD_SYNNEX_BASE_URL="+api.url+"/api/v3", "TD_SYNNEX_CA_CERT="+bundle,
			"TD_SYNNEX_ACCOUNT_ID="+testAccount, "TD_SYNNEX_ACCESS_TOKEN="+token)
		run(t, cliBin, env, "customers", "list", "--data-source", "live")
		if api.hits.Load() == 0 {
			t.Fatal("the API stub was never called, so no redirect was offered")
		}
		if other.carried(token) {
			t.Error("the token followed a redirect to another origin")
		}
	})

	t.Run("deliver webhook on another origin", func(t *testing.T) {
		const stdinRefresh = "hookRef" + "Lz3Xc6Vb9Nm2"
		const minted = "hookAcc" + "Qw4Er7Ty1Ui5"
		api := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"hookRot-Pq8Wd2Fz5Gx","expires_in":7200}`, minted)
		})
		// deliver allows plain http to loopback, so a loopback hook is
		// reachable without a certificate; only the kit stands in the way.
		var hookCarried atomic.Int64
		hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if b, _ := io.ReadAll(r.Body); bytes.Contains(b, []byte(minted)) {
				hookCarried.Add(1)
			}
		}))
		t.Cleanup(hook.Close)
		env := closedEnv(newHome(t), api.env("")...)
		out := runStdin(t, cliBin, env, stdinRefresh+"\n", "auth", "mint", "--refresh-token-stdin", "--print-access-token", "--json",
			"--deliver", "webhook:"+hook.URL+"/hook")
		assertAbsent(t, "auth mint --deliver", out, minted)
		if !api.carried(stdinRefresh) {
			t.Fatal("the token endpoint never received the refresh token, so nothing was minted to deliver")
		}
		if n := hookCarried.Load(); n != 0 {
			t.Errorf("the minted access token was delivered to %s (%d request(s))", hook.URL, n)
		}
	})

	t.Run("hostile base URL in a config profile", func(t *testing.T) {
		// The stored refresh token was issued by the api stub (its
		// credentials file records that origin). A config profile naming
		// another base URL derives the OAuth token URL from that host.
		const stored = "profRef" + "Yh2Uj5Ik8Ol3"
		api := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"access_token":"`+"profAcc-"+"Rt6Yu9Io2Pa"+`","refresh_token":"`+"profRot-"+"Df5Gh8Jk1Lz"+`","expires_in":7200}`)
		})
		hostile := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"access_token":"`+"hostAcc-"+"Mn3Bv6Cx9Za"+`","refresh_token":"`+"hostRot-"+"Qa2Ws5Ed8Rf"+`","expires_in":7200}`)
		})
		home := newHome(t)
		writeFile(t, credentialsFile(home), "refresh_token = \""+stored+"\"\nbase_url = \""+api.url+"/api/v3\"\n")
		writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+api.url+"/api/v3\"\n")
		profile := filepath.Join(t.TempDir(), "profile.toml")
		writeFile(t, profile, "base_url = \""+hostile.url+"/api/v3\"\n")
		env := closedEnv(home, "TD_SYNNEX_CA_CERT="+api.caFile, "TD_SYNNEX_ACCOUNT_ID="+testAccount)
		run(t, cliBin, env, "auth", "mint", "--config", profile, "--json")
		if hostile.carried(stored) {
			t.Errorf("the stored refresh token, issued by %s, was posted to the profile's base URL %s", api.url, hostile.url)
		}
		// The same profile with the run's base URL set in the environment.
		env = closedEnv(home, append(api.env(""), "TD_SYNNEX_CA_CERT="+api.caFile)...)
		run(t, cliBin, env, "auth", "mint", "--config", profile, "--json")
		if hostile.carried(stored) {
			t.Errorf("the stored refresh token went to the profile's base URL while TD_SYNNEX_BASE_URL named %s", api.url)
		}
	})
}

// TestJournalNeverRecordsAHeldCredential (class 4): the learn journal records
// TD_SYNNEX_LEARN_SESSION and a mistyped flag's name. A credential set as the
// session or pasted where a flag name goes landed in
// state/learn/journal-*.jsonl; the journal appends through the kit
// (credkit.AppendFile), which withholds an entry carrying a held value.
func TestJournalNeverRecordsAHeldCredential(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "jrnlAcc" + "Mx4Nc7Bv1Zq8"
	home := newHome(t)
	env := closedEnv(home, "TD_SYNNEX_ACCESS_TOKEN="+token, "TD_SYNNEX_ACCOUNT_ID="+testAccount, "TD_SYNNEX_LEARN_SESSION="+token)
	run(t, cliBin, env, "customers", "list", "--"+token)
	run(t, cliBin, env, "customers", "list", "--custmer", token)
	journals, _ := filepath.Glob(filepath.Join(home, "state", "learn", "journal-*.jsonl"))
	if len(journals) == 0 {
		t.Fatal("no journal segment was written, so nothing was checked")
	}
	assertNoFileCarries(t, home, nil, token)
}

// TestMCPConfirmPreviewNeverEchoesACredentialArgument (class 5): an MCP write
// tool previews its request until confirm is boolean true, and the preview
// echoed the arguments; a tool error can quote them too. A password inside a
// body argument (MCPArguments) is held before the handler runs, so neither the
// preview nor an error that quotes it reaches the client.
func TestMCPConfirmPreviewNeverEchoesACredentialArgument(t *testing.T) {
	_, mcpBin := binaries(t)
	const token = "mcpAcc" + "Ga3Sd6Fg9Hj2"
	const password = "tenantPw" + "Xc5Vb8Nm1Qw4"
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":{"code":400,"message":"invalid order: %s"}}`, url.QueryEscape(string(body)))
	})
	home := newHome(t)
	env := closedEnv(home, v.env(token)...)
	args := `"accountId":"` + testAccount + `","customerId":"42","displayName":"seats","orderItems":[{"skuId":"m365-e3","quantity":1,"password":"` + password + `"}]`
	out := mcpCall(t, mcpBin, env,
		`{"name":"customer_orders_create-order","arguments":{`+args+`}}`,
		`{"name":"customer_orders_create-order","arguments":{`+args+`,"confirm":true}}`)
	assertAbsent(t, "MCP create-order preview and error", out, password, password[:12])
	assertNoFileCarries(t, home, nil, password)
}
