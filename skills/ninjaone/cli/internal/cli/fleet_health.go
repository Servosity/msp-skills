// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// Component weights for the composite health score. Kept as named constants so
// the score is a transparent weighted sum, not an opaque assessment.
const (
	whPatch  = 0.35
	whBackup = 0.30
	whAV     = 0.20
	whStale  = 0.15
)

// orgHealthOut is one organization's itemized health score.
type orgHealthOut struct {
	OrgID        string  `json:"orgId"`
	Org          string  `json:"org"`
	Devices      int     `json:"devices"`
	Score        float64 `json:"score"`
	PatchScore   float64 `json:"patchScore"`
	BackupScore  float64 `json:"backupScore"`
	AVScore      float64 `json:"avScore"`
	StaleScore   float64 `json:"staleScore"`
	BackupGaps   int     `json:"backupGaps"`
	AVThreats    int     `json:"avThreats"`
	StaleDevices int     `json:"staleDevices"`
}

// pp:data-source local
func newNovelFleetHealthCmd(flags *rootFlags) *cobra.Command {
	var orgFilter string
	var staleDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "fleet-health",
		Short: "Transparent 0-100 health score per org (patch + backup + AV + stale)",
		Long: `Computes a transparent weighted health score per organization from the synced
reports: patch compliance, backup coverage, AV threat load, and stale-device
ratio. The component scores and their counts are itemized so the number is
explainable, not a black box.

Reads the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Health for every org
  ninjaone-cli fleet-health

  # One org as agent JSON (itemized deductions)
  ninjaone-cli fleet-health --org 42 --agent`,
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

			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
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
			gaps, _, err := computeBackupCoverage(db, devices, orgNames)
			if err != nil {
				return fmt.Errorf("computing backup coverage: %w", err)
			}
			threatRows, err := loadRows(db, rtAVThreats)
			if err != nil {
				return fmt.Errorf("loading av threats: %w", err)
			}
			stale := computeStaleDevices(devices, orgNames, staleDays, time.Now().UTC())

			out := computeFleetHealth(devices, orgNames, stats, gaps, threatRows, stale)
			if orgFilter != "" {
				out = filterHealthByOrg(out, orgFilter)
			}

			if wantsStructured(flags) {
				return flags.printJSON(cmd, out)
			}
			if note := emptyStoreNote(devices); note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), note)
			}
			if len(out) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No organizations to score (nothing synced).")
				return nil
			}
			rows := make([][]string, 0, len(out))
			for _, o := range out {
				rows = append(rows, []string{
					o.Org,
					fmt.Sprintf("%.0f", o.Score),
					fmt.Sprintf("%.0f", o.PatchScore),
					fmt.Sprintf("%.0f", o.BackupScore),
					fmt.Sprintf("%.0f", o.AVScore),
					fmt.Sprintf("%.0f", o.StaleScore),
				})
			}
			return flags.printTable(cmd, []string{"ORG", "HEALTH", "PATCH", "BACKUP", "AV", "STALE"}, rows)
		},
	}
	cmd.Flags().StringVar(&orgFilter, "org", "", "Limit to one organization id")
	cmd.Flags().IntVar(&staleDays, "stale-days", 14, "Days without contact counted against the stale component")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}

// computeFleetHealth folds the component metrics into a per-org weighted score.
// Split out for table-driven testing.
func computeFleetHealth(devices map[string]nvDevice, orgNames map[string]string, stats map[string]*orgPatchStat, gaps []backupCoverageRow, threatRows []map[string]any, stale []staleRow) []orgHealthOut {
	// Per-org device counts.
	devCount := map[string]int{}
	for _, d := range devices {
		devCount[d.OrgID]++
	}
	gapCount := map[string]int{}
	for _, g := range gaps {
		gapCount[g.OrgID]++
	}
	staleCount := map[string]int{}
	for _, s := range stale {
		staleCount[s.OrgID]++
	}
	threatCount := map[string]int{}
	for _, r := range threatRows {
		did := rowDeviceID(r)
		if d, ok := devices[did]; ok {
			threatCount[d.OrgID]++
		} else {
			threatCount[""]++
		}
	}

	clamp := func(f float64) float64 {
		if f < 0 {
			return 0
		}
		if f > 100 {
			return 100
		}
		return f
	}

	var out []orgHealthOut
	for orgID, n := range devCount {
		patchScore := float64(100)
		if s := stats[orgID]; s != nil {
			patchScore = s.compliancePct()
		}
		backupScore := float64(100)
		if n > 0 {
			backupScore = roundPct(float64(n-gapCount[orgID]) / float64(n) * 100)
		}
		avScore := clamp(100 - float64(threatCount[orgID])*5)
		staleScore := float64(100)
		if n > 0 {
			staleScore = roundPct(float64(n-staleCount[orgID]) / float64(n) * 100)
		}
		total := patchScore*whPatch + backupScore*whBackup + avScore*whAV + staleScore*whStale
		out = append(out, orgHealthOut{
			OrgID:        orgID,
			Org:          orgLabel(orgNames, orgID),
			Devices:      n,
			Score:        roundPct(total),
			PatchScore:   patchScore,
			BackupScore:  backupScore,
			AVScore:      avScore,
			StaleScore:   staleScore,
			BackupGaps:   gapCount[orgID],
			AVThreats:    threatCount[orgID],
			StaleDevices: staleCount[orgID],
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score < out[j].Score
		}
		return out[i].Org < out[j].Org
	})
	return out
}

func filterHealthByOrg(rows []orgHealthOut, orgID string) []orgHealthOut {
	var out []orgHealthOut
	for _, r := range rows {
		if r.OrgID == orgID {
			out = append(out, r)
		}
	}
	return out
}
