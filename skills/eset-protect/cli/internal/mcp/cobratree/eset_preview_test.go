// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cli"
	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestMirroredWritePreviewParsesOnlyAndWritesNothing guards the parse-only
// preview of mcp-mirror-write-confirm on the real command tree and the real
// companion CLI: a mirrored write called without confirm starts the CLI in
// parse-only mode, so its ordinary flags reach the CLI and a bad one is
// reported before the caller confirms, and nothing under the config, data,
// state or cache directories is created or changed. sync opens and migrates
// its store before it checks --dry-run, and learnings forget writes the learn
// store, so either one running would change the sandbox.
func TestMirroredWritePreviewParsesOnlyAndWritesNothing(t *testing.T) {
	pinMCPReadOnlyOff(t)
	// Built before the sandbox: go build writes its cache under HOME.
	bin := buildCompanionCLI(t)
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	// An inherited preview marker must not neuter a confirmed call.
	t.Setenv("ESET_PROTECT_MCP_CHILD", "preview")

	s := server.NewMCPServer("preview-test", "0.0.0")
	root := cli.RootCmd()
	RegisterAll(s, root, func() (string, error) { return bin, nil })
	RequireConfirmForMirroredWrites(s, root)
	call := func(name string, args map[string]any) *mcplib.CallToolResult {
		t.Helper()
		tool := s.GetTool(name)
		if tool == nil {
			t.Fatalf("tool %s is not registered", name)
		}
		req := mcplib.CallToolRequest{}
		req.Params.Name = name
		req.Params.Arguments = args
		res, err := tool.Handler(context.Background(), req)
		if err != nil || res == nil {
			t.Fatalf("%s: transport error %v", name, err)
		}
		return res
	}

	before := snapshotFiles(t, home)
	cases := []struct {
		tool string
		args map[string]any
		want []string
	}{
		{"sync", map[string]any{"since": "2h"}, []string{"eset-protect-cli sync", "--since=2h"}},
		{"learnings_forget", map[string]any{"query": "stale devices", "all": true}, []string{"eset-protect-cli learnings forget", "--all=true"}},
	}
	for _, c := range cases {
		res := call(c.tool, c.args)
		text := toolResultText(res)
		var preview map[string]any
		if res.IsError || json.Unmarshal([]byte(text), &preview) != nil || preview["confirmation_required"] != true {
			t.Fatalf("%s: want a confirmation preview, got isError=%v %q", c.tool, res.IsError, text)
		}
		parsed, _ := preview["parse_only"].(string)
		for _, w := range c.want {
			if !strings.Contains(parsed, w) {
				t.Errorf("%s: the CLI's parse-only output %q does not show %q; the ordinary flag did not reach the CLI", c.tool, parsed, w)
			}
		}
		if !strings.Contains(parsed, "nothing was run") {
			t.Errorf("%s: parse-only output %q does not say nothing was run", c.tool, parsed)
		}
	}
	// A value the CLI cannot parse is reported before the caller confirms.
	if res := call("sync", map[string]any{"max-pages": "not-a-number"}); !res.IsError || !strings.Contains(toolResultText(res), "max-pages") {
		t.Errorf("a preview with an unparsable --max-pages was not refused by the CLI: isError=%v %q", res.IsError, toolResultText(res))
	}
	if diff := diffSnapshots(before, snapshotFiles(t, home)); diff != "" {
		t.Fatalf("a preview created or changed local files:\n%s", diff)
	}

	// The other direction: confirmed, the same call runs and the walk sees
	// its write, so the empty diff above is not a blind walk. The inherited
	// preview marker set above is dropped for the confirmed child.
	res := call("learnings_forget", map[string]any{"query": "stale devices", "all": true, "confirm": true})
	if res.IsError || strings.Contains(toolResultText(res), "confirmation_required") {
		t.Fatalf("confirmed learnings_forget did not run: %q", toolResultText(res))
	}
	if diffSnapshots(before, snapshotFiles(t, home)) == "" {
		t.Fatal("confirmed learnings_forget wrote nothing the walk could see; the no-write proof above is blind")
	}
}

// buildCompanionCLI builds the real eset-protect-cli into a directory outside
// any sandbox.
func buildCompanionCLI(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the companion CLI")
	}
	bin := filepath.Join(t.TempDir(), "eset-protect-cli")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, "../../../cmd/eset-protect-cli").CombinedOutput(); err != nil {
		t.Fatalf("go build eset-protect-cli: %v\n%s", err, out)
	}
	return bin
}

// snapshotFiles records every entry under root: kind, mode, size and content
// hash, so a created, removed, truncated or rewritten file shows up.
func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := info.Mode().String()
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			entry += " " + hex.EncodeToString(sum[:])
		}
		snap[rel] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return snap
}

func diffSnapshots(before, after map[string]string) string {
	var lines []string
	for k, v := range after {
		if old, ok := before[k]; !ok {
			lines = append(lines, "created "+k)
		} else if old != v {
			lines = append(lines, "changed "+k)
		}
	}
	for k := range before {
		if _, ok := after[k]; !ok {
			lines = append(lines, "removed "+k)
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
