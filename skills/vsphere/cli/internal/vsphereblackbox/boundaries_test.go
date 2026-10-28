// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

// Classes the vsphere Codex review round 1 found, each proven on the built
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
// another vCenter, so loading it prints a refusal that repeats its base_url.
// The URL's path repeats the environment password and its userinfo carries a
// password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("a loose config is a Unix mode-bit refusal; Windows has no group/other bits")
	}
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	cfg := filepath.Join(e.home, "loose.toml")
	base := "https://ro:" + bbStored + "@other.example.com/" + bbPassword
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "VSPHERE_BASE_URL"), "VSPHERE_CONFIG="+cfg)
	cli := e.run(t, "version")
	mcp := e.mcpCalls(t, nil)
	if !strings.Contains(cli, withheldNotice) {
		t.Fatalf("the CLI printed no notice for the loose config's refusal; the test did not run:\n%.1500s", cli)
	}
	assertNoEcho(t, "CLI startup", cli)
	assertNoEcho(t, "MCP startup", mcp) // the server reads the config on its first tool call
}

// Class held-value-crosses-process-boundary: the MCP server mints both
// session ids and holds the credentials responses carry; teach is a mirrored
// tool whose CLI child writes its notes to the local store and holds none of
// those values. A control call proves the child really writes.
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find application", "resource-type": "vm", "resource": []any{"vm-1021"},
			"notes": notes, "no-validate": true, "confirm": true}}
	}
	none := map[string]any{}
	calls := []map[string]any{
		{"name": "vm_list", "arguments": none},                                     // the Automation session
		{"name": "vi_vm_snapshot", "arguments": map[string]any{"moId": "vm-1021"}}, // the VI/JSON session
		teach("control learning note"),
	}
	calls = append(calls, teach("rotated "+bbShortResp)) // too short for pieces (contract 2)
	for _, v := range []string{bbAPISess, bbVISess, bbGzipResp} {
		a, b := halves(v)
		// Whole, and split across a space and across a line.
		calls = append(calls, teach("rotated "+v), teach(a+" "+b), teach(a+"\n"+b))
	}
	out := e.mcpCalls(t, calls)
	if !strings.Contains(out, fmt.Sprintf(`"id":%d`, len(calls))) {
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
