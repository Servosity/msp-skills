// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"github.com/spf13/cobra"
)

func newNovelPatchCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "patch",
		Short:       "Windows patch posture rollups by client or site",
		Long:        "Roll pending Windows updates and reboots up to each client or site from the local store.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newNovelPatchPostureCmd(flags))
	return cmd
}
