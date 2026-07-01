// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Opt-in Corey-Quinn-flavored voice layer. Deterministic static phrase bank
// keyed to billing signals — no LLM call, suppressed under --json, off by
// default. Enable with --snark.
//
// pp:novel-static-reference
package cli

import "fmt"

// snarkLine returns a deterministic quip for a named signal, or "" if snark is
// off. idx selects among variants deterministically (callers pass a stable
// index so output is reproducible).
func snarkLine(on bool, signal string, idx int) string {
	if !on {
		return ""
	}
	bank := map[string][]string{
		"intro": {
			"Let's see what the cloud charged you for the privilege of existing this month.",
			"Pour one out for your credit card. Here's the damage.",
			"The meter never stops. Neither does my commentary.",
		},
		"big-jump": {
			"Your bill went up. Somewhere a us-east-1 load balancer is doing just fine, thanks for asking.",
			"That spike isn't a glitch — that's AWS quietly compounding. Congratulations.",
			"Bill's climbing. Have you tried turning the data transfer off and on again? (You can't. That's the joke.)",
		},
		"transfer": {
			"Data transfer: the line item AWS hopes you never read. You read it.",
			"Cross-AZ traffic — paying a toll to move bytes ten feet. Cloud economics, baby.",
			"NAT Gateway: it's like a toll booth, except the toll is your dignity and $0.045/GB.",
		},
		"idle": {
			"This instance has been awake and doing absolutely nothing, billed by the second. Relatable.",
			"An idle instance is just a very expensive space heater you can't see.",
			"Running at 2% CPU. That's not a workload, that's a houseplant with an EBS volume.",
		},
		"orphan": {
			"An orphaned snapshot: a tiny monument to a volume that no longer loves you, still on the bill.",
			"Unattached EBS — storage for data nobody asked for, billed forever. Very on-brand.",
			"This Elastic IP is associated with nothing and costs you anyway. The cloud's purest grift.",
		},
		"clean": {
			"No obvious waste found. Either you're disciplined or you haven't deployed anything yet.",
			"Clean scan. Suspicious. Run it again next month when the chaos returns.",
		},
		"savings": {
			"Here's money you're currently setting on fire. You're welcome.",
			"Found some waste. AWS won't tell you about this — that's my job.",
		},
	}
	variants, ok := bank[signal]
	if !ok || len(variants) == 0 {
		return ""
	}
	if idx < 0 {
		idx = 0
	}
	return variants[idx%len(variants)]
}

// snarkf prints a snark line to a string suitable for human output sections.
func snarkf(on bool, signal string, idx int) string {
	if s := snarkLine(on, signal, idx); s != "" {
		return fmt.Sprintf("\n  💸 %s\n", s)
	}
	return ""
}
