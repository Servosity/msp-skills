// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the usage-text half of the MCP filesystem gate
// (handfixes.json zammad-mcp-filesystem-usage-gate). The press's name lists
// (blockedDestinationFlags, blockedInputFlags) and mcp:write-flags/read-flags
// cover the flags it knows about; this rule refuses, at request time, any other
// shell-out flag whose own description says it names a local file or
// directory, so a newly generated path flag is refused rather than forwarded.

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
// MCP tool whose usage text names a filesystem location and which the denylists
// above do not refuse. It walks exactly the command set RegisterAll registers,
// so endpoint and framework commands, which never become shell-out tools, stay
// out of scope: that is what keeps the usage-text rule from firing on generated
// vendor API body fields. A non-empty result means a tool caller can still
// direct a read or a write on the server account's filesystem.
func UnblockedFilesystemPathFlags(root *cobra.Command) []string {
	if root == nil {
		return nil
	}
	found := map[string]bool{}
	walk(root, nil, func(cmd *cobra.Command, path []string) {
		switch classify(cmd) {
		case commandHidden, commandEndpoint, commandGroup, commandFramework:
			// The same set RegisterAll skips (walker.go): a help-only parent
			// group never becomes a tool, so its flags are out of scope.
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
			if blocked[flag.Name] || blockedDestinationFlags[flag.Name] {
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

// filesystemPathFlagsForCommand returns the names of cmd's local and inherited
// flags that isFilesystemPathFlag classifies as local paths, minus the
// documented vendorAPIPathFlags exceptions. blockedStructuredArgsForCommand
// unions this set, so these flags are dropped from the tool schema and a
// tool call naming one is refused.
func filesystemPathFlagsForCommand(cmd *cobra.Command) map[string]bool {
	out := map[string]bool{}
	if cmd == nil {
		return out
	}
	check := func(flag *pflag.Flag) {
		if flag == nil || !isFilesystemPathFlag(flag) {
			return
		}
		if _, ok := vendorAPIPathFlags[cmd.CommandPath()+" --"+flag.Name]; ok {
			return
		}
		out[flag.Name] = true
	}
	cmd.NonInheritedFlags().VisitAll(check)
	cmd.InheritedFlags().VisitAll(check)
	return out
}
