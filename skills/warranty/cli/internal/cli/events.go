// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel parent (handfixes.json warranty-novels).
// pp:data-source local

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelEventsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "events",
		Short:       "Coverage events that lookups record, read with a cursor",
		Example:     "  warranty-cli events list --since 0 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelEventsListCmd(flags))
	return cmd
}
