// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// The credential leak tests, on the built binaries (handfixes.json
// credkit-wiring). Set ABNORMAL_LEAK_BIN_DIR to run them against binaries built
// elsewhere: that is how the pre-kit build and a build with the wiring disabled
// were seen failing.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
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

	"abnormal-pp-cli/internal/credkit"
)

var (
	binOnce sync.Once
	binDir  string
	binErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if binDir != "" && os.Getenv("ABNORMAL_LEAK_BIN_DIR") == "" {
		_ = os.RemoveAll(binDir)
	}
	os.Exit(code)
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func binaries(t *testing.T) (cliBin, mcpBin string) {
	t.Helper()
	binOnce.Do(func() {
		if d := os.Getenv("ABNORMAL_LEAK_BIN_DIR"); d != "" {
			binDir = d
			return
		}
		binDir, binErr = os.MkdirTemp("", "abnormal-leak-bin-")
		if binErr != nil {
			return
		}
		for _, b := range []struct{ out, pkg string }{{"abnormal-cli", "../../cmd/abnormal-cli"}, {"abnormal-mcp", "../../cmd/abnormal-mcp"}} {
			if out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, exe(b.out)), b.pkg).CombinedOutput(); err != nil {
				binErr = fmt.Errorf("go build %s: %v\n%s", b.pkg, err, out)
				return
			}
		}
	})
	if binErr != nil {
		t.Fatal(binErr)
	}
	return filepath.Join(binDir, exe("abnormal-cli")), filepath.Join(binDir, exe("abnormal-mcp"))
}

// vendor is a TLS stub of the Abnormal API (the kit never sends a credential
// over plain http, loopback included). url is the base URL, /v1 included.
type vendor struct {
	url, caFile string
	hits        atomic.Int64
	auth        atomic.Value
}

func startVendor(t *testing.T, h http.HandlerFunc) *vendor {
	t.Helper()
	v := &vendor{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v.hits.Add(1)
		v.auth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/v1")
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	v.url = srv.URL + "/v1"
	v.caFile = filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(v.caFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return v
}

func (v *vendor) env(key string) []string {
	return []string{"ABNORMAL_BASE_URL=" + v.url, "ABNORMAL_CA_CERT=" + v.caFile, "ABNORMAL_API_TOKEN=" + key}
}

// closedEnv is a closed environment under one sandbox home.
func closedEnv(home string, extra ...string) []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home, "ABNORMAL_HOME=" + home,
		"TMPDIR=" + os.TempDir(), "NO_COLOR=1"}
	if runtime.GOOS == "windows" {
		env = append(env, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"), "TEMP="+os.TempDir(), "TMP="+os.TempDir())
	}
	return append(env, extra...)
}

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
	return runIn(t, bin, env, "", args...)
}

func runIn(t *testing.T, bin string, env []string, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
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
			t.Errorf("%s shows %q:\n%.3000s", label, s, out)
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
				if synccpuStoreFile(p) { // synccpu: the store is not checked for this program's own credentials
					return nil
				}
				t.Errorf("%s holds %q", p, s)
			}
		}
		return nil
	})
}

