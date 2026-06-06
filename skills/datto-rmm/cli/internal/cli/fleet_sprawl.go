// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type sprawlView struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	InstallCount int    `json:"installCount"`
	DeviceCount  int    `json:"deviceCount"`
}

// computeSprawl aggregates software across all devices by (Name, Version).
// InstallCount is the total number of installs seen; DeviceCount is the number
// of distinct devices carrying that exact name+version. nameFilter is an
// optional case-insensitive substring match on the software name.
func computeSprawl(software map[string][]fleetSoftware, nameFilter string) []sprawlView {
	filter := strings.ToLower(strings.TrimSpace(nameFilter))

	type agg struct {
		name     string
		version  string
		installs int
		devices  map[string]struct{}
	}
	groups := map[string]*agg{}

	for deviceUID, list := range software {
		for _, sw := range list {
			if filter != "" && !strings.Contains(strings.ToLower(sw.Name), filter) {
				continue
			}
			key := sw.Name + "\x00" + sw.Version
			g := groups[key]
			if g == nil {
				g = &agg{name: sw.Name, version: sw.Version, devices: map[string]struct{}{}}
				groups[key] = g
			}
			g.installs++
			g.devices[deviceUID] = struct{}{}
		}
	}

	out := make([]sprawlView, 0, len(groups))
	for _, g := range groups {
		out = append(out, sprawlView{
			Name:         g.name,
			Version:      g.version,
			InstallCount: g.installs,
			DeviceCount:  len(g.devices),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].InstallCount != out[j].InstallCount {
			return out[i].InstallCount > out[j].InstallCount
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Version < out[j].Version
	})
	return out
}

// pp:data-source local
func newNovelFleetSprawlCmd(flags *rootFlags) *cobra.Command {
	var name string
	var refresh bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "sprawl",
		Short:       "Rolls up audited software across the fleet to show install counts and the spread of versions",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet sprawl
  datto-rmm-cli fleet sprawl --name chrome
  datto-rmm-cli fleet sprawl --refresh --json`,
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

			if !hintIfUnsynced(cmd, db, fleetSoftwareResource) {
				hintIfStale(cmd, db, fleetSoftwareResource, flags.maxAge)
			}

			ctx := cmd.Context()

			if refresh {
				c, err := flags.newClient()
				if err != nil {
					return err
				}
				devices, err := loadFleetDevices(ctx, db)
				if err != nil {
					return err
				}
				for _, dev := range devices {
					if dev.UID == "" {
						continue
					}
					raw, err := c.Get(ctx, "/v2/audit/device/"+dev.UID+"/software", nil)
					if err != nil {
						continue
					}
					_ = db.Upsert(fleetSoftwareResource, dev.UID, raw)
				}
			}

			software, err := loadFleetSoftware(ctx, db)
			if err != nil {
				return err
			}
			view := computeSprawl(software, name)

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"NAME", "VERSION", "INSTALLS", "DEVICES"}
			rows := make([][]string, 0, len(view))
			for _, v := range view {
				rows = append(rows, []string{v.Name, v.Version, strconv.Itoa(v.InstallCount), strconv.Itoa(v.DeviceCount)})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Case-insensitive substring filter on software name")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Fetch per-device software live and upsert before aggregating")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
