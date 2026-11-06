// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

// Credential-rule classes the fleet Codex reviews found, each proven on the built
// binaries:
//   - output-before-chokepoint: the configuration and the credential store
//     print warnings while they load; credkit.Start is the first statement of
//     both mains, so every startup source is held before anything prints
//     (CLI and MCP);
//   - held-value-crosses-process-boundary: a mirrored MCP tool runs the CLI
//     as a child process (credkit.RunCommand), so a value only the MCP server
//     holds never reaches a child through an argument.

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
// another host, so loading it prints a refusal that repeats its base_url. The
// URL's path repeats the two environment credentials and the MCP HTTP token,
// and its userinfo carries a password nothing else holds.
func TestNothingPrintsBeforeTheChokepoint(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnvZoneUser)
	cfg := filepath.Join(e.home, "loose.toml")
	base := "https://ro:" + bbStored + "@other.example.com/" + bbCode + "/" + bbHTTPToken + "/" + bbSecret
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("base_url = %q\n", base)), 0o600); err != nil {
		t.Fatal(err)
	}
	loosen(t, cfg)
	// Control: without the loose config, startup withholds nothing.
	if out := e.run(t, "version"); strings.Contains(out, "credkit: withheld") {
		t.Fatalf("version withheld a line with no loose config:\n%.1500s", out)
	}
	e.env = append(withoutEnv(e.env, "AUTOTASK_BASE_URL"), "AUTOTASK_CONFIG="+cfg)
	cli := e.run(t, "doctor")
	mcp := e.mcpCalls(t, []map[string]any{{"name": "autotask_read", "arguments": map[string]any{"endpoint_id": "tickets.query-item", "params": map[string]any{"id": bbTicketID}}}})
	for where, out := range map[string]string{"CLI startup": cli, "MCP startup": mcp} {
		// The refusal quotes base_url, whose every value is held, so it is
		// withheld whole: the kit's notice in its place.
		if !strings.Contains(out, "refused") && !strings.Contains(out, "not applied") && !strings.Contains(out, "credkit: withheld") {
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
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credEnv)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find ticket", "resource-type": "tickets", "resource": []any{bbTicketID},
			"notes": notes, "no-validate": true}}
	}
	filter := []any{map[string]any{"op": "gte", "field": "id", "value": 0}}
	read := func(id string) map[string]any {
		return map[string]any{"name": "autotask_read", "arguments": map[string]any{"endpoint_id": id, "params": map[string]any{"filter": filter}}}
	}
	// The fetches answer first, so the server holds their credentials before
	// any teach call arrives (mcp-go serves one phase's calls concurrently).
	fetches := []map[string]any{
		read("tickets.query"),             // a gzip response secretKey
		read("contacts.query"),            // a short client portal password
		read("companies.query"),           // a BOM-prefixed integration vendor secret
		read("configuration-items.query"), // a webhook secretKey
	}
	// The control teaches its own query, so a later teach of the same query
	// cannot replace its note.
	control := teach("control learning note")
	control["arguments"].(map[string]any)["query"] = "control query"
	calls := []map[string]any{control}
	for _, v := range []string{bbGzipResp, bbShortResp, bbBOMResp, bbWebhookKey} {
		calls = append(calls, teach("rotated "+v))
		if len(v) < 16 {
			// A value under 8 characters is matched as a whole token only
			// (contract 2(b)), and a half under 8 is not a piece the kit
			// matches: kit finding short-value-split-across-tokens.
			continue
		}
		a, b := halves(v)
		// Split across a space and across a line, as splitObject sends them.
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
