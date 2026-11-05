// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: per-stage rollup with $-at-risk for a pipeline.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/store"
)

// pp:data-source local
func newNovelPipelineHealthCmd(flags *rootFlags) *cobra.Command {
	var idleDays int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "pipeline-health [pipeline-id-or-label]",
		Short:       "Per-stage rollup of count, $ total, open $ at risk, and oldest open-deal idle days",
		Long:        `Roll up deals by stage for one pipeline. count and total_amount cover every unarchived deal in the stage (stage_is_closed marks Closed Won/Lost stages); open_count, open_amount, at_risk_amount and oldest_idle_days cover open deals only, so a closed deal is never money at risk. A deal is closed when any signal says so: hs_is_closed, hs_is_closed_won, a closedwon/closedlost stage id, or a stage the stored pipeline marks isClosed. A deal in a stage the stored pipeline does not list (a stage added since the pipelines were fetched) gets its own row with stage_listed false, is named in unlisted_stage_ids, and draws a stderr warning; it never vanishes from the figures. Pass an explicit pipeline id, a label, or 'default' for HubSpot's default pipeline (id "default"; when no pipeline has that id, the first non-archived pipeline by id). Reads from the local SQLite mirror.`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     `  hubspot-cli pipeline-health default --idle-days 14`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if dbPath == "" {
				dbPath = defaultDBPath("hubspot-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "hubspot-deals-crm") {
				hintIfStale(cmd, db, "hubspot-deals-crm", flags.maxAge)
			}

			pipelineID, stages, err := resolvePipeline(db, args[0])
			if err != nil {
				return err
			}
			if err := requireKnownDealStates(cmd.Context(), db, "pipeline-health", pipelineID); err != nil {
				return err
			}
			if err := hsRequireCollected(cmd.Context(), db, "pipeline-health", "hubspot-deals-crm"); err != nil { // hand-wired (handfixes.json: hubspot-reports-read-only-collected-properties)
				return err
			}

			// count and total_amount are per stage and include closed deals
			// (labeled by stage_is_closed). Every open figure (open_count,
			// open_amount, at_risk_amount, oldest_idle_days) reads deal state
			// through the one predicate, dealClosedSQL, never through the
			// stage alone: a closed deal is never open money at risk.
			type stageRow struct {
				StageID        string   `json:"stage_id"`
				StageLabel     string   `json:"stage_label"`
				StageIsClosed  bool     `json:"stage_is_closed"`
				StageListed    bool     `json:"stage_listed"`
				Count          int64    `json:"count"`
				TotalAmount    float64  `json:"total_amount"`
				OpenCount      int64    `json:"open_count"`
				OpenAmount     float64  `json:"open_amount"`
				AtRiskAmount   float64  `json:"at_risk_amount"`
				OldestIdleDays int64    `json:"oldest_idle_days"`
				Probability    *float64 `json:"probability"`
			}

			// A deal whose stage the stored pipeline does not list would
			// otherwise drop out of every figure, so each such stage gets its
			// own row (stage_listed false). Its closed state comes from the
			// deal's own signals only.
			listed := map[string]bool{}
			for i := range stages {
				listed[stages[i].StageID] = true
			}
			unlisted, err := pipelineHealthUnlistedStages(cmd, db, pipelineID, listed)
			if err != nil {
				return err
			}
			for _, id := range unlisted {
				// HubSpot's default closed ids are closed in any pipeline.
				stages = append(stages, pipelineStage{StageID: id, IsClosed: id == "closedwon" || id == "closedlost"})
			}

			items := []stageRow{}
			unlistedDeals := int64(0)
			for _, st := range stages {
				var count, openCount, oldest int64
				var total, openAmount, atRisk float64
				if err := db.DB().QueryRowContext(cmd.Context(), pipelineHealthStageSQL, pipelineID, st.StageID, idleDays).
					Scan(&count, &total, &openCount, &openAmount, &atRisk, &oldest); err != nil {
					return fmt.Errorf("stage %s: %w", st.StageID, err)
				}
				if !listed[st.StageID] {
					unlistedDeals += count
				}
				items = append(items, stageRow{
					StageID:        st.StageID,
					StageLabel:     st.Label,
					StageIsClosed:  st.IsClosed,
					StageListed:    listed[st.StageID],
					Count:          count,
					TotalAmount:    total,
					OpenCount:      openCount,
					OpenAmount:     openAmount,
					AtRiskAmount:   atRisk,
					OldestIdleDays: oldest,
					Probability:    stageProbability(st),
				})
			}

			if len(unlisted) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d deal(s) in pipeline %s sit in stage(s) the stored pipeline does not list (%s); they are reported in rows with stage_listed false, and only their own hs_is_closed or a closedwon/closedlost id can mark them closed. To refresh the pipelines, %s.\n",
					unlistedDeals, pipelineID, strings.Join(unlisted, ", "), hsDealPipelinesRemedy)
			}
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"pipeline_id":        pipelineID,
					"idle_days":          idleDays,
					"stages":             items,
					"unlisted_stage_ids": unlisted,
				})
			}
			headers := []string{"stage_id", "stage_label", "stage_is_closed", "stage_listed", "count", "total_amount", "open_count", "open_amount", "at_risk_amount", "oldest_idle_days", "probability"}
			var rows [][]string
			for _, it := range items {
				rows = append(rows, []string{
					it.StageID, it.StageLabel,
					fmt.Sprintf("%t", it.StageIsClosed),
					fmt.Sprintf("%t", it.StageListed),
					fmt.Sprintf("%d", it.Count),
					formatAmount(it.TotalAmount),
					fmt.Sprintf("%d", it.OpenCount),
					formatAmount(it.OpenAmount),
					formatAmount(it.AtRiskAmount),
					fmt.Sprintf("%d", it.OldestIdleDays),
					probabilityCell(it.Probability),
				})
			}
			return flags.printTabular(cmd, headers, rows)
		},
	}
	cmd.Flags().IntVar(&idleDays, "idle-days", 14, "Idle threshold for the $-at-risk column")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// pipelineHealthStageSQL rolls up one (pipeline, stage). Args: pipeline id,
