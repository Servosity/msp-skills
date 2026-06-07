// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

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
		Long:        "Rolls pending Windows updates and reboots up to each client or site from the local store. For a per-client posture row across all signals (not just patches), use 'clients scorecard'.",
		Example:     "  tactical-rmm-cli patch posture --by client\n  tactical-rmm-cli patch posture --by site --json",
		Annotations: tRO(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			field := "client_name"
			if by == "site" {
				field = "site_name"
			}
			type row struct {
				Group          string `json:"group"`
				Agents         int    `json:"agents"`
				PatchesPending int    `json:"patches_pending"`
				RebootPending  int    `json:"reboot_pending"`
			}
			out := make([]row, 0)
			if s := novelLocalRead(cmd, flags, "agents"); s != nil {
				defer s.Close()
				q := "SELECT COALESCE(json_extract(data,'$." + field + "'),'(unknown)') g, COUNT(*), SUM(CASE WHEN json_extract(data,'$.has_patches_pending')=1 THEN 1 ELSE 0 END), SUM(CASE WHEN json_extract(data,'$.needs_reboot')=1 THEN 1 ELSE 0 END) FROM resources WHERE resource_type='agents' GROUP BY g ORDER BY 3 DESC"
				if rows, qe := s.DB().QueryContext(cmd.Context(), q); qe == nil {
					defer rows.Close()
					for rows.Next() {
						var g sql.NullString
						var c, pp, rb sql.NullInt64
						if rows.Scan(&g, &c, &pp, &rb) == nil {
							out = append(out, row{g.String, int(c.Int64), int(pp.Int64), int(rb.Int64)})
						}
					}
				}
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&by, "by", "client", "Group by: client or site")
	return cmd
}
