// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature: cross-company subscription change radar over
// the local subscriptions table. Originally scaffolded by the CLI Printing
// Press; body is hand-authored.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"appdirect-pp-cli/internal/cliutil"
	"appdirect-pp-cli/internal/store"
)

// pp:data-source local

type subChange struct {
	SubscriptionID string `json:"subscriptionId"`
	CompanyID      string `json:"companyId,omitempty"`
	CompanyName    string `json:"companyName,omitempty"`
	Product        string `json:"product,omitempty"`
	Status         string `json:"status"`
	CreatedOn      string `json:"createdOn,omitempty"`
	EndDate        string `json:"endDate,omitempty"`
}

type subsChangedView struct {
	Since                string         `json:"since"`
	Created              []subChange    `json:"created"`
	Ended                []subChange    `json:"ended"`
	Inactive             []subChange    `json:"inactive"`
	Counts               map[string]int `json:"counts"`
	ScannedSubscriptions int            `json:"scanned_subscriptions"`
	Note                 string         `json:"note,omitempty"`
}

func newNovelSubsChangedCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "changed",
		Short: "See subscriptions created, ended, or in an inactive status across all companies in a time window",
		Long: strings.TrimSpace(`
Use this command to see subscription lifecycle changes across all companies in
a time window, from the locally synced billing-v1-subscriptions table:

  - created:  subscriptions whose creationDate falls inside the window
  - ended:    subscriptions whose endDate falls inside the window
  - inactive: subscriptions currently SUSPENDED, CANCELLED, FAILED, or
              FREE_TRIAL_EXPIRED whose endDate (when present) is on or after
              the window start - including cancellations scheduled to take
              effect in the future

Do NOT use this command to reconcile billing; use 'reconcile' instead.
Run 'sync --resources billing-v1-subscriptions' first to populate the local
store.`),
		Example: strings.Trim(`
  # What changed this week across every company
  appdirect-cli subs changed --since 7d --json

  # Month-scale churn review, narrowed for agents
  appdirect-cli subs changed --since 30d --agent --select counts,inactive.companyName,inactive.status
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would diff local subscriptions by creationDate, endDate, and status")
				return nil
			}
			if flagSince == "" {
				flagSince = "7d"
			}
			sinceDur, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--since: %w", err))
			}
			now := time.Now().UTC()
			cutoff := now.Add(-sinceDur)

			if dbPath == "" {
				dbPath = defaultDBPath("appdirect-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtSubscriptions) {
				hintIfStale(cmd, db, rtSubscriptions, flags.maxAge)
			}

			subs, err := loadResourceObjects(cmd.Context(), db, rtSubscriptions)
			if err != nil {
				return err
			}
			companies, err := loadResourceObjects(cmd.Context(), db, rtCompanies)
			if err != nil {
				return err
			}
			names := companyNameIndex(companies)

			toChange := func(sub map[string]any) subChange {
				companyID := novelRefID(sub, "company")
				name := names[companyID]
				if name == "" {
					name = novelRefName(sub, "company")
				}
				created, _ := novelEpochMS(sub, "creationDate")
				end, _ := novelEpochMS(sub, "endDate")
				return subChange{
					SubscriptionID: novelStr(sub, "id"),
					CompanyID:      companyID,
					CompanyName:    name,
					Product:        novelRefName(sub, "product"),
					Status:         strings.ToUpper(novelStr(sub, "status")),
					CreatedOn:      isoOrEmpty(created),
					EndDate:        isoOrEmpty(end),
				}
			}

			inactiveStatuses := map[string]bool{
				"SUSPENDED": true, "CANCELLED": true,
				"FAILED": true, "FREE_TRIAL_EXPIRED": true,
			}

			created := make([]subChange, 0)
			ended := make([]subChange, 0)
			inactive := make([]subChange, 0)
			for _, sub := range subs {
				createdOn, hasCreated := novelEpochMS(sub, "creationDate")
				endDate, hasEnd := novelEpochMS(sub, "endDate")
				status := strings.ToUpper(novelStr(sub, "status"))

				if hasCreated && !createdOn.Before(cutoff) {
					created = append(created, toChange(sub))
				}
				if hasEnd && !endDate.Before(cutoff) && !endDate.After(now) {
					ended = append(ended, toChange(sub))
				}
				if inactiveStatuses[status] {
					// Include subs whose end is on or after the window start —
					// covering cancellations scheduled to take effect in the
					// future — and subs with no endDate at all.
					if !hasEnd || !endDate.Before(cutoff) {
						inactive = append(inactive, toChange(sub))
					}
				}
			}
			sort.SliceStable(created, func(i, j int) bool { return created[i].CreatedOn > created[j].CreatedOn })
			sort.SliceStable(ended, func(i, j int) bool { return ended[i].EndDate > ended[j].EndDate })
			sort.SliceStable(inactive, func(i, j int) bool { return inactive[i].Status < inactive[j].Status })

			view := subsChangedView{
				Since:    flagSince,
				Created:  created,
				Ended:    ended,
				Inactive: inactive,
				Counts: map[string]int{
					"created": len(created), "ended": len(ended), "inactive": len(inactive),
				},
				ScannedSubscriptions: len(subs),
			}
			if len(created)+len(ended)+len(inactive) == 0 {
				view.Note = fmt.Sprintf("scanned %d subscriptions with no lifecycle changes in the %s window; widen with --since or refresh with 'sync --resources billing-v1-subscriptions'", len(subs), flagSince)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Change window (e.g. 7d, 30d, 12h)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
