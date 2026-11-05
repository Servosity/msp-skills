// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// startup-config-held-first (handfixes.json): credkit.Start is the first
// statement of both mains, so every credential in config.toml is held before
// the program can print a byte, a config that does not parse included. A
// credential in a malformed config never appears in gradient-cli's startup
// output (stdout and stderr, plain and --json) or in gradient-mcp's stderr
// and JSON-RPC responses, whether the file is the discovered default or named
// explicitly (--config for the CLI, GRADIENT_CONFIG for the MCP server, which
// takes no --config). A run the kit refuses to start (exit 78,
// credkit.ExitRefused: a value it holds from the malformed file occurs in its
// public text) prints no credential either, and counts as a run.

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gradient-pp-cli/internal/credkit"
)

// malformedConfigs are config.toml bodies that do not parse, each with one
// credential in it, and one that parses with a base_url that does not (its
// password is what the program prints when nothing holds it: the case that
// proves this test fails without the kit, .review/seen/mutant-startup-no-kit.log).
// ownBase runs it on its own base_url instead of the fake's. Every credential
// is a run of letters no ordinary output shares an 8-character piece with.
var malformedConfigs = []struct {
	name, cred, body string
	ownBase          bool
}{
	{"unquoted bare value", "cfgbarevqzkjwxmtpqzk", "token = cfgbarevqzkjwxmtpqzk\n", false},
	{"valid line then a broken table", "cfgtablekvjqzwxmhpvq", "token = \"cfgtablekvjqzwxmhpvq\"\n[headers\nX-Trace = \"y\"\n", false},
	{"credential after 64 KiB of padding", "cfgpadqkzvjwxmrtqzvk", strings.Repeat("# "+strings.Repeat("p", 98)+"\n", 700) + "token = \"cfgpadqkzvjwxmrtqzvk\"\n= no key\n", false},
	{"inside auth_header", "cfgauthzvqkjxwmnpzvq", "auth_header = \"Bearer cfgauthzvqkjxwmnpzvq\"\n[[broken\n", false},
	{"inside headers", "cfghdrqvzkxjwmbtqvzk", "[headers]\nX-Api-Key = \"cfghdrqvzkxjwmbtqvzk\"\n[headers.\n", false},
	{"password in a base_url that does not parse", "cfgpwvqzkjwxmtpkzq", "token = \"cfgtokvqzkjwxmhzpq\"\nbase_url = \"https://u:cfgpwvqzkjwxmtpkzq@bad host/\"\n", true},
}

// writeConfig writes body as the discovered default config under the
// sandbox's GRADIENT_HOME, or as a file elsewhere when explicit, and returns
// its path.
func writeConfig(t *testing.T, home, body string, explicit bool) string {
	t.Helper()
	dir := filepath.Join(home, "gradient-home", "config")
	if explicit {
		dir = filepath.Join(home, "elsewhere")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStartupConfigHeldFirst(t *testing.T) {
	cliBin, mcpBin := binaries(t)
	base, ca := fakeAPI(t, `{"data":{"id":"vend-1","name":"HQ-Vendor"},"message":"ok","status":200}`)
	for _, c := range malformedConfigs {
		for _, explicit := range []bool{false, true} {
			where := "default config"
			if explicit {
				where = "explicit config"
			}
			t.Run(c.name+", "+where, func(t *testing.T) {
				home := t.TempDir()
				path := writeConfig(t, home, c.body, explicit)
				env := closedEnv(home, "GRADIENT_BASE_URL="+base, "GRADIENT_CA_CERT="+ca)
				if c.ownBase {
					env = closedEnv(home)
				}
				var pre []string
				if explicit {
					pre = []string{"--config", path}
				}
				ran := false
				for _, args := range [][]string{{"doctor"}, {"doctor", "--json"}, {"vendor", "get"}, {"vendor", "get", "--json"}} {
					out, errOut, code := runSplit(t, cliBin, env, append(append([]string{}, pre...), args...)...)
					if strings.TrimSpace(out+errOut) != "" || code == credkit.ExitRefused {
						ran = true
					}
					assertNoPiece(t, "gradient-cli "+strings.Join(args, " ")+" stdout", out, c.cred)
					assertNoPiece(t, "gradient-cli "+strings.Join(args, " ")+" stderr", errOut, c.cred)
				}
				if !ran {
					t.Fatalf("gradient-cli printed nothing at all, so this run proves nothing")
				}
				mcpEnv := env
				if explicit {
					mcpEnv = append(append([]string{}, env...), "GRADIENT_CONFIG="+path)
				}
				replies, errOut, code := mcpSession(t, mcpBin, mcpEnv)
				if !strings.Contains(replies, `"id":1`) && code != credkit.ExitRefused {
					t.Fatalf("gradient-mcp answered no initialize, so this run proves nothing:\nstdout:\n%s\nstderr:\n%s", replies, errOut)
				}
				assertNoPiece(t, "gradient-mcp JSON-RPC responses", replies, c.cred)
				assertNoPiece(t, "gradient-mcp stderr", errOut, c.cred)
			})
		}
	}
}

// assertNoPiece fails when text carries cred or any 8-character piece of it.
func assertNoPiece(t *testing.T, where, text, cred string) {
	t.Helper()
	for i := 0; i+8 <= len(cred); i++ {
		if strings.Contains(text, cred[i:i+8]) {
			t.Errorf("%s carries %q, a piece of the config credential:\n%s", where, cred[i:i+8], text)
			return
		}
	}
}

// mcpSession runs one gradient-mcp stdio session: initialize, tools/list, a
// typed endpoint tool and a mirrored CLI tool, then closes stdin. It returns
// every JSON-RPC message the server wrote, its stderr and its exit code.
func mcpSession(t *testing.T, mcpBin string, env []string) (stdout, stderr string, code int) {
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
				if strings.Contains(l, `"id":`+itoa(id)) {
					return
				}
			case <-deadline:
				return
			}
		}
	}
	send(1, "initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}})
	send(0, "notifications/initialized", map[string]any{})
	send(2, "tools/list", map[string]any{})
	send(3, "tools/call", map[string]any{"name": "vendor_get", "arguments": map[string]any{}})
	send(4, "tools/call", map[string]any{"name": "doctor", "arguments": map[string]any{}})
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

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
