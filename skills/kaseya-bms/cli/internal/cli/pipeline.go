// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: pipeline.

// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type pipelineStage struct {
	Status         string  `json:"status"`
	Count          int     `json:"count"`
	Amount         float64 `json:"amount"`
	WeightedAmount float64 `json:"weighted_amount"`
}

type slippedOpportunity struct {
	Subject   string  `json:"subject"`
	Account   string  `json:"account"`
	Status    string  `json:"status"`
	Amount    float64 `json:"amount"`
	CloseDate string  `json:"close_date"`
	DaysLate  int     `json:"days_late"`
}

type pipelineView struct {
	Stages        []pipelineStage      `json:"stages"`
	Slipped       []slippedOpportunity `json:"slipped"`
	TotalOpen     int                  `json:"total_open"`
	TotalAmount   float64              `json:"total_amount"`
	TotalWeighted float64              `json:"total_weighted"`
	Note          string               `json:"note,omitempty"`
}

// aggregatePipeline groups in-pipeline opportunities by status with totals,
// weighted value (amount x probability), and slipped-close detection. Pure
// function for table-driven tests.
func aggregatePipeline(rows []map[string]any, slippedLimit int, probabilityScale string, now time.Time) pipelineView {
	view := pipelineView{Stages: []pipelineStage{}, Slipped: []slippedOpportunity{}}
	byStatus := map[string]*pipelineStage{}

	for _, m := range rows {
		if !kbmsBool(m, "InPipeline") {
			continue
		}
		status := kbmsStr(m, "Status")
		if status == "" {
			status = "(no status)"
		}
		stage, ok := byStatus[status]
		if !ok {
			stage = &pipelineStage{Status: status}
			byStatus[status] = stage
		}
		amount, _ := kbmsNum(m, "Amount")
		probability, hasProb := kbmsNum(m, "Probability")
		weighted := amount
		if hasProb && probability >= 0 {
			switch strings.ToLower(probabilityScale) {
			case "fraction": // tenant serializes 0.0-1.0
				if probability <= 1 {
					weighted = amount * probability
				}
			default: // percent: 0-100
				if probability <= 100 {
					weighted = amount * probability / 100
				}
			}
		}
		stage.Count++
		stage.Amount += amount
		stage.WeightedAmount += weighted
		view.TotalOpen++
		view.TotalAmount += amount
		view.TotalWeighted += weighted

		if closeDate, ok := kbmsTime(m, "CloseDate"); ok && closeDate.Before(now) {
			view.Slipped = append(view.Slipped, slippedOpportunity{
				Subject:   kbmsStr(m, "Subject"),
				Account:   kbmsStr(m, "AccountName"),
				Status:    status,
				Amount:    amount,
				CloseDate: closeDate.Format("2006-01-02"),
				DaysLate:  int(now.Sub(closeDate).Hours() / 24),
			})
		}
	}

	for _, status := range kbmsSortedKeys(byStatus) {
		stage := byStatus[status]
		stage.Amount = kbmsRound2(stage.Amount)
		stage.WeightedAmount = kbmsRound2(stage.WeightedAmount)
		view.Stages = append(view.Stages, *stage)
	}
	sort.Slice(view.Stages, func(i, j int) bool {
		if view.Stages[i].Amount != view.Stages[j].Amount {
			return view.Stages[i].Amount > view.Stages[j].Amount
		}
		return view.Stages[i].Status < view.Stages[j].Status
	})
	sort.Slice(view.Slipped, func(i, j int) bool { return view.Slipped[i].DaysLate > view.Slipped[j].DaysLate })
	if slippedLimit > 0 && len(view.Slipped) > slippedLimit {
		view.Slipped = view.Slipped[:slippedLimit]
	}
	view.TotalAmount = kbmsRound2(view.TotalAmount)
	view.TotalWeighted = kbmsRound2(view.TotalWeighted)
	if view.TotalOpen == 0 {
		view.Note = "no in-pipeline opportunities in the local mirror; run 'sync --resources crm' to refresh"
	}
	return view
}

func newNovelPipelineCmd(flags *rootFlags) *cobra.Command {
	var slippedLimit int
	var probabilityScale string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Open opportunities grouped by stage with counts, total and weighted value, and slipped-close flags for the Monday sales call.",
		Long: strings.Trim(`
Groups the synced opportunity mirror by stage with weighted value
(amount x probability) and flags deals whose close date has slipped past
today - math the BMS CRM grid will not compute for you.`, "\n"),
		Example: strings.Trim(`
  # Monday pipeline prep
  kaseya-bms-cli pipeline --agent

  # Only the 5 most-overdue deals
  kaseya-bms-cli pipeline --slipped-limit 5 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would group in-pipeline opportunities by stage with weighted value and slip flags")
				return nil
			}
			if err := kbmsRejectLiveSource(flags, "pipeline"); err != nil {
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
			if !hintIfUnsynced(cmd, db, "crm") {
				hintIfStale(cmd, db, "crm", flags.maxAge)
			}
			rows, err := kbmsRows(cmd.Context(), db, "crm")
			if err != nil {
				return fmt.Errorf("querying opportunities: %w", err)
			}
			view := aggregatePipeline(rows, slippedLimit, probabilityScale, time.Now().UTC())
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&slippedLimit, "slipped-limit", 20, "Maximum slipped deals to list (0 = no limit)")
	cmd.Flags().StringVar(&probabilityScale, "probability-scale", "percent", "How the tenant serializes Probability: percent (0-100) or fraction (0.0-1.0)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default ~/.local/share/kaseya-bms-cli/data.db)")
	return cmd
}
