// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/blumira/handfixes.json under
// the "mcp-file-flags-refused" entry, and checked fleet-wide by
// tools/maintainer/check_mcp_file_flags.py.

package cobratree

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FlagWriteSinksAnnotation lists the Cobra flags of a command whose values
// name a file or directory on the machine that runs the MCP server (a file the
// command reads, writes or walks). The MCP server runs the companion CLI as
// the account that hosts it, so a tool call that could set one of these could
// make that account touch any path it can reach. The walker removes them from
// the tool's input schema and refuses a call that supplies one anyway; the CLI
// keeps them. An ancestor's annotation covers a persistent flag that ancestor
// supplies. Separate names with commas, semicolons or whitespace; a leading
// "--" is ignored. It is the same annotation cli-printing-press 4.30 and later
// honour, so a reprint keeps the command-side declarations working.
const FlagWriteSinksAnnotation = "mcp:write-flags"

// InlineOnlyFlagsAnnotation names flags that take either inline JSON or a
// local file (a path, @path, or - for stdin). Over MCP the inline form is the
// useful one, so the flag stays in the tool's schema, but a value that is not
// an inline JSON object is refused by name: anything else would make the
// server account read a file of the caller's choosing. The same annotation is
// honoured by connectors printed on later presses.
const InlineOnlyFlagsAnnotation = "mcp:inline-only-flags"

// mcpPositionalArgsField is the MCP argument that carries positional
// arguments. It is never a flag, even where a denylist names it.
const mcpPositionalArgsField = "args"

// blockedFlagsForCommand is every flag name an MCP caller may not set on cmd:
// the unambiguous destination names and the command's own (and inherited)
// mcp:write-flags declarations (always), plus the root control-plane flags
// EXCEPT where the command redefines one of those names as a local flag. A
// command-local flag named client / token / config is an int filter or an API
// body field, a different flag from the root one, so it stays available; the
// root flags are blocked only where they are inherited.
func blockedFlagsForCommand(cmd *cobra.Command) map[string]bool {
	out := map[string]bool{}
	local := localFlagNames(cmd)
	for name := range blockedRootFlags {
		if name != mcpPositionalArgsField && !local[name] {
			out[name] = true
		}
	}
	for name := range blockedDestinationFlags {
		out[name] = true
	}
	for name := range flagWriteSinkNames(cmd) {
		out[name] = true
	}
	return out
}

// DestinationFlagBlocked reports whether an MCP tool call is refused when it
// sets flagName on cmd.
func DestinationFlagBlocked(cmd *cobra.Command, flagName string) bool {
	return blockedFlagsForCommand(cmd)[flagName]
}

// localFlagNames is the set of cmd's own (non-inherited) flag names.
func localFlagNames(cmd *cobra.Command) map[string]bool {
	out := map[string]bool{}
	if cmd == nil {
		return out
	}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) { out[f.Name] = true })
	cmd.NonInheritedFlags().VisitAll(func(f *pflag.Flag) { out[f.Name] = true })
	return out
}

// declaredMCPFlagNames is every non-hidden flag cmd advertises as an MCP tool
// parameter (local and inherited), the exact set toolOptionsForFlags offers
// before the blocked ones are removed. A key outside this set is one the tool
// never declared.
func declaredMCPFlagNames(cmd *cobra.Command) map[string]bool {
	out := map[string]bool{}
	if cmd == nil {
		return out
	}
	add := func(f *pflag.Flag) {
		if f != nil && !f.Hidden && f.Deprecated == "" {
			out[f.Name] = true
		}
	}
	cmd.InheritedFlags().VisitAll(add)
	cmd.NonInheritedFlags().VisitAll(add)
	return out
}

// inlineOnlyFlagsForCommand is the set of cmd's mcp:inline-only-flags.
func inlineOnlyFlagsForCommand(cmd *cobra.Command) map[string]bool {
	if cmd == nil {
		return nil
	}
	return writeSinkAnnotationNamesFor(cmd.Annotations, InlineOnlyFlagsAnnotation)
}

