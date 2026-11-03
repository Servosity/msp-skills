// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/auvik/handfixes.json under
// "mcp-filesystem-destination-gate". Press 4.35.1 refuses the named
// destination and input flags natively (blockedDestinationFlags,
// blockedInputFlags, mcp:write-flags, mcp:read-flags); this file keeps the
// usage-text detector that fails the build when a regeneration grows a
// local-path flag none of those name.

package cobratree

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// filesystemPathFlagPhrases are the usage-text phrases that mark a flag whose
// value names a location on the machine running the MCP server.
//
// Matching the usage TEXT rather than the flag NAME is deliberate. The
// generated command surface carries API body fields called --path, --seed-path,
// --source-paths, --exclude-paths, --filename and --working-dir whose values
// are sent to the vendor API and never touch the server's own filesystem; a
// name-based rule would block those real API surfaces while still missing the
// next generated flag that does take a local path. The generated body fields
// describe themselves with a bare Title-case noun ("Path", "Seed path"), while
// a genuine local-filesystem flag spells out that it takes a file or directory
// path.
var filesystemPathFlagPhrases = []string{
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

// vendorAPIPathFlags are flags whose usage text trips isFilesystemPathFlag but
// which name a location in the VENDOR's world rather than on this account's
// disk, so forwarding them is correct. Keys use the same "<command path>
// --<flag>" shape UnblockedFilesystemPathFlags reports, and the value states why
// the flag is safe. Listing the exceptions here instead of loosening the phrase
// list keeps a newly generated local-path flag failing the build.
var vendorAPIPathFlags = map[string]string{}

// isFilesystemPathFlag reports whether flag's own description says it names a
// filesystem location.
func isFilesystemPathFlag(flag *pflag.Flag) bool {
	if flag == nil {
		return false
	}
	usage := strings.ToLower(flag.Usage)
	for _, phrase := range filesystemPathFlagPhrases {
		if strings.Contains(usage, phrase) {
			return true
		}
	}
	return false
}

// UnblockedFilesystemPathFlags reports every flag reachable through a shell-out
// MCP tool whose usage text names a filesystem location and which the
// per-command blocked set does not refuse. It walks exactly the command set
// RegisterAll registers, so endpoint and framework commands, which never become
// shell-out tools, stay out of scope: that is what keeps the usage-text rule
// from firing on generated vendor API body fields. A non-empty result means a
// tool caller can still direct a read or a write on the server account's
// filesystem.
func UnblockedFilesystemPathFlags(root *cobra.Command) []string {
	if root == nil {
		return nil
	}
	found := map[string]bool{}
	walk(root, nil, func(cmd *cobra.Command, path []string) {
		switch classify(cmd) {
		case commandHidden, commandEndpoint, commandFramework:
			return
		}
		if !cmd.Runnable() {
			return
		}
		blocked := blockedStructuredArgsForCommand(cmd)
		check := func(flag *pflag.Flag) {
			if flag == nil || flag.Hidden || flag.Deprecated != "" {
				return
			}
			if blocked[flag.Name] || blockedDestinationFlags[flag.Name] || blockedInputFlags[flag.Name] || blockedRootFlags[flag.Name] {
				return
			}
			if isFilesystemPathFlag(flag) {
				key := cmd.CommandPath() + " --" + flag.Name
				if _, ok := vendorAPIPathFlags[key]; ok {
					return
				}
				found[key] = true
			}
		}
		cmd.NonInheritedFlags().VisitAll(check)
		cmd.InheritedFlags().VisitAll(check)
	})
	out := make([]string, 0, len(found))
	for name := range found {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// extraPathFlagFloor carries the shipped floor names press 4.35.1 does not list
// in blockedDestinationFlags or blockedInputFlags. None of them is a flag on
// this CLI today; they stay so a regenerated command that grows one cannot
// take a host path from a tool argument.
var extraPathFlagFloor = map[string]bool{
	"from-csv":  true,
	"home":      true,
	"reconcile": true,
}

// mcpPathFlagFloor reports whether name is refused at forward time on every
// command, whatever per-command blocked set the caller passed: the union of the
// press's unambiguous destination and input names and the shipped floor.
func mcpPathFlagFloor(name string) bool {
	return blockedDestinationFlags[name] || blockedInputFlags[name] || extraPathFlagFloor[name]
}
