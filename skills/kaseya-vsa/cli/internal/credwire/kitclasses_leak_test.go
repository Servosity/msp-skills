// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-wiring): one test per
// class the kit adoption's second review named, from that review's inputs.

package credwire

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"kaseya-vsa-pp-cli/internal/store"
)

// scriptedProxy answers every CONNECT with reply and records what it was sent.
// With tunnelTo set it then relays the connection there, as a real proxy does.
func scriptedProxy(t *testing.T, reply, tunnelTo string) (addr string, heads func() string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	var seen strings.Builder
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					mu.Lock()
					seen.WriteString(line)
					mu.Unlock()
					if err != nil {
						return
					}
					if line == "\r\n" {
						break
					}
				}
				if _, err := io.WriteString(c, reply); err != nil || tunnelTo == "" {
					return
				}
				up, err := net.Dial("tcp", tunnelTo)
				if err != nil {
					return
				}
				defer up.Close()
				go func() { _, _ = io.Copy(up, br) }()
				_, _ = io.Copy(c, up)
			}()
		}
	}()
	return ln.Addr().String(), func() string {
		mu.Lock()
		defer mu.Unlock()
		return seen.String()
	}
}

var proxiedVendor = []string{"KASEYA_VSA_BASE_URL=https://h.vsax.net/api/v3", "KASEYA_VSA_TOKEN_ID=abcd1234",
	"KASEYA_VSA_TOKEN_SECRET=" + fixtureSecret, "NO_PROXY=", "no_proxy="}

