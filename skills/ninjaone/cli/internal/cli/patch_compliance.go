// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// orgComplianceOut is the per-organization compliance row emitted to JSON/table.
type orgComplianceOut struct {
	OrgID          string  `json:"orgId"`
	Org            string  `json:"org"`
	Devices        int     `json:"devices"`
	NonCompliant   int     `json:"nonCompliant"`
	CompliancePct  float64 `json:"compliancePct"`
	PendingPatches int     `json:"pendingPatches"`
	FailedPatches  int     `json:"failedPatches"`
	WorstDevice    string  `json:"worstDevice,omitempty"`
	WorstPending   int     `json:"worstDevicePending,omitempty"`
}

// pp:data-source local
func newNovelPatchComplianceCmd(flags *rootFlags) *cobra.Command {
	var minPct float64
	var dbPath string

	cmd := &cobra.Command{
		Use:   "patch-compliance",
		Short: "Per-org patch-compliance rollup (OS + software) from the local store",
		Long: `Joins the synced OS-patch and software-patch reports (which list pending,
failed, and rejected patches) against the device->organization map to produce
one compliance row per organization: percent of devices with no outstanding
patches, pending/failed counts, and the worst-offender device.

Reads the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Compliance across every organization
  ninjaone-cli patch-compliance

  # Only organizations below 95% compliant, as agent JSON
  ninjaone-cli patch-compliance --min-pct 95 --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtOsPatches) {
				hintIfStale(cmd, db, rtOsPatches, flags.maxAge)
			}

			devices, err := buildDeviceIndex(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			orgNames, err := buildOrgNames(db)
			if err != nil {
				return fmt.Errorf("loading organizations: %w", err)
			}
			stats, err := computePatchStats(db, devices)
			if err != nil {
				return fmt.Errorf("computing patch stats: %w", err)
			}

			out := buildComplianceRows(stats, orgNames, minPct)

			if wantsStructured(flags) {
				return flags.printJSON(cmd, out)
			}
			if note := emptyStoreNote(devices); note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), note)
			}
			if len(out) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No organizations to report (every org is compliant or nothing is synced).")
				return nil
			}
			rows := make([][]string, 0, len(out))
			for _, o := range out {
				rows = append(rows, []string{
					o.Org,
					fmt.Sprintf("%d", o.Devices),
					fmt.Sprintf("%.1f%%", o.CompliancePct),
					fmt.Sprintf("%d", o.PendingPatches),
					fmt.Sprintf("%d", o.FailedPatches),
					o.WorstDevice,
				})
			}
			return flags.printTable(cmd, []string{"ORG", "DEVICES", "COMPLIANT", "PENDING", "FAILED", "WORST DEVICE"}, rows)
		},
	}
	cmd.Flags().Float64Var(&minPct, "min-pct", 0, "Only show orgs below this compliance percent (0 = show all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}

// buildComplianceRows converts per-org stats into sorted output rows, applying
// the --min-pct filter. Split out for table-driven testing.
func buildComplianceRows(stats map[string]*orgPatchStat, orgNames map[string]string, minPct float64) []orgComplianceOut {
	var out []orgComplianceOut
	for _, s := range stats {
		pct := s.compliancePct()
		if minPct > 0 && pct >= minPct {
			continue
		}
		out = append(out, orgComplianceOut{
			OrgID:          s.OrgID,
			Org:            orgLabel(orgNames, s.OrgID),
			Devices:        s.Devices,
			NonCompliant:   s.NonCompliant,
			CompliancePct:  pct,
			PendingPatches: s.PendingPatches,
			FailedPatches:  s.FailedPatches,
			WorstDevice:    s.WorstDeviceName,
			WorstPending:   s.WorstDeviceCount,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CompliancePct != out[j].CompliancePct {
			return out[i].CompliancePct < out[j].CompliancePct
		}
		return out[i].Org < out[j].Org
	})
	return out
}
