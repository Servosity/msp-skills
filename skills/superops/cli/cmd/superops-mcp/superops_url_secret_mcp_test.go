// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a username or password in SUPEROPS_BASE_URL
// is never shown in an MCP tool result or error (superops-base-url-never-printed).

package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/cliutil/testenv"
)

const (
	mcpURLSecretUser = "mcpUserKp42Wd"
	mcpURLSecretPass = "test-secret-mcp-password"
)

const mcpURLSecretRawQuery = "query{ getTicketList(input:{page:1,pageSize:5}){ listInfo{ hasMore } } }"

func mcpURLSecretForms() []string {
	var out []string
	for _, v := range []string{mcpURLSecretUser, mcpURLSecretPass, mcpURLSecretUser + ":" + mcpURLSecretPass} {
		quoted, _ := json.Marshal(v)
		var uni strings.Builder
		for _, r := range v {
			fmt.Fprintf(&uni, `\u%04x`, r)
		}
		out = append(out,
			v,
			url.QueryEscape(v),
			url.PathEscape(v),
			base64.StdEncoding.EncodeToString([]byte(v)),
			base64.RawStdEncoding.EncodeToString([]byte(v)),
			base64.URLEncoding.EncodeToString([]byte(v)),
			base64.RawURLEncoding.EncodeToString([]byte(v)),
			hex.EncodeToString([]byte(v)),
			string(quoted[1:len(quoted)-1]),
			uni.String(),
		)
	}
	return out
}

func assertMCPNoURLSecret(t *testing.T, where string, texts ...string) {
	t.Helper()
	for _, text := range texts {
		low := strings.ToLower(text)
		for _, form := range mcpURLSecretForms() {
			if strings.Contains(low, strings.ToLower(form)) {
				t.Errorf("%s shows the base URL userinfo (%q):\n%s", where, form, text)
				return
			}
		}
	}
}

// buildURLSecretCLI builds the companion CLI the mirrored tools shell out to.
// It runs before the sandbox: go build needs the caller's module and build
// caches, which live under the real home.

var mcpURLSecretOpRe = regexp.MustCompile(`\b(get\w+List|getKbItems|getWorklogEntries)\s*\(`)

func serveMCPURLSecretStub(t *testing.T) (string, string) {
	t.Helper()
	return serveHolderTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/err") {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"errors": []any{map[string]any{
				"message": "denied: authorization " + r.Header.Get("Authorization") + " for " + r.Host + r.URL.String(),
			}}})
			return
		}
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		m := mcpURLSecretOpRe.FindStringSubmatch(req.Query)
		if r.Method != http.MethodPost || m == nil {
			_, _ = io.WriteString(w, `{"data":{}}`)
			return
		}
		var nodes []map[string]any
		if m[1] == "getTicketList" {
			nodes = append(nodes, map[string]any{"ticketId": "7300000000000009", "displayId": "M-9", "subject": "mcp url secret", "status": "Open", "updatedTime": "2026-01-01T00:00:00"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{m[1]: map[string]any{
			"nodes": nodes, "listInfo": map[string]any{"hasMore": false, "page": 1, "pageSize": 50, "totalCount": len(nodes)},
		}}})
	}))
}

func mcpURLSecretBase(hostport string) string {
	return "https://" + mcpURLSecretUser + ":" + mcpURLSecretPass + "@" + hostport
}

// callURLSecretTool makes one tools/call on a fresh server built from main, in
// this test's environment, and returns the responses and the server's stderr.
// A real process: the kit starts in main and keeps one held set per process.
func callURLSecretTool(t *testing.T, mcpBin, name string, args map[string]any) string {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	return runHolderMCP(t, mcpBin, os.Environ(), nil, map[string]any{"name": name, "arguments": args})
}

