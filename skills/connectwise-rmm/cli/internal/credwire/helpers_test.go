// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// Shared helpers of the credential leak tests on the built binaries
// (handfixes.json credkit-wiring). Set CONNECTWISE_RMM_LEAK_BIN_DIR to run
// them against binaries built elsewhere.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

// exe is the file name a built binary has on this platform.
func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// binaries builds connectwise-rmm-cli and connectwise-rmm-mcp once per test process.
func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	binOnce.Do(func() {
		if d := os.Getenv("CONNECTWISE_RMM_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "cwrmm-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"connectwise-rmm-cli", "../../cmd/connectwise-rmm-cli"}, {"connectwise-rmm-mcp", "../../cmd/connectwise-rmm-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, exe(b.out)), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("connectwise-rmm-cli")), filepath.Join(binDir, exe("connectwise-rmm-mcp"))
}

// closedEnv is a closed environment under one sandbox home: no variable of
// the operator's reaches the binary but PATH and what Windows needs to start
// one. CONNECTWISE_RMM_HOME roots the config, data, state and cache
// directories in the sandbox, so no run reads or writes the operator's real
// profile. HOME and USERPROFILE both point into the sandbox, so the legacy
// config path does too on every platform.
func closedEnv(home string, extra ...string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "TMPDIR=" + os.TempDir(), "NO_COLOR=1",
		"XDG_CONFIG_HOME=", "XDG_DATA_HOME=", "XDG_STATE_HOME=", "XDG_CACHE_HOME=",
		"CONNECTWISE_RMM_HOME=" + filepath.Join(home, "cw-home")}
	if runtime.GOOS == "windows" {
		for _, k := range []string{"SYSTEMROOT", "SYSTEMDRIVE", "TEMP", "TMP", "COMSPEC", "PATHEXT"} {
			env = append(env, k+"="+os.Getenv(k))
		}
		env = append(env, "APPDATA="+filepath.Join(home, "AppData", "Roaming"), "LOCALAPPDATA="+filepath.Join(home, "AppData", "Local"))
	}
	return append(env, extra...)
}

// runSplit runs bin with args under env and returns what it wrote to stdout
// and to stderr, and its exit code.
func runSplit(t *testing.T, bin string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runSplitStdin(t, bin, env, "", args...)
}

// runSplitStdin is runSplit with stdin as the process's standard input.
func runSplitStdin(t *testing.T, bin string, env []string, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run() // exit codes vary by case; the output is judged
	if ctx.Err() != nil {
		t.Fatalf("%s %s: timed out", filepath.Base(bin), strings.Join(args, " "))
	}
	return out.String(), errb.String(), cmd.ProcessState.ExitCode()
}

// filesUnder is every regular file under root with its content.
func filesUnder(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if data, rerr := os.ReadFile(path); rerr == nil {
			files[path] = string(data)
		}
		return nil
	})
	return files
}

// isCredentialStore reports whether path is one of the program's own 0600
// credential sources (config.toml, credentials.toml), where a credential is
// meant to be (credential leak contract: the store is not a leak).
func isCredentialStore(path string) bool {
	switch filepath.Base(path) {
	case "config.toml", "credentials.toml":
		return true
	}
	return false
}

// assertNoPiece fails when text carries cred or any 8-character piece of it.
func assertNoPiece(t *testing.T, where, text, cred string) {
	t.Helper()
	for i := 0; i+8 <= len(cred); i++ {
		if strings.Contains(text, cred[i:i+8]) {
			t.Errorf("%s carries %q, a piece of the credential:\n%.4000s", where, cred[i:i+8], text)
			return
		}
	}
}

// assertNoPieceInFiles is assertNoPiece over every file under root but the
// credential stores.
func assertNoPieceInFiles(t *testing.T, root, cred string) {
	t.Helper()
	for path, text := range filesUnder(t, root) {
		if isCredentialStore(path) {
			continue
		}
		assertNoPiece(t, path, text, cred)
	}
}

// fakeAPI is an https ConnectWise Asio fake that answers every request with
// answer(header, request), and the PEM file of its certificate for
// CONNECTWISE_RMM_CA_CERT. The kit sends a credential only over https, so the
// fake is TLS.
func fakeAPI(t *testing.T, answer func(http.Header, *http.Request) (int, string)) (baseURL, caFile string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(b)) // answer may read the body
		w.Header().Set("Content-Type", "application/json")
		status, body := answer(w.Header(), r)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, writeCA(t, srv.Certificate())
}

