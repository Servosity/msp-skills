// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type clientScore struct {
	Client           string   `json:"client"`
	GroupID          string   `json:"group_id,omitempty"`
	Devices          int      `json:"devices"`
	Online           int      `json:"online"`
	OnlinePct        float64  `json:"online_pct"`
	OpenCritical     int      `json:"open_critical"`
	AvgSecurityScore *float64 `json:"avg_security_score,omitempty"`
	ScoredDevices    int      `json:"scored_devices"`
	Stale            int      `json:"stale"`
	PendingUpdates   int      `json:"pending_updates"`

	// scoreSum accumulates security scores per bucket; unexported so it is
	// never marshaled. Keyed on the struct (not a name-keyed side map) so
	// two top-level groups sharing a display name cannot collide.
	scoreSum int
}

type clientScorecardResult struct {
	StaleDays int           `json:"stale_days"`
	SortBy    string        `json:"sort_by"`
	Clients   []clientScore `json:"clients"`
}

// lvlComputeClientScorecard rolls device, alert, and update metrics up to each
// top-level group (client): device count, online %, open critical/emergency
// alerts, average security score, stale count, and pending-update exposure.
func lvlComputeClientScorecard(devices []lvlDevice, groups []lvlGroup, alerts []lvlAlert, updates []lvlUpdate, staleDays int, sortBy string, now time.Time) clientScorecardResult {
	res := clientScorecardResult{StaleDays: staleDays, SortBy: sortBy}
	idx := lvlBuildGroupIndex(groups)

	// Map every group id to its top-level ancestor (client).
	topOf := map[string]string{}
	for _, g := range groups {
		if g.ParentID == "" {
			for id := range idx.descendants(g.ID) {
				topOf[id] = g.ID
			}
		}
	}
	// Groups whose parent chain never reaches a root (orphaned parents) fall
	// back to themselves so their devices are not silently dropped.
	for _, g := range groups {
		if _, ok := topOf[g.ID]; !ok {
			topOf[g.ID] = g.ID
		}
	}

	const noGroup = "(no group)"
	scores := map[string]*clientScore{}
	order := []string{}
	clientFor := func(groupID string) *clientScore {
		key := noGroup
		label := noGroup
		if groupID != "" {
			if top, ok := topOf[groupID]; ok {
				key = top
				label = idx.name(top)
			} else {
				// group_id references a group that was never synced: give it
				// its own bucket (labeled with the raw id) instead of
				// conflating it with genuinely ungrouped devices.
				key = groupID
				label = idx.name(groupID)
			}
		}
		s, ok := scores[key]
		if !ok {
			s = &clientScore{Client: label}
			if key != noGroup {
				s.GroupID = key
			}
			scores[key] = s
			order = append(order, key)
		}
		return s
	}

	deviceClient := map[string]*clientScore{}
	for _, d := range devices {
		s := clientFor(d.GroupID)
		deviceClient[d.ID] = s
		s.Devices++
		if d.Online {
			s.Online++
		}
		if d.SecurityScore != nil {
			s.ScoredDevices++
			s.scoreSum += *d.SecurityScore
		}
		if !d.MaintenanceMode {
			if days, ok := lvlDaysDark(d, now); ok && days >= float64(staleDays) {
				s.Stale++
			}
		}
	}
	for _, a := range alerts {
		if a.IsResolved || lvlSeverityWeight(a.Severity) < 3 {
			continue
		}
		if s, ok := deviceClient[a.DeviceID]; ok {
			s.OpenCritical++
		}
	}
	for _, u := range updates {
		if patchState(u) != "pending" {
			continue
		}
		if s, ok := deviceClient[u.DeviceID]; ok {
			s.PendingUpdates++
		}
	}

	for _, key := range order {
		s := scores[key]
		if s.Devices > 0 {
			s.OnlinePct = round1(float64(s.Online) / float64(s.Devices) * 100.0)
		}
		if s.ScoredDevices > 0 {
			avg := round1(float64(s.scoreSum) / float64(s.ScoredDevices))
			s.AvgSecurityScore = &avg
		}
		res.Clients = append(res.Clients, *s)
	}

	sort.SliceStable(res.Clients, func(i, j int) bool {
		a, b := res.Clients[i], res.Clients[j]
		switch sortBy {
		case "stale":
			if a.Stale != b.Stale {
				return a.Stale > b.Stale
			}
		case "devices":
			if a.Devices != b.Devices {
				return a.Devices > b.Devices
			}
		case "patches":
			if a.PendingUpdates != b.PendingUpdates {
				return a.PendingUpdates > b.PendingUpdates
			}
		case "online":
			if a.OnlinePct != b.OnlinePct {
				return a.OnlinePct < b.OnlinePct
			}
		case "score":
			av, bv := 101.0, 101.0
			if a.AvgSecurityScore != nil {
				av = *a.AvgSecurityScore
			}
			if b.AvgSecurityScore != nil {
				bv = *b.AvgSecurityScore
			}
			if av != bv {
				return av < bv
			}
		default: // criticals
			if a.OpenCritical != b.OpenCritical {
				return a.OpenCritical > b.OpenCritical
			}
		}
		return a.Client < b.Client
	})
	return res
}

