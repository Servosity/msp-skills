// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored novel feature: bulk-resolve the open alerts on the noisiest
// devices surfaced by the `fleet storms` ranking. Reads the ranking from the
// local store, then issues gated per-alert resolve calls against the live API.
// Default is plan-only; nothing resolves without --confirm.
package cli

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"datto-rmm-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type resolveStormDevice struct {
	DeviceUID  string   `json:"deviceUid"`
	DeviceName string   `json:"deviceName"`
	SiteName   string   `json:"siteName"`
	AlertCount int      `json:"alertCount"`
	AlertUIDs  []string `json:"alertUids"`
}

type resolveStormPlan struct {
	Devices     []resolveStormDevice `json:"devices"`
	TotalAlerts int                  `json:"totalAlerts"`
	Confirmed   bool                 `json:"confirmed"`
	Note        string               `json:"note,omitempty"`
}

type resolveStormFailure struct {
	AlertUID string `json:"alertUid"`
	Error    string `json:"error"`
}

type resolveStormResult struct {
	Resolved int                   `json:"resolved"`
	Failed   int                   `json:"failed"`
	Failures []resolveStormFailure `json:"failures,omitempty"`
}

// computeResolvePlan ranks devices by open-alert count within the window and
// returns the top devices meeting minCount, with the alert UIDs to resolve.
// Alerts with unparseable timestamps are counted (same posture as
// computeStorms): an alert that can't prove it is outside the window is
// treated as in it.
func computeResolvePlan(alerts []fleetAlert, days, top, minCount int, now time.Time) resolveStormPlan {
	type agg struct {
		name  string
		site  string
		uids  []string
		count int
	}
	groups := map[string]*agg{}
	cutoff := now.AddDate(0, 0, -days)
	for _, a := range alerts {
		if a.Resolved || a.AlertUID == "" || a.AlertSourceInfo.DeviceUID == "" {
			continue
		}
		if t, ok := parseDattoTime(a.Timestamp); ok && days > 0 && t.Before(cutoff) {
			continue
		}
		g := groups[a.AlertSourceInfo.DeviceUID]
		if g == nil {
			g = &agg{name: a.AlertSourceInfo.DeviceName, site: a.AlertSourceInfo.SiteName}
			groups[a.AlertSourceInfo.DeviceUID] = g
		}
		g.count++
		g.uids = append(g.uids, a.AlertUID)
	}

	devices := make([]resolveStormDevice, 0, len(groups))
	for uid, g := range groups {
		if g.count < minCount {
			continue
		}
		sort.Strings(g.uids)
		devices = append(devices, resolveStormDevice{
			DeviceUID:  uid,
			DeviceName: g.name,
			SiteName:   g.site,
			AlertCount: g.count,
			AlertUIDs:  g.uids,
		})
	}
	sort.SliceStable(devices, func(i, j int) bool {
		if devices[i].AlertCount != devices[j].AlertCount {
			return devices[i].AlertCount > devices[j].AlertCount
		}
		return devices[i].DeviceUID < devices[j].DeviceUID
	})
	if top > 0 && len(devices) > top {
		devices = devices[:top]
	}
	total := 0
	for _, d := range devices {
		total += d.AlertCount
	}
	return resolveStormPlan{Devices: devices, TotalAlerts: total}
}

// pp:data-source local
func newNovelFleetResolveStormCmd(flags *rootFlags) *cobra.Command {
	var days int
	var top int
	var minCount int
	var confirm bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "resolve-storm",
		Short: "Bulk-resolve the open alerts on the noisiest devices from the storm ranking",
		Long: strings.TrimSpace(`
Use this command to bulk-resolve open alerts on the noisiest devices/sites from the ranking.
Do NOT use it to only view the ranking; use 'fleet storms' instead.
Do NOT use it to resolve one known alert; use 'alert resolve' instead.

Default is plan-only: it prints which devices and how many alerts would be
resolved. Nothing is resolved without --confirm.`),
		Example: `  datto-rmm-cli fleet resolve-storm --days 7 --top 10
  datto-rmm-cli fleet resolve-storm --days 7 --top 5 --confirm --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would rank the noisiest devices from local open alerts and resolve their alerts (plan-only without --confirm)")
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

			if !hintIfUnsynced(cmd, db, fleetAlertsOpenResource) {
				hintIfStale(cmd, db, fleetAlertsOpenResource, flags.maxAge)
			}

			alerts, err := loadFleetAlerts(cmd.Context(), db)
			if err != nil {
				return err
			}
			plan := computeResolvePlan(alerts, days, top, minCount, time.Now().UTC())
			plan.Confirmed = confirm

			if !confirm {
				plan.Note = fmt.Sprintf("plan only: %d alerts across %d devices would be resolved; pass --confirm to resolve them", plan.TotalAlerts, len(plan.Devices))
				if shouldPrintJSON(cmd, flags) {
					return flags.printJSON(cmd, plan)
				}
				headers := []string{"DEVICE", "SITE", "OPEN ALERTS"}
				rows := make([][]string, 0, len(plan.Devices))
				for _, d := range plan.Devices {
					rows = append(rows, []string{d.DeviceName, d.SiteName, fmt.Sprintf("%d", d.AlertCount)})
				}
				if err := flags.printTable(cmd, headers, rows); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), plan.Note)
				return nil
			}

			// Side-effect floor: never dial out under the verifier.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "would resolve %d alerts across %d devices\n", plan.TotalAlerts, len(plan.Devices))
				return nil
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			result := resolveStormResult{Failures: []resolveStormFailure{}}
			for _, d := range plan.Devices {
				for _, uid := range d.AlertUIDs {
					path := "/v2/alert/" + url.PathEscape(uid) + "/resolve"
					if _, _, err := c.PostWithParams(cmd.Context(), path, nil, map[string]any{}); err != nil {
						result.Failed++
						result.Failures = append(result.Failures, resolveStormFailure{AlertUID: uid, Error: err.Error()})
						continue
					}
					result.Resolved++
				}
			}
			if result.Failed > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d resolve calls failed; %d alerts resolved\n",
					result.Failed, result.Resolved+result.Failed, result.Resolved)
			}
			if shouldPrintJSON(cmd, flags) {
				return flags.printJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "resolved %d alerts (%d failed)\n", result.Resolved, result.Failed)
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "Alert window in days (0 = all open alerts)")
	cmd.Flags().IntVar(&top, "top", 10, "Resolve alerts on at most this many of the noisiest devices (0 = all)")
	cmd.Flags().IntVar(&minCount, "min-count", 5, "Only include devices with at least this many open alerts")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Actually resolve the alerts (default is plan-only)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
