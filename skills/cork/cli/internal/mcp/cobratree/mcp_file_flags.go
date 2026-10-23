// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cork/handfixes.json under
// the "mcp-file-flags-refused" entry, and checked fleet-wide by
// tools/maintainer/check_mcp_file_flags.py.

package cobratree

import (
	"strings"

	"github.com/spf13/cobra"
)

// FlagWriteSinksAnnotation lists the Cobra flags of a command whose values
// name a file or directory on the machine that runs the MCP server (a file the
// command reads, writes or walks). The MCP server runs the companion CLI as
// the account that hosts it, so a tool call that could set one of these could
// make that account touch any path it can reach. blockedStructuredArgsForCommand
// adds them to the command's blocked set, which removes them from the tool's
// input schema and makes validateMCPArgumentNames refuse a call that supplies
// one anyway; the CLI keeps them. An ancestor's annotation covers a persistent flag that ancestor
// supplies. Separate names with commas, semicolons or whitespace; a leading
// "--" is ignored. It is the same annotation cli-printing-press 4.30 and later
// honour, so a reprint keeps the command-side declarations working.
const FlagWriteSinksAnnotation = "mcp:write-flags"

// DestinationFlagBlocked reports whether an MCP tool call is refused when it
// sets flagName on cmd.
func DestinationFlagBlocked(cmd *cobra.Command, flagName string) bool {
	return blockedStructuredArgsForCommand(cmd)[flagName]
}

// flagWriteSinkNames is the set of flags cmd's mcp:write-flags annotations
// declare, including an ancestor's declaration for a persistent flag that
// ancestor supplies to cmd.
func flagWriteSinkNames(cmd *cobra.Command) map[string]bool {
	if cmd == nil {
		return nil
	}
	out := map[string]bool{}
	for name := range writeSinkAnnotationNames(cmd.Annotations) {
		out[name] = true
	}
	for parent := cmd.Parent(); parent != nil; parent = parent.Parent() {
		for name := range writeSinkAnnotationNames(parent.Annotations) {
			if cmd.NonInheritedFlags().Lookup(name) == nil && cmd.InheritedFlags().Lookup(name) != nil && persistentSupplier(cmd, name) == parent {
				out[name] = true
			}
		}
	}
	return out
}

func writeSinkAnnotationNames(annotations map[string]string) map[string]bool {
	raw := strings.TrimSpace(annotations[FlagWriteSinksAnnotation])
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
