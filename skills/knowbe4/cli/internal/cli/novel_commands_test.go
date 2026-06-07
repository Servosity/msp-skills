// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Smoke tests for the hand-built transcendence + User Event commands. Behavioral
// correctness of the analytics lives in internal/insights; these assert the
// commands construct, expose their flags, and short-circuit cleanly under
// --dry-run (the verify contract).

package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestNovelCommandsDryRunExitZero(t *testing.T) {
	flags := &rootFlags{dryRun: true}
	builders := []func(*rootFlags) *cobra.Command{
		newNovelRepeatClickersCmd,
		newNovelUntrainedClickersCmd,
		newNovelCoverageGapsCmd,
		newNovelRiskLeaderboardCmd,
		newNovelRiskDriftCmd,
		newNovelPhishProneTrendCmd,
		newNovelGroupRiskContributionCmd,
		newNovelQbrCmd,
	}
	for _, b := range builders {
		c := b(flags)
		c.SetArgs(nil)
		var buf bytes.Buffer
		c.SetOut(&buf)
		c.SetErr(&buf)
		if err := c.Execute(); err != nil {
			t.Errorf("%s dry-run returned error: %v", c.Name(), err)
		}
	}
}

func TestEventsParentBuilds(t *testing.T) {
	c := newEventsCmd(&rootFlags{})
	if c.Name() != "events" {
		t.Fatalf("events command name = %q", c.Name())
	}
	if len(c.Commands()) < 7 {
		t.Fatalf("events should have at least 7 subcommands, got %d", len(c.Commands()))
	}
}

func TestEventsCreateDryRunNoKey(t *testing.T) {
	// create --dry-run must short-circuit before requiring the API key.
	flags := &rootFlags{dryRun: true}
	c := newEventsCreateCmd(flags)
	c.SetArgs(nil)
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetErr(&buf)
	if err := c.Execute(); err != nil {
		t.Errorf("events create --dry-run returned error: %v", err)
	}
}
