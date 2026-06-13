// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"veeam-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type licenseUsageOrg struct {
	Organization string  `json:"organization"`
	OrgUID       string  `json:"org_uid,omitempty"`
	OrgType      string  `json:"org_type,omitempty"`
	UsedPoints   float64 `json:"used_points"`
	Delta        float64 `json:"delta"`
	HasPrior     bool    `json:"has_prior"`
}

type licenseUsageView struct {
	Organizations []licenseUsageOrg `json:"organizations"`
	TotalUsed     float64           `json:"total_used"`
	Count         int               `json:"count"`
}

func newNovelLicenseUsageCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int
	var noSnapshot bool

	cmd := &cobra.Command{
		Use:   "license-usage",
		Short: "Per-organization license consumption (used points) with the delta since the previous run.",
		Long: strings.Trim(`
Roll up per-organization license consumption (used points) from the local
mirror, sorted by usage. Each run records a local snapshot, so the `+"`delta`"+`
column shows the change in used points since the last time this command ran —
compounding state the stateless VSPC usage endpoint does not return.

Reads the local SQLite mirror — run `+"`veeam-cli sync`"+` first. Pass
--no-snapshot to inspect without recording a new sample.`, "\n"),
		Example: strings.Trim(`
  veeam-cli license-usage
  veeam-cli license-usage --agent --select organizations.organization,organizations.used_points,organizations.delta
  veeam-cli license-usage --no-snapshot`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			db, err := veeamOpenStore(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()

			rows, _ := veeamLoad(ctx, db, "licensing-usage-organizations", "organizations-companies-usage")
			now := time.Now()
			recordSnapshots := !noSnapshot && !cliutil.IsVerifyEnv() && !cliutil.IsDogfoodEnv()

			out := make([]licenseUsageOrg, 0, len(rows))
			var total float64
			for _, r := range rows {
				uid := vstr(r, "organizationUid")
				name := firstNonEmpty(vstr(r, "organizationName"), vstr(r, "name"), uid, "(organization)")
				used := vnum(r, "usedPoints")
				org := licenseUsageOrg{
					Organization: name,
					OrgUID:       uid,
					OrgType:      vstr(r, "organizationType"),
					UsedPoints:   used,
				}
				if uid != "" {
					if prior, _, ok, derr := db.LatestUsageSnapshot(ctx, uid, now); derr == nil && ok {
						org.Delta = used - prior
						org.HasPrior = true
					}
					if recordSnapshots {
						_ = db.RecordUsageSnapshot(ctx, uid, used, now)
					}
				}
				out = append(out, org)
				total += used
			}

			sort.SliceStable(out, func(i, j int) bool {
				if out[i].UsedPoints != out[j].UsedPoints {
					return out[i].UsedPoints > out[j].UsedPoints
				}
				return strings.ToLower(out[i].Organization) < strings.ToLower(out[j].Organization)
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}

			view := licenseUsageView{Organizations: out, TotalUsed: total, Count: len(out)}
			table := make([]map[string]any, 0, len(out))
			for _, o := range out {
				delta := "—"
				if o.HasPrior {
					delta = fmt.Sprintf("%+.1f", o.Delta)
				}
				table = append(table, map[string]any{
					"organization": o.Organization,
					"used_points":  o.UsedPoints,
					"delta":        delta,
				})
			}
			return veeamEmit(cmd, flags, view, table, "No license usage in the local mirror. Run `veeam-cli sync` first, then re-check.")
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum organizations to return (0 = all)")
	cmd.Flags().BoolVar(&noSnapshot, "no-snapshot", false, "Do not record a new usage snapshot this run")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite mirror path (default: standard cache location)")
	return cmd
}
