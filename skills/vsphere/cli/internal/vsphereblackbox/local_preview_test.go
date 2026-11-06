// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"vsphere-pp-cli/internal/mcp/cobratree"
)

// homeTree is every file under the test home but tmp, with its content hash.
func homeTree(t *testing.T, home string) string {
	t.Helper()
	var rows []string
	err := filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p == filepath.Join(home, "tmp") {
				return filepath.SkipDir
			}
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rows = append(rows, fmt.Sprintf("%s %x", p, sha256.Sum256(b)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// The proof behind cobratree.LocalWriterDryRunPreviews: over the real MCP
// server and the real CLI, a local writer's unconfirmed call changes no file,
// on a home that already holds a synced store and a teach note, and sends
// nothing to vCenter. Every listed command must have a call here. The control
// proves the snapshot sees a write: the same call with confirm does change it.
func TestLocalWriterPreviewsChangeNothing(t *testing.T) {
	t.Parallel()
	calls := map[string]struct {
		tool string
		args map[string]any
	}{
		"export":            {"export", map[string]any{"resource": "vm", "limit": 5}},
		"sync":              {"sync", map[string]any{"resources": []any{"vm"}}},
		"workflow archive":  {"workflow_archive", map[string]any{"max-pages": 1}},
		"learnings confirm": {"learnings_confirm", map[string]any{"id": "1"}},
		"learnings forget":  {"learnings_forget", map[string]any{"query": "note", "all": true}},
		"learnings reject":  {"learnings_reject", map[string]any{"id": "1"}},
		"playbook amend":    {"playbook_amend", map[string]any{"select": "id"}},
		"teach":             {"teach", map[string]any{"select": "id"}},
		"teach-lookup":      {"teach_lookup", map[string]any{"select": "id"}},
		"teach-pattern":     {"teach_pattern", map[string]any{"select": "id"}},
		"teach-playbook":    {"teach_playbook", map[string]any{"select": "id"}},
	}
	for path := range cobratree.LocalWriterDryRunPreviews {
		if _, ok := calls[path]; !ok {
			t.Errorf("%q previews through --dry-run with no proof here", path)
		}
	}
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	e.run(t, "sync", "--resources", "vm")
	before, sent := homeTree(t, e.home), len(sv.requests())
	if !strings.Contains(before, ".db") {
		t.Fatalf("the seed sync left no store:\n%s", before)
	}
	for path := range cobratree.LocalWriterDryRunPreviews {
		c := calls[path]
		out := e.mcpCalls(t, []map[string]any{{"name": c.tool, "arguments": c.args}})
		if !strings.Contains(out, "Preview only") {
			t.Errorf("%s: no preview:\n%s", path, out)
		}
		if after := homeTree(t, e.home); after != before {
			t.Errorf("%s: the preview changed the home:\nbefore\n%s\nafter\n%s\noutput\n%s", path, before, after, out)
			before = after
		}
	}
	if got := sv.requests()[sent:]; len(got) != 0 {
		t.Errorf("a preview reached vCenter: %v", got)
	}

	// On a home with no store yet, a preview creates none.
	fresh := newEnv(t, sv, true)
	for path := range cobratree.LocalWriterDryRunPreviews {
		c := calls[path]
		fresh.mcpCalls(t, []map[string]any{{"name": c.tool, "arguments": c.args}})
		if got := homeTree(t, fresh.home); strings.Contains(got, ".db") {
			t.Errorf("%s: the preview created a store on a fresh home:\n%s", path, got)
		}
	}

	// Control: the same tool with confirm writes.
	e.mcpCalls(t, []map[string]any{{"name": "sync", "arguments": map[string]any{"resources": []any{"vm"}, "confirm": true}}})
	if homeTree(t, e.home) == before {
		t.Error("confirm: true changed nothing, so this snapshot cannot see a write")
	}
}

// The MCP server's stderr passes the output filter's pipe, so an exit right
// after a write lost the message: a refused HTTP start printed nothing (found
// on the granola print). Each refused start must say why: HTTP with no bearer
// token names the variable, an unknown transport names the choices.
func TestMCPExitKeepsItsStderr(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, true)
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"--transport", "http"}, 1, "VSPHERE_MCP_HTTP_TOKEN"},
		{[]string{"--transport", "bogus"}, 2, "supported: stdio, http"},
	} {
		cmd := exec.Command(bbMCP, c.args...)
		for _, kv := range e.env {
			if !strings.HasPrefix(kv, "VSPHERE_MCP_HTTP_TOKEN=") {
				cmd.Env = append(cmd.Env, kv)
			}
		}
		cmd.Dir = e.home
		var errb bytes.Buffer
		cmd.Stderr = &errb
		err := cmd.Run()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != c.code {
			t.Fatalf("%v: want exit %d, got %v (stderr %q)", c.args, c.code, err, errb.String())
		}
		if !strings.Contains(errb.String(), c.want) {
			t.Fatalf("%v: a refused start must say why, stderr: %q", c.args, errb.String())
		}
	}
}

// doctor's verdict words are not credentials: the output filter must not turn
// them into [redacted] (found on the granola print), in either format.
func TestDoctorShowsItsCredentialsVerdict(t *testing.T) {
	t.Parallel()
	e := newEnv(t, echoServer(t), true)
	var report map[string]any
	out := e.run(t, "doctor", "--json")
	if err := json.Unmarshal([]byte(out[:strings.LastIndex(out, "}")+1]), &report); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	verdict, _ := report["credentials"].(string)
	if verdict == "" || strings.Contains(verdict, "redacted") || strings.Contains(verdict, "withheld") {
		t.Errorf("doctor --json credentials = %q\n%s", verdict, out)
	}
	if text := e.run(t, "doctor"); strings.Contains(text, "[redacted]") || !strings.Contains(text, "Credentials:") {
		t.Errorf("doctor text:\n%s", text)
	}
}

// Every failing command prints its error once and after the output filter,
// whatever the failure: main printed a second, unfiltered copy on the granola
// print, so auth set-token <key> showed the key in full.
func TestCommandErrorPrintedOnceAfterTheFilter(t *testing.T) {
	t.Parallel()
	const pw = "ErrorPathPw-Q7n9x"
	sv := echoServer(t)
	for _, args := range [][]string{
		{"auth", "set-credentials", bbUser, pw, "extra"},
		{"auth", "set-credentials", bbUser, pw, "--nosuchflag"},
		{"auth", "set-token", pw},
		{"nosuchcommand", pw},
		{"vm", "get", "--nosuchflag=" + pw},
		{"--password=" + pw, "--nosuchflag"},
		{"doctor", "--password", pw, "--nosuchflag"},
	} {
		out := newEnv(t, sv, true).run(t, args...)
		if strings.Contains(out, pw) {
			t.Errorf("%v printed the value:\n%s", args, out)
		}
		// An error line that repeats the value is withheld whole, with one notice.
		if n := strings.Count(out, "Error:") + strings.Count(out, withheldNotice); n != 1 {
			t.Errorf("%v printed its error %d times:\n%s", args, n, out)
		}
	}
}
