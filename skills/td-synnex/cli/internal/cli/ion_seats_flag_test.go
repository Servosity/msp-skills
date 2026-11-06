// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: subscriptions set --seats is parsed as a decimal through the real flag.

package cli

import "testing"

func TestSubscriptionsSetSeatsFlagIsDecimal(t *testing.T) {
	cmd := newSubscriptionsSetCmd(&rootFlags{})
	if err := cmd.ParseFlags([]string{"--seats", "012"}); err != nil {
		t.Fatalf("--seats 012: %v", err)
	}
	if got := cmd.Flags().Lookup("seats").Value.String(); got != "12" {
		t.Fatalf("--seats 012 parsed as %s, want 12", got)
	}
	if !cmd.Flags().Changed("seats") {
		t.Fatal("--seats not marked changed")
	}
	for _, bad := range []string{"0x0C", "0b1100", "1_2"} {
		c := newSubscriptionsSetCmd(&rootFlags{})
		if err := c.ParseFlags([]string{"--seats", bad}); err == nil {
			t.Fatalf("--seats %s must be refused", bad)
		}
	}
}
