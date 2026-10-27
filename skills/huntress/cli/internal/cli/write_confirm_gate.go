// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the CLI write-confirmation gate, pinned by
// skills/huntress/handfixes.json under "cli-writes-require-yes" and by
// write_confirm_gate_test.go. The root PersistentPreRunE in root.go calls
// enforceWriteConfirmation once for every command, so the gate lives in one
// place and covers every generated endpoint command, including ones a future
// reprint adds.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// writeConfirmReadPOSTPaths is the explicit allowlist of pp:path values whose
// POST only reads: the request returns data and changes nothing in the
// tenant. It is the only way an endpoint command with a write method skips the
// gate. Exemption is decided by this exact path, never by a command's name or
// verb.
var writeConfirmReadPOSTPaths = map[string]bool{
	// siem: runs an ESQL query against the SIEM logs and returns the rows.
	"/v1/siem/query": true,
}

// writeConfirmCommands lists the commands without a pp:endpoint annotation
// that still send state-changing requests to the Huntress API, keyed by the
// command path below the root, with what they send.
var writeConfirmCommands = map[string]string{
	// import: one POST to the resource's create path for every input record.
	"import": "a POST for every record in its input",
}

func init() {
	// Help examples are rendered without running PersistentPreRunE, so they
	// are brought in line with the gate when the tree is built.
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		confirmWriteExamples(root)
	})
}

// writeConfirmTarget reports whether cmd changes the Huntress tenant and so
// needs --yes or --dry-run, and describes what it would send. Any endpoint
// method other than a read method is a write, including a missing or unknown
// one: the gate fails closed.
func writeConfirmTarget(cmd *cobra.Command) (string, bool) {
	if cmd == nil {
		return "", false
	}
	path := strings.Fields(cmd.CommandPath())
	if len(path) > 1 {
		if what, ok := writeConfirmCommands[strings.Join(path[1:], " ")]; ok {
			return what, true
		}
	}
	if strings.TrimSpace(cmd.Annotations["pp:endpoint"]) == "" {
		return "", false
	}
	method := strings.ToUpper(strings.TrimSpace(cmd.Annotations["pp:method"]))
	apiPath := strings.TrimSpace(cmd.Annotations["pp:path"])
	switch method {
	case "GET", "HEAD", "OPTIONS":
		return "", false
	case "POST":
		if writeConfirmReadPOSTPaths[apiPath] {
			return "", false
		}
	case "":
		method = "a request with no declared method"
	}
	return strings.TrimSpace(method + " " + apiPath), true
}

// writeConfirmationGiven: --dry-run, typed or from a run profile, previews and
// sends nothing. --yes counts only when typed on this command line, so a run
// profile saved with --yes cannot pre-approve every later write.
func writeConfirmationGiven(flags *rootFlags) bool {
	if flags.dryRun {
		return true
	}
	if !flags.yes {
		return false
	}
	_, fromProfile := profileAppliedFlagNames["yes"]
	return !fromProfile
}

// enforceWriteConfirmation refuses a command that changes the Huntress tenant
// unless the caller passed --yes (send it) or --dry-run (preview it). It runs
// from the root PersistentPreRunE, before any API client exists, so a refused
// command sends nothing. The refusal is a usage error (exit 2); --json and
// --agent callers also get a JSON envelope on stdout, the same shape the
// endpoint commands use for their other usage errors.
func enforceWriteConfirmation(cmd *cobra.Command, flags *rootFlags) error {
	if flags == nil {
		return nil
	}
	what, gated := writeConfirmTarget(cmd)
	if !gated || writeConfirmationGiven(flags) {
		return nil
	}
	err := fmt.Errorf("refusing to run %q without confirmation: it sends %s to the Huntress API and changes your tenant; re-run with --dry-run to preview the request, or with --yes to send it", cmd.CommandPath(), what)
	jsonChanged := cmd.Flags().Changed("json")
	if flags.asJSON || (flags.agent && !jsonChanged) {
		// The refusal must reach the caller whole, so --select and --compact
		// do not filter it.
		printFlags := *flags
		printFlags.asJSON = true
		printFlags.compact = false
		printFlags.selectFields = ""
		usage := endpointUsageLine(cmd)
		_ = printJSONFiltered(cmd.OutOrStdout(), map[string]any{
			"error":                 funnelShownError(err),
			"usage":                 usage + " --yes",
			"preview":               usage + " --dry-run",
			"confirmation_required": true,
		}, &printFlags)
	}
	return usageErr(err)
}

// confirmWriteExamples rewrites the help example of every gated command so it
// never shows an invocation the gate would refuse. An example line without
// --yes or --dry-run becomes a --dry-run preview followed by the --yes run, or
// just the --yes run when the example already shows a preview.
func confirmWriteExamples(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		if _, gated := writeConfirmTarget(child); gated && strings.TrimSpace(child.Example) != "" {
			child.Example = withWriteConfirmation(child.Example, child.Root().Name())
		}
		confirmWriteExamples(child)
	}
}

func withWriteConfirmation(example, rootName string) string {
	lines := strings.Split(example, "\n")
	showsPreview := false
	for _, line := range lines {
		if lineHasFlag(line, "--dry-run") {
			showsPreview = true
		}
	}
	out := make([]string, 0, 2*len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.Contains(line, rootName+" ") ||
			lineHasFlag(line, "--yes") || lineHasFlag(line, "--dry-run") {
			out = append(out, line)
			continue
		}
		base := strings.TrimRight(line, " \t")
		if !showsPreview {
			out = append(out, base+" --dry-run")
		}
		out = append(out, base+" --yes")
	}
	return strings.Join(out, "\n")
}

func lineHasFlag(line, flag string) bool {
	for _, field := range strings.Fields(line) {
		if field == flag || field == flag+"=true" {
			return true
		}
	}
	return false
}
