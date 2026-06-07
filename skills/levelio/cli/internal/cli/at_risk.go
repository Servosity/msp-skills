// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type atRiskDevice struct {
	Rank               int      `json:"rank"`
	Hostname           string   `json:"hostname"`
	DeviceID           string   `json:"device_id"`
	GroupID            string   `json:"group_id,omitempty"`
	RiskScore          float64  `json:"risk_score"`
	OpenAlerts         int      `json:"open_alerts"`
	AlertSeverityScore int      `json:"alert_severity_score"`
	PendingUpdates     int      `json:"pending_updates"`
	SecurityScore      *int     `json:"security_score"`
	DaysDark           float64  `json:"days_dark"`
	Online             bool     `json:"online"`
	Reasons            []string `json:"reasons"`
}

type atRiskResult struct {
	Top              int            `json:"top"`
	GroupFilter      string         `json:"group_filter,omitempty"`
	EvaluatedDevices int            `json:"evaluated_devices"`
	Count            int            `json:"count"`
	Devices          []atRiskDevice `json:"devices"`
}

// lvlComputeAtRisk ranks devices by a weighted composite of open alerts, pending
// updates, low security score, staleness, and being offline.
func lvlComputeAtRisk(devices []lvlDevice, alerts []lvlAlert, updates []lvlUpdate, groups []lvlGroup, top int, groupFilter string, now time.Time) atRiskResult {
	res := atRiskResult{Top: top, GroupFilter: groupFilter}

	// Per-device open-alert aggregation.
	type alertAgg struct {
		open int
		sev  int
	}
	alertsByDev := map[string]*alertAgg{}
	for _, a := range alerts {
		if a.IsResolved || a.DeviceID == "" {
			continue
		}
		x, ok := alertsByDev[a.DeviceID]
		if !ok {
			x = &alertAgg{}
			alertsByDev[a.DeviceID] = x
		}
		x.open++
		x.sev += lvlSeverityWeight(a.Severity)
	}

	// Per-device pending-update count.
	pendingByDev := map[string]int{}
	for _, u := range updates {
		if u.DeviceID != "" && patchState(u) == "pending" {
			pendingByDev[u.DeviceID]++
		}
	}

	var scope map[string]bool
	if groupFilter != "" {
		idx := lvlBuildGroupIndex(groups)
		scope = idx.descendants(groupFilter)
	}

	for _, d := range devices {
		if scope != nil && !scope[d.GroupID] {
			continue
		}
		res.EvaluatedDevices++

		var score float64
		var reasons []string

		openCount, sevScore := 0, 0
		if x := alertsByDev[d.ID]; x != nil {
			openCount, sevScore = x.open, x.sev
		}
		if openCount > 0 {
			score += float64(sevScore)
			reasons = append(reasons, fmt.Sprintf("%d open alert(s) (severity %d)", openCount, sevScore))
		}
		pending := pendingByDev[d.ID]
		if pending > 0 {
			add := math.Min(float64(pending), 10)
			score += add
			reasons = append(reasons, fmt.Sprintf("%d pending update(s)", pending))
		}
		if d.SecurityScore != nil && *d.SecurityScore < 80 {
			add := math.Min(float64(80-*d.SecurityScore)/10.0, 8)
			score += add
			reasons = append(reasons, fmt.Sprintf("security score %d", *d.SecurityScore))
		}
		daysDark := 0.0
		if dd, ok := lvlDaysDark(d, now); ok {
			daysDark = round1(dd)
			if dd >= 7 {
				add := math.Min(dd/7.0, 5)
				score += add
				reasons = append(reasons, fmt.Sprintf("dark %.1fd", dd))
			}
		}
		if !d.Online {
			score += 2
			reasons = append(reasons, "offline")
		}

		if score <= 0 {
			continue
		}
		res.Devices = append(res.Devices, atRiskDevice{
			Hostname:           lvlDeviceLabel(d),
			DeviceID:           d.ID,
			GroupID:            d.GroupID,
			RiskScore:          round1(score),
			OpenAlerts:         openCount,
			AlertSeverityScore: sevScore,
			PendingUpdates:     pending,
			SecurityScore:      d.SecurityScore,
			DaysDark:           daysDark,
			Online:             d.Online,
			Reasons:            reasons,
		})
	}

	sort.SliceStable(res.Devices, func(i, j int) bool {
		if res.Devices[i].RiskScore != res.Devices[j].RiskScore {
			return res.Devices[i].RiskScore > res.Devices[j].RiskScore
		}
		if res.Devices[i].OpenAlerts != res.Devices[j].OpenAlerts {
			return res.Devices[i].OpenAlerts > res.Devices[j].OpenAlerts
		}
		return res.Devices[i].Hostname < res.Devices[j].Hostname
	})
	if top > 0 && len(res.Devices) > top {
		res.Devices = res.Devices[:top]
	}
	for i := range res.Devices {
		res.Devices[i].Rank = i + 1
	}
	res.Count = len(res.Devices)
	return res
}

// pp:data-source local
func newNovelAtRiskCmd(flags *rootFlags) *cobra.Command {
	var top int
	var groupFilter string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "at-risk",
		Short:       "Rank the worst endpoints by a composite risk score (alerts + patches + score + staleness)",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Rank devices by a single weighted risk score that combines open alerts
(weighted by severity), pending OS updates, low security score, how long the
device has been dark, and whether it is offline — computed offline from the
local store. Each result lists the reasons that drove its score. Use --top to
cap the list and --group to scope to a group and its descendants.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # The 20 worst endpoints across every axis
  levelio-cli at-risk --top 20

  # Worst endpoints inside one group subtree, JSON for agents
  levelio-cli at-risk --top 10 --group <group-id> --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("levelio-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w\nRun 'levelio-cli sync' first.", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "devices") {
				hintIfStale(cmd, db, "devices", flags.maxAge)
			}

			devices, err := lvlDevices(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			alerts, err := lvlAlerts(db)
			if err != nil {
				return fmt.Errorf("loading alerts: %w", err)
			}
			updates, err := lvlUpdates(db)
			if err != nil {
				return fmt.Errorf("loading updates: %w", err)
			}
			groups, err := lvlGroups(db)
			if err != nil {
				return fmt.Errorf("loading groups: %w", err)
			}
			res := lvlComputeAtRisk(devices, alerts, updates, groups, top, groupFilter, time.Now().UTC())

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "%d at-risk device(s) of %d evaluated\n", res.Count, res.EvaluatedDevices)
			if res.Count == 0 {
				return nil
			}
			fmt.Fprintln(out, "RANK\tRISK\tALERTS\tPENDING\tONLINE\tHOSTNAME\tREASONS")
			for _, d := range res.Devices {
				fmt.Fprintf(out, "%d\t%.1f\t%d\t%d\t%t\t%s\t%s\n",
					d.Rank, d.RiskScore, d.OpenAlerts, d.PendingUpdates, d.Online, d.Hostname, strings.Join(d.Reasons, "; "))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&top, "top", 20, "Show at most this many highest-risk devices (0 = all)")
	cmd.Flags().StringVar(&groupFilter, "group", "", "Scope to this group id and its descendants")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
