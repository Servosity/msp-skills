// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelAppsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "apps",
		Short:       "Enterprise applications (service principals) and their consent grants",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelAppsConsentCmd(flags))
	return cmd
}
