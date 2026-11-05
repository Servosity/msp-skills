// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Set CLOUDALLY_LEAK_BIN_DIR to run them against binaries built
// elsewhere: that is how the pre-kit build was seen failing them.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
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
)

// Fixture credentials: every 8-character window mixes letters and digits, so
// none of them occurs in the program's ordinary output.
const (
	clientID     = "cid_W2e4" + "R6t8Y0u2"
	clientSecret = "cs_q7Zx9" + "Kp2Lm4Vb8Nw"
	accessToken  = "at_P9o8I" + "7u6Y5t4R3e2"
	refreshTok   = "rt_M1n2B" + "3v4C5x6Z7l8"
	partnerPass  = "pp_H5g4F" + "3d2S1a0Q9w8"
	oldPassword  = "op_r4T8y" + "Q2wE6u1I5o9"
	newPassword  = "np_k3J7h" + "G1fD5s9A2z6"
	storageKey   = "sk_V6c5X" + "4z3L2k1J0h9"
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
		if d := os.Getenv("CLOUDALLY_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "cloudally-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"cloudally-cli", "../../cmd/cloudally-cli"}, {"cloudally-mcp", "../../cmd/cloudally-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, exe(b.out)), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("cloudally-cli")), filepath.Join(binDir, exe("cloudally-mcp"))
}

// closedEnv is a closed environment under one sandbox home.
func closedEnv(home string, extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "CLOUDALLY_HOME=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1"}, extra...)
}

func run(t *testing.T, bin string, env []string, stdin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run() // exit codes vary by case; the output is judged
	// A refused start prints next to nothing, which every absence check would pass on.
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 78 {
		t.Errorf("%s %s: the start was refused, so this run proves nothing:\n%s", filepath.Base(bin), strings.Join(args[:min(3, len(args))], " "), out.String())
	}
	return out.String()
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

// mcpSession runs one MCP stdio session: initialize, then each request, and
// returns stdout plus stderr once every request is answered.
func mcpSession(t *testing.T, bin string, env []string, args []string, requests ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
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
	for i, r := range requests {
		fmt.Fprintf(stdin, `{"jsonrpc":"2.0","id":%d,%s}`+"\n", i+2, r)
	}
	var out strings.Builder
	answered := 0
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for answered < len(requests) && sc.Scan() {
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
	if answered != len(requests) {
		t.Errorf("MCP answered %d of %d requests:\n%s\n%s", answered, len(requests), out.String(), stderr.String())
	}
	return out.String() + stderr.String()
}

// TestMalformedConfigHeldBeforeAnythingPrints: a flag error prints before any
// command loads config, from a config.toml that does not parse, in both binaries.
func TestMalformedConfigHeldBeforeAnythingPrints(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	cases := []struct{ name, file, secret string }{
		{"broken-line-after", "client_secret = \"" + clientSecret + "\"\nbroken = = longenoughvalue\n", clientSecret},
		{"bare-value", "access_token = " + accessToken + "\nbroken = = longenoughvalue\n", accessToken},
		{"unterminated-array-before", "extra = [\"filler-value\",\nrefresh_token = \"" + refreshTok + "\"\n", refreshTok},
		{"partner-password", "partner_password = '" + partnerPass + "'\nbroken = = longenoughvalue\n", partnerPass},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "config.toml"), c.file)
			env := closedEnv(home)
			assertAbsent(t, "CLI startup flag error", run(t, cliBin, env, "", "--timeout", c.secret, "version"), c.secret)
			assertAbsent(t, "MCP startup flag error", run(t, mcpBin, env, "", "--transport", c.secret), c.secret)
		})
	}
}

