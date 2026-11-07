// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"database/sql"

	"github.com/spf13/cobra"
)

func newNovelPatchPostureCmd(flags *rootFlags) *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:         "posture",
		Short:       "Per-client/site pending-patch and reboot rollup",
		Long:        "Rolls pending Windows updates and reboots up to each client or site from the local store. Site names repeat across clients, so --by site keeps one row per site within its client: \"group\" is the site name and \"client\" names its client. For a per-client posture row across all signals (not just patches), use 'clients scorecard'.",
		Example:     "  tactical-rmm-cli patch posture --by client\n  tactical-rmm-cli patch posture --by site --json",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			type row struct {
				Group          string `json:"group"`
				Client         string `json:"client,omitempty"` // --by site only
				Agents         int    `json:"agents"`
				PatchesPending int    `json:"patches_pending"`
				RebootPending  int    `json:"reboot_pending"`
			}
			out := make([]row, 0)
			if s := novelLocalRead(cmd, flags, "agents"); s != nil {
				defer s.Close()
				q := "SELECT COALESCE(json_extract(data,'$.client_name'),'(unknown)') g, '' cl, COUNT(*), SUM(CASE WHEN json_extract(data,'$.has_patches_pending')=1 THEN 1 ELSE 0 END), SUM(CASE WHEN json_extract(data,'$.needs_reboot')=1 THEN 1 ELSE 0 END) FROM resources WHERE resource_type='agents' GROUP BY g ORDER BY 4 DESC"
				if by == "site" {
					// tactical-rmm handfix site-identity-client-and-site: a site is
					// (client, site), never its name alone (see trmm_site_identity.go).
					q = "SELECT " + trmmAgentSiteNameSQL + " g, " + trmmAgentClientSQL + " cl, COUNT(*), SUM(CASE WHEN json_extract(a.data,'$.has_patches_pending')=1 THEN 1 ELSE 0 END), SUM(CASE WHEN json_extract(a.data,'$.needs_reboot')=1 THEN 1 ELSE 0 END) FROM " + trmmAgentSiteFrom + " WHERE a.resource_type='agents' GROUP BY cl, g ORDER BY 4 DESC, cl, g"
				}
				if rows, qe := s.DB().QueryContext(cmd.Context(), q); qe == nil {
					defer rows.Close()
					for rows.Next() {
						var g, cl sql.NullString
						var c, pp, rb sql.NullInt64
						if rows.Scan(&g, &cl, &c, &pp, &rb) == nil {
							out = append(out, row{g.String, cl.String, int(c.Int64), int(pp.Int64), int(rb.Int64)})
						}
					}
				}
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&by, "by", "client", "Group by: client or site (a site row also names its client)")
	return cmd
}
