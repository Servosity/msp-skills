// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// Paging and filters follow CIPP: Get-QuarantineMessage pages with Page/PageSize
// until a short page (Push-ListMailQuarantineAllTenants.ps1), and pending release
// requests are ReleaseStatus Requested over the 30 days Exchange keeps
// (Get-CIPPAlertQuarantineReleaseRequests.ps1).
const quarantinePageSize = 1000

var quarantineMaxPages = 50

type quarantineSnapshotView struct {
	Tenant   string `json:"tenant"`
	Requests int    `json:"release_requests"`
	Pages    int    `json:"pages"`
	StoredAt string `json:"stored_at"`
}

func newNovelQuarantineSnapshotCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot <tenantId>",
		Short: "Save one tenant's pending quarantine release requests locally for the cross-tenant queue.",
		Long: `Fetches every quarantined message with ReleaseStatus Requested received in the last 30 days and
replaces this tenant's stored set in the local store. Run it once per tenant, each with that
tenant's token, then read all of them together with 'quarantine queue'.

Use this command to refresh one tenant's release requests in the local store. Do NOT use this
command to read the queue; use 'quarantine queue' instead.`,
		Example:     "  exchange-online-cli quarantine snapshot 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent",
		Annotations: map[string]string{"mcp:local-write": "true", "pp:live-happy-path": "true", "pp:data-source": "live", "pp:happy-args": "tenantId=0550b473-9fd2-4dbb-a058-a1640b4bf458"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "quarantine snapshot")
			}
			tenant, err := exoTenantArg(cmd, args, 0)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			now := time.Now().UTC()
			v := quarantineSnapshotView{Tenant: tenant}
			var all []exoRow
			for page := 1; ; page++ {
				rows, err := exoGet(ctx, c, exoBeta(tenant, "Get-QuarantineMessage"), exoRow{"PageSize": quarantinePageSize, "Page": page,
					"ReleaseStatus": "Requested", "StartReceivedDate": now.AddDate(0, 0, -30).Format("2006-01-02T15:04:05"), "EndReceivedDate": now.Format("2006-01-02T15:04:05")}, "")
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				all = append(all, rows...)
				v.Pages = page
				if len(rows) < quarantinePageSize {
					break
				}
				if page >= quarantineMaxPages {
					return apiErr(fmt.Errorf("more than %d pages of release requests; nothing stored", quarantineMaxPages))
				}
			}
			// The meta row keeps a tenant with zero requests visible in the queue.
			at, err := replaceSnapshots(ctx, tenant,
				snapshotSet{"quarantine_request", all, func(r exoRow) string { return rs(r, "Identity") }},
				snapshotSet{"quarantine_meta", []exoRow{{"requests": len(all)}}, func(exoRow) string { return "snapshot" }})
			if err != nil {
				return err
			}
			v.Requests, v.StoredAt = len(all), at.Format(time.RFC3339)
			return printJSONFiltered(cmd.OutOrStdout(), v, flags)
		},
	}
	return cmd
}
