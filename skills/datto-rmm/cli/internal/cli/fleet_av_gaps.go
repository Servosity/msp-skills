// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type avAVView struct {
	AntivirusProduct string `json:"antivirusProduct"`
	AntivirusStatus  string `json:"antivirusStatus"`
}

type avView struct {
	Hostname  string   `json:"hostname"`
	SiteName  string   `json:"siteName"`
	Online    bool     `json:"online"`
	LastSeen  string   `json:"lastSeen"`
	Antivirus avAVView `json:"antivirus"`
}

// normalizeAvStatus lowercases and replaces '-' and '_' with spaces so that
// "not-running" matches "Not Running".
func normalizeAvStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.Join(strings.Fields(s), " ")
}

// computeAvGaps returns non-deleted devices with an antivirus gap. With no
// status filter, a gap is any device whose antivirusStatus is empty or not
// equal (case-insensitive) to "running". With a status filter, it returns
// devices whose antivirusStatus matches the filter (normalized). Sorted by
// SiteName, then Hostname.
func computeAvGaps(devices []fleetDevice, status string) []avView {
	out := []avView{}
	wantStatus := normalizeAvStatus(status)

	for _, d := range devices {
		if d.Deleted {
			continue
		}
		cur := normalizeAvStatus(d.Antivirus.AntivirusStatus)
		var match bool
		if wantStatus != "" {
			match = cur == wantStatus
		} else {
			match = !avIsHealthy(d.Antivirus.AntivirusStatus)
		}
		if !match {
			continue
		}
		lastSeen := ""
		if t, ok := parseDattoTime(d.LastSeen); ok {
			lastSeen = t.Format(time.RFC3339)
		}
		out = append(out, avView{
			Hostname: d.Hostname,
			SiteName: d.SiteName,
			Online:   d.Online,
			LastSeen: lastSeen,
			Antivirus: avAVView{
				AntivirusProduct: d.Antivirus.AntivirusProduct,
				AntivirusStatus:  d.Antivirus.AntivirusStatus,
			},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SiteName != out[j].SiteName {
			return out[i].SiteName < out[j].SiteName
		}
		return out[i].Hostname < out[j].Hostname
	})
	return out
}

// pp:data-source local
func newNovelFleetAvGapsCmd(flags *rootFlags) *cobra.Command {
	var status string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "av-gaps",
		Short:       "Finds every device fleet-wide where antivirus is missing, disabled, or not running",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet av-gaps
  datto-rmm-cli fleet av-gaps --status not-running --json`,
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
			view := computeAvGaps(devices, status)

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"HOSTNAME", "SITE", "ONLINE", "PRODUCT", "AV STATUS"}
			rows := make([][]string, 0, len(view))
			for _, v := range view {
				online := "false"
				if v.Online {
					online = "true"
				}
				rows = append(rows, []string{v.Hostname, v.SiteName, online, v.Antivirus.AntivirusProduct, v.Antivirus.AntivirusStatus})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Match a specific antivirus status (e.g. not-running); default flags anything not running")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
