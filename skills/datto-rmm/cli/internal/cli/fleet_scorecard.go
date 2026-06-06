// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type scorecardView struct {
	Site               string  `json:"site"`
	SiteUID            string  `json:"siteUid"`
	DeviceCount        int     `json:"deviceCount"`
	OnlineCount        int     `json:"onlineCount"`
	OpenAlerts         int     `json:"openAlerts"`
	PatchOkPct         float64 `json:"patchOkPct"`
	AvOkPct            float64 `json:"avOkPct"`
	WarrantyExpiring90 int     `json:"warrantyExpiring90"`
	AgentsBehind       int     `json:"agentsBehind"`
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// computeScorecard fuses device, alert, and site signals for a single site
// matched by name or uid (case-insensitive). Returns ok=false when no device
// and no site matches the key.
func computeScorecard(siteKey string, devices []fleetDevice, alerts []fleetAlert, sites []fleetSite, now time.Time) (scorecardView, bool) {
	key := strings.ToLower(strings.TrimSpace(siteKey))

	// Fleet-wide latest agent version for the agents-behind count.
	fleetLatest := ""
	for _, d := range devices {
		if d.CagVersion == "" {
			continue
		}
		if fleetLatest == "" || compareVersions(d.CagVersion, fleetLatest) > 0 {
			fleetLatest = d.CagVersion
		}
	}

	var card scorecardView
	matched := false
	patchTotal, patchOk := 0, 0
	avTotal, avOk := 0, 0
	warrantyLimit := now.AddDate(0, 0, 90)

	for _, d := range devices {
		if strings.ToLower(d.SiteName) != key && strings.ToLower(d.SiteUID) != key {
			continue
		}
		matched = true
		if card.Site == "" {
			card.Site = d.SiteName
		}
		if card.SiteUID == "" {
			card.SiteUID = d.SiteUID
		}
		card.DeviceCount++
		if d.Online && !d.Deleted {
			card.OnlineCount++
		}

		patchTotal++
		if d.PatchManagement.PatchesApprovedPending+d.PatchManagement.PatchesNotApproved == 0 {
			patchOk++
		}

		avTotal++
		if avIsHealthy(d.Antivirus.AntivirusStatus) {
			avOk++
		}

		if exp, ok := parseWarranty(d.WarrantyDate); ok && !exp.After(warrantyLimit) {
			card.WarrantyExpiring90++
		}

		if d.CagVersion != "" && fleetLatest != "" && compareVersions(d.CagVersion, fleetLatest) < 0 {
			card.AgentsBehind++
		}
	}

	// A site may exist with no devices; match against the sites table too.
	for _, s := range sites {
		if strings.ToLower(s.Name) == key || strings.ToLower(s.UID) == key {
			matched = true
			if card.Site == "" {
				card.Site = s.Name
			}
			if card.SiteUID == "" {
				card.SiteUID = s.UID
			}
		}
	}

	if !matched {
		return scorecardView{}, false
	}

	for _, a := range alerts {
		if a.Resolved {
			continue
		}
		if strings.ToLower(a.AlertSourceInfo.SiteName) == key || strings.ToLower(a.AlertSourceInfo.SiteUID) == key ||
			(card.SiteUID != "" && a.AlertSourceInfo.SiteUID == card.SiteUID) {
			card.OpenAlerts++
		}
	}

	if patchTotal > 0 {
		card.PatchOkPct = round1(float64(patchOk) / float64(patchTotal) * 100)
	}
	if avTotal > 0 {
		card.AvOkPct = round1(float64(avOk) / float64(avTotal) * 100)
	}

	return card, true
}

// pp:data-source local
func newNovelFleetScorecardCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:         "scorecard <site>",
		Short:       "Produces a one-shot per-site health card fusing device counts, alerts, patch and AV coverage",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  datto-rmm-cli fleet scorecard "Acme Corp"
  datto-rmm-cli fleet scorecard 0f3c... --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) == 0 {
				return fmt.Errorf("fleet scorecard: provide a site name or UID")
			}
			siteKey := args[0]

			db, err := openFleetStore(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, fleetDevicesResource) {
				hintIfStale(cmd, db, fleetDevicesResource, flags.maxAge)
			}

			ctx := cmd.Context()
			devices, err := loadFleetDevices(ctx, db)
			if err != nil {
				return err
			}
			alerts, err := loadFleetAlerts(ctx, db)
			if err != nil {
				return err
			}
			sites, err := loadFleetSites(ctx, db)
			if err != nil {
				return err
			}

			card, ok := computeScorecard(siteKey, devices, alerts, sites, time.Now().UTC())
			if !ok {
				return fmt.Errorf("fleet scorecard: no site matching %q in the local store (run 'datto-rmm-cli sync' first)", siteKey)
			}

			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, card)
			}
			headers := []string{"METRIC", "VALUE"}
			rows := [][]string{
				{"Site", card.Site},
				{"Site UID", card.SiteUID},
				{"Devices", strconv.Itoa(card.DeviceCount)},
				{"Online", strconv.Itoa(card.OnlineCount)},
				{"Open Alerts", strconv.Itoa(card.OpenAlerts)},
				{"Patch OK %", strconv.FormatFloat(card.PatchOkPct, 'f', -1, 64)},
				{"AV OK %", strconv.FormatFloat(card.AvOkPct, 'f', -1, 64)},
				{"Warranty <90d", strconv.Itoa(card.WarrantyExpiring90)},
				{"Agents Behind", strconv.Itoa(card.AgentsBehind)},
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