// TestDiscoveryCoversEveryFileLoadReads: a credential in any file a run could read
// is held before a flag error prints, the use-account sidecar and an explicit
// config named by CLOUDALLY_CONFIG included.
func TestDiscoveryCoversEveryFileLoadReads(t *testing.T) {
	cliBin, _ := binaries(t)
	files := map[string]struct {
		path func(home string) string
		body string
	}{
		"explicit-config":     {func(h string) string { return filepath.Join(h, "tenant", "config.toml") }, `client_secret = "` + clientSecret + `"` + "\n"},
		"explicit-store":      {func(h string) string { return filepath.Join(h, "tenant", "data", "credentials.toml") }, `client_secret = "` + clientSecret + `"` + "\n"},
		"global-config":       {func(h string) string { return filepath.Join(h, "config", "config.toml") }, `client_secret = "` + clientSecret + `"` + "\n"},
		"global-store":        {func(h string) string { return filepath.Join(h, "data", "credentials.toml") }, `access_token = "` + clientSecret + `"` + "\n"},
		"legacy-config":       {func(h string) string { return filepath.Join(h, ".config", "cloudally-cli", "config.toml") }, `client_secret = "` + clientSecret + `"` + "\n"},
		"home-flag-config":    {func(h string) string { return filepath.Join(h, "other", "config", "config.toml") }, `client_secret = "` + clientSecret + `"` + "\n"},
		"home-flag-store":     {func(h string) string { return filepath.Join(h, "other", "data", "credentials.toml") }, `refresh_token = "` + clientSecret + `"` + "\n"},
		"use-account-sidecar": {func(h string) string { return filepath.Join(h, "state", "auth-context.json") }, `{"scope":"account","partner_access_token":"` + clientSecret + `"}`},
		"home-flag-sidecar":   {func(h string) string { return filepath.Join(h, "other", "state", "auth-context.json") }, `{"partner_refresh_token":"` + clientSecret + `"}`},
	}
	for name, f := range files {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, f.path(home), f.body)
			args := []string{"--config", filepath.Join(home, "tenant", "config.toml"), "--home", filepath.Join(home, "other"), "--timeout", clientSecret, "version"}
			assertAbsent(t, name, run(t, cliBin, closedEnv(home), "", args...), clientSecret)
			// CLOUDALLY_CONFIG names the explicit config the way --config does.
			env := closedEnv(home, "CLOUDALLY_CONFIG="+filepath.Join(home, "tenant", "config.toml"))
			assertAbsent(t, name+" (CLOUDALLY_CONFIG)", run(t, cliBin, env, "", "--home", filepath.Join(home, "other"), "--timeout", clientSecret, "version"), clientSecret)
		})
	}
}

// TestCredentialFlagsAndStdinAreHeld: a credential flag, a --stdin body and a
// --password-stdin password are held before a preview or an error prints them.
func TestCredentialFlagsAndStdinAreHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	env := closedEnv(home)
	out := run(t, cliBin, env, "", "user", "update-password", "--password", oldPassword, "--new-password", newPassword, "--dry-run")
	assertAbsent(t, "flag preview", out, oldPassword, newPassword)
	body := "{\n  \"password\": \"" + oldPassword + "\",\n  \"newPassword\": \"" + newPassword + "\"\n}\n"
	out = run(t, cliBin, env, body, "user", "update-password", "--stdin", "--dry-run")
	assertAbsent(t, "--stdin preview", out, oldPassword, newPassword)
	out = run(t, cliBin, env, "", "recovery", "export", "--storage-secret-key", storageKey, "--timeout", storageKey)
	assertAbsent(t, "credential flag echoed by a flag error", out, storageKey)
	out = run(t, cliBin, env, partnerPass+"\n", "auth", "login", "--email", "ops@example.com", "--password-stdin", "--timeout", partnerPass)
	assertAbsent(t, "--password-stdin echoed by a flag error", out, partnerPass)
	assertNoFileCarries(t, home, nil, oldPassword, newPassword, storageKey, partnerPass)
}

