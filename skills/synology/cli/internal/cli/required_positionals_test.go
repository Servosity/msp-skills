// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(required-positionals).

package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// positionalCommands walks the whole command tree (every command, mirrored
// to MCP or not) and returns the runnable ones whose Use declares a
// positional, with their path below the root.
func positionalCommands(t *testing.T) map[string]*cobra.Command {
	t.Helper()
	out := map[string]*cobra.Command{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		if c.Runnable() && c.HasParent() && usePositionalToken.MatchString(c.Use) {
			out[strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" ")] = c
		}
	}
	walk(RootCmd())
	return out
}

// TestMissingRequiredPositionalIsRefused: abb triage and abb device need a
// positional but declared it optional ("[task]", "[host-name]"), so their
// MCP tools did not require it and a call without it returned the command's
// help with isError false; across the CLI the same omission exited 0 (help
// for recall/learnings forget/search even with --json, a dry-run plan for
// abb triage/device, learnings confirm/reject and fleet add/remove) or 1
// (export, profile). It runs the real Execute (the binary's exit codes).
// The rule now, for every command: a positional the
// command needs is declared "<name>" (so the MCP schema requires it), and a
// call that omits it is refused with exit 2 naming it (a bare call prints
// the help, exit 0); --dry-run does not waive it. A command whose
// positionals are all optional answers a machine call without them.
func TestMissingRequiredPositionalIsRefused(t *testing.T) {
	sweepIsolate(t)
	cmds := positionalCommands(t)
	for _, want := range []string{"abb triage", "abb device", "export", "learnings confirm", "learnings reject",
		"learnings forget", "recall", "fleet add", "fleet remove"} {
		if c, ok := cmds[want]; !ok || len(requiredPositionals(c.Use)) == 0 {
			t.Fatalf("%s must declare its positional required (Use %q)", want, cmds[want].Use)
		}
	}
	required, optional := 0, 0
	for path, c := range cmds {
		argv := strings.Fields(path)
		need := requiredPositionals(c.Use)
		if len(need) == 0 {
			// Every positional optional: a machine call without one is not a
			// usage refusal (if it were, the positional is required and the
			// declaration is wrong).
			optional++
			if out, errText, code := sweepExecute(t, append(argv, "--json")...); code == 2 {
				t.Errorf("%s --json: exit 2 without its optional positional (%q): declare it <...> if the command needs it\n%s%s", path, c.Use, out, errText)
			}
			continue
		}
		required++
		for _, extra := range [][]string{{"--json"}, {"--agent"}, {"--dry-run", "--json"}} {
			out, errText, code := sweepExecute(t, append(append([]string{}, argv...), extra...)...)
			if code != 2 || (!strings.Contains(errText, need[0]) && !strings.Contains(errText, " arg(s), ")) || strings.Contains(out, "\"dry_run\"") {
				t.Errorf("%s %v without %s: exit %d, want 2 naming it\nstdout: %.200s\nstderr: %.300s", path, extra, need[0], code, out, errText)
			}
		}
		// Bare: the help (exit 0), or exit 2 where Cobra's own Args
		// validator refuses before any hook runs.
		out, errText, code := sweepExecute(t, argv...)
		if c.Args == nil && !(code == 0 && strings.Contains(out, "Usage:")) {
			t.Errorf("%s (bare): exit %d, want the help (exit 0)\n%.200s%.200s", path, code, out, errText)
		}
		if c.Args != nil && code != 2 {
			t.Errorf("%s (bare): exit %d, want a usage error (exit 2)\n%.200s%.200s", path, code, out, errText)
		}
		if _, errText, code := sweepExecute(t, append(append([]string{}, argv...), "  ", "--json")...); code != 2 {
			t.Errorf("%s with a blank positional: exit %d, want 2\n%.300s", path, code, errText)
		}
	}
	if required < 9 || optional < 3 {
		t.Fatalf("fixture: %d commands with a required positional, %d with only optional ones", required, optional)
	}
}

// TestRequiredPositionalsReadUseLikeTheMCPSchema: the CLI rule reads Use the
// way cobratree's typemap does.
func TestRequiredPositionalsReadUseLikeTheMCPSchema(t *testing.T) {
	for use, want := range map[string]string{
		"triage <task>":                      "<task>",
		"triage [task]":                      "",
		"export <resource> [id]":             "<resource>",
		"save <name> [--<flag> <value> ...]": "<name>",
		"get <slug> [<slug>...]":             "<slug>",
		"pair <a> <b>":                       "<a> <b>",
		"list":                               "",
		"run [<id>...]":                      "",
	} {
		if got := strings.Join(requiredPositionals(use), " "); got != want {
			t.Errorf("requiredPositionals(%q) = %q, want %q", use, got, want)
		}
	}
	for msg, want := range map[string]bool{
		"accepts 1 arg(s), received 0":                 true,
		"accepts between 1 and 2 arg(s), received 0":   true,
		"accepts at most 1 arg(s), received 2":         true,
		"requires at least 1 arg(s), only received 0":  true,
		"the NAS accepts 1 arg(s), received 0 somehow": false,
	} {
		if got := isCobraArgCountError(msg); got != want {
			t.Errorf("isCobraArgCountError(%q) = %v", msg, got)
		}
	}
}
