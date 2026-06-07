// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-implemented novel feature: stale open-opportunity detector over the
// local opportunities table. Originally scaffolded by the CLI Printing Press;
// body is hand-authored.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"time"

	"github.com/spf13/cobra"

	"appdirect-pp-cli/internal/store"
)

// pp:data-source local

type staleOpportunity struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	OwnerEmail    string `json:"ownerEmail,omitempty"`
	CustomerEmail string `json:"customerEmail,omitempty"`
	CreatedOn     string `json:"createdOn,omitempty"`
	AgeDays       int    `json:"ageDays"`
}

type pipelineStaleView struct {
	Days                 int                `json:"days"`
	Stale                []staleOpportunity `json:"stale"`
	TotalOpen            int                `json:"total_open"`
	ScannedOpportunities int                `json:"scanned_opportunities"`
	Note                 string             `json:"note,omitempty"`
}

func newNovelPipelineStaleCmd(flags *rootFlags) *cobra.Command {
	var flagDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Find open opportunities created more than N days ago, oldest first",
		Long: strings.TrimSpace(`
Use this command to find opportunities with no recent progress. It lists OPEN
opportunities from the locally synced assisted-sales-v1-opportunities table
whose creation date is older than --days, oldest first, so stalled deals
surface before they die quietly.

Do NOT use this command for total pipeline counts by status or owner; use
'pipeline' instead. Run 'sync --resources assisted-sales-v1-opportunities'
first to populate the local store.`),
		Example: strings.Trim(`
  # Open opportunities older than two weeks
  appdirect-cli pipeline stale --days 14 --json

  # Narrowed for agent triage
  appdirect-cli pipeline stale --days 30 --agent --select stale.name,stale.ownerEmail,stale.ageDays
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would list local OPEN opportunities older than the --days threshold")
				return nil
			}
			if flagDays <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--days must be a positive number of days, got %d", flagDays))
			}
			now := time.Now().UTC()
			threshold := now.Add(-time.Duration(flagDays) * 24 * time.Hour)

			if dbPath == "" {
				dbPath = defaultDBPath("appdirect-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, rtOpportunities) {
				hintIfStale(cmd, db, rtOpportunities, flags.maxAge)
			}

			opps, err := loadResourceObjects(cmd.Context(), db, rtOpportunities)
			if err != nil {
				return err
			}

			stale := make([]staleOpportunity, 0)
			totalOpen := 0
			for _, opp := range opps {
				if !strings.EqualFold(novelStr(opp, "status"), "OPEN") {
					continue
				}
				totalOpen++
				created, ok := novelEpochMS(opp, "createdOn")
				if !ok || created.After(threshold) {
					continue
				}
				owner := novelNested(opp, "ownerUser")
				customer := novelNested(opp, "customerUser")
				stale = append(stale, staleOpportunity{
					ID:            novelStr(opp, "id"),
					Name:          novelStr(opp, "name"),
					OwnerEmail:    novelStr(owner, "email"),
					CustomerEmail: novelStr(customer, "email"),
					CreatedOn:     isoOrEmpty(created),
					AgeDays:       ageDays(created, now),
				})
			}
			sort.SliceStable(stale, func(i, j int) bool { return stale[i].AgeDays > stale[j].AgeDays })

			view := pipelineStaleView{
				Days:                 flagDays,
				Stale:                stale,
				TotalOpen:            totalOpen,
				ScannedOpportunities: len(opps),
			}
			if len(stale) == 0 {
				view.Note = fmt.Sprintf("scanned %d opportunities (%d open) with none older than %d days; lower --days or refresh with 'sync --resources assisted-sales-v1-opportunities'", len(opps), totalOpen, flagDays)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&flagDays, "days", 14, "Age threshold in days for an open opportunity to count as stale")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
