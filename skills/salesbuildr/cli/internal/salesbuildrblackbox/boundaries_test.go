// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package salesbuildrblackbox

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
// another Salesbuildr host, so loading it prints a refusal that repeats its base_url.
// The URL's path repeats the environment token and its userinfo carries a
// password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("loose config permissions are POSIX mode bits; Windows has no group or other bits")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvToken)
	cfg := filepath.Join(e.home, "loose.toml")
	base := "https://ro:" + bbStored + "@other.example.com/" + bbToken
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "SALESBUILDR_BASE_URL"), "SALESBUILDR_CONFIG="+cfg)
	cli := e.run(t, "version")
	mcp := e.mcpCalls(t, nil)
	// The refusal repeats the held values, so the kit withholds that line
	// whole: the CLI writes the kit's stderr notice in its place; the MCP
	// server, the same config load, may drop the line with no notice (the kit
	// drops a notice it cannot place whole), so the CLI is the proof the case
	// ran and both are scanned (rocketcyber precedent).
	if !strings.Contains(cli, "refused") && !strings.Contains(cli, kitNotice("stderr")) {
		t.Fatalf("CLI startup printed no refusal for the loose config; the test did not run:\n%.1500s", cli)
	}
	for where, out := range map[string]string{"CLI startup": cli, "MCP startup": mcp} {
		assertNoEcho(t, where, out)
	}
}

// Class held-value-crosses-process-boundary: the MCP server holds the
// api key and the MCP HTTP token (its secret environment) and the password
// members the fetches answer (a declared response field); teach is a mirrored
// tool whose CLI child writes its notes to the local store, so the child must
// get the held set (credential leak contract section 1: a child process gets
// the held set or is not started). A control call proves the child really
// writes. (An accessToken member is not a declared response field, so it is
// not held under the contract: see outOfContract.)
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvToken)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find company", "resource-type": "company", "resource": []any{bbCompany},
			"notes": notes, "no-validate": true}}
	}
	read := func(id string, params map[string]any) map[string]any {
		return map[string]any{"name": "salesbuildr_read", "arguments": map[string]any{"endpoint_id": id, "params": params}}
	}
	// The fetches answer first (mcp-go serves one phase's calls concurrently),
	// so the teach calls arrive at a server that has used the token and holds
	// the password members the fetches answered.
	fetches := []map[string]any{
		read("company.public-get-list", map[string]any{}),                 // a gzip response credential
		read("opportunity.public-get-by-id", map[string]any{"id": bbOpp}), // a short response credential
		read("category.public-get-all-categories", map[string]any{}),      // a BOM-prefixed response credential
	}
	calls := []map[string]any{teach("control learning note")}
	for _, v := range []string{bbToken, bbHTTPToken, bbShortResp, bbBOMResp} {
		calls = append(calls, teach("rotated "+v))
		if len(v) < 8 {
			// A short value is held whole and in its encodings; its halves
			// (three and four characters) are not a credential the kit reads
			// across a space (kit note in .review/followups.md).
			continue
		}
		a, b := halves(v)
		// Split the ways the kit rejoins (across a space and across a line),
		// as splitObject sends them.
		calls = append(calls, teach(a+" "+b), teach(a+"\n"+b))
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