// Every MCP tool kind, for every way the base URL reaches a result: typed
// endpoint tools (in-process client), tools that mirror a CLI command (the
// companion binary), the local sql and context tools, with a reachable
// server, a closed port, an HTTP error that echoes the request, a base URL
// that does not parse, and an untrusted config. No result, error or server
// log shows the username or the password, in any form.
func TestMCPBaseURLUserinfoIsNeverShown(t *testing.T) {
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	mcpBin := buildHolderBinary(t, "superops-mcp", ".")
	restore, err := cliutil.SetHomeOverride("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("SUPEROPS_CLI_PATH", cliBin)
	t.Setenv("SUPEROPS_API_TOKEN", "sop-mcp-url-secret-token-8b2e")
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-url")
	t.Setenv("SUPEROPS_REGION", "")
	t.Setenv(cliutil.VerifyEnvVar, "")
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")

	host, ca := serveMCPURLSecretStub(t)
	t.Setenv("SUPEROPS_CA_CERT", ca)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := ln.Addr().String()
	_ = ln.Close()

	type call struct {
		tool string
		args map[string]any
		// want is text the response must hold, so a call that never
		// reached its path fails instead of passing.
		want string
	}
	rawQuery := map[string]any{"args": mcpURLSecretRawQuery}
	for _, tc := range []struct {
		name    string
		base    string
		dogfood bool
		calls   []call
	}{
		{"reachable", mcpURLSecretBase(host), false, []call{
			{"context", nil, "superops"},
			{"tickets_list", nil, `\"data\"`},
			{"alerts_list", nil, `\"data\"`},
			{"raw_query", rawQuery, "getTicketList"},
			// A successful sync prints its events to stderr, which the
			// mirror does not relay; the sql calls below read its rows.
			{"sync", map[string]any{"resources": "tickets"}, ""},
			{"workflow_status", nil, "tickets"},
			{"sql", map[string]any{"query": "SELECT * FROM sync_state"}, "scope_key"},
			{"sql", map[string]any{"query": "SELECT tenant FROM superops_store_tenant"}, "base_url=https://" + host},
		}},
		{"transport-retries", mcpURLSecretBase(closed), false, []call{
			{"tickets_list", nil, refusedDialText()},
		}},
		{"transport", mcpURLSecretBase(closed), true, []call{
			{"alerts_list", nil, refusedDialText()},
			{"raw_query", rawQuery, refusedDialText()},
			{"sync", map[string]any{"resources": "tickets"}, "sync_error"},
		}},
		{"http-error", mcpURLSecretBase(host) + "/err", false, []call{
			// The stub repeats the Authorization header: the in-process tool's unit is
			// withheld by the kit, the mirrored CLI's error is masked by the client.
			{"tickets_list", nil, "withheld"},
			{"raw_query", rawQuery, "401"},
			{"sync", map[string]any{"resources": "tickets"}, "sync_"},
		}},
		{"displaced", "http://" + mcpURLSecretUser + ":1/" + mcpURLSecretPass + "%40" + host, true, []call{
			{"tickets_list", nil, "base URL refused"},
			{"raw_query", rawQuery, "base URL refused"},
		}},
		{"bad-port", mcpURLSecretBase("127.0.0.1:notaport"), true, []call{
			{"tickets_list", nil, "invalid port"},
			{"raw_query", rawQuery, "invalid port"},
		}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SUPEROPS_BASE_URL", tc.base)
			if tc.dogfood {
				t.Setenv(cliutil.DogfoodEnvVar, "1")
			} else {
				t.Setenv(cliutil.DogfoodEnvVar, "")
			}
			for _, c := range tc.calls {
				resp := callURLSecretTool(t, mcpBin, c.tool, c.args)
				assertMCPNoURLSecret(t, tc.name+" "+c.tool, resp)
				if c.want != "" && !strings.Contains(resp, c.want) {
					t.Errorf("%s %s never showed %q (the call did not reach its path):\n%s", tc.name, c.tool, c.want, resp)
				}
			}
		})
	}

	// A query or fragment on the base URL is refused at config load. Its
	// value is one no case registers with the process-wide funnel, so only
	// the refusal keeps it out of tool errors.
	const queryValue = "McpQueryValueRt58"
	for _, base := range []string{"http://" + closed + "/?sig=" + queryValue, "http://" + closed + "#" + queryValue} {
		t.Run("query-or-fragment "+base, func(t *testing.T) {
			t.Setenv("SUPEROPS_BASE_URL", base)
			t.Setenv(cliutil.DogfoodEnvVar, "1")
			for _, c := range []call{{"tickets_list", nil, ""}, {"raw_query", rawQuery, ""}} {
				resp := callURLSecretTool(t, mcpBin, c.tool, c.args)
				if strings.Contains(resp, queryValue) {
					t.Errorf("%s shows the base URL query or fragment:\n%s", c.tool, resp)
				}
				if !strings.Contains(resp, "base URL refused") {
					t.Errorf("%s never showed the refusal:\n%s", c.tool, resp)
				}
			}
		})
	}

	// An untrusted config names the base URL; SUPEROPS_BASE_URL is unset.
	t.Run("untrusted-config", func(t *testing.T) {
		t.Setenv("SUPEROPS_BASE_URL", "")
		t.Setenv(cliutil.DogfoodEnvVar, "1")
		dir, err := cliutil.ConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(path, []byte(fmt.Sprintf("base_url = %q\n", mcpURLSecretBase(host))), 0o600); err != nil {
			t.Fatal(err)
		}
		makeReadableByOthers(t, path)
		t.Cleanup(func() { _ = os.Remove(path) })
		for _, c := range []call{
			{"tickets_list", nil, "untrusted config names base_url"},
			{"raw_query", rawQuery, "untrusted config names base_url"},
			{"context", nil, "superops"},
		} {
			resp := callURLSecretTool(t, mcpBin, c.tool, c.args)
			assertMCPNoURLSecret(t, "untrusted-config "+c.tool, resp)
			if !strings.Contains(resp, c.want) {
				t.Errorf("untrusted-config %s never showed %q:\n%s", c.tool, c.want, resp)
			}
		}
	})
}

// refusedDialText is how the OS words a dial to a closed local port.
func refusedDialText() string {
	if runtime.GOOS == "windows" {
		return "actively refused"
	}
	return "connection refused"
}
