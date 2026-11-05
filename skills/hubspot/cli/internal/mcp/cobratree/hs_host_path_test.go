// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"

	"hubspot-pp-cli/internal/cli"
	"hubspot-pp-cli/internal/mcp/cobratree"
)

// hostPathPhrases are help-text fragments that say a flag's value names a
// file or directory on the machine running the command (the phrase list the
// 4.32.5 hand gate proved on this connector). Matching help text, not flag
// names, keeps generated vendor body fields such as --path or --filename,
// whose values go to HubSpot, out of the rule.
var hostPathPhrases = []string{
	"file path",
	"path to ",
	"database path",
	"output directory",
	"audit directory",
	"directory path",
	"store path",
	"mirror path",
	"receipt destination",
	"root directory for",
}

func namesHostPath(usage string) bool {
	usage = strings.ToLower(usage)
	for _, p := range hostPathPhrases {
		if strings.Contains(usage, p) {
			return true
		}
	}
	return false
}

// vendorPathFlags are flags whose help trips namesHostPath but whose value is
// never opened on this machine. Empty today; add a name only with the reason.
var vendorPathFlags = map[string]string{}

// TestNoMirroredToolTakesAHostPath walks every shell-out tool the MCP server
// registers and fails when a tool still accepts a flag whose help says it
// names a local file or directory. The MCP server runs as the account that
// hosts it, so such a flag lets a tool call read or write any file that
// account can reach. The rule is the flag's own help text, not its name, so a
// novel command that grows a new path flag fails here until it declares the
// flag with mcp:read-flags or mcp:write-flags.
func TestNoMirroredToolTakesAHostPath(t *testing.T) {
	root := cli.RootCmd()
	s := server.NewMCPServer("t", "0")
	cobratree.RegisterAll(s, root, func() (string, error) { return "/nonexistent/hubspot-cli", nil })
	tools := s.ListTools()
	if len(tools) == 0 {
		t.Fatal("RegisterAll registered no shell-out tools; the walk is not exercising anything")
	}
	var bad []string
	for name, st := range tools {
		path, _ := st.Tool.Meta.AdditionalFields["pp:cli-command"].(string)
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil || cmd == nil {
			t.Fatalf("tool %s: cannot resolve command %q: %v", name, path, err)
		}
		for prop := range st.Tool.InputSchema.Properties {
			f := cmd.Flags().Lookup(prop)
			if f == nil || f.Value.Type() == "bool" {
				continue
			}
			if _, ok := vendorPathFlags[prop]; ok {
				continue
			}
			if namesHostPath(f.Usage) {
				bad = append(bad, name+" --"+prop+" ("+f.Usage+")")
			}
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("MCP tool still accepts a host-path flag: %s", b)
	}
}