// refuseMCPArgs returns the refusal for a tool call that supplies an argument
// the tool does not accept, or "" when the call is fine. Three cases, each
// naming the offending key so the caller is not left guessing:
//   - an inline-only flag (fleet push --body, raw --vars) whose value is not an
//     inline JSON object: a path would make the server read a local file;
//   - a declared flag that is blocked (a filesystem destination, or a root
//     control-plane flag): refused, never dropped, because a silent drop ran
//     the command without the input the caller asked for and returned an
//     answer that looked complete;
//   - a key the tool never declared (an argument carrying "=", or a flag that
//     belongs to a different command and would otherwise be forwarded verbatim
//     by the shell-out layer, reaching a subcommand through the args field).
func refuseMCPArgs(args map[string]any, declared, blocked, inlineOnly map[string]bool, takesArgs bool) string {
	var inline []string
	for name := range inlineOnly {
		v, ok := args[name]
		if !ok || v == nil || blocked[name] {
			continue
		}
		if s, isString := v.(string); !isString || !strings.HasPrefix(strings.TrimSpace(s), "{") {
			inline = append(inline, name)
		}
	}
	if len(inline) > 0 {
		sort.Strings(inline)
		return fmt.Sprintf("refusing --%s: over MCP it accepts only an inline JSON object encoded as a string; a file path, @path or stdin would make the MCP server read a file on its own machine", inline[0])
	}
	var blockedNames, unknownNames []string
	for k := range args {
		if k == mcpPositionalArgsField {
			if takesArgs {
				continue
			}
			unknownNames = append(unknownNames, k)
			continue
		}
		switch {
		case strings.Contains(k, "="):
			unknownNames = append(unknownNames, k)
		case blocked[k]:
			blockedNames = append(blockedNames, k)
		case !declared[k]:
			unknownNames = append(unknownNames, k)
		}
	}
	if len(blockedNames) > 0 {
		sort.Strings(blockedNames)
		quoted := make([]string, len(blockedNames))
		for i, n := range blockedNames {
			quoted[i] = "--" + n
		}
		return fmt.Sprintf("refusing %s: not accepted over MCP. It names a file or directory on the machine running the MCP server, or changes where the CLI sends requests or finds its configuration and credentials, so only the operator can set it, on the command line.", strings.Join(quoted, ", "))
	}
	if len(unknownNames) > 0 {
		sort.Strings(unknownNames)
		quoted := make([]string, len(unknownNames))
		for i, n := range unknownNames {
			quoted[i] = "--" + n
		}
		return fmt.Sprintf("refusing %s: this command does not accept it over MCP; use the tool schema's named parameters. A flag the schema does not list is dropped rather than forwarded to the companion CLI.", strings.Join(quoted, ", "))
	}
	return ""
}

// flagWriteSinkNames is the set of flags cmd's mcp:write-flags annotations
// declare, including an ancestor's declaration for a persistent flag that
// ancestor supplies to cmd.
func flagWriteSinkNames(cmd *cobra.Command) map[string]bool {
	if cmd == nil {
		return nil
	}
	out := map[string]bool{}
	for name := range writeSinkAnnotationNamesFor(cmd.Annotations, FlagWriteSinksAnnotation) {
		out[name] = true
	}
	for parent := cmd.Parent(); parent != nil; parent = parent.Parent() {
		for name := range writeSinkAnnotationNamesFor(parent.Annotations, FlagWriteSinksAnnotation) {
			if cmd.NonInheritedFlags().Lookup(name) == nil && cmd.InheritedFlags().Lookup(name) != nil && persistentSupplier(cmd, name) == parent {
				out[name] = true
			}
		}
	}
	return out
}

// writeSinkAnnotationNamesFor parses the flag-name list stored under key.
func writeSinkAnnotationNamesFor(annotations map[string]string, key string) map[string]bool {
	raw := strings.TrimSpace(annotations[key])
	if raw == "" {
		return nil
	}
	out := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	}) {
		if name := strings.TrimLeft(strings.TrimSpace(part), "-"); name != "" {
			out[name] = true
		}
	}
	return out
}

// persistentSupplier is the nearest ancestor whose persistent flags define
// name; Cobra keeps that flag and ignores a more distant one.
func persistentSupplier(cmd *cobra.Command, name string) *cobra.Command {
	for parent := cmd.Parent(); parent != nil; parent = parent.Parent() {
		if parent.PersistentFlags().Lookup(name) != nil {
			return parent
		}
	}
	return nil
}
