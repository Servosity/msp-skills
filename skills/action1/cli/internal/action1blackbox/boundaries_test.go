// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package action1blackbox

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
// another Action1 host, so loading it prints a refusal that repeats its base_url.
// The URL's path repeats the environment credential (the static ACTION1_OAUTH2
// token, or the client secret of the pair), which the refused route never
// applies, and its userinfo carries a password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX mode bits: Windows has no group/other or read-only mode bits for a loose config file")
	}
	for _, mode := range []credMode{credEnvToken, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			cfg := filepath.Join(e.home, "loose.toml")
			base := "https://ro:" + bbStored + "@other.example.com/" + mode.held()
			if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(cfg, 0o644); err != nil {
				t.Fatal(err)
			}
			e.env = append(withoutEnv(e.env, "ACTION1_BASE_URL"), "ACTION1_CONFIG="+cfg)
			cli := e.run(t, "version")
			mcp := e.mcpCalls(t, nil)
			// The refusal repeats the held values, so the kit withholds that line
			// whole: the CLI writes the kit's stderr notice in its place; the MCP
			// server, the same config load, may drop the line with no notice (the
			// kit drops a notice it cannot place whole), so the CLI is the proof
			// the case ran and both are scanned (rocketcyber precedent).
			if !strings.Contains(cli, "refused") && !strings.Contains(cli, kitNotice("stderr")) {
				t.Fatalf("CLI startup printed no refusal for the loose config; the test did not run:\n%.1500s", cli)
			}
			for where, out := range map[string]string{"CLI startup": cli, "MCP startup": mcp} {
				assertNoEcho(t, where, out)
			}
		})
	}
}

// Class held-value-crosses-process-boundary: the MCP server holds the
// credentials responses carry and the token it minted for the client pair;
// teach is a mirrored tool whose CLI child writes its notes to the local store
// and loads none of those values itself (under ACTION1_NO_CONFIG_WRITE the
// child cannot even read the minted token from disk), so the child must get the
// held set (credential leak contract section 1: a child process gets the held
// set or is not started). A control call proves the child really writes.
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credPair)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find organization", "resource-type": "organizations", "resource": []any{bbOrg},
			"notes": notes, "no-validate": true}}
	}
	read := func(id string) map[string]any {
		return map[string]any{"name": "action1_read", "arguments": map[string]any{"endpoint_id": id}}
	}
	// The fetches answer first, so the server holds their credentials before
	// any teach call arrives (mcp-go serves one phase's calls concurrently).
	fetches := []map[string]any{
		read("organizations.get"), // the minted token pair, and a gzip response credential
		read("enterprise.get"),    // a short response credential
		read("me.me"),             // a BOM-prefixed response credential
	}
	calls := []map[string]any{teach("control learning note"), teach("rotated " + bbShortResp)}
	for _, v := range []string{bbMintAccess, bbMintRefresh, bbGzipResp, bbBOMResp} {
		a, b := halves(v)
		// Whole, and split across a space and across a line: each half is an
		// 8-character piece of the held value (contract section 2(b)). The
		// short credential goes whole only: its halves are pieces shorter than
		// 8 characters, an advisory case (contract section 6, item 6).
		calls = append(calls, teach("rotated "+v), teach(a+" "+b), teach(a+"\n"+b))
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
