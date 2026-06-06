// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// pp:novel-static-reference
// eolEntry maps an OS name substring to its end-of-support date. Matching is
// case-insensitive and longest-substring-wins so "Windows Server 2012 R2"
// beats "Windows Server 2012". Dates are vendor end-of-support; update as the
// support calendar advances. Source: Microsoft/Apple/distro lifecycle pages.
type eolEntry struct {
	match string
	eol   time.Time
}

func mustDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// pp:novel-static-reference
var osEOLTable = []eolEntry{
	{"windows xp", mustDate("2014-04-08")},
	{"windows vista", mustDate("2017-04-11")},
	{"windows 7", mustDate("2020-01-14")},
	{"windows 8.1", mustDate("2023-01-10")},
	{"windows 8", mustDate("2016-01-12")},
	{"windows 10", mustDate("2025-10-14")},
	{"windows server 2008 r2", mustDate("2020-01-14")},
	{"windows server 2008", mustDate("2020-01-14")},
	{"windows server 2012 r2", mustDate("2023-10-10")},
	{"windows server 2012", mustDate("2023-10-10")},
	{"windows server 2016", mustDate("2027-01-12")},
	{"windows server 2019", mustDate("2029-01-09")},
	{"centos 6", mustDate("2020-11-30")},
	{"centos 7", mustDate("2024-06-30")},
	{"centos 8", mustDate("2021-12-31")},
	{"ubuntu 16.04", mustDate("2021-04-30")},
	{"ubuntu 18.04", mustDate("2023-05-31")},
	{"ubuntu 20.04", mustDate("2025-04-30")},
	{"mac os x 10.13", mustDate("2020-12-01")},
	{"macos 10.14", mustDate("2021-10-25")},
	{"macos 10.15", mustDate("2022-09-12")},
	{"macos 11", mustDate("2023-09-26")},
	{"macos 12", mustDate("2024-10-28")},
}

// osEOLOut is one EOL-exposed device row.
type osEOLOut struct {
	DeviceID    string `json:"deviceId"`
	DeviceName  string `json:"deviceName"`
	OrgID       string `json:"orgId"`
	Org         string `json:"org"`
	OS          string `json:"os"`
	EOLDate     string `json:"eolDate"`
	DaysPastEOL int    `json:"daysPastEol"`
}

// pp:data-source local
func newNovelOsEolCmd(flags *rootFlags) *cobra.Command {
	var orgFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "os-eol",
		Short: "List devices on end-of-life operating systems, grouped by org",
		Long: `Matches each synced device's operating system against a curated end-of-life
reference table and lists devices whose OS is past its vendor end-of-support
date. A security/compliance answer no NinjaOne tool surfaces.

Reads the local store; run 'ninjaone-cli sync' first.`,
		Example: `  # Every EOL-exposed device
  ninjaone-cli os-eol

  # One org, agent JSON
  ninjaone-cli os-eol --org 42 --agent`,
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

			if !hintIfUnsynced(cmd, db, rtOperatingSys) {
				hintIfStale(cmd, db, rtOperatingSys, flags.maxAge)
			}

			devices, err := buildDeviceIndex(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			orgNames, err := buildOrgNames(db)
			if err != nil {
				return fmt.Errorf("loading organizations: %w", err)
			}
			// Enrich OS names from the operating-systems report when device.os
			// is empty (some tenants populate one but not the other).
			osByDevice := map[string]string{}
			if osRows, err := loadRows(db, rtOperatingSys); err == nil {
				for _, r := range osRows {
					osByDevice[rowDeviceID(r)] = nvStr(r, "name", "displayName", "osName")
				}
			}

			out := computeOsEOL(devices, orgNames, osByDevice, time.Now().UTC())
			if orgFilter != "" {
				out = filterEOLByOrg(out, orgFilter)
			}

			if wantsStructured(flags) {
				return flags.printJSON(cmd, out)
			}
			if note := emptyStoreNote(devices); note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), note)
			}
			if len(out) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No devices on end-of-life operating systems.")
				return nil
			}
			rows := make([][]string, 0, len(out))
			for _, o := range out {
				rows = append(rows, []string{o.Org, o.DeviceName, o.OS, o.EOLDate, fmt.Sprintf("%d", o.DaysPastEOL)})
			}
			return flags.printTable(cmd, []string{"ORG", "DEVICE", "OS", "EOL DATE", "DAYS PAST"}, rows)
		},
	}
	cmd.Flags().StringVar(&orgFilter, "org", "", "Limit to one organization id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: local data store)")
	return cmd
}

// lookupEOL returns the EOL date for an OS name using longest-substring match.
func lookupEOL(osName string) (time.Time, bool) {
	name := strings.ToLower(osName)
	var best eolEntry
	found := false
	for _, e := range osEOLTable {
		if strings.Contains(name, e.match) {
			if !found || len(e.match) > len(best.match) {
				best = e
				found = true
			}
		}
	}
	if !found {
		return time.Time{}, false
	}
	return best.eol, true
}

// computeOsEOL returns devices whose OS is past end-of-life. Split out for tests.
func computeOsEOL(devices map[string]nvDevice, orgNames, osByDevice map[string]string, now time.Time) []osEOLOut {
	var out []osEOLOut
	for _, d := range devices {
		osName := d.OSName
		if osName == "" {
			osName = osByDevice[d.ID]
		}
		if osName == "" {
			continue
		}
		eol, ok := lookupEOL(osName)
		if !ok || !eol.Before(now) {
			continue
		}
		out = append(out, osEOLOut{
			DeviceID:    d.ID,
			DeviceName:  d.Name,
			OrgID:       d.OrgID,
			Org:         orgLabel(orgNames, d.OrgID),
			OS:          osName,
			EOLDate:     eol.Format("2006-01-02"),
			DaysPastEOL: int(now.Sub(eol).Hours() / 24),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DaysPastEOL != out[j].DaysPastEOL {
			return out[i].DaysPastEOL > out[j].DaysPastEOL
		}
		return out[i].DeviceName < out[j].DeviceName
	})
	return out
}

func filterEOLByOrg(rows []osEOLOut, orgID string) []osEOLOut {
	var out []osEOLOut
	for _, r := range rows {
		if r.OrgID == orgID {
			out = append(out, r)
		}
	}
	return out
}
