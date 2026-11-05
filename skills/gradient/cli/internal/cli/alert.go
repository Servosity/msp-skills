// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelAlertCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "alert",
		Short:       "Work with alert",
		Example:     "  gradient-cli alert send --account 123456789 --title 'Backup failure' --description 'Nightly job failed' --wait --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelAlertSendCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelAlertTraceCmd(flags))
	return cmd
}
