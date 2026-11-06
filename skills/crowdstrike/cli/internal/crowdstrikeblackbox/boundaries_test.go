// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package crowdstrikeblackbox

// Credential-rule classes the fleet Codex reviews found, each proven on the built
// binaries:
//   - output-before-chokepoint: the configuration and the credential store
//     print warnings while they load, so credkit.Start runs first in both
//     mains and the kit delivers nothing until every startup source is held;
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
// another CrowdStrike host, so loading it prints a refusal that repeats its base_url.
// The URL's path repeats the environment credential (the client secret of the
// pair, with or without a stored token beside it), which the refused route
// never applies, and its userinfo carries a password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("needs POSIX mode bits: Windows has no group/other or read-only mode bits for a loose config file")
	}
	for _, mode := range []credMode{credPairOverStored, credPair} {
		t.Run(mode.String(), func(t *testing.T) {
			sv := echoServer(t)
			e := newEnv(t, sv, mode)
			cfg := filepath.Join(e.home, "loose.toml")
			base := "https://ro:" + bbArgvSec + "@other.example.com/" + mode.held()
			if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(cfg, 0o644); err != nil {
				t.Fatal(err)
			}
			e.env = append(withoutEnv(e.env, "CROWDSTRIKE_BASE_URL"), "CROWDSTRIKE_CONFIG="+cfg)
			cli := e.run(t, "version")
			mcp := e.mcpCalls(t, nil)
			// The refusal repeats the held base_url, so it arrives as the kit's notice.
			if !strings.Contains(cli, withheldNotice) && !strings.Contains(cli, "refused") {
				t.Fatalf("the CLI printed no notice for the loose config's refusal; the test did not run:\n%.1500s", cli)
			}
			assertNoEcho(t, "CLI startup", cli)
			assertNoEcho(t, "MCP startup", mcp) // the server reads the config on its first tool call
		})
	}
}

// Class held-value-crosses-process-boundary: the MCP server holds the
// credentials responses carry and the token it minted for the client pair;
// teach is a mirrored tool whose CLI child writes its notes to the local store
// and holds none of those values (under CROWDSTRIKE_NO_CONFIG_WRITE the child
// cannot even read the minted token from disk). A control call proves the
// child really writes.
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credPair)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find host", "resource-type": "devices", "resource": []any{bbHost},
			"notes": notes, "no-validate": true}}
	}
	read := func(id string, params map[string]any) map[string]any {
		args := map[string]any{"endpoint_id": id}
		if params != nil {
			args["params"] = params
		}
		return map[string]any{"name": "crowdstrike_read", "arguments": args}
	}
	// The fetches answer first, so the server holds their credentials before
	// any teach call arrives (mcp-go serves one phase's calls concurrently).
	fetches := []map[string]any{
		read("devices.combined-by-filter", nil),                                 // the minted token, and a gzip response credential
		read("policy.get-prevention-policies", map[string]any{"ids": bbPolicy}), // a short response credential
		read("devices.get-online-state-v1", map[string]any{"ids": bbHost}),      // a BOM-prefixed response credential
	}
	calls := []map[string]any{teach("control learning note")}
	calls = append(calls, teach("rotated "+bbShortResp)) // too short for pieces (contract 2)
	for _, v := range []string{bbMintAccess, bbGzipResp, bbBOMResp} {
		a, b := halves(v)
		// Whole, and split across a space and across a line.
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
