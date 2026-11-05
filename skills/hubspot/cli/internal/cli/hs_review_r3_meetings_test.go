// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Adversarial review (publish lane, 2026-10-24): HubSpot stores meeting
// outcomes in upper case, so the documented `--value Scheduled` must match.

import (
	"strings"
	"testing"
	"time"

	"hubspot-pp-cli/internal/cliutil/testenv"
)

func TestR3EverHadValueMatchesAnyCase(t *testing.T) {
	testenv.Isolate(t)
	s, path := r1Store(t)
	r2WriterHistory(t, s, "meetings", "m1", "hs_meeting_outcome", "SCHEDULED", time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	s.Close()
	for _, v := range []string{"Scheduled", "SCHEDULED", "scheduled"} {
		out, err := r1Run(t, "", "meetings", "ever-had", "--property", "hs_meeting_outcome", "--value", v, "--from", "2026-04-01", "--to", "2026-04-30", "--db", path, "--json")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, `"m1"`) {
			t.Fatalf("--value %s missed the SCHEDULED meeting: %s", v, out)
		}
	}
}
