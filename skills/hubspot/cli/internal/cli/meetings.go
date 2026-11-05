// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelMeetingsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "meetings",
		Short:       "Property history & audit",
		Example:     "  hubspot-cli meetings ever-had --property hs_meeting_outcome --value SCHEDULED --from 2026-04-01 --to 2026-04-30 --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelMeetingsEverHadCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelMeetingsHistoryCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelMeetingsStatusReportCmd(flags))
	return cmd
}
