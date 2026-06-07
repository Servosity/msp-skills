// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type sinceItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	DeviceHostname string `json:"device_hostname,omitempty"`
	Severity       string `json:"severity,omitempty"`
	Category       string `json:"category,omitempty"`
	At             string `json:"at"`
}

type sinceSummary struct {
	NewAlerts        int `json:"new_alerts"`
	ResolvedAlerts   int `json:"resolved_alerts"`
	PublishedUpdates int `json:"published_updates"`
	InstalledUpdates int `json:"installed_updates"`
	ActiveDevices    int `json:"active_devices"`
}

type sinceResult struct {
	WindowHours      float64      `json:"window_hours"`
	Since            string       `json:"since"`
	Summary          sinceSummary `json:"summary"`
	NewAlerts        []sinceItem  `json:"new_alerts"`
	ResolvedAlerts   []sinceItem  `json:"resolved_alerts"`
	PublishedUpdates []sinceItem  `json:"published_updates"`
	InstalledUpdates []sinceItem  `json:"installed_updates"`
}

const sinceSampleCap = 50

func sortAndCap(items []sinceItem) []sinceItem {
	sort.SliceStable(items, func(i, j int) bool { return items[i].At > items[j].At })
	if len(items) > sinceSampleCap {
		return items[:sinceSampleCap]
	}
	return items
}

// lvlComputeSince reports alerts, updates, and device activity within a recent
// time window, computed offline from stored timestamps.
func lvlComputeSince(alerts []lvlAlert, updates []lvlUpdate, devices []lvlDevice, window time.Duration, now time.Time) sinceResult {
	cutoff := now.Add(-window)
	res := sinceResult{WindowHours: round1(window.Hours()), Since: cutoff.UTC().Format(time.RFC3339)}

	inWindow := func(ts string) bool {
		t, ok := lvlParseTime(ts)
		return ok && !t.Before(cutoff)
	}

	for _, a := range alerts {
		if inWindow(a.StartedAt) {
			res.Summary.NewAlerts++
			res.NewAlerts = append(res.NewAlerts, sinceItem{ID: a.ID, Name: a.Name, DeviceHostname: a.DeviceHostname, Severity: a.Severity, At: a.StartedAt})
		}
		if a.IsResolved && inWindow(a.ResolvedAt) {
			res.Summary.ResolvedAlerts++
			res.ResolvedAlerts = append(res.ResolvedAlerts, sinceItem{ID: a.ID, Name: a.Name, DeviceHostname: a.DeviceHostname, Severity: a.Severity, At: a.ResolvedAt})
		}
	}
	for _, u := range updates {
		if inWindow(u.PublishedOn) {
			res.Summary.PublishedUpdates++
			res.PublishedUpdates = append(res.PublishedUpdates, sinceItem{ID: u.ID, Name: u.Name, DeviceHostname: u.DeviceHostname, Category: u.Category, At: u.PublishedOn})
		}
		if inWindow(u.InstalledOn) {
			res.Summary.InstalledUpdates++
			res.InstalledUpdates = append(res.InstalledUpdates, sinceItem{ID: u.ID, Name: u.Name, DeviceHostname: u.DeviceHostname, Category: u.Category, At: u.InstalledOn})
		}
	}
	for _, d := range devices {
		if inWindow(d.LastSeenAt) {
			res.Summary.ActiveDevices++
		}
	}

	res.NewAlerts = sortAndCap(res.NewAlerts)
	res.ResolvedAlerts = sortAndCap(res.ResolvedAlerts)
	res.PublishedUpdates = sortAndCap(res.PublishedUpdates)
	res.InstalledUpdates = sortAndCap(res.InstalledUpdates)
	return res
}

// pp:data-source local
func newNovelSinceCmd(flags *rootFlags) *cobra.Command {
	var hours float64
	var days float64
	var dbPath string

	cmd := &cobra.Command{
		Use:         "since",
		Short:       "Show what changed in a recent window: new/resolved alerts, updates, device activity",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Report what moved in a recent time window — alerts that started or
resolved, updates published or installed, and how many devices checked in —
computed offline from stored timestamps. Set the window with --hours (default 24)
or --days. Lists are capped at 50 samples each; the summary holds full counts.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Everything that changed in the last 24 hours
  levelio-cli since --hours 24

  # The last 7 days, JSON for agents
  levelio-cli since --days 7 --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			window := time.Duration(hours * float64(time.Hour))
			if days > 0 {
				window = time.Duration(days * 24 * float64(time.Hour))
			}
			if window <= 0 {
				return fmt.Errorf("window must be positive: set --hours or --days")
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
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			alerts, err := lvlAlerts(db)
			if err != nil {
				return fmt.Errorf("loading alerts: %w", err)
			}
			updates, err := lvlUpdates(db)
			if err != nil {
				return fmt.Errorf("loading updates: %w", err)
			}
			devices, err := lvlDevices(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			res := lvlComputeSince(alerts, updates, devices, window, time.Now().UTC())

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			s := res.Summary
			fmt.Fprintf(out, "Since %s (%.0fh): %d new alert(s), %d resolved, %d update(s) published, %d installed, %d device(s) active\n",
				res.Since, res.WindowHours, s.NewAlerts, s.ResolvedAlerts, s.PublishedUpdates, s.InstalledUpdates, s.ActiveDevices)
			printSinceList(out, "New alerts", res.NewAlerts)
			printSinceList(out, "Published updates", res.PublishedUpdates)
			return nil
		},
	}
	cmd.Flags().Float64Var(&hours, "hours", 24, "Window size in hours")
	cmd.Flags().Float64Var(&days, "days", 0, "Window size in days (overrides --hours when > 0)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func printSinceList(out io.Writer, title string, items []sinceItem) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s:\n", title)
	for _, it := range items {
		host := it.DeviceHostname
		if host != "" {
			host = "\t" + host
		}
		fmt.Fprintf(out, "  %s\t%s%s\n", it.At, strings.TrimSpace(it.Name), host)
	}
}
