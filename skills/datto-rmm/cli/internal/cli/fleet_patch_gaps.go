// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strconv"

	"github.com/spf13/cobra"
)

type patchView struct {
	Hostname     string `json:"hostname"`
	SiteName     string `json:"siteName"`
	MissingCount int    `json:"missingCount"`
	PatchStatus  string `json:"patchStatus"`
	Installed    int    `json:"installed"`
}

// computePatchGaps returns non-deleted devices whose missing-patch count
// (approved-pending + not-approved) is at least minMissing. Sorted by
// MissingCount descending.
func computePatchGaps(devices []fleetDevice, minMissing int) []patchView {
	out := []patchView{}
	for _, d := range devices {
		if d.Deleted {
			continue
		}
		missing := d.PatchManagement.PatchesApprovedPending + d.PatchManagement.PatchesNotApproved
		if missing < minMissing {
			continue
		}
		out = append(out, patchView{
			Hostname:     d.Hostname,
			SiteName:     d.SiteName,
			MissingCount: missing,
			PatchStatus:  d.PatchManagement.PatchStatus,
			Installed:    d.PatchManagement.PatchesInstalled,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].MissingCount > out[j].MissingCount })
	return out
}

// pp:data-source local
func newNovelFleetPatchGapsCmd(flags *rootFlags) *cobra.Command {
	var minMissing int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "patch-gaps",
		Short:       "Ranks every device by missing-patch count across all sites so you remediate the most-exposed endpoints first",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet patch-gaps
  datto-rmm-cli fleet patch-gaps --min-missing 5 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openFleetStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, fleetDevicesResource) {
				hintIfStale(cmd, db, fleetDevicesResource, flags.maxAge)
			}

			devices, err := loadFleetDevices(cmd.Context(), db)
			if err != nil {
				return err
			}
			view := computePatchGaps(devices, minMissing)

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"HOSTNAME", "SITE", "MISSING", "STATUS", "INSTALLED"}
			rows := make([][]string, 0, len(view))
			for _, v := range view {
				rows = append(rows, []string{v.Hostname, v.SiteName, strconv.Itoa(v.MissingCount), v.PatchStatus, strconv.Itoa(v.Installed)})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().IntVar(&minMissing, "min-missing", 1, "Only show devices missing at least this many patches")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
