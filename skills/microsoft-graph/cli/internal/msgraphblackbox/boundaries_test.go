// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package msgraphblackbox

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
// another Graph host, so loading it prints a refusal that repeats its base_url.
// The URL's path repeats the environment token and its userinfo carries a
// password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the loose-config refusal needs Unix group/other mode bits")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	cfg := filepath.Join(e.home, "loose.toml")
	base := "https://ro:" + bbStored + "@other.example.com/" + bbToken
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "MICROSOFT_GRAPH_BASE_URL"), "MICROSOFT_GRAPH_CONFIG="+cfg)
	cli := e.run(t, "version")
	// The server reads the config on its first tool call, not at startup.
	mcp := e.mcpCalls(t, []map[string]any{{"name": "users_list", "arguments": map[string]any{}}})
	for where, out := range map[string]string{"CLI startup": cli, "MCP startup": mcp} {
		// The refusal repeats the base_url, so the kit withholds it whole.
		if !strings.Contains(out, "refused") && !strings.Contains(out, withheldNotice) {
			t.Fatalf("%s printed no refusal for the loose config; the test did not run:\n%.1500s", where, out)
		}
		assertNoEcho(t, where, out)
	}
}

// Class held-value-crosses-process-boundary: the MCP server holds the
// credentials responses carry; teach is a mirrored tool whose CLI child
// writes its notes to the local store and holds none of those values. A
// control call proves the child really writes.
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find user", "resource-type": "users", "resource": []any{"5c802b19-3600-83f1-1767-7b9edc7f38ab"},
			"notes": notes, "no-validate": true}}
	}
	none := map[string]any{}
	// The fetches answer first, so the server holds their credentials before
	// any teach call arrives (mcp-go serves one phase's calls concurrently).
	fetches := []map[string]any{
		{"name": "phone-numbers_list", "arguments": none}, // a gzip response credential
		{"name": "teams-users_list", "arguments": none},   // a short response credential
		{"name": "licenses_skus", "arguments": none},      // a BOM-prefixed response credential
	}
	calls := []map[string]any{teach("control learning note")}
	for _, v := range []string{bbGzipResp, bbShortResp, bbBOMResp} {
		a, b := halves(v)
		// Whole, and split the ways the chokepoint rejoins (across a space
		// and across a line), as splitObject sends them.
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