// TestRunProfileCredentialIsHeld: a saved profile value under a credential flag's
// name is a source, every duplicate included, and an ordinary profile (a
// one-letter name, a saved --json true) does not refuse the start.
func TestRunProfileCredentialIsHeld(t *testing.T) {
	cliBin, _ := binaries(t)
	home := t.TempDir()
	store := filepath.Join(home, "config", "profiles.json")
	writeFile(t, store, `{"profiles":{"p":{"name":"p","values":{"json":"true","acc-cred":"`+oldPassword+`","acc-cred":"`+newPassword+`","storage-secret-key":"`+storageKey+`"}}}}`)
	env := closedEnv(home)
	for _, args := range [][]string{{"profile", "show", "p"}, {"profile", "list", "--json"}, {"profile", "use", "p"}, {"--profile", "p", "version"}} {
		assertAbsent(t, strings.Join(args, " "), run(t, cliBin, env, "", args...), oldPassword, newPassword, storageKey)
	}
	assertNoFileCarries(t, home, []string{store}, oldPassword, newPassword, storageKey)
}

// TestNoCredentialLeavesOverPlainHTTP: the API client, and the login, refresh
// and partner-access-token calls outside it, all send through the outbound
// guard, which refuses every held credential over plain http.
func TestNoCredentialLeavesOverPlainHTTP(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"accessToken":"x","refreshToken":"y","expiresIn":3600}`)
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	env := closedEnv(home, "CLOUDALLY_BASE_URL="+srv.URL, "CLOUDALLY_CLIENT_ID="+clientID, "CLOUDALLY_CLIENT_SECRET="+clientSecret)
	writeFile(t, filepath.Join(home, "data", "credentials.toml"),
		"access_token = \""+accessToken+"\"\nrefresh_token = \""+refreshTok+"\"\ntoken_expiry = 2000-01-01T00:00:00Z\n")
	writeFile(t, filepath.Join(home, "state", "auth-context.json"), `{"scope":"partner","partner_access_token":"`+accessToken+`"}`)
	for _, c := range []struct {
		stdin string
		args  []string
	}{
		{"", []string{"user", "me", "--json"}},
		{partnerPass + "\n", []string{"auth", "login", "--partner", "--email", "ops@example.com", "--password-stdin"}},
		{"", []string{"auth", "use-account", "5f1c2a"}},
		{"", []string{"distributor", "accounts", "list", "--json"}},
	} {
		out := run(t, cliBin, env, c.stdin, c.args...)
		assertAbsent(t, strings.Join(c.args, " "), out, clientID, clientSecret, accessToken, refreshTok, partnerPass)
	}
	out := mcpSession(t, mcpBin, env, nil, `"method":"tools/call","params":{"name":"cloudally_execute","arguments":{"endpoint_id":"user.me"}}`)
	assertAbsent(t, "MCP execute", out, clientID, clientSecret, accessToken, refreshTok)
	if n := hits.Load(); n != 0 {
		t.Errorf("the plain-http server received %d requests; every one carried a held credential", n)
	}
	assertNoFileCarries(t, home, []string{filepath.Join(home, "data", "credentials.toml"), filepath.Join(home, "state", "auth-context.json")},
		clientID, clientSecret, partnerPass)
}

// TestMCPCredentialArgumentIsHeld: an MCP tool argument that carries a
// credential is held before the tool runs, so neither the confirm preview, nor
// the error of a confirmed send with no API credentials, repeats it.
func TestMCPCredentialArgumentIsHeld(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	env := closedEnv(home)
	out := mcpSession(t, mcpBin, env, nil,
		`"method":"tools/list"`,
		`"method":"tools/call","params":{"name":"cloudally_execute","arguments":{"endpoint_id":"user.update-password","params":{"password":"`+oldPassword+`","newPassword":"`+newPassword+`"}}}`,
		`"method":"tools/call","params":{"name":"cloudally_execute","arguments":{"endpoint_id":"user.update-password","confirm":true,"params":{"password":"`+oldPassword+`","newPassword":"`+newPassword+`"}}}`,
	)
	if !strings.Contains(out, `"tools":[`) {
		t.Errorf("tools/list was withheld or failed:\n%s", out)
	}
	assertAbsent(t, "MCP session", out, oldPassword, newPassword)
	assertNoFileCarries(t, home, nil, oldPassword, newPassword)
}