// stage id, idle days. A deal is open only when dealClosedSQL says no closed
// signal is present; idle is NULL (never at risk, never oldest) when no
// activity timestamp parses.
var pipelineHealthStageSQL = `
SELECT
  COUNT(*),
  COALESCE(SUM(amount), 0),
  COALESCE(SUM(is_open), 0),
  COALESCE(SUM(CASE WHEN is_open = 1 THEN amount END), 0),
  COALESCE(SUM(CASE WHEN is_open = 1 AND idle_days >= ?3 THEN amount END), 0),
  COALESCE(MAX(CASE WHEN is_open = 1 THEN idle_days END), 0)
FROM (
  SELECT
    CAST(json_extract(data, '$.properties.amount') AS REAL) AS amount,
    CASE WHEN ` + dealClosedSQL("") + ` THEN 0 ELSE 1 END AS is_open,
    CAST((julianday('now') - julianday(COALESCE(
      json_extract(data, '$.properties.notes_last_contacted'),
      json_extract(data, '$.properties.notes_last_updated'),
      json_extract(data, '$.properties.hs_lastmodifieddate'),
      created_at
    ))) AS INTEGER) AS idle_days
  FROM hubspot_deals_crm
  WHERE COALESCE(archived, 0) = 0
    AND json_extract(data, '$.properties.pipeline') = ?1
    AND COALESCE(json_extract(data, '$.properties.dealstage'), '') = ?2
)`

// pipelineHealthUnlistedStages returns, sorted, every dealstage an unarchived
// deal in the pipeline carries that the stored pipeline does not list ("" for
// a deal with no dealstage).
func pipelineHealthUnlistedStages(cmd *cobra.Command, db *store.Store, pipelineID string, listed map[string]bool) ([]string, error) {
	rows, err := db.DB().QueryContext(cmd.Context(), `SELECT DISTINCT COALESCE(json_extract(data, '$.properties.dealstage'), '')
FROM hubspot_deals_crm
WHERE COALESCE(archived, 0) = 0 AND json_extract(data, '$.properties.pipeline') = ?
ORDER BY 1`, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("reading pipeline %s deal stages: %w", pipelineID, err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reading pipeline %s deal stages: %w", pipelineID, err)
		}
		if !listed[id] {
			out = append(out, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading pipeline %s deal stages: %w", pipelineID, err)
	}
	return out, nil
}

// resolvePipeline returns (pipelineID, []pipelineStage) for a user-supplied
// pipeline id, label, or the literal "default". "default" is HubSpot's own id
// for the default deal pipeline, so a pipeline with that id wins; only when
// none exists does it fall back to the first non-archived pipeline by id (a
// custom numeric id like "12345" sorts before "default" and must not win).
func resolvePipeline(db *store.Store, arg string) (string, []pipelineStage, error) {
	// Read through the one shared pipeline reader (hand-fix handfixes.json
	// sync-collects-deal-pipelines), so pipelines sync stores and pipelines a
	// live read wrote through are both found.
	pipes, err := hsStoredPipelines(db)
	if err != nil {
		return "", nil, err
	}
	var pick *storedPipeline
	if arg == "default" {
		// HubSpot's own default deal pipeline id is "default"; otherwise the
		// first unarchived pipeline by id.
		for i := range pipes {
			if pipes[i].Archived {
				continue
			}
			if pipes[i].ID == "default" {
				pick = &pipes[i]
				break
			}
			if pick == nil {
				pick = &pipes[i]
			}
		}
		if pick == nil {
			return "", nil, fmt.Errorf("no deal pipelines in the local store: to store them, %s", hsDealPipelinesRemedy)
		}
	} else {
		for i := range pipes {
			if pipes[i].ID == arg {
				pick = &pipes[i]
				break
			}
		}
		if pick == nil {
			for i := range pipes {
				var meta struct {
					Label string `json:"label"`
				}
				if json.Unmarshal([]byte(pipes[i].Data), &meta) == nil && meta.Label == arg {
					pick = &pipes[i]
					break
				}
			}
		}
		if pick == nil {
			return "", nil, fmt.Errorf("no deal pipeline found for %q in the local store (try 'default'; to store the pipelines, %s)", arg, hsDealPipelinesRemedy)
		}
	}
	out, err := parsePipelineStages(pick.Data)
	if err != nil {
		return "", nil, fmt.Errorf("parsing pipeline %s stages: %w", pick.ID, err)
	}
	return pick.ID, out, nil
}

// stageProbability is the stage's HubSpot probability, or nil when the stored
// pipeline gives none: an unknown probability is never shown as 0.
func stageProbability(st pipelineStage) *float64 {
	if !st.HasProbability {
		return nil
	}
	p := st.Probability
	return &p
}

func probabilityCell(p *float64) string {
	if p == nil {
		return "unknown"
	}
	return fmt.Sprintf("%.2f", *p)
}