// pp:data-source local
func newNovelClientScorecardCmd(flags *rootFlags) *cobra.Command {
	var staleDays int
	var sortBy string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "client-scorecard",
		Short:       "One row per top-level group (client): devices, online %, criticals, avg score, stale, patch exposure",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Roll device, alert, and update metrics up to each top-level group (client):
device count, online percentage, open critical/emergency alerts, average
security score, stale-device count, and pending-update exposure — the
QBR-ready per-client posture table, computed offline from the local store.
Genuinely ungrouped devices are reported under "(no group)"; devices whose
group was never synced get their own row labeled with the raw group id.

Use this command for a one-row-per-CLIENT (top-level group) posture rollup for
QBR reporting. Do NOT use it to render the full nested hierarchy (use
'group-tree') or a pure security-score distribution (use 'security-posture').

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Per-client posture, worst (most open criticals) first
  levelio-cli client-scorecard

  # Sort by patch exposure, JSON for agents
  levelio-cli client-scorecard --sort patches --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			out := cmd.OutOrStdout()
			switch sortBy {
			case "criticals", "stale", "devices", "patches", "online", "score":
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--sort must be one of criticals|stale|devices|patches|online|score, got %q", sortBy))
			}
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
			groups, err := lvlGroups(db)
			if err != nil {
				return fmt.Errorf("loading groups: %w", err)
			}
			alerts, err := lvlAlerts(db)
			if err != nil {
				return fmt.Errorf("loading alerts: %w", err)
			}
			updates, err := lvlUpdates(db)
			if err != nil {
				return fmt.Errorf("loading updates: %w", err)
			}
			res := lvlComputeClientScorecard(devices, groups, alerts, updates, staleDays, sortBy, time.Now().UTC())

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "%d client(s) (top-level groups), stale threshold %dd\n", len(res.Clients), res.StaleDays)
			if len(res.Clients) == 0 {
				return nil
			}
			fmt.Fprintln(out, "DEVICES\tONLINE%\tCRITICAL\tAVG_SCORE\tSTALE\tPENDING\tCLIENT")
			for _, c := range res.Clients {
				score := "-"
				if c.AvgSecurityScore != nil {
					score = fmt.Sprintf("%.1f", *c.AvgSecurityScore)
				}
				fmt.Fprintf(out, "%d\t%.1f%%\t%d\t%s\t%d\t%d\t%s\n",
					c.Devices, c.OnlinePct, c.OpenCritical, score, c.Stale, c.PendingUpdates, c.Client)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 14, "Days dark before a device counts as stale in the rollup")
	cmd.Flags().StringVar(&sortBy, "sort", "criticals", "Sort clients by: criticals|stale|devices|patches|online|score")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
