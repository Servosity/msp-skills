// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type softwareInstallRow struct {
	DeviceID     string `json:"device_id"`
	Device       string `json:"device"`
	Organization string `json:"organization"`
	Name         string `json:"name"`
	Publisher    string `json:"publisher"`
	Version      string `json:"version"`
}

type softwareVersionRow struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Devices int    `json:"devices"`
}

type softwareAbsentRow struct {
	DeviceID     string `json:"device_id"`
	Device       string `json:"device"`
	Organization string `json:"organization"`
}

// softwareView reports how many devices had no inventory at all: those are
// neither "installed" nor "absent".
type softwareView struct {
	Mode            string `json:"mode"`
	Pattern         string `json:"pattern"`
	Match           string `json:"match"` // "exact" or "contains"
	Results         any    `json:"results"`
	ScannedDevices  int    `json:"scanned_devices"`
	NoInventory     int    `json:"devices_without_inventory"`
	MatchingDevices int    `json:"devices_with_app"`                // devices with at least one matching install
	AbsentDevices   int    `json:"devices_without_app"`             // devices with inventory but no matching install
	BelowDevices    int    `json:"devices_below_version,omitempty"` // --below: devices with an install older than the floor
	Note            string `json:"note,omitempty"`
}

func newNovelSoftwareCmd(flags *rootFlags) *cobra.Command {
	var flagBelow, flagOrg, dbPath string
	var versions, absent, contains bool
	var limit int

	cmd := &cobra.Command{
		Use:   "software [name]",
		Short: "Find which devices run an application, the version spread, devices below a version, or devices missing it.",
		Long: `Searches every synced device's installed software (the asset InstalledSoftware section) for an
application by its exact name, case-insensitive ("Google Chrome" does not match "Chrome Remote
Desktop Host"); --contains matches any application whose name contains the text instead. Reads the
local mirror only: run 'kaseya-vsa-cli sync --resources organizations,assets' first; with nothing
synced the result is empty and scanned_devices is 0.

Modes: default lists each matching install; --versions shows the version spread; --below VERSION
lists installs older than a dotted version floor; --absent lists devices with inventory that do NOT
have the application. Devices with no InstalledSoftware section are counted in
devices_without_inventory and are never reported as absent.

Use this command to find installed applications across the fleet, their version spread, or devices
missing an application. Do NOT use this command for pending Windows or OS updates or KBs; use
'patch-exposure' instead.`,
		Example: strings.Trim(`
  kaseya-vsa-cli software "Google Chrome" --versions --agent
  kaseya-vsa-cli software "Google Chrome" --below 140.0 --agent
  kaseya-vsa-cli software "Huntress Agent" --absent --org "Acme Dental"
  kaseya-vsa-cli software anydesk --contains --agent`, "\n"),
		// pp:no-error-path-probe: a pattern nothing matches is a valid empty search, not bad input.
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "name=Google Chrome", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "software")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an application name is required (e.g. software \"Google Chrome\")"))
			}
			modes := 0
			for _, on := range []bool{versions, absent, flagBelow != ""} {
				if on {
					modes++
				}
			}
			if modes > 1 {
				return usageErr(fmt.Errorf("--versions, --below and --absent are mutually exclusive"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			pattern := strings.TrimSpace(args[0])
			view := softwareView{Mode: softwareMode(versions, absent, flagBelow), Pattern: pattern, Match: "exact"}
			if contains {
				view.Match = "contains"
			}
			db, err := openFleetStore(cmd, flags, dbPath, "organizations", "assets")
			if err != nil {
				return err
			}
			if db == nil {
				view.Results = make([]any, 0)
				view.Note = softwareUnsyncedNote
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			defer db.Close()
			orgs, err := loadNameIndex(ctx, db, "organizations")
			if err != nil {
				return err
			}
			assets, err := loadFleetAssets(ctx, db)
			if err != nil {
				return err
			}
			searchSoftware(&view, assets, orgs, flagOrg, flagBelow)
			if view.ScannedDevices == 0 {
				view.Note = softwareUnsyncedNote
			}
			view.Results = limitSoftware(view.Results, limit)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "scanned %d devices (%d without inventory), %d matching\n", view.ScannedDevices, view.NoInventory, view.MatchingDevices)
			switch rows := view.Results.(type) {
			case []softwareInstallRow:
				for _, r := range rows {
					fmt.Fprintf(w, "%-24s %-24s %-30s %s\n", r.Device, r.Organization, r.Name, r.Version)
				}
			case []softwareVersionRow:
				for _, r := range rows {
					fmt.Fprintf(w, "%-30s %-20s devices=%d\n", r.Name, r.Version, r.Devices)
				}
			case []softwareAbsentRow:
				for _, r := range rows {
					fmt.Fprintf(w, "%-24s %s\n", r.Device, r.Organization)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&contains, "contains", false, "Match any application whose name contains the text instead of the exact name")
	cmd.Flags().BoolVar(&versions, "versions", false, "Show the version spread of matching applications instead of each install")
	cmd.Flags().StringVar(&flagBelow, "below", "", "Only installs older than this dotted version (e.g. 140.0.7339)")
	cmd.Flags().BoolVar(&absent, "absent", false, "List devices with inventory that do NOT have a matching application")
	cmd.Flags().StringVar(&flagOrg, "org", "", "Only devices in this organization (numeric ID or name substring)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum result rows (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the store sync writes)")
	return cmd
}

const softwareUnsyncedNote = "no synced assets: run 'kaseya-vsa-cli sync --resources organizations,assets' first; an empty result here does not mean the application is absent"

func softwareMode(versions, absent bool, below string) string {
	switch {
	case versions:
		return "versions"
	case absent:
		return "absent"
	case below != "":
		return "below"
	}
	return "installs"
}

func searchSoftware(v *softwareView, assets []*fleetAsset, orgs map[int64]string, orgFilterValue, below string) {
	needle := strings.ToLower(strings.TrimSpace(v.Pattern))
	matches := func(name string) bool {
		name = strings.ToLower(strings.TrimSpace(name))
		if v.Match == "contains" {
			return strings.Contains(name, needle)
		}
		return name == needle
	}
	installs := make([]softwareInstallRow, 0)
	absentRows := make([]softwareAbsentRow, 0)
	spread := map[[2]string]int{}
	for _, a := range assets {
		orgName := orgs[a.OrganizationID]
		if !orgFilter(orgFilterValue, a.OrganizationID, orgName) {
			continue
		}
		v.ScannedDevices++
		if !a.hasSoftwareData() {
			v.NoInventory++
			continue
		}
		hasApp, matched := false, false
		seen := map[[2]string]bool{}
		for _, s := range a.software {
			if !matches(s.Name) {
				continue
			}
			hasApp = true // presence is counted before the version filter
			if below != "" && compareVersions(s.Version, below) >= 0 {
				continue
			}
			matched = true
			installs = append(installs, softwareInstallRow{DeviceID: a.Identifier, Device: a.Name, Organization: orgName, Name: s.Name, Publisher: s.Publisher, Version: s.Version})
			key := [2]string{s.Name, s.Version}
			if !seen[key] {
				seen[key] = true
				spread[key]++
			}
		}
		if below != "" && matched {
			v.BelowDevices++
		}
		if hasApp {
			v.MatchingDevices++
		} else {
			v.AbsentDevices++
			if v.Mode == "absent" {
				absentRows = append(absentRows, softwareAbsentRow{DeviceID: a.Identifier, Device: a.Name, Organization: orgName})
			}
		}
	}
	switch v.Mode {
	case "versions":
		out := make([]softwareVersionRow, 0, len(spread))
		for k, n := range spread {
			out = append(out, softwareVersionRow{Name: k[0], Version: k[1], Devices: n})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Name != out[j].Name {
				return out[i].Name < out[j].Name
			}
			return compareVersions(out[i].Version, out[j].Version) > 0
		})
		v.Results = out
	case "absent":
		sort.Slice(absentRows, func(i, j int) bool {
			if absentRows[i].Organization != absentRows[j].Organization {
				return absentRows[i].Organization < absentRows[j].Organization
			}
			return absentRows[i].Device < absentRows[j].Device
		})
		v.Results = absentRows
	default:
		sort.Slice(installs, func(i, j int) bool {
			if installs[i].Device != installs[j].Device {
				return installs[i].Device < installs[j].Device
			}
			return installs[i].Name < installs[j].Name
		})
		v.Results = installs
	}
}

func limitSoftware(v any, n int) any {
	if n <= 0 {
		return v
	}
	switch s := v.(type) {
	case []softwareInstallRow:
		if len(s) > n {
			return s[:n]
		}
	case []softwareVersionRow:
		if len(s) > n {
			return s[:n]
		}
	case []softwareAbsentRow:
		if len(s) > n {
			return s[:n]
		}
	}
	return v
}
