// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type patchCategory struct {
	Category  string `json:"category"`
	Pending   int    `json:"pending"`
	Installed int    `json:"installed"`
	Errored   int    `json:"errored"`
	Total     int    `json:"total"`
}

type patchSummary struct {
	TotalUpdates       int `json:"total_updates"`
	Pending            int `json:"pending"`
	Installed          int `json:"installed"`
	Errored            int `json:"errored"`
	DevicesWithPending int `json:"devices_with_pending"`
}

type patchResult struct {
	CategoryFilter string             `json:"category_filter,omitempty"`
	Summary        patchSummary       `json:"summary"`
	ByCategory     []patchCategory    `json:"by_category"`
	Errors         []patchErrorDetail `json:"errors,omitempty"`
}

// patchErrorDetail is one errored update install — the remediation fix-list row.
type patchErrorDetail struct {
	Hostname string `json:"hostname"`
	DeviceID string `json:"device_id"`
	Update   string `json:"update"`
	Category string `json:"category"`
	Error    string `json:"error"`
}

// patchState classifies a single update into one of pending/installed/errored.
func patchState(u lvlUpdate) string {
	switch {
	case strings.TrimSpace(u.Error) != "":
		return "errored"
	case strings.TrimSpace(u.InstalledOn) != "":
		return "installed"
	case u.IsAvailable:
		return "pending"
	default:
		return "other"
	}
}

// lvlComputePatchPosture aggregates OS updates into pending/installed/errored
// counts overall and by category.
func lvlComputePatchPosture(updates []lvlUpdate, categoryFilter string, includeErrors bool) patchResult {
	res := patchResult{CategoryFilter: categoryFilter}
	cf := strings.ToLower(strings.TrimSpace(categoryFilter))

	type agg struct{ pending, installed, errored, total int }
	cats := map[string]*agg{}
	order := []string{}
	pendingDevices := map[string]bool{}

	for _, u := range updates {
		if cf != "" && !strings.Contains(strings.ToLower(u.Category), cf) {
			continue
		}
		cat := orUnknown(u.Category)
		a, ok := cats[cat]
		if !ok {
			a = &agg{}
			cats[cat] = a
			order = append(order, cat)
		}
		a.total++
		res.Summary.TotalUpdates++
		switch patchState(u) {
		case "pending":
			a.pending++
			res.Summary.Pending++
			if u.DeviceID != "" {
				pendingDevices[u.DeviceID] = true
			}
		case "installed":
			a.installed++
			res.Summary.Installed++
		case "errored":
			a.errored++
			res.Summary.Errored++
			if includeErrors {
				res.Errors = append(res.Errors, patchErrorDetail{
					Hostname: orUnknown(u.DeviceHostname),
					DeviceID: u.DeviceID,
					Update:   u.Name,
					Category: orUnknown(u.Category),
					Error:    u.Error,
				})
			}
		}
	}
	res.Summary.DevicesWithPending = len(pendingDevices)

	for _, c := range order {
		a := cats[c]
		res.ByCategory = append(res.ByCategory, patchCategory{
			Category: c, Pending: a.pending, Installed: a.installed, Errored: a.errored, Total: a.total,
		})
	}
	sort.SliceStable(res.ByCategory, func(i, j int) bool {
		if res.ByCategory[i].Pending != res.ByCategory[j].Pending {
			return res.ByCategory[i].Pending > res.ByCategory[j].Pending
		}
		return res.ByCategory[i].Category < res.ByCategory[j].Category
	})
	sort.SliceStable(res.Errors, func(i, j int) bool {
		if res.Errors[i].Hostname != res.Errors[j].Hostname {
			return res.Errors[i].Hostname < res.Errors[j].Hostname
		}
		return res.Errors[i].Update < res.Errors[j].Update
	})
	return res
}

// pp:data-source local
func newNovelPatchPostureCmd(flags *rootFlags) *cobra.Command {
	var category string
	var showErrors bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "patch-posture",
		Short:       "Aggregate OS updates across the fleet: pending vs installed vs errored, by category",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Roll up the synced OS-update inventory into pending, installed, and
errored counts overall and per category, computed offline from the local store.
Pending = available and not yet installed; installed = has an installed-on date;
errored = the install reported an error. Narrow to one category with --category
(case-insensitive substring, e.g. "security"). Add --errors to list each errored
install (device, update, error text) as a remediation fix-list.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Fleet-wide patch posture
  levelio-cli patch-posture

  # Only security updates, JSON for agents
  levelio-cli patch-posture --category security --agent

  # The errored-install fix-list
  levelio-cli patch-posture --errors --agent
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
			if !hintIfUnsynced(cmd, db, "updates") {
				hintIfStale(cmd, db, "updates", flags.maxAge)
			}

			updates, err := lvlUpdates(db)
			if err != nil {
				return fmt.Errorf("loading updates: %w", err)
			}
			res := lvlComputePatchPosture(updates, category, showErrors)

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			s := res.Summary
			fmt.Fprintf(out, "%d update(s): %d pending, %d installed, %d errored across %d device(s) with pending\n",
				s.TotalUpdates, s.Pending, s.Installed, s.Errored, s.DevicesWithPending)
			if len(res.ByCategory) == 0 {
				return nil
			}
			fmt.Fprintln(out, "PENDING\tINSTALLED\tERRORED\tCATEGORY")
			for _, c := range res.ByCategory {
				fmt.Fprintf(out, "%d\t%d\t%d\t%s\n", c.Pending, c.Installed, c.Errored, c.Category)
			}
			if showErrors && len(res.Errors) > 0 {
				fmt.Fprintf(out, "\n%d errored install(s):\n", len(res.Errors))
				fmt.Fprintln(out, "DEVICE\tUPDATE\tERROR")
				for _, e := range res.Errors {
					fmt.Fprintf(out, "%s\t%s\t%s\n", e.Hostname, e.Update, e.Error)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "Filter to updates whose category matches this substring (e.g. security)")
	cmd.Flags().BoolVar(&showErrors, "errors", false, "List each errored install (device, update, error) as a remediation fix-list")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
