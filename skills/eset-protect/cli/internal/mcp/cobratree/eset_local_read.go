// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cobratree

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FlagReadSourcesAnnotation lists Cobra flag names whose value names a file
// on the host that the command opens and reads. It is the read-side twin of
// FlagWriteSinksAnnotation, for a path flag whose name the rule in
// isLocalReadFlagName does not already catch. Same list syntax.
const FlagReadSourcesAnnotation = "mcp:read-flags"

// localReadFlagNames are the flag names on this CLI that open a path on the
// machine the MCP server runs on. The MCP server runs commands as its own
// account, so forwarding one lets a caller read any file that account can
// read: policies decode --file returns a JSON file verbatim, teach-playbook
// --notes-file and teach --playbook-notes-file copy any file into the
// learning store that playbook list and recall return, import --input reads a
// file and posts it, and --db/--legacy-db/--legacy-config/--config open an
// arbitrary SQLite or config file instead of the CLI's own. Refused in every
// MCP mode (handfixes.json: mcp-refuses-local-file-reads).
var localReadFlagNames = map[string]bool{
	"config":              true,
	"db":                  true,
	"file":                true,
	"home":                true,
	"input":               true,
	"legacy-config":       true,
	"legacy-db":           true,
	"notes-file":          true,
	"playbook-file":       true,
	"playbook-notes-file": true,
}

// localReadFlagSuffixes catch the naming pattern of a path flag, so a
// reprint that adds --foo-file or --bar-db is refused without an edit here.
var localReadFlagSuffixes = []string{"-file", "-db", "-dir", "-config"}

func isLocalReadFlagName(name string) bool {
	if localReadFlagNames[name] {
		return true
	}
	for _, suffix := range localReadFlagSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// localReadFlags returns every flag cmd accepts (its own and inherited) that
// opens a local path, by name rule or by FlagReadSourcesAnnotation on cmd or
// an ancestor that supplies the inherited flag.
func localReadFlags(cmd *cobra.Command) map[string]bool {
	out := map[string]bool{}
	if cmd == nil {
		return out
	}
	visit := func(flag *pflag.Flag) {
		if flag != nil && isLocalReadFlagName(flag.Name) {
			out[flag.Name] = true
		}
	}
	cmd.NonInheritedFlags().VisitAll(visit)
	cmd.InheritedFlags().VisitAll(visit)
	for name := range annotationNamedFlags(cmd.Annotations, FlagReadSourcesAnnotation) {
		out[name] = true
	}
	for parent := cmd.Parent(); parent != nil; parent = parent.Parent() {
		for name := range annotationNamedFlags(parent.Annotations, FlagReadSourcesAnnotation) {
			if inheritedWriteSink(cmd, name) && persistentFlagSupplier(cmd, name) == parent {
				out[name] = true
			}
		}
	}
	return out
}

// localReadRefusal names the first local-path flag present in args. The
// walker already drops these flags from the tool schema; this refuses a
// caller that sends one anyway, before the CLI is spawned, in every mode.
func localReadRefusal(cmd *cobra.Command, args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	names := make([]string, 0, len(args))
	for name := range args {
		names = append(names, name)
	}
	sort.Strings(names)
	read := localReadFlags(cmd)
	for _, name := range names {
		// A write destination the generic gate already rejects keeps its
		// own message; an explicitly listed reader (--db) gets this one.
		if !read[name] || (!localReadFlagNames[name] && DestinationFlagBlocked(cmd, name)) {
			continue
		}
		path := "command"
		if cmd != nil {
			path = cmd.CommandPath()
		}
		return fmt.Sprintf("refusing %s --%s over MCP: it names a file on the MCP host, and MCP tools do not read local files; run the command in a terminal instead", path, name)
	}
	return ""
}

// requiresLocalReadFlag reports whether cmd marks a local-path flag
// required, so the tool could only ever fail once that flag is refused.
func requiresLocalReadFlag(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	required := false
	read := localReadFlags(cmd)
	visit := func(flag *pflag.Flag) {
		if flag != nil && read[flag.Name] && isRequired(flag) {
			required = true
		}
	}
	cmd.NonInheritedFlags().VisitAll(visit)
	cmd.InheritedFlags().VisitAll(visit)
	return required
}
