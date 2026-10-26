// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type bitlockerGapRow struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	OrgID      string `json:"orgId"`
	Org        string `json:"org"`
	NodeClass  string `json:"nodeClass"`
	Volume     string `json:"volume"`
	Protection string `json:"protectionStatus"`
	Conversion string `json:"conversionStatus,omitempty"`
	Method     string `json:"encryptionMethod,omitempty"`
}

// computeBitlockerGaps lists every volume whose BitLocker protection is not
// PROTECTED. Only Windows devices carry BitLocker, so other node classes are
// skipped unless allOS; a device whose class is not synced is kept (an audit
// shows what it cannot classify rather than hiding it). Returns the gaps and
// the number of volumes skipped as non-Windows.
func computeBitlockerGaps(vols []nvVolume, allOS bool, orgFilter string) ([]bitlockerGapRow, int) {
	out := []bitlockerGapRow{}
	skipped := 0
	for _, v := range vols {
		if orgFilter != "" && v.OrgID != orgFilter {
			continue
		}
		if !allOS && v.NodeClass != "" && !strings.HasPrefix(v.NodeClass, "WINDOWS") {
			skipped++
			continue
		}
		if v.Capacity <= 0 {
			// Optical drives and empty readers have no capacity and nothing to encrypt.
			continue
		}
		protection := "NOT_REPORTED"
		row := bitlockerGapRow{DeviceID: v.DeviceID, DeviceName: v.DeviceName, OrgID: v.OrgID, Org: v.Org,
			NodeClass: v.NodeClass, Volume: v.Volume}
		if v.BitLocker != nil {
			if s := nvStr(v.BitLocker, "protectionStatus"); s != "" {
				protection = strings.ToUpper(s)
			}
			row.Conversion = nvStr(v.BitLocker, "conversionStatus")
			row.Method = nvStr(v.BitLocker, "encryptionMethod")
		}
		if protection == "PROTECTED" {
			continue
		}
		row.Protection = protection
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Org != out[j].Org {
			return out[i].Org < out[j].Org
		}
		if out[i].DeviceName != out[j].DeviceName {
			return out[i].DeviceName < out[j].DeviceName
		}
		return out[i].Volume < out[j].Volume
	})
	return out, skipped
}

// bitlockerStatusCoverage counts the volumes computeBitlockerGaps considers and how many of
// them carry a bitLockerStatus object at all. None carrying one means the store was synced
// without include=bl (spec.yaml /v2/queries/volumes), not that every volume is unprotected.
func bitlockerStatusCoverage(vols []nvVolume, allOS bool, orgFilter string) (inScope, reported int) {
	for _, v := range vols {
		if orgFilter != "" && v.OrgID != orgFilter {
			continue
		}
		if !allOS && v.NodeClass != "" && !strings.HasPrefix(v.NodeClass, "WINDOWS") {
			continue
		}
		if v.Capacity <= 0 {
			continue
		}
		inScope++
		if v.BitLocker != nil {
			reported++
		}
	}
	return inScope, reported
}

// pp:data-source local
func newNovelBitlockerGapsCmd(flags *rootFlags) *cobra.Command {
	var allOS bool
	var orgFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "bitlocker-gaps",
		Short: "List synced volumes whose BitLocker protection is not PROTECTED, by organization",
		Long: `Reads BitLocker status (protectionStatus, conversionStatus, encryptionMethod)
from the synced volumes report and lists every volume that is not PROTECTED:
UNPROTECTED, PENDING, UNKNOWN, or NOT_REPORTED when the report carries no
BitLocker status for it. Sorted by organization, then device.

BitLocker exists only on Windows, so volumes of devices whose node class is
not WINDOWS_* are skipped (counted on stderr) unless --all-os is set. A device
whose node class is not in the local store is kept, so nothing is silently
dropped. Volumes with no capacity (optical drives) are not listed.

Reads the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Every unprotected Windows volume
  ninjaone-cli bitlocker-gaps

  # One organization, as JSON, for an insurance questionnaire
  ninjaone-cli bitlocker-gaps --org 2 --json`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
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
			if !hintIfUnsynced(cmd, db, rtVolumes) {
				hintIfStale(cmd, db, rtVolumes, flags.maxAge)
			}

			vols, err := loadVolumes(db)
			if err != nil {
				return err
			}
			gaps, skipped := computeBitlockerGaps(vols, allOS, strings.TrimSpace(orgFilter))
			if len(vols) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no volumes in local store; run 'ninjaone-cli sync --resources queries-volumes' first")
			} else if skipped > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %d volume(s) on non-Windows devices skipped (use --all-os to include them)\n", skipped)
			}
			if inScope, reported := bitlockerStatusCoverage(vols, allOS, strings.TrimSpace(orgFilter)); inScope > 0 && reported == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: none of the %d synced volume(s) in scope carries BitLocker status, so each reads NOT_REPORTED: the volumes were synced without include=bl. Run 'ninjaone-cli sync --resources queries-volumes' to fetch it.\n", inScope)
			}
			if wantsStructured(flags) {
				return flags.printJSON(cmd, gaps)
			}
			if len(gaps) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Every synced volume in scope is BitLocker PROTECTED.")
				return nil
			}
			rows := make([][]string, 0, len(gaps))
			for _, g := range gaps {
				rows = append(rows, []string{g.Org, g.DeviceName, g.Volume, g.Protection, g.Conversion})
			}
			return flags.printTable(cmd, []string{"ORG", "DEVICE", "VOLUME", "PROTECTION", "CONVERSION"}, rows)
		},
	}
	cmd.Flags().BoolVar(&allOS, "all-os", false, "Include volumes of non-Windows devices")
	cmd.Flags().StringVar(&orgFilter, "org", "", "Limit to one organization id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}
