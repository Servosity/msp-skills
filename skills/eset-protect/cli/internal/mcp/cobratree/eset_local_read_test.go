// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cli"
	"eset-protect-pp-cli/internal/cliutil"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestMCPRefusesLocalFileReads guards the hand-fix
// mcp-refuses-local-file-reads on the real command tree: no mirrored tool, in
// normal or read-only mode, offers a flag that opens a path on the MCP host,
// and a caller that sends one anyway is refused before the CLI runs. The
// companion CLI is an argv echo, so a call that got through would succeed and
// fail the refusal assertion.
func TestMCPRefusesLocalFileReads(t *testing.T) {
	bin := writeArgvHelper(t)
	dir := t.TempDir()
	jsonSecret := filepath.Join(dir, "claude_desktop_config.json")
	textSecret := filepath.Join(dir, "credentials")
	if err := os.WriteFile(jsonSecret, []byte(`{"env":{"ESET_PROTECT_PASSWORD":"JSON-SECRET-SENTINEL"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(textSecret, []byte("aws_secret_access_key = PLAIN-SECRET-SENTINEL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tool string
		args map[string]any
		flag string
	}{
		{"policies_decode", map[string]any{"file": jsonSecret}, "file"},
		{"teach_playbook", map[string]any{"query": "find stale devices", "notes-file": textSecret}, "notes-file"},
		{"teach_playbook", map[string]any{"query": "find stale devices", "playbook-file": jsonSecret}, "playbook-file"},
		{"teach", map[string]any{"query": "stale devices", "resource": "devices", "playbook-notes-file": textSecret}, "playbook-notes-file"},
		{"teach", map[string]any{"query": "stale devices", "resource": "devices", "playbook-file": jsonSecret}, "playbook-file"},
		{"fleet_health", map[string]any{"db": jsonSecret}, "db"},
	}
	for _, mode := range []string{"", "1"} {
		t.Run("read-only="+mode, func(t *testing.T) {
			t.Setenv(cliutil.MCPReadOnlyEnvVar, mode)
			s := server.NewMCPServer("test", "0.0.0")
			RegisterAll(s, cli.RootCmd(), func() (string, error) { return bin, nil })
			tools := s.ListTools()
			if len(tools) == 0 {
				t.Fatal("no mirrored tools registered")
			}
			for name, tool := range tools {
				for prop := range tool.Tool.InputSchema.Properties {
					if isLocalReadFlagName(prop) {
						t.Errorf("tool %s offers --%s, which opens a file on the MCP host", name, prop)
					}
				}
			}
			if _, ok := tools["import"]; ok {
				t.Error("import is offered over MCP, but it cannot run without --input, a local file")
			}
			for _, c := range cases {
				tool := tools[c.tool]
				if tool == nil {
					t.Fatalf("tool %s is not registered; the refusal cannot be checked", c.tool)
				}
				result, err := tool.Handler(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: c.args}})
				if err != nil {
					t.Fatalf("%s: transport error: %v", c.tool, err)
				}
				got := toolResultText(result)
				if result == nil || !result.IsError || !strings.Contains(got, "--"+c.flag+" over MCP") {
					t.Fatalf("%s --%s was not refused: isError=%v text=%q", c.tool, c.flag, result != nil && result.IsError, got)
				}
				if strings.Contains(got, bin) || strings.Contains(got, "SECRET-SENTINEL") {
					t.Fatalf("%s --%s reached the CLI: %q", c.tool, c.flag, got)
				}
			}
		})
	}
}

// mcpPathWordRE finds flag names and help text that talk about files, paths
// or databases.
var mcpPathWordRE = regexp.MustCompile(`(?i)\b(file|files|filename|path|paths|directory|dir|folder|database|sqlite|jsonl)\b`)

// notALocalPath lists flags whose name or help mentions a file or path but
// whose value never names a file on the MCP host. A new flag that matches
// mcpPathWordRE must be classified: refused by isLocalReadFlagName or a
// destination list, or added here with the reason it is not a local path.
var notALocalPath = map[string]string{
	"clean":            "respond scan: lets ESET clean infected files on the target device",
	"fail-on":          "doctor: health-gate selector",
	"filter-file-name": "quarantine filter sent to ESET, not opened locally",
	"format":           "export: output format name",
	"global-param":     "sync: key=value query parameter",
	"hash":             "executables find: a file hash value",
	"notes":            "teach-playbook: inline notes text",
	"param":            "sync: key=value query parameter",
	"path":             "policies decode: dotted path into the decoded JSON",
	"path-context":     "sync: {key} placeholder value for request paths",
	"playbook-json":    "teach: inline playbook JSON",
	"sha1":             "detections spread: a file hash value",
	"source-name":      "web-address-rules: data source name sent to ESET",
	"stdin":            "reads the request body from stdin, which is empty under MCP",
}

// TestEveryPathFlagIsClassifiedForMCP closes the class behind
// mcp-refuses-local-file-reads: it walks every command and fails on a flag or
// positional that looks like a local path but that neither the local-read rule
// nor a destination list refuses, so a reprint cannot add a new file reader to
// the MCP surface unnoticed.
func TestEveryPathFlagIsClassifiedForMCP(t *testing.T) {
	positionalPathRE := regexp.MustCompile(`(?i)[<\[][^>\]]*(file|path|dir|db)[^>\]]*[>\]]`)
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if m := positionalPathRE.FindString(cmd.Use); m != "" {
			t.Errorf("%s takes a path-like positional %s; refuse it over MCP or rename it", cmd.CommandPath(), m)
		}
		check := func(flag *pflag.Flag) {
			if flag == nil || !(mcpPathWordRE.MatchString(flag.Name) || mcpPathWordRE.MatchString(strings.ReplaceAll(flag.Name, "-", " ")) || mcpPathWordRE.MatchString(flag.Usage)) {
				return
			}
			if isLocalReadFlagName(flag.Name) || blockedDestinationFlags[flag.Name] || blockedRootFlags[flag.Name] || notALocalPath[flag.Name] != "" {
				return
			}
			t.Errorf("%s --%s mentions a file or path (%q) but is neither refused over MCP nor listed in notALocalPath", cmd.CommandPath(), flag.Name, flag.Usage)
		}
		cmd.NonInheritedFlags().VisitAll(check)
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(cli.RootCmd())
}
