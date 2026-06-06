// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newNovelAccountBriefCmd reports what CHANGED on one account since a point in
// time — the pre-call delta that company-360's full snapshot cannot answer.
// pp:data-source local
func newNovelAccountBriefCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var dbPath string
	cmd := &cobra.Command{
		Use:   "account-brief [companyID]",
		Short: "What changed on one account since a point in time — tickets, opportunities, contract risk.",
		Long: `Time-window a single company's cross-entity activity against lastActivityDate: tickets opened or touched in the window, opportunities created or moved, and contracts running hot. Run ` + "`sync`" + ` first.

Use this command for what CHANGED on an account since a point in time. Do NOT use it for the full current snapshot; use 'company-360'.`,
		Example: strings.Trim(`
  autotask-cli account-brief 1234
  autotask-cli account-brief 1234 --since 7d --agent
  autotask-cli account-brief 1234 --since 30d --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("company id is required"))
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			companyID := strings.TrimSpace(args[0])
			if _, err := strconv.Atoi(companyID); err != nil {
				return usageErr(fmt.Errorf("company id must be an integer, got %q", companyID))
			}
			window := flagSince
			if strings.TrimSpace(window) == "" {
				window = "7d"
			}
			dur, ok := parseNovelDuration(window)
			if !ok {
				return usageErr(fmt.Errorf("invalid --since %q (use forms like 24h, 7d, 2w, or a bare integer for days)", window))
			}
			cutoff := time.Now().Add(-dur)

			db, err := openNovelStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "tickets") {
				hintIfStale(cmd, db, "tickets", flags.maxAge)
			}
			if !hintIfUnsynced(cmd, db, "opportunities") {
				hintIfStale(cmd, db, "opportunities", flags.maxAge)
			}

			tickets, err := listEntity(db, "tickets")
			if err != nil {
				return apiErr(err)
			}
			opps, _ := listEntity(db, "opportunities")
			contracts, _ := listEntity(db, "contracts")
			blocks, _ := listEntity(db, "contract-blocks")

			type ticketRow struct {
				ID           int64  `json:"id"`
				Title        string `json:"title,omitempty"`
				Status       string `json:"status,omitempty"`
				Created      string `json:"createDate,omitempty"`
				LastActivity string `json:"lastActivityDate,omitempty"`
			}
			var newTickets, touchedTickets []ticketRow
			openCount := 0
			for _, t := range tickets {
				if strAt(t, "companyID", "companyId") != companyID {
					continue
				}
				if isTicketOpen(t) {
					openCount++
				}
				id, _ := intAt(t, "id")
				row := ticketRow{
					ID:           id,
					Title:        strAt(t, "title"),
					Status:       strAt(t, "status"),
					Created:      strAt(t, "createDate", "createdDate"),
					LastActivity: strAt(t, "lastActivityDate", "lastActivityDateTime"),
				}
				if created, ok := timeAt(t, "createDate", "createdDate"); ok && !created.Before(cutoff) {
					newTickets = append(newTickets, row)
					continue
				}
				if la, ok := timeAt(t, "lastActivityDate", "lastActivityDateTime"); ok && !la.Before(cutoff) {
					touchedTickets = append(touchedTickets, row)
				}
			}
			sort.Slice(newTickets, func(i, j int) bool { return newTickets[i].Created > newTickets[j].Created })
			sort.Slice(touchedTickets, func(i, j int) bool { return touchedTickets[i].LastActivity > touchedTickets[j].LastActivity })

			type oppRow struct {
				ID    int64  `json:"id"`
				Title string `json:"title,omitempty"`
				Stage string `json:"stage,omitempty"`
			}
			var movedOpps []oppRow
			for _, o := range opps {
				if strAt(o, "companyID", "companyId") != companyID {
					continue
				}
				la, ok := timeAt(o, "lastActivityDate", "lastActivityDateTime", "createDate")
				if !ok || la.Before(cutoff) {
					continue
				}
				id, _ := intAt(o, "id")
				movedOpps = append(movedOpps, oppRow{ID: id, Title: strAt(o, "title"), Stage: strAt(o, "stage")})
			}

			// Contracts running hot for this company: blocks >=80% consumed.
			type hotContract struct {
				ContractID    string  `json:"contractID"`
				PercentBurned float64 `json:"percentBurned"`
			}
			companyContracts := map[string]bool{}
			for _, c := range contracts {
				if strAt(c, "companyID", "companyId") == companyID {
					if cid := strAt(c, "id"); cid != "" {
						companyContracts[cid] = true
					}
				}
			}
			type burnAcc struct{ purchased, used float64 }
			burnByContract := map[string]*burnAcc{}
			for _, b := range blocks {
				cid := strAt(b, "contractID", "contractId")
				if !companyContracts[cid] {
					continue
				}
				if burnByContract[cid] == nil {
					burnByContract[cid] = &burnAcc{}
				}
				acc := burnByContract[cid]
				purchased, used, _ := accrueBlock(b)
				acc.purchased += purchased
				acc.used += used
			}
			var hot []hotContract
			for cid, acc := range burnByContract {
				if acc.purchased <= 0 {
					continue
				}
				pct := (acc.used / acc.purchased) * 100
				if pct >= 80 {
					hot = append(hot, hotContract{ContractID: cid, PercentBurned: pct})
				}
			}
			sort.Slice(hot, func(i, j int) bool { return hot[i].PercentBurned > hot[j].PercentBurned })

			out := map[string]any{
				"companyID": companyID,
				"since":     window,
				"cutoff":    cutoff.Format(time.RFC3339),
				"summary": map[string]int{
					"newTickets":          len(newTickets),
					"touchedTickets":      len(touchedTickets),
					"openTickets":         openCount,
					"opportunitiesMoved":  len(movedOpps),
					"contractsRunningHot": len(hot),
				},
				"newTickets":          newTickets,
				"touchedTickets":      touchedTickets,
				"opportunitiesMoved":  movedOpps,
				"contractsRunningHot": hot,
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "", "change window: 24h, 7d (default), 2w, or a bare integer of days")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/autotask-cli/data.db)")
	return cmd
}
