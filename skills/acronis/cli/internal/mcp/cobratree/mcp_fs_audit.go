// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/acronis/handfixes.json under the
// "mcp-filesystem-destination-gate" entry. Press 4.34.0 refuses filesystem flags
// at call time from name lists plus mcp:write-flags / mcp:read-flags annotations
// (blockedStructuredArgsForCommand). This file is the regeneration check on top:
// it classifies every mirrored flag by its own usage text, so a newly generated
// local-path flag that no list or annotation names fails the build
// (TestNoShellOutToolCanDirectAFilesystemPath) instead of becoming a read or
// write primitive for an MCP caller.

package cobratree

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// filesystemPathFlagPhrases are usage-string fragments that mark a flag as
// naming a filesystem location the server account would read or write.
//
// Matching the usage TEXT rather than the flag NAME is deliberate. The
// generated command surface carries API body fields called --path, --seed-path,
// --source-paths, --exclude-paths, --filename and --working-dir whose values
// are sent to the vendor API and never touch the server's own filesystem; a
// name-based rule would block those real API surfaces while still missing the
// next generated flag that does take a local path.
var filesystemPathFlagPhrases = []string{
	"file path",
	"path to ",
	"database path",
	"database file",
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
// disk, so forwarding them is correct. Keys use the "<command path> --<flag>"
// shape UnblockedFilesystemPathFlags reports; the value states why it is safe.
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
// RegisterAll mirrors, so endpoint, group and framework commands stay out of
// scope. A non-empty result means a tool caller can still direct a read or a
// write on the server account's filesystem.
func UnblockedFilesystemPathFlags(root *cobra.Command) []string {
	if root == nil {
		return nil
	}
	found := map[string]bool{}
	walk(root, nil, func(cmd *cobra.Command, path []string) {
		switch classify(cmd) {
		case commandHidden, commandEndpoint, commandGroup, commandFramework:
			return
		}
		if !cmd.Runnable() {
			return
		}
		blocked := blockedStructuredArgsForCommand(cmd)
		check := func(flag *pflag.Flag) {
			if flag == nil || flag.Hidden || flag.Deprecated != "" || blocked[flag.Name] {
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
