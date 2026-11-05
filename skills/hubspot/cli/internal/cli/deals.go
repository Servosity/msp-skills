// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelDealsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "deals",
		Short:       "Work with deals",
		Example:     "  hubspot-cli deals forecast --pipeline default --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelDealsForecastCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelDealsTopCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelDealsUnownedCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelDealsVelocityCmd(flags))
	return cmd
}
