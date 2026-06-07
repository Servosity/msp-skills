// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source local
package cli

import (
	"github.com/spf13/cobra"
)

func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "health",
		Short:       "One-shot fleet posture snapshot",
		Long:        "Whole-fleet posture in one command: online/offline/overdue agents, pending reboots, outstanding patches, pending actions, failing checks, active alerts by severity, and client/site counts. Reads only the local store; run 'tactical-rmm-cli sync' first.",
		Example:     "  tactical-rmm-cli fleet health\n  tactical-rmm-cli fleet health --json",
		Annotations: map[string]string{"mcp:read-only": "true"}, // read-only: queries the local store only
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			res := map[string]interface{}{
				"agents":          map[string]int{"total": 0, "online": 0, "offline": 0, "overdue": 0},
				"needs_reboot":    0,
				"patches_pending": 0,
				"pending_actions": 0,
				"checks":          map[string]int{"failing": 0, "agents_with_failing": 0},
				"alerts":          map[string]interface{}{"active": 0, "by_severity": map[string]int{"info": 0, "warning": 0, "error": 0}},
				"clients":         0,
				"sites":           0,
			}
			if s := novelLocalRead(cmd, flags, ""); s != nil {
				defer s.Close()
				db := s.DB()
				ctx := cmd.Context()
				const a = "SELECT COUNT(*) FROM resources WHERE resource_type='agents'"
				res["agents"] = map[string]int{
					"total":   tQInt(ctx, db, a),
					"online":  tQInt(ctx, db, a+" AND json_extract(data,'$.status')='online'"),
					"offline": tQInt(ctx, db, a+" AND json_extract(data,'$.status')='offline'"),
					"overdue": tQInt(ctx, db, a+" AND json_extract(data,'$.status')='overdue'"),
				}
				res["needs_reboot"] = tQInt(ctx, db, a+" AND json_extract(data,'$.needs_reboot')=1")
				res["patches_pending"] = tQInt(ctx, db, a+" AND json_extract(data,'$.has_patches_pending')=1")
				res["pending_actions"] = tQInt(ctx, db, "SELECT COALESCE(SUM(json_extract(data,'$.pending_actions_count')),0) FROM resources WHERE resource_type='agents'")
				res["checks"] = map[string]int{
					"failing":             tQInt(ctx, db, "SELECT COALESCE(SUM(json_extract(data,'$.checks.failing')),0) FROM resources WHERE resource_type='agents'"),
					"agents_with_failing": tQInt(ctx, db, a+" AND COALESCE(json_extract(data,'$.checks.failing'),0)>0"),
				}
				res["alerts"] = map[string]interface{}{
					"active": tQInt(ctx, db, "SELECT COUNT(*) FROM resources WHERE resource_type='alerts' AND COALESCE(json_extract(data,'$.resolved'),0)=0"),
					"by_severity": map[string]int{
						"info":    tQInt(ctx, db, "SELECT COUNT(*) FROM resources WHERE resource_type='alerts' AND json_extract(data,'$.severity')='info'"),
						"warning": tQInt(ctx, db, "SELECT COUNT(*) FROM resources WHERE resource_type='alerts' AND json_extract(data,'$.severity')='warning'"),
						"error":   tQInt(ctx, db, "SELECT COUNT(*) FROM resources WHERE resource_type='alerts' AND json_extract(data,'$.severity')='error'"),
					},
				}
				res["clients"] = tQInt(ctx, db, "SELECT COUNT(*) FROM resources WHERE resource_type='clients'")
				res["sites"] = tQInt(ctx, db, "SELECT COUNT(DISTINCT json_extract(data,'$.site_name')) FROM resources WHERE resource_type='agents' AND json_extract(data,'$.site_name') IS NOT NULL")
			}
			return flags.printJSON(cmd, res)
		},
	}
	return cmd
}