// mcpCall runs one MCP stdio session of tools/call params and returns stdout
// plus stderr, with every call answered.
func mcpCall(t *testing.T, bin string, env []string, args []string, calls ...string) string {
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

// TestExportSplitAcrossRecordsNeverLandsWhole is the review's input for the class
// "credential split across units": page 1 of the export ends with the first half
// of the token, after about 12.8 KB of padding rows, and page 2 starts with the
// second half. The export file is one kit stream, so it never holds both.
func TestExportSplitAcrossRecordsNeverLandsWhole(t *testing.T) {
	cliBin, _ := binaries(t)
	const first, second = "RecordSplit", "SecretABCDEFGH"
	token := first + second
	var pages atomic.Int64
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resources" {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{}`)
			return
		}
		var rows []string
		switch r.URL.Query().Get("pageNumber") {
		case "", "1":
			pages.Add(1)
			for i := 1; i < 100; i++ {
				rows = append(rows, fmt.Sprintf(`{"resourceId":"pad-%02d","name":"%s"}`, i, strings.Repeat("x", 100)))
			}
			rows = append(rows, `{"resourceId":"r-last","name":"tail `+first+`"}`)
		case "2":
			pages.Add(1)
			rows = append(rows, `{"resourceId":"r-next","name":"`+second+` head"}`)
		}
		fmt.Fprintf(w, `{"resources":[%s]}`, strings.Join(rows, ","))
	})
	home := t.TempDir()
	outFile := filepath.Join(home, "X.jsonl")
	out := run(t, cliBin, closedEnv(home, v.env(token)...), "export", "api_resources", "--output", outFile, "--rate-limit", "1", "--no-learn")
	if pages.Load() < 2 {
		t.Fatalf("the export read %d pages, so the split was never exercised:\n%s", pages.Load(), out)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("no export file: %v\n%s", err, out)
	}
	if !bytes.Contains(data, []byte("pad-01")) {
		t.Errorf("the export wrote none of the records that carry no credential:\n%.500s", data)
	}
	if bytes.Contains(data, []byte(token)) || bytes.Contains(data, []byte(first)) && bytes.Contains(data, []byte(second)) {
		t.Errorf("the export file holds the token whole or both halves of it")
	}
	assertAbsent(t, "export output", out, token, first, second)
}

// TestShortTokenDoubleBase64NeverPrinted: a short token is held whole, and a
// vendor that reflects it base64-encoded twice is read through both layers. The
// review's input, K7, occurs as a whole token in this program's own text, so the
// kit refuses the start and says so: nothing is sent and nothing prints. q7Z
// does not, so it runs the whole path.
func TestShortTokenDoubleBase64NeverPrinted(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	for _, c := range []struct {
		short   string
		refused bool
	}{{"K7", true}, {"q7Z", false}} {
		t.Run(c.short, func(t *testing.T) {
			once := base64.StdEncoding.EncodeToString([]byte(c.short))
			twice := base64.StdEncoding.EncodeToString([]byte(once))
			v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(w, `{"threats":[{"threatId":"t-short","subject":%q}],"pageNumber":1}`, twice)
			})
			home := t.TempDir()
			env := closedEnv(home, v.env(c.short)...)
			if c.refused {
				out, code := runCode(t, cliBin, env, "threats", "retrieve", "--json", "--no-learn")
				if code != credkit.ExitRefused || !strings.Contains(out, "refusing to start") {
					t.Errorf("exit %d, want the named refusal:\n%s", code, out)
				}
				assertAbsent(t, "refused start", out, twice, once)
				if n := v.hits.Load(); n != 0 {
					t.Errorf("a refused start sent %d requests", n)
				}
				return
			}
			assertAbsent(t, "threats retrieve", run(t, cliBin, env, "threats", "retrieve", "--json", "--no-learn"), twice, once)
			assertAbsent(t, "MCP read", mcpCall(t, mcpBin, env, nil, `{"name":"abnormal_read","arguments":{"endpoint_id":"threats.retrieve"}}`), twice, once)
			if got, _ := v.auth.Load().(string); got != "Bearer "+c.short {
				t.Fatalf("the vendor received Authorization %q, so the token never went out", got)
			}
			assertNoFileCarries(t, home, nil, twice, once)
		})
	}
}

