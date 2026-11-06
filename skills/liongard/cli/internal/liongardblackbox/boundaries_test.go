// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package liongardblackbox

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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// loosenFile lets other users read p on every platform: the config loader's
// loose-file check reads mode bits on POSIX and the DACL on Windows, where
// chmod changes neither. BUILTIN\Users is named by SID so the grant does not
// depend on the locale. A failed grant fails the test: a loose-file case that
// silently stays tight proves nothing.
func loosenFile(t *testing.T, p string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("icacls", p, "/grant", "*S-1-5-32-545:R").CombinedOutput(); err != nil {
			t.Fatalf("icacls grant on %s: %v\n%s", p, err, out)
		}
		return
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
}

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
// another Liongard host, so loading it prints a refusal that repeats its
// base_url. The URL's path repeats the environment API key and the session
// token, and its userinfo carries a password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvKey)
	cfg := filepath.Join(e.home, "loose.toml")
	base := "https://ro:" + bbStored + "@other.example.com/" + bbAPIKey + "/" + bbSession
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	loosenFile(t, cfg)
	e.env = append(withoutEnv(e.env, "LIONGARD_BASE_URL"), "LIONGARD_CONFIG="+cfg)
	cli := e.run(t, "version")
	mcp := e.mcpCalls(t, nil)
	for where, out := range map[string]string{"CLI startup": cli, "MCP startup": mcp} {
		// The refusal repeats the file's base_url, so the shared credential
		// kit withholds it whole: its notice is the refusal.
		if !strings.Contains(out, "refused") && !strings.Contains(out, kitNotice) {
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
	e := newEnv(t, sv, credEnvKey)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find environment", "resource-type": "environments", "resource": []any{bbEnvID},
			"notes": notes, "no-validate": true}}
	}
	read := func(id string) map[string]any {
		return map[string]any{"name": "liongard_read", "arguments": map[string]any{"endpoint_id": id}}
	}
	// The fetches answer first, so the server holds their credentials before
	// any teach call arrives (mcp-go serves one phase's calls concurrently).
	fetches := []map[string]any{
		read("environments.get"), // a gzip response credential
		read("launchpoints.get"), // a short response credential
		read("agents.get"),       // a BOM-prefixed response credential
	}
	calls := []map[string]any{teach("control learning note")}
	// Halves under 8 characters split by a space or a line are outside the
	// credential leak contract (section 6 item 6), so only whole values are
	// sent; the shared credential kit refuses to start a child whose argv
	// carries one.
	for _, v := range []string{bbGzipResp, bbShortResp, bbBOMResp} {
		calls = append(calls, teach("rotated "+v))
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
