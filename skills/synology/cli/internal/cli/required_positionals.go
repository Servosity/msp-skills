// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. One rule for a missing required positional, read from the
// same declaration the MCP schema is built from. HAND-FIX(required-positionals).

package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// A command declares its positionals once, in Use: "<task>" is required,
// "[id]" is optional. The MCP mirror (cobratree's typemap) marks every
// "<...>" positional required in the tool's input schema, so the server's
// required-argument guard refuses a tool call without it; this file makes
// the CLI refuse the same call, so a command that needs a positional is
// never answered with exit 0 (its help, or a dry-run plan) when it is
// missing.
//
// The rule, for every command whose Use declares a required positional
// that the call leaves missing or blank:
//   - a bare call (no arguments, no flags) prints the command's help, exit 0;
//   - any other call (--json, --agent, --dry-run, any flag) is a usage error,
//     exit 2, naming the missing positional; nothing runs. --dry-run does not
//     waive it (only the Printing Press verify/dogfood harness does, as for a
//     missing required flag).
//
// It runs before every other pre-run step (profile, delivery, store and
// tenant-gate setup), so a refused call has no side effect.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		prev := root.PersistentPreRunE
		root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := refuseMissingPositionals(cmd, flags, args); err != nil {
				return err
			}
			if prev != nil {
				return prev(cmd, args)
			}
			return nil
		}
	})
}

// usePositionalToken is cobratree's positionalTokenPattern: the MCP schema
// and this rule read Use the same way.
var usePositionalToken = regexp.MustCompile(`(?:^|\s)(<[^>]+>|\[[^\]]+\])`)

// requiredPositionals lists, in order, the positionals use declares
// required ("<name>"), once each; a token holding "--" (a flag template such
// as "[--<flag> <value> ...]") is not a positional.
func requiredPositionals(use string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range usePositionalToken.FindAllStringSubmatch(use, -1) {
		raw := strings.TrimSpace(m[1])
		if strings.Contains(raw, "--") {
			continue
		}
		name := strings.NewReplacer("<", "", ">", "", "[", "", "]", "").Replace(raw)
		name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "..."))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if strings.HasPrefix(raw, "<") {
			out = append(out, "<"+name+">")
		}
	}
	return out
}

func refuseMissingPositionals(cmd *cobra.Command, flags *rootFlags, args []string) error {
	required := requiredPositionals(cmd.Use)
	given := 0
	for given < len(required) && given < len(args) && strings.TrimSpace(args[given]) != "" {
		given++
	}
	if given == len(required) {
		return nil
	}
	if len(args) == 0 && cmd.Flags().NFlag() == 0 {
		return pflag.ErrHelp // a bare call: Cobra prints the help, exit 0
	}
	if flags != nil && flags.dryRun && requiredInputWaived() {
		return nil
	}
	return usageErr(fmt.Errorf("%s requires %s (missing or empty); nothing was run (see '%s --help')",
		cmd.CommandPath(), strings.Join(required[given:], " "), cmd.CommandPath()))
}

// cobraArgCount matches Cobra's own positional-count refusals (ExactArgs,
// RangeArgs, MinimumNArgs, MaximumNArgs), which reach Execute before any
// RunE: they are usage errors (exit 2) like every other missing positional.
var cobraArgCount = regexp.MustCompile(`^(accepts (at most |between \d+ and )?\d+ arg\(s\), received \d+|requires at least \d+ arg\(s\), only received \d+)$`)

func isCobraArgCountError(msg string) bool {
	return cobraArgCount.MatchString(msg)
}