// TestMalformedConfigHeldBeforeAnythingPrints: a flag error prints before any
// command loads config, from a config TOML rejects, in both binaries.
func TestMalformedConfigHeldBeforeAnythingPrints(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	// A quoted span of an unterminated string can be ordinary text, and a file
	// that does not parse has every token held (its key names included, which
	// this program prints itself); holding them may refuse the start, which
	// prints nothing the file held.
	refusable := map[string]bool{"unterminated-multi-line": true, "inline-table-comma": true}
	cases := []struct{ name, file, secret string }{
		{"comment-glued-to-quote", "api_token = \"ab_live_glued0101\"#longcomment\nbroken = = longenoughvalue\n", "ab_live_glued0101"},
		{"inline-table-comma", "headers = { X-Api-Key = \",ab_live_comma123\" }\nbroken = = longenoughvalue\n", ",ab_live_comma123"},
		{"unterminated-array", "extra = [\"ab_live_array8910\",\napi_token = \"ab_live_after1112\"\n", "ab_live_array8910"},
		{"key-after-unterminated-array", "extra = [\"filler-value\",\napi_token = \"ab_live_after1314\"\n", "ab_live_after1314"},
		{"multi-line-string", "access_token = \"\"\"\nab_live_multi1516\n\"\"\"\nbroken = = longenoughvalue\n", "ab_live_multi1516"},
		{"unterminated-multi-line", "note = \"\"\"\napi_token = \"ab_live_inside1718\"\n", "ab_live_inside1718"},
		{"escaped-quote-decoded", "api_token = \"ab_live\\\"esc2122\"\nbroken = = longenoughvalue\n", `ab_live"esc2122`},
		{"bare-value", "client_secret = ab_live_bare2526\nbroken = = longenoughvalue\n", "ab_live_bare2526"},
		{"base-url-password", "base_url = \"https://u:ab_live_url2930@api.abnormalplatform.com/v1\"\nbroken = = longenoughvalue\n", "ab_live_url2930"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, filepath.Join(home, "config", "config.toml"), c.file)
			env := closedEnv(home)
			runner := run
			if refusable[c.name] {
				runner = func(t *testing.T, bin string, env []string, args ...string) string {
					out, _ := runCode(t, bin, env, args...)
					return out
				}
			}
			assertAbsent(t, "CLI startup flag error", runner(t, cliBin, env, "--timeout", c.secret, "version"), c.secret)
			assertAbsent(t, "MCP startup flag error", runner(t, mcpBin, env, "--transport", c.secret), c.secret)
		})
	}
}

// TestMCPHTTPWithoutTokenSaysWhy: --transport http with no ABNORMAL_MCP_HTTP_TOKEN
// refuses to serve, says why on stderr, and exits non-zero. The message used to be
// lost: os.Exit ran before the old output filter delivered it.
func TestMCPHTTPWithoutTokenSaysWhy(t *testing.T) {
	_, mcpBin := binaries(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", "127.0.0.1:0")
	cmd.Env = closedEnv(t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	code := cmd.ProcessState.ExitCode()
	if code == 0 || code == credkit.ExitRefused || !strings.Contains(stderr.String(), "ABNORMAL_MCP_HTTP_TOKEN must be set") {
		t.Errorf("exit %d, want a non-zero exit with the reason on stderr:\nstderr: %q\nstdout: %q", code, stderr.String(), stdout.String())
	}
}

// TestDiscoveryCoversEveryFileLoadReads: a credential in any file a run could read
// is held before a flag error prints.
func TestDiscoveryCoversEveryFileLoadReads(t *testing.T) {
	cliBin, _ := binaries(t)
	const secret = "ab_live_discover3132"
	toml := `api_token = "` + secret + `"` + "\n"
	files := map[string]struct {
		path func(home string) string
		body string
	}{
		"explicit-config":  {func(h string) string { return filepath.Join(h, "tenant", "config.toml") }, toml},
		"explicit-store":   {func(h string) string { return filepath.Join(h, "tenant", "data", "credentials.toml") }, toml},
		"global-store":     {func(h string) string { return filepath.Join(h, "data", "credentials.toml") }, toml},
		"global-config":    {func(h string) string { return filepath.Join(h, "config", "config.toml") }, toml},
		"legacy-config":    {func(h string) string { return filepath.Join(h, ".config", "abnormal-cli", "config.toml") }, toml},
		"home-flag-store":  {func(h string) string { return filepath.Join(h, "other", "data", "credentials.toml") }, toml},
		"home-flag-config": {func(h string) string { return filepath.Join(h, "other", "config", "config.toml") }, toml},
		"profile-store": {func(h string) string { return filepath.Join(h, "config", "profiles.json") },
			`{"profiles":{"p":{"name":"p","values":{"api_token":"` + secret + `"}}}}`},
	}
	for name, f := range files {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, f.path(home), f.body)
			args := []string{"--config", filepath.Join(home, "tenant", "config.toml"), "--home", filepath.Join(home, "other"), "--timeout", secret, "version"}
			assertAbsent(t, name, run(t, cliBin, closedEnv(home), args...), secret)
		})
	}
}

