// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: unbilled.

// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type unbilledRow struct {
	Account     string  `json:"account"`
	Hours       float64 `json:"hours"`
	Entries     int     `json:"entries"`
	OldestStart string  `json:"oldest_start,omitempty"`
}

type unbilledView struct {
	Items        []unbilledRow `json:"items"`
	TotalHours   float64       `json:"total_hours"`
	TotalEntries int           `json:"total_entries"`
	Note         string        `json:"note,omitempty"`
}

// aggregateUnbilled sums billable, approved, not-yet-billed time logs per
// account. The BMS time-log DTO carries no rate fields, so the rollup is
// hours-based; pair it with contract rate cards for dollars. Pure function
// for table-driven tests.
func aggregateUnbilled(timelogRows []map[string]any, unit string, requireApproved bool) unbilledView {
	view := unbilledView{Items: []unbilledRow{}}
	byAccount := map[string]*unbilledRow{}
	oldest := map[string]time.Time{}

	for _, m := range timelogRows {
		if !kbmsBool(m, "IsBillable") || kbmsBool(m, "IsBilled") {
			continue
		}
		if requireApproved && !kbmsBool(m, "IsApproved") {
			continue
		}
		account := kbmsStr(m, "AccountName")
		if account == "" {
			account = "(no account)"
		}
		raw, ok := kbmsNum(m, "Timespent")
		if !ok {
			continue
		}
		row, exists := byAccount[account]
		if !exists {
			row = &unbilledRow{Account: account}
			byAccount[account] = row
		}
		hours := kbmsHoursFromTimespent(raw, unit)
		row.Hours += hours
		row.Entries++
		view.TotalHours += hours
		view.TotalEntries++
		if started, ok := kbmsTime(m, "StartDate"); ok {
			if cur, seen := oldest[account]; !seen || started.Before(cur) {
				oldest[account] = started
			}
		}
	}

	for _, account := range kbmsSortedKeys(byAccount) {
		row := byAccount[account]
		row.Hours = kbmsRound2(row.Hours)
		if t, ok := oldest[account]; ok {
			row.OldestStart = t.Format("2006-01-02")
		}
		view.Items = append(view.Items, *row)
	}
	sort.Slice(view.Items, func(i, j int) bool {
		if view.Items[i].Hours != view.Items[j].Hours {
			return view.Items[i].Hours > view.Items[j].Hours
		}
		return view.Items[i].Account < view.Items[j].Account
	})
	view.TotalHours = kbmsRound2(view.TotalHours)
	if view.TotalEntries == 0 {
		view.Note = "no billable, unbilled time logs in the local mirror; run 'sync --resources timelogs' to refresh (or pass --include-unapproved)"
	}
	return view
}

func newNovelUnbilledCmd(flags *rootFlags) *cobra.Command {
	var unit string
	var includeUnapproved bool
	var dbPath string

	cmd := &cobra.Command{
		Use:   "unbilled",
		Short: "Billable, approved, not-yet-billed time grouped by account - the month-end ready-to-bill review in hours.",
		Long: strings.Trim(`
Use this command for billable-but-unbilled hours grouped by account.
Do NOT use this command for prepaid-hour depletion; use 'contract-burn' instead.

The BMS time-log API carries no rate fields, so the rollup is hours-based;
multiply by your contract rate cards for dollar figures.`, "\n"),
		Example: strings.Trim(`
  # What is ready to bill right now
  kaseya-bms-cli unbilled --agent

  # Include time still waiting on approval
  kaseya-bms-cli unbilled --include-unapproved --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would sum billable, unbilled time logs per account from the local mirror")
				return nil
			}
			if err := kbmsRejectLiveSource(flags, "unbilled"); err != nil {
				return err
			}
			if dbPath == "" {
				dbPath = defaultDBPath("kaseya-bms-cli")
			}
			db, err := kbmsOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "timelogs") {
				hintIfStale(cmd, db, "timelogs", flags.maxAge)
			}
			rows, err := kbmsRows(cmd.Context(), db, "timelogs")
			if err != nil {
				return fmt.Errorf("querying time logs: %w", err)
			}
			view := aggregateUnbilled(rows, unit, !includeUnapproved)
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&unit, "timespent-unit", "minutes", "Unit BMS uses for Timespent values: minutes, hours, or seconds")
	cmd.Flags().BoolVar(&includeUnapproved, "include-unapproved", false, "Also count billable time that has not been approved yet")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/kaseya-bms-cli/data.db)")
	return cmd
}
