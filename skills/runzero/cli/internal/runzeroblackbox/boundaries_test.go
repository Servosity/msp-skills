// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

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
	"net/url"
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
// URL's path repeats the environment's key (RUNZERO_API_KEY, bare and as its
// "Bearer <key>" header value) and the MCP HTTP token, and its userinfo
// carries a password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the loose-config path needs POSIX group/other mode bits, which Windows does not have")
	}
	t.Parallel() // its own fake runZero and home
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	cfg := filepath.Join(e.home, "loose.toml")
	base := "https://ro:" + bbFileAccess + "@other.example.com/" + bbKey + "/" + url.PathEscape(bearer(bbKey)) +
		"/Bearer%20" + bbKey + "/" + bbHTTPToken
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	e.env = append(withoutEnv(e.env, "RUNZERO_BASE_URL"), "RUNZERO_CONFIG="+cfg)
	cli := e.run(t, "version")
	// The MCP server loads its configuration on the first call (the
	// credential kit holds the file at startup without printing anything).
	mcp := e.mcpCalls(t, []map[string]any{read("org.get-sites", nil)})
	for where, out := range map[string]string{"CLI startup": cli, "MCP startup": mcp} {
		// The refusal repeats held values, so the credential kit withholds
		// it whole: its notice is the refusal having printed.
		if !strings.Contains(out, "refused") && !strings.Contains(out, "not applied") && !strings.Contains(out, kitNotice("stderr")) {
			t.Fatalf("%s printed no refusal for the loose config; the test did not run:\n%.1500s", where, out)
		}
		assertNoEcho(t, where, out)
	}
}

// Class held-value-crosses-process-boundary: the MCP server holds the
// credentials responses carry (an API key's token, an organization's export
// and download tokens, a user's password in a gzip-encoded list, a site's
// token in a BOM-prefixed one); teach is a mirrored tool whose CLI child
// writes its notes to the local store and holds none of those values. A
// control call proves the child really writes.
func TestMCPChildNeverReceivesAHeldValue(t *testing.T) {
	t.Parallel() // its own fake runZero and home
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find asset", "resource-type": "org", "resource": []any{bbAssetID},
			"notes": notes, "no-validate": true}}
	}
	// The fetches answer first, so the server holds their credentials before
	// any teach call arrives (mcp-go serves one phase's calls concurrently).
	fetches := []map[string]any{
		read("account.get-keys", nil),          // an API key's token
		read("account.get-organizations", nil), // export_token and download_token, gzip-encoded
		read("account.get-users", nil),         // a short password in a gzip list
		read("org.get-sites", nil),             // a token in a BOM-prefixed list
	}
	calls := []map[string]any{teach("control learning note")}
	for _, v := range []string{bbKeyToken, bbExportTok, bbDownloadTok, bbShortResp, bbBOMResp} {
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
		// The control call's own answer first: the reason it wrote nothing
		// (the session output is credential-free, assertNoEcho above).
		var control string
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, fmt.Sprintf(`"id":%d,`, len(fetches)+1)) {
				control = line
			}
		}
		t.Fatalf("the control teach call wrote no learning; the test did not run:\ncontrol answer: %.1500s\n%.2000s", control, out)
	}
}
