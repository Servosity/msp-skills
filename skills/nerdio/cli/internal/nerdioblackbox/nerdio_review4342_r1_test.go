// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package nerdioblackbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex review r1 (4.34.2 reprint), class tenant secret written to disk: the
// MCP server refuses a child argv that repeats a tenant secret it showed, but
// a secret shown by a mirrored CLI child (the `export secure-variables` tool)
// was registered only in that child, so the server never knew it and the
// next tool call (`teach --notes <the value>`) wrote it into the learnings
// store. A tenant secret is now shown only by a process whose registry the
// child-argv check reads: a mirrored child holds every tenant secret it reads
// as a credential, so the credential kit withholds the unit that carries one
// from its stdout (the MCP server's in-process reads, nerdio_read, still show
// them and register them), and an argv repeating one the server showed is
// refused.
// After the session no file under the home holds a tenant secret.
func TestMCPChildShowsNoTenantSecretAndNoneReachesAFile(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	teach := func(notes string) map[string]any {
		return map[string]any{"name": "teach", "arguments": map[string]any{
			"query": "find variable", "resource": []any{"1"}, "resource-type": "secure-variables",
			"notes": notes, "no-validate": true}}
	}
	// What the agent was shown by the mirrored export tool, repeated as the
	// notes of the next call (the failing input: the server did not know the
	// child had shown a tenant secret, so it could not refuse the repeat).
	shown := mcpReplies(e.mcpCalls(t, []map[string]any{{"name": "export", "arguments": map[string]any{"resource": "secure-variables"}}}))[1]
	if !strings.Contains(shown, "DomainJoinPassword") && !strings.Contains(shown, withheldNotice) {
		t.Fatalf("the export tool did not return the secure variables list; the test did not run:\n%.1500s", shown)
	}
	if strings.Contains(shown, bbSecureVar) {
		t.Errorf("a mirrored CLI child showed a tenant secret the MCP server cannot register:\n%.1500s", shown)
	}
	out := e.mcpPhases(t,
		[]map[string]any{teach("from the export: " + shown)},
		[]map[string]any{{"name": "nerdio_read", "arguments": map[string]any{"endpoint_id": "secure-variables.account-list",
			"params": map[string]any{"account_id": 101}}}},
		[]map[string]any{teach("rotate " + bbAcctSecVar + " next week")},
	)
	replies := mcpReplies(out)
	if !strings.Contains(replies[2], bbAcctSecVar) {
		t.Errorf("nerdio_read (in-process) did not show the secure variable the agent asked for:\n%.1500s", replies[2])
	}
	if !strings.Contains(replies[3], "refused") {
		t.Errorf("a child argv repeating a tenant secret the server showed was not refused:\n%.1500s", replies[3])
	}
	var scanned int
	_ = filepath.WalkDir(e.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		scanned++
		rel, _ := filepath.Rel(e.home, path)
		if s := holdsTenantSecret(string(data)); s != "" {
			t.Errorf("%s holds the tenant secret %q after the MCP session", rel, s)
		}
		return nil
	})
	if scanned < 2 {
		t.Fatalf("only %d files under the home; the session did not run", scanned)
	}
}
