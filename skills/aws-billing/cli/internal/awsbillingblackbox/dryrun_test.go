// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// dry-run-sends-nothing: under --dry-run the SDK transport refuses every
// request (the AssumeRole a role profile would send first included) and the
// generated client sends nothing, from a cold start.

import (
	"fmt"
	"strings"
	"testing"
)

func TestDryRunSendsNothing(t *testing.T) {
	t.Parallel()
	for _, src := range []source{srcEnv, srcRole} {
		e := newEnv(t, src)
		var out strings.Builder
		for _, args := range [][]string{
			{"sync", "--dry-run"},
			{"sync", "--json", "--dry-run"},
			{"bill", "--dry-run"},
			{"bill", "--data-source", "live", "--dry-run"},
			{"compare", "--data-source", "live", "--dry-run"},
			{"consolidated", "--data-source", "live", "--dry-run"},
			{"forecast", "--dry-run"},
			{"dimensions", "SERVICE", "--dry-run"},
			{"accounts", "list", "--data-source", "live", "--dry-run"},
			{"waste", "rank", "--data-source", "live", "--dry-run"},
			{"report", "--period", "this-month", "--data-source", "live", "--post-slack", "--slack-channel", "C123", "--yes", "--dry-run"},
			{"export", "costs", "--dry-run"},
			{"workflow", "archive", "--dry-run"},
		} {
			o, code := e.runCode(t, "", bbCLI, args...)
			// A flag error would send nothing either: every run must parse.
			if strings.Contains(o, "unknown flag") || strings.Contains(o, "unknown command") || code == 2 {
				t.Errorf("%s %v did not run (exit %d):\n%.800s", src, args, code, o)
			}
			out.WriteString(o)
		}
		if !strings.Contains(out.String(), "would sync") {
			t.Errorf("%s: sync --dry-run printed no preview:\n%.2000s", src, out.String())
		}
		if got := e.aws.seen(); len(got) != 0 {
			t.Errorf("%s: --dry-run sent %v to AWS", src, got)
		}
		if got := e.api.seen(); len(got) != 0 {
			t.Errorf("%s: --dry-run sent %v through the generated client", src, got)
		}
		if runs := e.slackRuns(t); len(runs) != 0 {
			t.Errorf("%s: --dry-run started slack-pp-cli %d time(s)", src, len(runs))
		}
		assertNoEcho(t, fmt.Sprintf("%s dry-run output", src), out.String())
		// Control: the same reads without --dry-run reach the fakes.
		e.run(t, "bill", "--data-source", "live")
		e.run(t, "export", "costs")
		if e.aws.count("GetCostAndUsage") == 0 || len(e.api.seen()) == 0 {
			t.Errorf("%s control: the reads never reached the fakes: aws %v, api %v", src, e.aws.seen(), e.api.seen())
		}
		if src == srcRole && e.aws.count("AssumeRole") == 0 {
			t.Errorf("control: the role profile never assumed its role: %v", e.aws.seen())
		}
	}
}
