// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (novel file, not generated).
//
// Picks the read command `doctor` tells the operator to run to prove their token
// works end-to-end.
//
// The generated suggestReadCommand walks the tree and returns the FIRST leaf
// that looks like a read, which for ThreatLocker is `applications get`. That
// command requires --application-id, and the generator deliberately prints help
// instead of pflag's terse error on a bare invocation, so it exits 0 without
// ever touching the API. An operator follows doctor's own instruction, sees a
// clean exit, and concludes their credentials are fine.
//
// That is not hypothetical: it cost the reporter of #208 a diagnostic cycle
// while the real defect was elsewhere, and they filed it as #217.
//
// isSuggestableReadLeaf cannot tell the difference, because the requirement is
// enforced inside RunE rather than through cobra: there is no annotation, no
// MarkFlagRequired, and no positional in Use for it to see. Fixing that properly
// belongs in the generator, which would let every connector's doctor rank
// candidates instead of taking the first.
//
// Upstream tracker: finding 3 of mvanhorn/cli-printing-press#4482. It was
// originally item 7 of #4165, but that issue was closed as completed with this
// finding explicitly out of scope, so #4165 is NOT live - do not cite it.
// Re-verified against press v4.31.4: still reproduces (the guard now rejects
// required positionals and framework commands, but a required non-positional
// flag is still invisible to it). Until it lands, this connector names a
// command that actually runs.
//
// The candidates below take NO required input, so a bare call reaches the API.
// TestDoctorVerifyCandidatesAreRunnable proves that against a fixture rather
// than trusting this comment: it drives each candidate and asserts an HTTP
// request was issued.

package cli

import "github.com/spf13/cobra"

// doctorVerifyCandidates are read commands, in preference order, whose bare
// invocation issues a real API request. Ordered cheapest-first: organizations is
// a small collection on any tenant, computers can be large.
var doctorVerifyCandidates = []string{
	"organizations list",
	"computers list",
	"reports list",
}

// threatlockerVerifyCommand returns the first candidate that exists in the
// command tree, or "" when none do (a renamed command must not produce a
// suggestion that cannot run). The caller falls back to the generic
// "run any read command" message, which is strictly better than naming a
// command that silently exits 0.
func threatlockerVerifyCommand(root *cobra.Command) string {
	if root == nil {
		return ""
	}
	for _, candidate := range doctorVerifyCandidates {
		if commandPathExists(root, candidate) {
			return candidate
		}
	}
	return ""
}

// commandPathExists reports whether a space-separated command path resolves to a
// runnable leaf in the tree.
func commandPathExists(root *cobra.Command, path string) bool {
	cmd, _, err := root.Find(splitCommandPath(path))
	if err != nil || cmd == nil {
		return false
	}
	// Find falls back to the nearest parent when a segment is missing, so
	// confirm the resolved command really is the leaf we asked for.
	return cmd.Name() == lastSegment(path) && cmd.Runnable()
}

func splitCommandPath(path string) []string {
	var out []string
	start := -1
	for i := 0; i <= len(path); i++ {
		if i == len(path) || path[i] == ' ' {
			if start >= 0 {
				out = append(out, path[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return out
}

func lastSegment(path string) string {
	parts := splitCommandPath(path)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