// TestOutboundCarriesNoCredentialOffItsOrigin: a token in the feedback endpoint's
// URL and in a webhook's URL never leaves for that host.
func TestOutboundCarriesNoCredentialOffItsOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "sentinel-token-123456"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"threats":[{"threatId":"t1"}],"pageNumber":1}`)
	})
	sink := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	env := append(closedEnv(home, v.env(token)...), "ABNORMAL_FEEDBACK_ENDPOINT="+sink.url+"/receipt/"+token, "SSL_CERT_FILE="+sink.caFile)
	assertAbsent(t, "feedback", run(t, cliBin, env, "feedback", "harmless", "--send"), token)
	if n := sink.hits.Load(); n != 0 {
		t.Errorf("feedback: %d requests carried the token to another origin", n)
	}
	assertAbsent(t, "webhook", run(t, cliBin, env, "threats", "retrieve", "--json", "--deliver", "webhook:"+sink.url+"/hook?k="+token), token)
	if n := sink.hits.Load(); n != 0 {
		t.Errorf("webhook: %d requests carried the token to another origin", n)
	}
	if v.hits.Load() == 0 {
		t.Errorf("the API was never called, so the webhook had nothing to deliver")
	}
}

// TestTLSKeyServesAndItsTextIsHeld: a real private key lets the HTTPS server
// start, and the key's text is held from startup.
func TestTLSKeyServesAndItsTextIsHeld(t *testing.T) {
	_, mcpBin := binaries(t)
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	key := filepath.Join(home, "key.pem")
	writeFile(t, key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	start := time.Now()
	out, code := runCode(t, mcpBin, closedEnv(home), "--tls-key", key, "--help")
	if code == credkit.ExitRefused || time.Since(start) > 30*time.Second {
		t.Fatalf("a real key refused the start: exit %d after %s:\n%s", code, time.Since(start), out)
	}
	for _, form := range []string{base64.StdEncoding.EncodeToString(der), hex.EncodeToString(der), strings.ToUpper(hex.EncodeToString(der))} {
		assertAbsent(t, "the key's text", run(t, mcpBin, closedEnv(home), "--tls-key", key, "--transport", form), form)
	}
}

// TestStdinBodyIsHeldBeforeAPreview: a --stdin body is held whole and line by line
// before a dry run prints it, however it is laid out.
func TestStdinBodyIsHeldBeforeAPreview(t *testing.T) {
	cliBin, _ := binaries(t)
	const value = "stdin-only-value-4567"
	for name, body := range map[string]string{
		"compact":   `{"name":"Imported","description":"` + value + `"}`,
		"pretty":    "{\n  \"name\": \"Imported\",\n  \"description\": \"" + value + "\"\n}\n",
		"nested":    `{"name":"Imported","meta":{"value":"` + value + `"}}`,
		"malformed": `{"name":"Imported","description":"` + value + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			out, code := runIn(t, cliBin, closedEnv(home, "ABNORMAL_API_TOKEN=ab_live_token_33344455"), body,
				"api-resources", "resources-create-create", "--stdin", "--dry-run", "--no-learn")
			if code == credkit.ExitRefused {
				t.Errorf("a --stdin body refused the start:\n%s", out)
			}
			assertAbsent(t, "--stdin --dry-run", out, value)
			assertNoFileCarries(t, home, nil, value)
		})
	}
}

// TestStdinBodyPrettyAndEchoed: a body from jq (one member per line) starts and is
// sent whole. An answer that shares no part of the body reaches the operator; one
// that repeats a value of the body is withheld, because a declared stdin is held
// whole, and the write still happened (exit 0).
func TestStdinBodyPrettyAndEchoed(t *testing.T) {
	cliBin, _ := binaries(t)
	const value = "stdin-only-value-4567"
	var got, answer atomic.Value
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Store(string(b))
		io.WriteString(w, answer.Load().(string))
	})
	for name, body := range map[string]string{
		"pretty":  "{\n  \"name\": \"Acme Dental Group\",\n  \"description\": \"" + value + "\"\n}\n",
		"compact": `{"name":"Acme Dental Group","description":"` + value + `"}`,
	} {
		for _, c := range []struct {
			name, answer string
			shown        bool
		}{
			{"plain-answer", `{"resourceId":"r-g1","status":"created"}`, true},
			{"echoed-answer", `{"resourceId":"r-g1","status":"created","name":"Acme Dental Group"}`, false},
		} {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				answer.Store(c.answer)
				home := t.TempDir()
				out, code := runIn(t, cliBin, closedEnv(home, v.env("ab_live_token_33344455")...), body,
					"api-resources", "resources-create-create", "--stdin", "--json", "--no-learn", "--yes")
				assertAbsent(t, "answer", out, value)
				if code != 0 {
					t.Errorf("exit %d:\n%s", code, out)
				}
				if shown := strings.Contains(out, "r-g1"); shown != c.shown {
					t.Errorf("answer shown = %v, want %v:\n%s", shown, c.shown, out)
				}
				var sent map[string]any
				raw, _ := got.Load().(string)
				if json.Unmarshal([]byte(raw), &sent) != nil || sent["description"] != value {
					t.Errorf("the vendor received %q", raw)
				}
				assertNoFileCarries(t, home, nil, value)
			})
		}
	}
}