func writeCA(t *testing.T, cert *x509.Certificate) string {
	t.Helper()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return caFile
}

// tokenJSON is an Asio token response carrying tok.
func tokenJSON(tok string) string {
	return fmt.Sprintf(`{"access_token":%q,"expires_in":3600,"token_type":"Bearer"}`, tok)
}

// companyPage is an Asio companies page whose one record carries note.
func companyPage(note string) string {
	return fmt.Sprintf(`[{"id":"c0000000-0000-0000-0000-000000001042","name":"HQ-Record","note":%q}]`, note)
}

// isToken reports whether r is a request to the Asio token endpoint.
func isToken(r *http.Request) bool { return strings.HasSuffix(r.URL.Path, "/v1/token") }

// rpc is one JSON-RPC request of an MCP session.
type rpc struct {
	method string
	params any
}

func call(name string, args map[string]any) rpc {
	return rpc{"tools/call", map[string]any{"name": name, "arguments": args}}
}

// reply is the JSON-RPC message in out that answers request id, or "".
func reply(out string, id int) string {
	for _, l := range strings.Split(out, "\n") {
		var m struct {
			ID *int `json:"id"`
		}
		if json.Unmarshal([]byte(l), &m) == nil && m.ID != nil && *m.ID == id {
			return l
		}
	}
	return ""
}

// mcpRun runs one connectwise-rmm-mcp stdio session: initialize (id 1), then
// each request in turn (ids 2, 3, ...), each sent once the one before it
// answered, then closes stdin. It returns every JSON-RPC message the server
// wrote, its stderr and its exit code.
func mcpRun(t *testing.T, mcpBin string, env []string, reqs ...rpc) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(mcpBin)
	cmd.Env = env
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errb strings.Builder // read only after Wait, when exec's copy is done
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(outPipe)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	enc := json.NewEncoder(in)
	var got []string
	send := func(id int, method string, params any) {
		msg := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
		if id > 0 {
			msg["id"] = id
		}
		_ = enc.Encode(msg)
		if id == 0 {
			return
		}
		deadline := time.After(120 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					return
				}
				got = append(got, l)
				if strings.Contains(l, fmt.Sprintf(`"id":%d`, id)) {
					return
				}
			case <-deadline:
				return
			}
		}
	}
	send(1, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}})
	send(0, "notifications/initialized", map[string]any{})
	for i, r := range reqs {
		send(i+2, r.method, r.params)
	}
	_ = in.Close()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	for l := range lines {
		got = append(got, l)
	}
	return strings.Join(got, "\n"), errb.String(), cmd.ProcessState.ExitCode()
}

// hostAPI is an https fake that serves host (a real Asio host name) through
// an HTTP CONNECT proxy on loopback, so a run that dials host by name reaches
// the fake offline. It returns the proxy URL for HTTPS_PROXY, the CA file for
// CONNECTWISE_RMM_CA_CERT, and a function reporting the requests the fake
// served (each "METHOD path auth=<present|absent>").
func hostAPI(t *testing.T, host string, answer func(http.Header, *http.Request) (int, string)) (proxyURL, caFile string, served func() []string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var log []string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		auth := "absent"
		if r.Header.Get("Authorization") != "" {
			auth = "present"
		}
		mu.Lock()
		log = append(log, fmt.Sprintf("%s %s %s auth=%s", r.Method, r.Host, r.URL.Path, auth))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		status, body := answer(w.Header(), r)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key, Leaf: cert}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	target := srv.Listener.Addr().String()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != host+":443" {
			http.Error(w, "this proxy tunnels only to "+host, http.StatusForbidden)
			return
		}
		up, err := net.Dial("tcp", target)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			_ = up.Close()
			return
		}
		conn, rw, err := hj.Hijack()
		if err != nil {
			_ = up.Close()
			return
		}
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		go func() { _, _ = io.Copy(up, rw); _ = up.Close() }()
		_, _ = io.Copy(conn, up)
		_ = conn.Close()
	}))
	t.Cleanup(proxy.Close)
	return proxy.URL, writeCA(t, cert), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), log...)
	}
}
