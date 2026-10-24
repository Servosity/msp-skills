// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelBillingCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "billing",
		Short:       "Work with billing",
		Example:     "  cove-cli billing changes --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2", "pp:requires-tier": "credentials"}, // the Example runs a child that signs in
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelBillingChangesCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelBillingUsageCmd(flags))
	return cmd
}
