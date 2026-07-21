// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

func newWorkspacesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspaces",
		Short: "List Granola workspaces",
	}
	cmd.AddCommand(newWorkspacesListCmd(flags))
	return cmd
}

func newWorkspacesListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workspaces from cache, falling back to live",
		Example: strings.Trim(`
  granola-cli workspaces list
  granola-cli workspaces list --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			c, err := openGranolaCache()
			if err == nil && len(c.Workspaces) > 0 {
				out := make([]map[string]any, 0, len(c.Workspaces))
				for _, w := range c.Workspaces {
					var inner map[string]any
					_ = json.Unmarshal(w.Workspace, &inner)
					out = append(out, map[string]any{
						"plan_type": w.PlanType,
						"role":      w.Role,
						"workspace": inner,
					})
				}
				return emitJSON(cmd, flags, out)
			}
			if flags.dataSource == "local" {
				// err is nil here (openGranolaCache soft-fails); construct a real
				// error rather than notFoundErr(nil), which would render <nil>.
				return notFoundErr(fmt.Errorf("no workspaces in the local store; run 'granola-cli sync' first"))
			}
			// The workspaces list lives only in Granola's sealed desktop store /
			// internal API (a decrypted pre-v7.4x cache would have populated
			// c.Workspaces above). On v7.4x+ it is unavailable to a third-party
			// binary; report that honestly instead of surfacing a raw decrypt error.
			ic, err := granola.NewInternalClient()
			if err != nil {
				return apiErr(fmt.Errorf("workspaces list unavailable: it lives only in Granola's internal API, sealed on Granola v7.4x+"))
			}
			ws, err := ic.GetWorkspaces()
			if err != nil {
				return apiErr(err)
			}
			return emitJSON(cmd, flags, ws)
		},
	}
	return cmd
}
