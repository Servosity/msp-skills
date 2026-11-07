// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kbmsblackbox

// Credential-rule classes the fleet Codex reviews found, each proven on the built
// binaries:
//   - output-before-chokepoint: the configuration and the credential store
//     print warnings while they load, so the chokepoint is installed first
//     and delivers nothing until every startup source is held (CLI and MCP);
//   - held-value-crosses-process-boundary: a mirrored MCP tool runs the CLI
//     as a child process, which holds only what it loads itself, so a value
//     only the MCP server holds never reaches a child through an argument.

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withoutEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}

// Class output-before-chokepoint: a config file other users can change names
// another Kaseya BMS host, so loading it prints a refusal that repeats its base_url.
// The URL's path repeats an environment credential (the login password, with
// or without a pre-minted KASEYA_BMS_TOKEN beside it), which the refused route
// never applies, and its userinfo carries a password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	for _, mode := range []credMode{credToken, credLogin} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			cfg := filepath.Join(e.home, "loose.toml")
			base := "https://ro:" + bbArgvSec + "@other.example.com/" + mode.held()
			if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
				t.Fatal(err)
			}
			loosen(t, cfg)
			e.env = append(withoutEnv(e.env, "KASEYA_BMS_BASE_URL"), "KASEYA_BMS_CONFIG="+cfg)
			cli := e.run(t, "version")
			// The MCP server loads the configuration when a tool needs it (the
			// kit read the files at its start and printed nothing).
			mcp := e.mcpCalls(t, []map[string]any{{"name": "kaseya-bms_read", "arguments": map[string]any{"endpoint_id": "servicedesk.get-tickets"}}})
			for where, out := range map[string]string{"CLI startup": cli, "MCP startup": mcp} {
				// The refusal repeats a held value (the URL path), so the kit
				// withholds it whole: its notice is the refusal.
				if !strings.Contains(out, "refused") && !strings.Contains(out, "credkit: withheld from stderr") {
					t.Fatalf("%s printed no refusal for the loose config; the test did not run:\n%.1500s", where, out)
				}
				assertNoEcho(t, where, out)
			}
			if _, mints, _, _ := sv.counts(); mints != 0 {
				t.Errorf("%s: the refused route minted %d time(s)", mode, mints)
			}
		})
	}
}

// Class held-value-crosses-process-boundary: the MCP server holds the
// credentials responses carry and the token it minted for the login; teach
// is a mirrored tool whose CLI child writes its notes to the local store and
// holds only what it loads itself (the cached token from the credential store,
// never a response credential the server saw). A control call proves the
// child really writes.
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	t.Parallel() // its own server, home and env: independent of every other test
	sv := echoServer(t)
	e := newEnv(t, sv, credLogin)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find ticket", "resource-type": "servicedesk", "resource": []any{bbTicket},
			"notes": notes, "no-validate": true}}
	}
	read := func(id string, params map[string]any) map[string]any {
		args := map[string]any{"endpoint_id": id}
		if params != nil {
			args["params"] = params
		}
		return map[string]any{"name": "kaseya-bms_read", "arguments": args}
	}
	// The fetches answer first, so the server holds their credentials before
	// any teach call arrives (mcp-go serves one phase's calls concurrently).
	fetches := []map[string]any{
		read("servicedesk.get-tickets", nil),                                             // the minted token
		read("integrations.get-itgaccess-info", nil),                                     // a gzip response credential
		read("integrations.get-itgpassword-value", map[string]any{"passwordId": "1001"}), // a short bare-Result credential
	}
	calls := []map[string]any{teach("control learning note")}
	for _, v := range []string{bbMintAccess, bbGzipResp, bbShortResp} {
		calls = append(calls, teach("rotated "+v))
		if len(v) < 8 {
			// The kit matches a value under 8 characters only as a whole
			// token, so a split one is not sent (.review/kit-findings.md).
			continue
		}
		a, b := halves(v)
		// Split across a space and across a line, as splitObject sends them:
		// each half carries an 8-character piece of the value.
		calls = append(calls, teach(a+" "+b), teach(a+"\n"+b))
	}
	out := e.mcpPhases(t, fetches, calls)
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(fetches)+len(calls))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	if _, _, minted, _ := sv.counts(); minted == 0 {
		t.Fatalf("the MCP server minted no token; the test did not run:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	var sawControl bool
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), "credentials") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(e.home, path)
		sawControl = sawControl || bytes.Contains(data, []byte("control learning note"))
		assertNoEcho(t, "file "+rel, string(data))
		return nil
	})
	if !sawControl {
		t.Fatalf("the control teach call wrote no learning; the test did not run:\n%.2000s", out)
	}
}
