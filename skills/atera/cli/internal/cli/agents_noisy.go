// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type noisyDevice struct {
	DeviceGuid    string `json:"DeviceGuid,omitempty"`
	MachineName   string `json:"MachineName"`
	CustomerName  string `json:"CustomerName"`
	AlertCount    int    `json:"AlertCount"`
	CriticalCount int    `json:"CriticalCount"`
	WarningCount  int    `json:"WarningCount"`
	LastAlert     string `json:"LastAlert"`
	// lastAlertT is the parsed instant backing LastAlert; comparing raw
	// strings breaks across mixed timestamp formats.
	lastAlertT time.Time
}

// pp:data-source local
func newNovelAgentsNoisyCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int
	var limit int
	var includeArchived bool

	cmd := &cobra.Command{
		Use:   "noisy",
		Short: "Rank the machines and customers generating the most alerts over a window — the chronic problem devices.",
		Long: "Use this command to find machines generating the most alerts over a window\n" +
			"(chronic problem devices). Do NOT use it to triage the current open-alert\n" +
			"queue; use 'alerts triage' instead.\n\n" +
			"Joins synced alerts to agents by DeviceGuid and ranks devices by alert volume\n" +
			"within --days — the recurring offenders a flat alert feed never surfaces.\n" +
			"Reads the local store; run `atera-cli sync` first.",
		Example:     "  atera-cli agents noisy --days 7 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("atera-cli")
			}
			s, nvOK, err := nvOpenRead(dbPath)
			if err != nil {
				return fmt.Errorf("opening store: %w", err)
			}
			if nvOK {
				defer s.Close()
			}

			if !hintIfUnsynced(cmd, s, "alerts") {
				hintIfStale(cmd, s, "alerts", flags.maxAge)
			}

			alerts, err := nvLoad(s, "alerts")
			if err != nil {
				return fmt.Errorf("loading alerts: %w", err)
			}
			agents, err := nvLoad(s, "agents")
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}

			// Index agent names by DeviceGuid for the join.
			machineByGuid := map[string]string{}
			customerByGuid := map[string]string{}
			for _, a := range agents {
				guid := nvStr(a, "DeviceGuid")
				if guid == "" {
					continue
				}
				machineByGuid[guid] = nvStr(a, "MachineName")
				customerByGuid[guid] = nvStr(a, "CustomerName")
			}

			now := nvNow()
			cutoff := now.AddDate(0, 0, -days)
			byDevice := map[string]*noisyDevice{}
			for _, o := range alerts {
				if !includeArchived && nvBool(o, "Archived") {
					continue
				}
				created, ok := nvTime(o, "Created")
				if !ok || created.Before(cutoff) {
					continue
				}
				// Group by DeviceGuid; fall back to the alert's own device/agent
				// identity when the guid is absent so no alert silently drops.
				key := nvStr(o, "DeviceGuid")
				machine := machineByGuid[key]
				customer := customerByGuid[key]
				if machine == "" {
					machine = nvStr(o, "DeviceName")
				}
				if customer == "" {
					customer = nvStr(o, "CustomerName")
				}
				if key == "" {
					if machine != "" {
						key = "device:" + machine
					} else if aid, ok := nvInt(o, "AgentId"); ok {
						key = fmt.Sprintf("agent:%d", aid)
						if machine == "" {
							machine = key
						}
					} else {
						key = "(unknown)"
						machine = "(unknown)"
					}
				}
				d := byDevice[key]
				if d == nil {
					d = &noisyDevice{
						DeviceGuid:   nvStr(o, "DeviceGuid"),
						MachineName:  machine,
						CustomerName: customer,
					}
					byDevice[key] = d
				}
				d.AlertCount++
				switch severityRank(nvStr(o, "Severity")) {
				case 3:
					d.CriticalCount++
				case 2:
					d.WarningCount++
				}
				if created.After(d.lastAlertT) {
					d.lastAlertT = created
					d.LastAlert = nvStr(o, "Created")
				}
			}

			results := make([]noisyDevice, 0, len(byDevice))
			for _, d := range byDevice {
				results = append(results, *d)
			}
			// Loudest first; criticals break ties.
			sort.SliceStable(results, func(i, j int) bool {
				if results[i].AlertCount != results[j].AlertCount {
					return results[i].AlertCount > results[j].AlertCount
				}
				return results[i].CriticalCount > results[j].CriticalCount
			})
			if limit > 0 && len(results) > limit {
				results = results[:limit]
			}

			return nvEmit(cmd, flags, results, func() {
				w := cmd.OutOrStdout()
				if len(results) == 0 {
					fmt.Fprintf(w, "No alerts in the last %d days.\n", days)
					return
				}
				rows := make([][]string, 0, len(results))
				for _, r := range results {
					rows = append(rows, []string{
						r.MachineName, r.CustomerName,
						fmt.Sprintf("%d", r.AlertCount),
						fmt.Sprintf("%d", r.CriticalCount),
						r.LastAlert,
					})
				}
				nvTable(w, []string{"MACHINE", "CUSTOMER", "ALERTS", "CRITICAL", "LAST"}, rows)
			})
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "Window in days — count alerts created within this many days")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum devices to return")
	cmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived alerts in the count (default: open only)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/atera-cli/data.db)")
	return cmd
}
