// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring).

package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
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

	"github.com/mark3labs/mcp-go/server"

	"google-workspace-pp-cli/internal/cli"
	"google-workspace-pp-cli/internal/cliutil"
	"google-workspace-pp-cli/internal/credkit"
	"google-workspace-pp-cli/internal/credwire"
)

// The service-account grant posts a signed JWT (the assertion, a credential)
// to Google's pinned token endpoint. An endpoint that answers 400 with the
// assertion in its error body must not put it on stderr or in an MCP tool
// error. The endpoint is pinned (no override), so this test runs the program
// in a child of the test binary, with credkit.Start as the mains call it and
// a TLS server standing in for oauth2.googleapis.com, dialed through
// http.DefaultTransport, which the token client sends with inside the kit's
// outbound guard.
//
// GW_TOKEN_LEAK_KIT=off runs the same child without the kit (how the class was
// seen failing first).
const tokenLeakHelperEnv = "GW_TOKEN_LEAK_HELPER"

func TestTokenEndpointEchoOfAssertionIsWithheld(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives Unix config-file modes; the Windows build is vetted and compiled by the gates")
	}
	home := t.TempDir()
	cfg := filepath.Join(home, "config")
	key := filepath.Join(cfg, "keys", "acme.json")
	writeTokenLeakFile(t, key, serviceAccountKeyJSON(t))
	writeTokenLeakFile(t, filepath.Join(cfg, "tenants.toml"), fmt.Sprintf(
		"[[tenants]]\nname = \"acme\"\ndomain = \"acme.example\"\nadmin = \"admin@acme.example\"\nkey_file = %q\nscopes = [\"https://www.googleapis.com/auth/admin.directory.user.readonly\"]\n", key))

	t.Run("cli stderr", func(t *testing.T) {
		assertions := filepath.Join(t.TempDir(), "assertions")
		out := runTokenLeakHelper(t, home, assertions, "cli", "", "users", "list")
		assertAssertionAbsent(t, "CLI", out, assertions)
	})
	t.Run("mcp tool error", func(t *testing.T) {
		assertions := filepath.Join(t.TempDir(), "assertions")
		in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"google-workspace_read","arguments":{"endpoint_id":"users.list"}}}` + "\n"
		out := runTokenLeakHelper(t, home, assertions, "mcp", in)
		if !strings.Contains(out, `"id":2`) {
			t.Fatalf("the MCP call was not answered:\n%s", out)
		}
		assertAssertionAbsent(t, "MCP", out, assertions)
	})
}

// TestSpacefastAnonymousUpdatePassesTheKit: an anonymous Spacefast page is
// updated with the key it was created with, sent as the bearer. The kit binds
// that key to Spacefast's API (Origins minted:spacefast-page-key), so the
// update is sent, not refused as a credential bound to the Google API hosts;
// and the record that keeps the key is written outside the kit, so the second
// run finds it.
func TestSpacefastAnonymousUpdatePassesTheKit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("drives Unix config-file modes; the Windows build is vetted and compiled by the gates")
	}
	home := t.TempDir()
	auths := filepath.Join(t.TempDir(), "spacefast-auth")
	assertions := filepath.Join(t.TempDir(), "assertions")
	out, code := runChild(t, home, assertions, auths, "cli", "", "version", "--deliver", "spacefast:gw-report")
	if code != 0 || !strings.Contains(out, "Published on Spacefast") {
		t.Fatalf("create: exit %d:\n%s", code, out)
	}
	out, code = runChild(t, home, assertions, auths, "cli", "", "version", "--deliver", "spacefast:gw-report")
	if code != 0 {
		t.Fatalf("update: exit %d:\n%s", code, out)
	}
	if strings.Contains(out, spacefastPageKey) {
		t.Errorf("update printed the page key it holds:\n%s", out)
	}
	data, _ := os.ReadFile(auths)
	if lines := strings.Split(strings.TrimSpace(string(data)), "\n"); len(lines) != 2 || lines[0] != "auth=" || lines[1] != "auth=Bearer "+spacefastPageKey {
		t.Errorf("Spacefast got %q, want an anonymous create and then an update with the page key", lines)
	}
}

// TestTokenLeakHelper is the child process; it does nothing in a normal run.
func TestTokenLeakHelper(t *testing.T) {
	mode := os.Getenv(tokenLeakHelperEnv)
	if mode == "" {
		t.Skip("child process of TestTokenEndpointEchoOfAssertionIsWithheld")
	}
	var args []string
	_ = json.Unmarshal([]byte(os.Getenv("GW_TOKEN_LEAK_ARGS")), &args)
	kit := os.Getenv("GW_TOKEN_LEAK_KIT") != "off"
	if kit {
		// This package's tests replace the seams (credkit_seam_test.go); the
		// child runs the real ones, as the mains do.
		cliutil.OutboundGuard = credkit.Transport
		cliutil.Hold = credkit.Hold
		cliutil.HoldBody = credkit.HoldBody
	}
	os.Args = append([]string{"google-workspace-" + mode}, args...)
	if kit {
		credkit.Start(credwire.Declarations)
	}
	standInForGoogleAndSpacefast(t, os.Getenv("GW_TOKEN_LEAK_ASSERTIONS"), os.Getenv("GW_TOKEN_LEAK_SPACEFAST"))
	code := 0
	switch mode {
	case "cli":
		if err := cli.Execute(); err != nil {
			code = cli.ExitCode(err)
		}
	case "mcp":
		opts := []server.ServerOption{server.WithToolCapabilities(false)}
		var in io.Reader = os.Stdin
		var out io.Writer = os.Stdout
		if kit {
			opts = append([]server.ServerOption{server.WithToolHandlerMiddleware(credkit.MCPTools)}, opts...)
			in, out = credkit.MCPIn(os.Stdin), credkit.MCPOut()
		}
		s := server.NewMCPServer("Google Workspace", "test", opts...)
		RegisterTools(s)
		if err := server.NewStdioServer(s).Listen(context.Background(), in, out); err != nil {
			fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		}
	}
	if kit {
		credkit.Exit(code)
	}
	os.Exit(code)
}

// standInForGoogleAndSpacefast serves Google's token endpoint and Spacefast's
// API from this process: one TLS server with a certificate for both hosts.
// The token endpoint answers 400 with the assertion it was sent in its error
// body and records the assertion so the parent can look for it. Spacefast
// answers a publish: an anonymous one (no Authorization) gets a page key, and
// every Authorization it is sent is recorded. http.DefaultTransport (an
// *http.Transport, as the API client requires) dials it for those hosts and
// trusts its certificate; the kit clones that transport for every request.
func standInForGoogleAndSpacefast(t *testing.T, assertions, spacefast string) {
	var mu sync.Mutex
	record := func(path, line string) {
		mu.Lock()
		defer mu.Unlock()
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			fmt.Fprintln(f, line)
			_ = f.Close()
		}
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Host == "api.spacefast.com" {
			auth := r.Header.Get("Authorization")
			record(spacefast, "auth="+auth)
			rec := map[string]any{"space": map[string]any{"id": "spc_1", "liveUrl": "https://demo.space.fast/"}, "next": map[string]any{"action": "none"}}
			if auth == "" {
				rec["claim"] = map[string]any{"key": spacefastPageKey, "url": "https://demo.space.fast/__/" + spacefastPageKey, "claimUrl": "https://spacefast.com/claim#" + spacefastPageKey}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": rec})
			return
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		assertion := form.Get("assertion")
		record(assertions, assertion)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "Invalid JWT: " + assertion})
	}))
	cert, pool := standInCert(t)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	addr := srv.Listener.Addr().String()
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dt := http.DefaultTransport.(*http.Transport)
	dt.DialContext = func(ctx context.Context, network, a string) (net.Conn, error) {
		if a == "oauth2.googleapis.com:443" || a == "api.spacefast.com:443" {
			a = addr
		}
		return dialer.DialContext(ctx, network, a)
	}
	dt.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

// spacefastPageKey is the anonymous page key the Spacefast stand-in issues.
const spacefastPageKey = "sfc_pagekey7Hq2Wm9Xr4"

func standInCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "oauth2.googleapis.com"},
		DNSNames:              []string{"oauth2.googleapis.com", "api.spacefast.com"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func runTokenLeakHelper(t *testing.T, home, assertions, mode, stdin string, args ...string) string {
	t.Helper()
	out, _ := runChild(t, home, assertions, "", mode, stdin, args...)
	return out
}

// runChild runs TestTokenLeakHelper as a child process and returns its output
// and exit code.
func runChild(t *testing.T, home, assertions, spacefast, mode, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	a, _ := json.Marshal(args)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTokenLeakHelper$", "-test.count=1")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "GOOGLE_WORKSPACE_HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg-config"), "XDG_DATA_HOME=" + filepath.Join(home, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(home, "xdg-state"), "XDG_CACHE_HOME=" + filepath.Join(home, "xdg-cache"),
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1",
		tokenLeakHelperEnv + "=" + mode, "GW_TOKEN_LEAK_ARGS=" + string(a), "GW_TOKEN_LEAK_ASSERTIONS=" + assertions,
		"GW_TOKEN_LEAK_SPACEFAST=" + spacefast, "GW_TOKEN_LEAK_KIT=" + os.Getenv("GW_TOKEN_LEAK_KIT")}
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	_ = cmd.Run()
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == credkit.ExitRefused {
		t.Fatalf("the child's start was refused, so this run proves nothing:\n%s", out.String())
	}
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return out.String(), code
}

func assertAssertionAbsent(t *testing.T, label, out, assertions string) {
	t.Helper()
	f, err := os.Open(assertions)
	if err != nil {
		t.Fatalf("%s: no assertion reached the token endpoint, so nothing was echoed:\n%s", label, out)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	seen := 0
	for sc.Scan() {
		jwt := strings.TrimSpace(sc.Text())
		if jwt == "" {
			continue
		}
		seen++
		parts := strings.Split(jwt, ".")
		if len(parts) != 3 {
			t.Fatalf("%s: the token endpoint got %q, not a JWT", label, jwt)
		}
		// The signature is the part only this key could make; the whole
		// assertion and its claims are checked too.
		for _, p := range []string{jwt, parts[1], parts[2]} {
			if strings.Contains(out, p) {
				t.Errorf("%s output carries the assertion (%d chars of it):\n%s", label, len(p), out)
			}
		}
	}
	if seen == 0 {
		t.Fatalf("%s: no assertion reached the token endpoint, so nothing was echoed:\n%s", label, out)
	}
}

func serviceAccountKeyJSON(t *testing.T) string {
	t.Helper()
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(pk)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]string{
		"type":           "service_account",
		"client_email":   "sa@proj.iam.gserviceaccount.com",
		"client_id":      "112233445566778899",
		"private_key_id": "kid" + "7e6d5c4b3a2910f8e7d6c5b4a3928170",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	return string(b)
}

func writeTokenLeakFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
