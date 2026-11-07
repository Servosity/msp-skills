// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeperblackbox

// Credential-rule classes the fleet Codex reviews found, each proven on the built
// binaries:
//   - output-before-chokepoint: the configuration and the credential store
//     print warnings while they load, so the kit is installed first and delivers
//     nothing until every startup source is held (CLI and MCP);
//   - held-value-crosses-process-boundary: a mirrored MCP tool runs the CLI
//     as a child process, and a value only the MCP server holds never reaches
//     a child through an argument (the kit's RunCommand refuses it).

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Class output-before-chokepoint: a config file other users can change names
// another Service Mode host, so loading it prints a refusal that repeats its base_url.
// The URL's path repeats the environment token and its userinfo carries a
// password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	cfg := filepath.Join(e.home, "loose.toml")
	base := "https://ro:" + bbStored + "@other.example.com/" + bbToken
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	makeLoose(t, cfg)
	e = e.without("KEEPER_BASE_URL").with("KEEPER_CONFIG=" + cfg)
	cli := e.run(t, "version")
	// The refusal repeats the base_url, so the kit withholds it whole.
	if !strings.Contains(cli, bbWithheld) {
		t.Fatalf("the CLI printed no notice for the loose config; the test did not run:\n%.1500s", cli)
	}
	assertNoEcho(t, "CLI startup", cli)
	// The MCP server reads the config at its first tool call.
	assertNoEcho(t, "MCP startup", e.mcpCalls(t, nil))
	assertNoEcho(t, "MCP first call", e.mcpCalls(t, []map[string]any{{"name": "health_get", "arguments": map[string]any{}}}))
}

// Class held-value-crosses-process-boundary: the MCP server holds the
// credentials responses carry; recall is a mirrored tool whose CLI child
// holds none of those values. A control call proves the child really runs.
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	recall := func(query string) map[string]any {
		return map[string]any{"name": "recall", "arguments": map[string]any{"query": query}}
	}
	none := map[string]any{}
	// The fetches answer first, so the server holds their credentials before
	// any recall call arrives (mcp-go serves one phase's calls concurrently).
	// queue_get-status and health_get are endpoint tools the MCP server serves
	// in-process; the novel tools run as CLI children and hold their own.
	fetches := []map[string]any{
		{"name": "queue_get-status", "arguments": none}, // gzip: a long and a short response credential
		{"name": "health_get", "arguments": none},       // a BOM-prefixed response credential
	}
	calls := []map[string]any{recall("control learning note")}
	for _, v := range []string{bbGzipResp, bbShortResp, bbBOMResp} {
		a, b := halves(v)
		// Whole, and split across a space and across a line, as splitObject sends them.
		calls = append(calls, recall("rotated "+v), recall(a+" "+b), recall(a+"\n"+b))
	}
	out := e.mcpPhases(t, fetches, calls)
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(fetches)+len(calls))) {
		t.Fatalf("MCP session did not answer every call:\n%.2000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	if !strings.Contains(out, "control learning note") {
		t.Fatalf("the control recall call did not reach its child; the test did not run:\n%.2000s", out)
	}
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), "credentials") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(e.home, path)
		assertNoEcho(t, "file "+rel, string(data))
		return nil
	})
}