// TestUsedConfigDecidesTheAllowedOrigin: the config the program really uses names
// the origin a credential may go to. A discovered default's base_url is only held,
// so a credential it holds never goes there.
func TestUsedConfigDecidesTheAllowedOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "ab_live_origin_44455566"
	used := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"threats":[],"pageNumber":1}`)
	})
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \""+other.url+"\"\n")
	explicit := filepath.Join(home, "tenant.toml")
	writeFile(t, explicit, "base_url = \""+used.url+"\"\napi_token = \""+token+"\"\n")
	env := closedEnv(home, "ABNORMAL_CA_CERT="+used.caFile)
	out := run(t, cliBin, env, "--config", explicit, "threats", "retrieve", "--json", "--no-learn", "--no-cache")
	assertAbsent(t, "threats retrieve", out, token)
	if used.hits.Load() == 0 {
		t.Errorf("the explicit config's base URL was refused or unused:\n%s", out)
	}
	if n := other.hits.Load(); n != 0 {
		t.Errorf("%d requests went to the discovered default's base URL", n)
	}
}

// TestCredentialStoreNamesNoOrigin: config.Load takes no base URL from the store,
// so a base_url there must not become the host a credential may go to.
func TestCredentialStoreNamesNoOrigin(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "store-origin-credential-5512"
	other := startVendor(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{}`) })
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "config", "config.toml"), "# nothing here\n")
	writeFile(t, filepath.Join(home, "data", "credentials.toml"), "base_url = \""+other.url+"\"\n")
	env := closedEnv(home, "ABNORMAL_API_TOKEN="+token, "ABNORMAL_CA_CERT="+other.caFile,
		"ABNORMAL_FEEDBACK_ENDPOINT="+other.url+"/receipt/"+token)
	assertAbsent(t, "feedback", run(t, cliBin, env, "feedback", "harmless", "--send"), token)
	if n := other.hits.Load(); n != 0 {
		t.Errorf("%d requests carried the token to the store's base_url", n)
	}
	// The program calls the built-in API, so that is the origin the token may go to.
	// A dead proxy ends the request before it leaves this machine.
	env = append(env, "HTTPS_PROXY=http://127.0.0.1:1")
	out := run(t, cliBin, env, "threats", "retrieve", "--json", "--no-learn", "--no-cache", "--timeout", "2s")
	if !strings.Contains(out, "proxyconnect") {
		t.Errorf("the request never left the kit: the store's base_url replaced the origin the program calls:\n%s", out)
	}
}

