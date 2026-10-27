// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"ninjaone-pp-cli/internal/store"
)

const bytesPerGiB = 1 << 30

// nvVolume is one synced /v2/queries/volumes row joined to its device.
type nvVolume struct {
	DeviceID   string
	DeviceName string
	OrgID      string
	Org        string
	NodeClass  string
	Volume     string
	Label      string
	Capacity   float64
	Free       float64
	HasSizes   bool
	BitLocker  map[string]any
}

// loadVolumes reads the synced volumes report and joins device and org names.
func loadVolumes(db *store.Store) ([]nvVolume, error) {
	rows, err := loadRows(db, rtVolumes)
	if err != nil {
		return nil, fmt.Errorf("loading volumes: %w", err)
	}
	devices, err := buildDeviceIndex(db)
	if err != nil {
		return nil, fmt.Errorf("loading devices: %w", err)
	}
	orgNames, err := buildOrgNames(db)
	if err != nil {
		return nil, fmt.Errorf("loading organizations: %w", err)
	}
	out := make([]nvVolume, 0, len(rows))
	for _, r := range rows {
		v := nvVolume{DeviceID: rowDeviceID(r)}
		v.Volume = nvStr(r, "driveLetter", "name")
		v.Label = nvStr(r, "label")
		capacity, okCap := nvFloat(r, "capacity")
		free, okFree := nvFloat(r, "freeSpace")
		v.Capacity, v.Free = capacity, free
		v.HasSizes = okCap && okFree && capacity > 0 && free >= 0 && free <= capacity
		if bl, ok := r["bitLockerStatus"].(map[string]any); ok {
			v.BitLocker = bl
		}
		d, known := devices[v.DeviceID]
		v.DeviceName = v.DeviceID
		if known {
			v.DeviceName, v.OrgID, v.NodeClass = d.Name, d.OrgID, d.NodeClass
		}
		v.Org = orgLabel(orgNames, v.OrgID)
		out = append(out, v)
	}
	return out, nil
}

type lowDiskRow struct {
	DeviceID   string  `json:"deviceId"`
	DeviceName string  `json:"deviceName"`
	OrgID      string  `json:"orgId"`
	Org        string  `json:"org"`
	Volume     string  `json:"volume"`
	Label      string  `json:"label,omitempty"`
	CapacityGB float64 `json:"capacityGiB"`
	FreeGB     float64 `json:"freeGiB"`
	FreePct    float64 `json:"freePct"`
}

// computeLowDisk returns every volume under the free-space floor, worst first,
// and how many volumes were skipped because the report gave no usable sizes.
func computeLowDisk(vols []nvVolume, minFreePct, minFreeGiB float64, orgFilter string) ([]lowDiskRow, int) {
	out := []lowDiskRow{}
	skipped := 0
	for _, v := range vols {
		if orgFilter != "" && v.OrgID != orgFilter {
			continue
		}
		if !v.HasSizes {
			skipped++
			continue
		}
		pct := v.Free / v.Capacity * 100
		freeGiB := v.Free / bytesPerGiB
		if pct >= minFreePct && (minFreeGiB <= 0 || freeGiB >= minFreeGiB) {
			continue
		}
		out = append(out, lowDiskRow{
			DeviceID: v.DeviceID, DeviceName: v.DeviceName, OrgID: v.OrgID, Org: v.Org,
			Volume: v.Volume, Label: v.Label,
			CapacityGB: math.Round(v.Capacity/bytesPerGiB*10) / 10,
			FreeGB:     math.Round(freeGiB*10) / 10,
			FreePct:    roundPct(pct),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FreePct != out[j].FreePct {
			return out[i].FreePct < out[j].FreePct
		}
		if out[i].DeviceName != out[j].DeviceName {
			return out[i].DeviceName < out[j].DeviceName
		}
		return out[i].Volume < out[j].Volume
	})
	return out, skipped
}

// pp:data-source local
func newNovelLowDiskCmd(flags *rootFlags) *cobra.Command {
	var minFreePct, minFreeGiB float64
	var orgFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "low-disk",
		Short: "List synced volumes under a free-space floor, worst first, with device and organization",
		Long: `Scans the synced volumes report (capacity and free space per volume) for
volumes under a free-space floor, worst first, with the owning device and
organization. A volume is low when its free space is under --min-free-pct
percent of capacity, or under --min-free-gb GiB when that floor is set.
Volumes the report gives no usable sizes for (optical drives, empty readers)
are counted on stderr, never guessed.

Reads the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Volumes under 10% free (default)
  ninjaone-cli low-disk

  # Under 15% free or under 20 GiB free, one organization, as JSON
  ninjaone-cli low-disk --min-free-pct 15 --min-free-gb 20 --org 42 --json`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if minFreePct < 0 || minFreePct > 100 {
				return usageErr(fmt.Errorf("--min-free-pct must be between 0 and 100, got %g", minFreePct))
			}
			if minFreeGiB < 0 {
				return usageErr(fmt.Errorf("--min-free-gb must not be negative, got %g", minFreeGiB))
			}
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
			low, skipped := computeLowDisk(vols, minFreePct, minFreeGiB, strings.TrimSpace(orgFilter))
			if len(vols) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no volumes in local store; run 'ninjaone-cli sync --resources queries-volumes' first")
			} else if skipped > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %d volume(s) skipped: the report gave no usable capacity/free space\n", skipped)
			}
			if wantsStructured(flags) {
				return flags.printJSON(cmd, low)
			}
			if len(low) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No volumes under %g%% free.\n", minFreePct)
				return nil
			}
			rows := make([][]string, 0, len(low))
			for _, r := range low {
				rows = append(rows, []string{r.Org, r.DeviceName, r.Volume,
					fmt.Sprintf("%.1f", r.FreeGB), fmt.Sprintf("%.1f", r.CapacityGB), fmt.Sprintf("%.1f%%", r.FreePct)})
			}
			return flags.printTable(cmd, []string{"ORG", "DEVICE", "VOLUME", "FREE GiB", "SIZE GiB", "FREE %"}, rows)
		},
	}
	cmd.Flags().Float64Var(&minFreePct, "min-free-pct", 10, "Flag volumes with less than this percent of capacity free")
	cmd.Flags().Float64Var(&minFreeGiB, "min-free-gb", 0, "Also flag volumes with less than this many GiB free (0 = off)")
	cmd.Flags().StringVar(&orgFilter, "org", "", "Limit to one organization id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}
