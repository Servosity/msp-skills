// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelTicketCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "ticket",
		Short:       "Work with ticket",
		Example:     "  zammad-cli ticket note 12345 --body 'Investigated, awaiting logs' --internal --dry-run",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelTicketNoteCmd(flags))
	return cmd
}