// TestChildGetsTheToken: an MCP mirror tool runs the CLI, and the kit passes the
// child its credentials outside the environment. The token reaches the vendor.
func TestChildGetsTheToken(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	const token = "ab-child-4142434-x"
	v := startVendor(t, func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"threats":[],"pageNumber":1}`)
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env(token), "ABNORMAL_CLI_PATH="+cliBin)...)
	out := mcpCall(t, mcpBin, env, nil, `{"name":"sync","arguments":{"resources":["threats"]}}`)
	assertAbsent(t, "sync", out, token, "not started", "withheld from child")
	if v.hits.Load() == 0 {
		t.Fatalf("the child reached no vendor endpoint:\n%s", out)
	}
	if got, _ := v.auth.Load().(string); !strings.Contains(got, token) {
		t.Errorf("the child sent Authorization %q, want the handed token", got)
	}
}

// TestWholeRunNeverEmitsAHeldCredential drives every source and every sink: the
// fake API repeats every credential in every encoding, and every stdout, stderr
// and file outside the credential files is scanned in every form.
func TestWholeRunNeverEmitsAHeldCredential(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	secrets := []string{
		"envTok3n+A1b2C3d4/E5f6G7h8",
		"mcpHttpT0ken+0123456789/abcdefghijKLMNOP",
		"cfgAuthH3ader-Z1x2C3v4B5n6M7",
	}
	stored := "storedT0ken_Q9w8E7r6T5y4U3i2"
	all := append(slices.Clone(secrets), stored)
	var echo atomic.Bool
	item := `{"threatId":"t1","subject":"Acme Dental","caseId":"c1","vendorDomain":"acme.example"`
	v := startVendor(t, func(w http.ResponseWriter, r *http.Request) {
		if !echo.Load() {
			fmt.Fprintf(w, `{"threats":[%s}],"cases":[%s}],"vendors":[%s}],"pageNumber":1}`, item, item, item)
			return
		}
		fields := echoFields(all)
		switch p := r.URL.Path; {
		case p == "/cases":
			w.Header().Set("X-Request-ID", base32.StdEncoding.EncodeToString([]byte(secrets[0])))
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"message":"rate limited"}`)
		case p == "/vendors":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"message":"bad request","detail":{%s}}`, fields)
		case p == "/auditlogs":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<html><head><title>%s</title></head></html>", base32.StdEncoding.EncodeToString([]byte(secrets[0])))
		default:
			fmt.Fprintf(w, `{"threats":[%s,%s}],"pageNumber":1}`, item, fields)
		}
	})
	home := t.TempDir()
	env := closedEnv(home, append(v.env(secrets[0]), "ABNORMAL_MCP_HTTP_TOKEN="+secrets[1])...)
	var hits []string
	scan := func(where, text string) {
		for _, s := range all {
			for form, needle := range credentialForms(s) {
				if strings.Contains(text, needle) {
					hits = append(hits, where+": "+form)
				}
			}
		}
	}
	out, code := runIn(t, cliBin, closedEnv(home, "ABNORMAL_BASE_URL="+v.url, "ABNORMAL_CA_CERT="+v.caFile), stored, "auth", "set-token")
	if code != 0 {
		t.Fatalf("auth set-token: exit %d\n%s", code, out)
	}
	scan("auth set-token", out)
	cfg := filepath.Join(home, "config", "config.toml")
	existing, _ := os.ReadFile(cfg)
	writeFile(t, cfg, string(existing)+fmt.Sprintf("\nauth_header = %q\n", "Bearer "+secrets[2]))

	if o := run(t, cliBin, env, "threats", "retrieve", "--json"); !strings.Contains(o, "Acme Dental") {
		t.Fatalf("control run did not reach the API:\n%s", o)
	}
	run(t, cliBin, env, "sync")
	echo.Store(true)
	for _, args := range [][]string{
		{"doctor", "--json"},
		{"auth", "status", "--json"},
		{"threats", "retrieve", "--json"},
		{"threats", "retrieve", "--agent"},
		{"threats", "retrieve", "--dry-run"},
		{"threats", "retrieve", "--json", "--deliver", "file:" + filepath.Join(home, "delivered.json")},
		{"export", "threats", "--output", filepath.Join(home, "export.jsonl")},
		{"sync"},
		{"cases", "retrieve", "--json"},
		{"vendors", "--json"},
		{"auditlogs", "--json"},
		{"search", "Acme", "--json"},
		{"search", secrets[0], "--json"},
		{"recall", secrets[0], "--json"},
		{"recall", secrets[2], "--json"},
		{"feedback", "noted " + secrets[0]},
	} {
		scan(strings.Join(args[:min(2, len(args))], " "), run(t, cliBin, env, args...))
	}
	scan("MCP session", mcpCall(t, mcpBin, env, nil,
		`{"name":"abnormal_read","arguments":{"endpoint_id":"threats.retrieve"}}`,
		`{"name":"abnormal_search","arguments":{"query":"threats"}}`,
		`{"name":"sql","arguments":{"query":"select * from resources limit 5"}}`,
	))
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(p) == "credentials.toml" || p == cfg {
			return nil
		}
		if synccpuStoreFile(p) { // synccpu: the store is not checked for this program's own credentials
			return nil
		}
		if data, rerr := os.ReadFile(p); rerr == nil {
			rel, _ := filepath.Rel(home, p)
			scan("file "+rel, string(data))
		}
		return nil
	})
	if len(hits) > 0 {
		t.Fatalf("%d held-credential hits:\n%s", len(hits), strings.Join(hits[:min(40, len(hits))], "\n"))
	}
}

func echoFields(secrets []string) string {
	var parts []string
	for i, s := range secrets {
		b := []byte(s)
		for j, enc := range []string{
			s, "Bearer " + s, base64.StdEncoding.EncodeToString(b), base64.RawURLEncoding.EncodeToString(b),
			base32.StdEncoding.EncodeToString(b), base32.HexEncoding.EncodeToString(b), hex.EncodeToString(b),
			url.QueryEscape(s),
		} {
			v, _ := json.Marshal(enc)
			parts = append(parts, fmt.Sprintf(`"echo_%d_%d":%s`, i, j, v))
		}
	}
	return strings.Join(parts, ",")
}

func credentialForms(v string) map[string]string {
	b := []byte(v)
	base := map[string]string{
		"raw":           v,
		"lowercase":     strings.ToLower(v),
		"uppercase":     strings.ToUpper(v),
		"base64":        base64.RawStdEncoding.EncodeToString(b),
		"base64url":     base64.RawURLEncoding.EncodeToString(b),
		"base32":        base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b),
		"base32 lower":  strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)),
		"base32hex":     base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(b),
		"hex":           hex.EncodeToString(b),
		"hex upper":     strings.ToUpper(hex.EncodeToString(b)),
		"query-escaped": url.QueryEscape(v),
		"path-escaped":  url.PathEscape(v),
	}
	out := map[string]string{}
	for form, n := range base {
		out[form] = n
		if jb, err := json.Marshal(n); err == nil {
			if j := string(jb[1 : len(jb)-1]); j != n {
				out[form+" (json-escaped)"] = j
			}
		}
	}
	return out
}

// TestMCPHTTPRefusalsPassTheKit: the HTTP server's refusals and results leave
// through credkit.MCPHTTP, so a refused request echoes nothing held.
func TestMCPHTTPRefusalsPassTheKit(t *testing.T) {
	_, mcpBin := binaries(t)
	const token = "mcp-bearer-4344-" + "0123456789abcdef-long"
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", addr)
	cmd.Env = closedEnv(t.TempDir(), "ABNORMAL_MCP_HTTP_TOKEN="+token)
	var serverOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &serverOut, &serverOut
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	post := func(auth string, origin string) (int, string) {
		var last error
		for i := 0; i < 50; i++ {
			req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if auth != "" {
				req.Header.Set("Authorization", "Bearer "+auth)
			}
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				last = err
				time.Sleep(100 * time.Millisecond)
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return resp.StatusCode, string(b)
		}
		t.Fatalf("the server never answered: %v\n%s", last, serverOut.String())
		return 0, ""
	}
	if code, body := post("", ""); code != http.StatusUnauthorized || strings.Contains(body, token) {
		t.Errorf("no token: %d %s", code, body)
	}
	if code, body := post("wrong-"+token, ""); code != http.StatusUnauthorized || strings.Contains(body, token) {
		t.Errorf("wrong token echoed: %d %s", code, body)
	}
	if code, body := post(token, "https://evil.example"); code != http.StatusForbidden || strings.Contains(body, token) {
		t.Errorf("foreign origin: %d %s", code, body)
	}
	if code, body := post(token, ""); code != http.StatusOK || !strings.Contains(body, "serverInfo") || strings.Contains(body, token) {
		t.Errorf("good token: %d %s", code, body)
	}
}

// TestStartupFlagTheParserDidNotUseIsRefused: startup reads --config and --home by
// every occurrence, and the kit takes the allowed origin from what it reads. A flag
// that takes a value can swallow one (--select --config x), so a file the run never
// uses could name the host a credential may go to. Such a run stops before it acts.
func TestStartupFlagTheParserDidNotUseIsRefused(t *testing.T) {
	cliBin, _ := binaries(t)
	const token = "swallow-fixture-value"
	// Any connection to the decoy means the kit let a request carrying the token go
	// to a host only an unused flag named.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var dials atomic.Int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			dials.Add(1)
			c.Close()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	decoyURL := "https://" + ln.Addr().String()
	home := t.TempDir()
	usedFile := filepath.Join(home, "used.toml")
	decoyFile := filepath.Join(home, "decoy.toml")
	writeFile(t, usedFile, "base_url = \"https://api.abnormalplatform.com/v1\"\n")
	writeFile(t, decoyFile, "base_url = \""+decoyURL+"\"\n")
	deliver := "webhook:" + decoyURL + "/hook?k=" + token
	env := closedEnv(home, "ABNORMAL_API_TOKEN="+token, "HTTPS_PROXY=", "NO_PROXY=127.0.0.1")
	for name, args := range map[string][]string{
		"control-used-only": {"version", "--config", usedFile, "--deliver", deliver, "--no-learn"},
		"swallowed-last":    {"version", "--config", usedFile, "--select", "--config", decoyFile, "--deliver", deliver, "--no-learn"},
		"swallowed-only":    {"version", "--select", "--config", decoyFile, "--deliver", deliver, "--no-learn"},
		"swallowed-home":    {"version", "--select", "--home", filepath.Join(home, "other"), "--deliver", deliver, "--no-learn"},
	} {
		t.Run(name, func(t *testing.T) {
			before := dials.Load()
			out, _ := runCode(t, cliBin, env, args...)
			if n := dials.Load() - before; n != 0 {
				t.Errorf("%d connections carried the token to a host only an unused flag named:\n%s", n, out)
			}
			assertAbsent(t, "output", out, token)
		})
	}
	// A --config the parser uses, written once, is not refused.
	if out, code := runCode(t, cliBin, env, "--config", usedFile, "version", "--no-learn"); code != 0 {
		t.Errorf("an ordinary --config: exit %d\n%s", code, out)
	}
}

// TestMCPRefusesAConfigFlagItDoesNotHave: abnormal-mcp has no --config or --home,
// but startup reads every occurrence and the kit takes the allowed origin from it. A
// flag that takes a value (--addr --config x) or a stray positional hides one from
// the parser, so the server refuses to start rather than run on a file it never uses.
func TestMCPRefusesAConfigFlagItDoesNotHave(t *testing.T) {
	_, mcpBin := binaries(t)
	home := t.TempDir()
	used := filepath.Join(home, "used.toml")
	decoy := filepath.Join(home, "decoy.toml")
	writeFile(t, used, "base_url = \"https://api.abnormalplatform.com/v1\"\n")
	writeFile(t, decoy, "base_url = \"https://collector.example\"\n")
	env := closedEnv(home, "ABNORMAL_CONFIG="+used, "ABNORMAL_API_TOKEN=mcp-fixture-value")
	for name, args := range map[string][]string{
		"swallowed-config": {"--addr", "--config", decoy},
		"after-positional": {"x", "--config", decoy},
		"swallowed-home":   {"--addr", "--home", filepath.Join(home, "other")},
	} {
		t.Run(name, func(t *testing.T) {
			out, code := runCode(t, mcpBin, env, args...)
			if code != 2 || !strings.Contains(out, "takes no --config or --home") {
				t.Errorf("exit %d, want a refusal (2):\n%s", code, out)
			}
		})
	}
	if out, code := runCode(t, mcpBin, env, "--transport", "stdio"); code != 0 {
		t.Errorf("an ordinary start: exit %d\n%s", code, out)
	}
}

// synccpuStoreFile reports whether path is the local SQLite store or one of its
// side files. synccpu (owner GO 2026-11-09) dropped the store's credential check
// except for held vendor response values, so a credential this program holds from
// its own configuration may sit in the store; every other file is still checked.
func synccpuStoreFile(path string) bool {
	for _, s := range []string{".db", ".db-wal", ".db-shm", ".db-journal"} {
		if strings.HasSuffix(path, s) {
			return true
		}
	}
	return false
}
