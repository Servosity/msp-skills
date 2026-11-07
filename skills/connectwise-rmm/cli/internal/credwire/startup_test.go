// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// credkit-wiring (handfixes.json): credkit.Start is the first statement of
// both mains, so every credential in config.toml is held before the program
// can print a byte, a config that does not parse included. A credential in a
// malformed config never appears in connectwise-rmm-cli's startup output
// (stdout and stderr, plain and --json) or in connectwise-rmm-mcp's stderr and
// JSON-RPC responses, whether the file is the discovered default or named
// explicitly (--config for the CLI, CONNECTWISE_RMM_CONFIG for the MCP server,
// which takes no --config). A run the kit refuses to start (exit 78,
// credkit.ExitRefused: a value it holds from the malformed file occurs in its
// public text) prints no credential either, and counts as a run.

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectwise-rmm-pp-cli/internal/credkit"
)

// malformedConfigs are config.toml bodies that do not parse, each with one
// credential in it, and one that parses with a base_url that does not (its
// password is what the program prints when nothing holds it). ownBase runs it
// on its own base_url instead of the fake's. Every credential is a run of
// letters no ordinary output shares an 8-character piece with.
var malformedConfigs = []struct {
	name, cred, body string
	ownBase          bool
}{
	{"unquoted bare client secret", "cfgbarevqzkjwxmtpqzk", "client_secret = cfgbarevqzkjwxmtpqzk\n", false},
	{"legacy secret then a broken table", "cfgtablekvjqzwxmhpvq", "rmm_client_secret = \"cfgtablekvjqzwxmhpvq\"\n[headers\nX-Trace = \"y\"\n", false},
	{"credential after 64 KiB of padding", "cfgpadqkzvjwxmrtqzvk", strings.Repeat("# "+strings.Repeat("p", 98)+"\n", 700) + "access_token = \"cfgpadqkzvjwxmrtqzvk\"\n= no key\n", false},
	{"inside auth_header", "cfgauthzvqkjxwmnpzvq", "auth_header = \"Bearer cfgauthzvqkjxwmnpzvq\"\n[[broken\n", false},
	{"inside headers", "cfghdrqvzkxjwmbtqvzk", "[headers]\nX-Api-Key = \"cfghdrqvzkxjwmbtqvzk\"\n[headers.\n", false},
	{"password in a base_url that does not parse", "cfgpwvqzkjwxmtpkzq", "client_secret = \"cfgsecvqzkjwxmhzpq\"\nbase_url = \"https://u:cfgpwvqzkjwxmtpkzq@bad host/\"\n", true},
}

// writeConfig writes body as the discovered default config under the
// sandbox's CONNECTWISE_RMM_HOME, or as a file elsewhere when explicit, and
// returns its path.
func writeConfig(t *testing.T, home, body string, explicit bool) string {
	t.Helper()
	dir := filepath.Join(home, "cw-home", "config")
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
	base, ca := fakeAPI(t, func(_ http.Header, r *http.Request) (int, string) {
		if isToken(r) {
			return 200, tokenJSON("stoktokqzvkjwxmhzpqk")
		}
		return 200, companyPage("HQ-Note")
	})
	for _, c := range malformedConfigs {
		for _, explicit := range []bool{false, true} {
			where := "default config"
			if explicit {
				where = "explicit config"
			}
			t.Run(c.name+", "+where, func(t *testing.T) {
				home := t.TempDir()
				path := writeConfig(t, home, c.body, explicit)
				env := closedEnv(home, "CONNECTWISE_RMM_BASE_URL="+base, "CONNECTWISE_RMM_CA_CERT="+ca)
				if c.ownBase {
					env = closedEnv(home)
				}
				var pre []string
				if explicit {
					pre = []string{"--config", path}
				}
				ran := false
				for _, args := range [][]string{{"doctor"}, {"doctor", "--json"}, {"companies", "list"}, {"companies", "list", "--json"}} {
					out, errOut, code := runSplit(t, cliBin, env, append(append([]string{}, pre...), args...)...)
					if strings.TrimSpace(out+errOut) != "" || code == credkit.ExitRefused {
						ran = true
					}
					assertNoPiece(t, "connectwise-rmm-cli "+strings.Join(args, " ")+" stdout", out, c.cred)
					assertNoPiece(t, "connectwise-rmm-cli "+strings.Join(args, " ")+" stderr", errOut, c.cred)
				}
				if !ran {
					t.Fatalf("connectwise-rmm-cli printed nothing at all, so this run proves nothing")
				}
				mcpEnv := env
				if explicit {
					mcpEnv = append(append([]string{}, env...), "CONNECTWISE_RMM_CONFIG="+path)
				}
				replies, errOut, code := mcpRun(t, mcpBin, mcpEnv,
					rpc{"tools/list", map[string]any{}},
					call("connectwise-rmm_read", map[string]any{"endpoint_id": "companies.list"}),
					call("companies_list", map[string]any{}))
				if !strings.Contains(replies, `"id":1`) && code != credkit.ExitRefused {
					t.Fatalf("connectwise-rmm-mcp answered no initialize, so this run proves nothing:\nstdout:\n%s\nstderr:\n%s", replies, errOut)
				}
				assertNoPiece(t, "connectwise-rmm-mcp JSON-RPC responses", replies, c.cred)
				assertNoPiece(t, "connectwise-rmm-mcp stderr", errOut, c.cred)
			})
		}
	}
}