// The CONNECT is judged as it is written: a held token carrying the text Go
// adds to it (its User-Agent) never reaches the proxy.
func TestKitProxyConnectJudgesWhatIsWritten(t *testing.T) {
	cli, _ := binaries(t)
	addr, heads := scriptedProxy(t, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n", "")
	s := newSandbox(t)
	run(t, cli, s.env(append(proxiedVendor, "HTTPS_PROXY=http://"+addr,
		"KASEYA_VSA_MCP_HTTP_TOKEN=McpOpaqueGo-http-clientToken593682471095")...), "organizations", "list", "--json")
	if got := heads(); strings.Contains(got, "Go-http-client") {
		t.Errorf("the proxy received a piece of a held token:\n%s", got)
	}
}

// The proxy's real answer reaches the kit: a cookie it sets is held before
// the error that carries its status line can print.
func TestKitProxyConnectResponseIsHeld(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "newSecretQ4821"
	addr, heads := scriptedProxy(t, "HTTP/1.1 502 "+secret+"\r\nSet-Cookie: session="+secret+"\r\nContent-Length: 0\r\n\r\n", "")
	s := newSandbox(t)
	res := run(t, cli, s.env(append(proxiedVendor, "HTTPS_PROXY=http://"+addr)...), "organizations", "list", "--json")
	if !strings.HasPrefix(heads(), "CONNECT h.vsax.net:443 ") {
		t.Fatalf("the proxy got no CONNECT: %q", heads())
	}
	requireAbsent(t, "stdout", res.stdout, secret)
	requireAbsent(t, "stderr", res.stderr, secret)
}

// A proxy answer Go cannot parse has no response for the kit to hold, so
// nothing the proxy wrote may reach the error.
func TestKitProxyConnectParseErrorCarriesNothing(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "FreshCookieQ4821"
	for _, reply := range []string{
		"HTTP/1.1 502 Bad Gateway\r\nSet-Cookie: session=" + secret + "\r\nContent-Length: " + secret + "\r\n\r\n",
		secret + "\r\n\r\n",
		"HTTP/1.1 " + secret + "\r\n\r\n",
		"HTTP/1.1 502 Bad\r\n" + secret + "\r\n\r\n",
	} {
		addr, heads := scriptedProxy(t, reply, "")
		s := newSandbox(t)
		res := run(t, cli, s.env(append(proxiedVendor, "HTTPS_PROXY=http://"+addr)...), "organizations", "list", "--json")
		if heads() == "" {
			t.Fatalf("the proxy got no CONNECT for %q", reply)
		}
		requireAbsent(t, "stdout", res.stdout, secret)
		requireAbsent(t, "stderr", res.stderr, secret)
	}
}

// A request still reaches the vendor through a real tunnel: a 200 to CONNECT
// has no body, so the exchange must not wait for one.
func TestKitProxyTunnelCarriesTheRequest(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	srv := newRDAPServer(t, s, 200, "[]")
	addr, heads := scriptedProxy(t, "HTTP/1.1 200 Connection Established\r\n\r\n", srv.Listener.Addr().String())
	res := run(t, cli, s.env(append(proxiedVendor, "HTTPS_PROXY=http://"+addr, "KASEYA_VSA_CA_CERT="+srv.caFile,
		"KASEYA_VSA_TLS_SERVER_NAME=example.com")...), "organizations", "list", "--json", "--timeout", "20s")
	if srv.requests() == 0 {
		t.Errorf("no request came through the tunnel: heads=%q stdout=%q stderr=%q", heads(), res.stdout, res.stderr)
	}
}

// Every value of a config file is held at the per-call reload, whatever it is
// made of. The review's input: a short punctuation value under [headers], which
// the missing-CA error then prints.
func TestKitMCPReloadHoldsPunctuationValues(t *testing.T) {
	_, mcp := binaries(t)
	for _, secret := range []string{"!!!!!!!", "-------", "%%%%"} {
		s := newSandbox(t)
		cfg := s.writePrivate(t, "reload.toml", "")
		rewrite := func() {
			body := "ca_cert = \"/definitely-missing/" + secret + ".pem\"\n[headers]\nX-Api-Key = \"" + secret + "\"\n"
			if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		frames := mcpCallAfter(t, mcp, s.env("KASEYA_VSA_CONFIG="+cfg), rewrite, "kaseya-vsa_read",
			map[string]any{"endpoint_id": "organizations.list"})
		requireAbsent(t, "frames", frames, secret)
	}
}

// mcpCallAfter is mcpCall with before run once the server has started.
func mcpCallAfter(t *testing.T, bin string, env []string, before func(), tool string, args map[string]any) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_, _ = io.Copy(io.Discard, stdout)
		_ = cmd.Wait()
	}()
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`+"\n")
	if !sc.Scan() {
		t.Fatal("no initialize answer")
	}
	before()
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	_, _ = io.WriteString(stdin, string(call)+"\n")
	var all strings.Builder
	for sc.Scan() {
		all.WriteString(sc.Text() + "\n")
		if strings.Contains(sc.Text(), `"id":2`) {
			break
		}
	}
	if !strings.Contains(all.String(), `"id":2`) {
		t.Fatalf("no answer to the call: %s", all.String())
	}
	return all.String()
}

// An explicit config still consults the default config's base_url, or the
// legacy one's, to judge the global store. The review's input: that URL's
// password in the origin-mismatch refusal.
func TestKitExplicitConfigHoldsTheDefaultConfigs(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "OriginPass4821"
	body := "base_url = \"https://u:" + secret + "@other.vsax.net/api/v3\"\n"
	for name, rel := range map[string]string{
		"default": "config/config.toml",
		"legacy":  ".config/kaseya-vsa-cli/config.toml",
	} {
		t.Run(name, func(t *testing.T) {
			s := newSandbox(t)
			for p, b := range map[string]string{
				rel:                     body,
				"data/credentials.toml": "vsa_token_id = \"abcd1234\"\nvsa_token_secret = \"" + fixtureSecret + "\"\n",
			} {
				full := filepath.Join(s.home, p)
				if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(b), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg := s.writePrivate(t, "profile/config.toml", "base_url = \"https://tenant.vsax.net/api/v3\"\n")
			res := run(t, cli, s.env(), "--config", cfg, "auth", "status")
			requireAbsent(t, "stdout", res.stdout, secret)
			requireAbsent(t, "stderr", res.stderr, secret)
		})
	}
}

// The same when the default config appears after the server started: the
// per-call reload holds it when the global store's check reads it.
func TestKitReloadHoldsTheDefaultConfig(t *testing.T) {
	_, mcp := binaries(t)
	const secret = "OriginPass4821"
	s := newSandbox(t)
	store := filepath.Join(s.home, "data", "credentials.toml")
	if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store, []byte("vsa_token_id = \"abcd1234\"\nvsa_token_secret = \""+fixtureSecret+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	def := filepath.Join(s.home, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(def), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := s.writePrivate(t, "profile/config.toml", "base_url = \"https://tenant.vsax.net/api/v3\"\n")
	frames := mcpCallAfter(t, mcp, s.env("KASEYA_VSA_CONFIG="+cfg), func() {
		if err := os.WriteFile(def, []byte("base_url = \"https://u:"+secret+"@other.vsax.net/api/v3\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}, "kaseya-vsa_read", map[string]any{"endpoint_id": "organizations.list"})
	requireAbsent(t, "frames", frames, secret)
}

// What the discovered default names is only held: it lends the explicit
// selection no origin. Here the selected server (from KASEYA_VSA_SERVER, which
// the kit does not read) is also the default config's base_url, and the profile
// names none, so the kit has no reason to let the profile's credential go there.
func TestKitDefaultConfigLendsNoOrigin(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	srv := newRDAPServer(t, s, 200, "[]")
	def := filepath.Join(s.home, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(def), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(def, []byte("base_url = \""+srv.URL+"/api/v3\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := s.writePrivate(t, "profile/config.toml", "vsa_token_id = \"abcd1234\"\nvsa_token_secret = \""+fixtureSecret+"\"\n")
	res := run(t, cli, s.env("KASEYA_VSA_SERVER="+strings.TrimPrefix(srv.URL, "https://"), "KASEYA_VSA_CA_CERT="+srv.caFile),
		"--config", cfg, "organizations", "list", "--json")
	if out := res.stdout + res.stderr; srv.requests() != 0 || !strings.Contains(out, "credkit: withheld from outbound request") {
		t.Errorf("the default config's host was allowed the profile's credential: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
}

// Every word after set-credentials is held unless it is a flag the command
// declares. The review's input: an all-hyphen secret, printed by the --home
// refusal before the command's own holding runs.
func TestKitSetCredentialsHoldsEveryWord(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	const secret = "--------"
	home := s.writePrivate(t, secret, "not a directory")
	res := run(t, cli, s.env(), "--home", home, "auth", "set-credentials", "--", "idFoo4821", secret)
	requireAbsent(t, "stdout", res.stdout, secret)
	requireAbsent(t, "stderr", res.stderr, secret)

	s = newSandbox(t)
	res = run(t, cli, s.env(), "auth", "set-credentials", "--help")
	if !strings.Contains(res.stdout, "--help") {
		t.Errorf("a declared flag name was held: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
}

// A webhook's custom headers authenticate it to its receiver: held when they
// come from the flag, from a --stdin body or from an MCP argument, previews
// included, and when the vendor returns them.
func TestKitWebhookHeadersAreHeld(t *testing.T) {
	cli, mcp := binaries(t)
	const secret = "WebhookOpaqueQ4821"
	body := `{"Authorization":"Bearer ` + secret + `"}`
	base := "KASEYA_VSA_BASE_URL=https://h.vsax.net/api/v3"

	s := newSandbox(t)
	res := run(t, cli, s.env(base), "webhooks", "create", "--name", "x", "--priorities", `["Low"]`, "--url", "https://receiver.example",
		"--headers", body, "--dry-run")
	requireAbsent(t, "flag stdout", res.stdout, secret)
	requireAbsent(t, "flag stderr", res.stderr, secret)

	s = newSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cli, "webhooks", "create", "--stdin", "--dry-run")
	cmd.Env = s.env(base)
	cmd.Stdin = strings.NewReader(`{"Name":"x","Headers":` + body + `}`)
	out, _ := cmd.CombinedOutput()
	requireAbsent(t, "stdin", string(out), secret)

	s = newSandbox(t)
	frames, stderr := mcpCall(t, mcp, s.env(base), "kaseya-vsa_execute", map[string]any{
		"endpoint_id": "webhooks.create", "params": map[string]any{"Name": "x", "Headers": map[string]any{"Authorization": "Bearer " + secret}}})
	requireAbsent(t, "mcp frames", frames, secret)
	requireAbsent(t, "mcp stderr", stderr, secret)

	s = newSandbox(t)
	srv := newRDAPServer(t, s, 200, `{"Id":1,"Headers":{"Authorization":"Bearer `+secret+`"}}`)
	res = run(t, cli, s.env("KASEYA_VSA_BASE_URL="+srv.URL, "KASEYA_VSA_CA_CERT="+srv.caFile,
		"KASEYA_VSA_TOKEN_ID=abcd1234", "KASEYA_VSA_TOKEN_SECRET="+fixtureSecret), "webhooks", "get", "1", "--json")
	if srv.requests() == 0 {
		t.Fatalf("no request: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
	requireAbsent(t, "response stdout", res.stdout, secret)
	requireAbsent(t, "response stderr", res.stderr, secret)
}

// tree is every path under dir.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	if err := filepath.WalkDir(dir, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out = append(out, p)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// Every tool the confirm gate covers leaves the machine and the vendor as it
// found them until confirm is true. The review's input was sync, whose preview
// created the database; the list is the server's own, so a tool added later
// is covered.
func TestKitUnconfirmedPreviewChangesNothing(t *testing.T) {
	_, mcp := binaries(t)
	var hits int
	var mu sync.Mutex
	vendor := http.NewServeMux()
	vendor.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		_, _ = io.WriteString(w, "[]")
	})
	lnSrv := newVendorServer(t, vendor)

	s := newSandbox(t)
	frames, _ := mcpListTools(t, mcp, s.env("KASEYA_VSA_BASE_URL="+lnSrv))
	gated := 0
	for _, tool := range frames {
		if tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint || tool.Meta["pp:tenant-gate"] != "child-cli" {
			continue
		}
		gated++
		t.Run(tool.Name, func(t *testing.T) {
			s := newSandbox(t)
			cfg := filepath.Join(s.home, "config", "config.toml")
			if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cfg, []byte("vsa_token_id = \"abcd1234\"\nvsa_token_secret = \""+fixtureSecret+"\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			env := s.env("KASEYA_VSA_BASE_URL=" + lnSrv)
			mcpListTools(t, mcp, env)
			before := tree(t, s.home)
			args := map[string]any{}
			for _, name := range tool.InputSchema.Required {
				switch tool.InputSchema.Properties[name].Type {
				case "number", "integer":
					args[name] = 1
				case "boolean":
					args[name] = false
				case "array":
					args[name] = []string{"mcp-probe"}
				case "object":
					args[name] = map[string]any{}
				default:
					args[name] = "mcp-probe"
				}
			}
			if tool.Name == "export" {
				args["resource"] = "organizations"
			}
			out, stderr := mcpCall(t, mcp, env, tool.Name, args)
			// import has no input over MCP (its --input is a host file), so its
			// dry run stops on that; everything else previews.
			if !strings.Contains(out, "confirmation_required") && tool.Name != "import" {
				t.Errorf("no preview: frames=%s stderr=%s", out, stderr)
			}
			if after := tree(t, s.home); !slices.Equal(before, after) {
				var added []string
				for _, p := range after {
					if !slices.Contains(before, p) {
						added = append(added, strings.TrimPrefix(p, s.home))
					}
				}
				t.Errorf("the preview created %v", added)
			}
		})
	}
	if gated < 10 {
		t.Fatalf("only %d gated tools found", gated)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 0 {
		t.Errorf("a preview sent %d request(s) to the vendor", hits)
	}
}

// snapshot is every path under dir with its bytes.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range tree(t, dir) {
		info, err := os.Lstat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.IsDir() {
			out[strings.TrimPrefix(p, dir)] = info.Mode().String()
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[strings.TrimPrefix(p, dir)] = fmt.Sprintf("%v %d %s", info.Mode(), info.ModTime().UnixNano(), b)
	}
	return out
}

// snapshotChanges names what differs between two snapshots, but for SQLite's
// own sidecars of a read-only open of a WAL store.
func snapshotChanges(before, after map[string]string) []string {
	var changed []string
	for p, b := range after {
		if strings.HasSuffix(p, "-shm") || strings.HasSuffix(p, "-wal") {
			continue
		}
		if old, ok := before[p]; !ok || old != b {
			changed = append(changed, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			changed = append(changed, p)
		}
	}
	slices.Sort(changed)
	return changed
}

// A tool annotated read-only skips the confirm gate, so it may change nothing.
// The review's input was learnings_stats, which pruned aged rows; the list is
// the server's own, on a store that holds an aged row.
func TestKitReadOnlyToolsChangeNothing(t *testing.T) {
	_, mcp := binaries(t)
	vendor := http.NewServeMux()
	vendor.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "[]") })
	url := newVendorServer(t, vendor)
	s := newSandbox(t)
	tools, _ := mcpListTools(t, mcp, s.env("KASEYA_VSA_BASE_URL="+url))
	seen := 0
	for _, tool := range tools {
		if tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint || tool.Meta["pp:tenant-gate"] != "child-cli" {
			continue
		}
		seen++
		for _, seeded := range []bool{false, true} {
			name := tool.Name + "/fresh"
			if seeded {
				name = tool.Name + "/seeded"
			}
			t.Run(name, func(t *testing.T) {
				s := newSandbox(t)
				env := s.env("KASEYA_VSA_BASE_URL=" + url)
				if seeded {
					seedLooseLearnStore(t, s.home)
				}
				mcpListTools(t, mcp, env)
				before := snapshot(t, s.home)
				args := map[string]any{}
				for _, name := range tool.InputSchema.Required {
					switch tool.InputSchema.Properties[name].Type {
					case "number", "integer":
						args[name] = 1
					case "boolean":
						args[name] = false
					case "array":
						args[name] = []string{"mcp-probe"}
					case "object":
						args[name] = map[string]any{}
					default:
						args[name] = "mcp-probe"
					}
				}
				mcpCall(t, mcp, env, tool.Name, args)
				after := snapshot(t, s.home)
				changed := snapshotChanges(before, after)
				if len(changed) > 0 {
					t.Errorf("a read-only tool changed %v", changed)
				}
			})
		}
	}
	if seen < 5 {
		t.Fatalf("only %d read-only tools found", seen)
	}
}

// seedAgedLearnEvent leaves a store under home holding a row older than the
// retention window, so a prune has something to delete. It runs in a process of
// its own: this one's credential kit holds what earlier tests held.
func seedAgedLearnEvent(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, "data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSeedAgedLearnEventHelper$")
	cmd.Env = append(os.Environ(), "KIT_SEED_STORE="+filepath.Join(dir, "data.db"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed: %v\n%s", err, out)
	}
}

// seedLooseLearnStore is that store as an older release or a restored backup
// leaves it, group and world readable: tightening it is a change too.
func seedLooseLearnStore(t *testing.T, home string) {
	t.Helper()
	seedAgedLearnEvent(t, home)
	if runtime.GOOS == "windows" { // no group or other mode bits to loosen
		return
	}
	dir := filepath.Join(home, "data")
	for path, mode := range map[string]os.FileMode{dir: 0o755, filepath.Join(dir, "data.db"): 0o644} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSeedAgedLearnEventHelper(t *testing.T) {
	path := os.Getenv("KIT_SEED_STORE")
	if path == "" {
		t.Skip("helper for seedAgedLearnEvent")
	}
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.DB().Exec(`INSERT INTO learn_events (ts, event, surface) VALUES ('2020-01-01T00:00:00Z', 'teach', 'mcp')`); err != nil {
		t.Fatal(err)
	}
}

func newVendorServer(t *testing.T, h http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "http://" + ln.Addr().String()
}

type listedTool struct {
	Name        string         `json:"name"`
	Meta        map[string]any `json:"_meta"`
	Annotations struct {
		ReadOnlyHint *bool `json:"readOnlyHint"`
	} `json:"annotations"`
	InputSchema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	} `json:"inputSchema"`
}

// mcpListTools starts the server, lists its tools and exits.
func mcpListTools(t *testing.T, bin string, env []string) ([]listedTool, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
`)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	for _, line := range strings.Split(out.String(), "\n") {
		var msg struct {
			ID     int `json:"id"`
			Result struct {
				Tools []listedTool `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &msg) == nil && msg.ID == 2 {
			return msg.Result.Tools, errb.String()
		}
	}
	t.Fatalf("no tools/list answer: %s %s", out.String(), errb.String())
	return nil, ""
}
