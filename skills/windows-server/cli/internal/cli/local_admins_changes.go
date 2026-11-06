// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `local-admins changes`. Recorded in
// handfixes.json (fixed-scripts).
// pp:data-source live
// pp:client-call the API call is a fixed WinRM script run through winsrv.Runner, not the HTTP client.

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"windows-server-pp-cli/internal/winsrv"
)

func newNovelLocalAdminsChangesCmd(flags *rootFlags) *cobra.Command {
	var hours int
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Who added or removed each local Administrators member, when, and whether they are still a member.",
		Long: `Reads Security events 4732 (member added) and 4733 (member removed) for the
builtin Administrators group in the last --hours on every configured server
(or the one --server names), with who made the change and whether the
member is still in the group now.

These events are written only when the "Security Group Management" audit
subcategory logs Success. When it does not, the server gets a row with a
warning instead of a silent empty list, and so does a read that hit its cap.`,
		Example: "  windows-server-cli local-admins changes --hours 168 --agent",
		Args:    cobra.NoArgs,
		Annotations: map[string]string{
			"mcp:read-only": "true", "pp:data-source": "live",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if hours < 1 || hours > 720 {
				return usageErr(fmt.Errorf("--hours must be from 1 to 720"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "local-admins changes on "+winsrvTarget()+"")
			}
			profiles, credErrs, err := winsrvServers(flags, true)
			if err != nil {
				return err
			}
			runner := newWinsrvRunner(flags)
			results := eachServer(cmd.Context(), profiles, credErrs, func(ctx context.Context, p winsrv.Profile) (any, error) {
				raw, err := runner.Run(ctx, p, "local_admin_changes", map[string]any{"hours": hours})
				if err != nil {
					return nil, err
				}
				res, err := decodeRow(raw)
				if err != nil {
					return nil, err
				}
				return adminChangeRows(p.Name, res), nil
			})
			rows := []winsrv.Row{}
			for _, r := range results {
				if r.err != nil {
					rows = append(rows, errorRow(r.profile.Name, r.err))
					continue
				}
				rows = append(rows, r.value.([]winsrv.Row)...)
			}
			if err := printWinsrv(cmd, flags, rows, "server", "action", "member_name", "changed_by", "still_member", "warning", "error"); err != nil {
				return err
			}
			return scriptExit(allFailed(results))
		},
	}
	cmd.Flags().IntVar(&hours, "hours", 168, "How far back to look, in hours (1 to 720)")
	return cmd
}

func adminChangeRows(server string, res winsrv.Row) []winsrv.Row {
	members := map[string]bool{}
	if list, ok := res["member_sids"].([]any); ok {
		for _, s := range list {
			members[winsrv.Str(s)] = true
		}
	}
	out := []winsrv.Row{}
	changes, _ := res["changes"].([]any)
	switch audit := winsrv.Str(res["audit_policy"]); {
	case audit == "" && len(changes) == 0:
		// Unread policy and no events: the empty list may mean "not audited".
		out = append(out, winsrv.Row{"server": server, "id": server + "/audit", "audit_policy": "unknown",
			"warning": "could not read the Security Group Management audit policy: an empty list may mean additions and removals are not recorded on this server"})
	case audit != "" && !strings.Contains(audit, "Success"):
		out = append(out, winsrv.Row{"server": server, "id": server + "/audit", "audit_policy": audit,
			"warning": "Security Group Management auditing does not log Success (" + audit + "): additions and removals are not recorded on this server"})
	}
	if winsrv.Bool(res["capped"]) {
		out = append(out, winsrv.Row{"server": server, "id": server + "/capped",
			"warning": "the read hit its cap and kept only the newest Administrators changes: older ones in the window are not listed; use a smaller --hours"})
	}
	for _, c := range changes {
		row, ok := c.(map[string]any)
		if !ok {
			continue
		}
		row["server"] = server
		row["id"] = server + "/" + winsrv.Str(row["record_id"])
		row["still_member"] = members[winsrv.Str(row["member_sid"])]
		out = append(out, row)
	}
	return out
}
