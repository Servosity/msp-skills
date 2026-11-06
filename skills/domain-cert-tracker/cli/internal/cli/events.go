// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel parent (handfixes.json tracker-novels).
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelEventsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "events",
		Short:       "Typed change events that each sweep records, read with a cursor",
		Example:     "  domain-cert-tracker-cli events list --since 0 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelEventsListCmd(flags))
	return cmd
}
