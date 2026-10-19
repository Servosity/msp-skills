// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelCmdbCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "cmdb",
		Short:       "Assets (CMDB) workflows: find the cloudId and workspaceId, list tickets linked to an object, reconcile against an RMM/PSA export",
		Example:     "  jira-service-management-cli cmdb discover --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelCmdbDiscoverCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelCmdbReconcileCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelCmdbTicketsCmd(flags))
	return cmd
}
