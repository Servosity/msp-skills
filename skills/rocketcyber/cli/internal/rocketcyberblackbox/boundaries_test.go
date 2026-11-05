// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rocketcyberblackbox

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
	"runtime"
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
// another host, so loading it prints a refusal that repeats its base_url. The
// URL's path repeats the environment's key (ROCKETCYBER_API_TOKEN) and the MCP HTTP
// token, and its userinfo carries a password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("loose config permissions are POSIX mode bits; Windows has no group or other bits")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	cfg := filepath.Join(e.home, "loose.toml")
	base := "https://ro:" + bbStored + "@other.example.com/" + bbToken + "/" + bbHTTPToken
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "ROCKETCYBER_BASE_URL"), "ROCKETCYBER_CONFIG="+cfg)
	cli := e.run(t, "version")
	mcp := e.mcpCalls(t, nil)
	// The refusal repeats the held values, so the kit withholds that line
	// whole: the CLI writes the kit's stderr notice in its place; the MCP
	// server, the same config load, may drop the line with no notice (the
	// kit drops a notice it cannot place whole), so the CLI is the proof
	// the case ran and both are scanned.
	if !strings.Contains(cli, "refused") && !strings.Contains(cli, "not applied") && !strings.Contains(cli, kitNotice("stderr")) {
		t.Fatalf("CLI startup printed no refusal for the loose config; the test did not run:\n%.1500s", cli)
	}
	for where, out := range map[string]string{"CLI startup": cli, "MCP startup": mcp} {
		assertNoEcho(t, where, out)
	}
}

// Class held-value-crosses-process-boundary: the MCP server holds the API
// token and the MCP HTTP token (its secret environment); teach is a mirrored
// tool whose CLI child writes its notes to the local store, so the child must
// get the held set (credential leak contract section 1: a child process gets
// the held set or is not started). A control call proves the child really
// writes. (The credential-named members the fetches answer are not held
// under the contract, RocketCyber declares no response field: see
// outOfContract.)
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find agent", "resource-type": "agents", "resource": []any{bbAgentID},
			"notes": notes, "no-validate": true}}
	}
	// The fetches answer first (mcp-go serves one phase's calls
	// concurrently), so the teach calls arrive at a server that has used the
	// token.
	fetches := []map[string]any{
		tool("account_get", map[string]any{}),    // gzip OAuth credentials (access and refresh tokens, a client secret, an encryption key and a username)
		tool("agents_list", map[string]any{}),    // a short password in a gzip list
		tool("incidents_list", map[string]any{}), // a BOM-prefixed api_key and a credentials container
		tool("apps_list", map[string]any{}),      // an app's access_token
	}
	calls := []map[string]any{teach("control learning note")}
	for _, v := range []string{bbToken, bbHTTPToken} {
		a, b := halves(v)
		// Whole, and split across a space and across a line, as splitObject
		// sends them.
		calls = append(calls, teach("rotated "+v), teach(a+" "+b), teach(a+"\n"+b))
	}
	out := e.mcpPhases(t, fetches, calls)
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(fetches)+len(calls))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
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
