// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (handfixes.json hubspot-deal-state-known): a report that keeps
// only OPEN deals must know each deal's state. dealClosedSQL decides closed
// from hs_is_closed / hs_is_closed_won, the default closed stage ids, and the
// stages a stored pipeline marks isClosed. A deal in a custom stage that no
// stored pipeline lists, without hs_is_closed (sync does not request it), has
// an UNKNOWN state: counting it as open would put a closed deal in an open
// figure, so the report refuses and names the command that fetches the
// pipelines.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"hubspot-pp-cli/internal/store"
)

// hsDefaultDealStageIDs are the stage ids of HubSpot's default deal pipeline,
// whose open/closed state is fixed: closedwon and closedlost are closed, the
// rest are open.
const hsDefaultDealStageIDs = `'appointmentscheduled', 'qualifiedtobuy', 'presentationscheduled', 'decisionmakerboughtin', 'contractsent', 'closedwon', 'closedlost'`

// storedStageIDsSQL selects every stage id any stored pipeline lists.
const storedStageIDsSQL = `SELECT COALESCE(json_extract(st.value, '$.id'), json_extract(st.value, '$.stageId'))
  FROM (` + hsPipelineRowsSQL + `) p, json_each(p.data, '$.stages') st
  WHERE COALESCE(json_extract(st.value, '$.id'), json_extract(st.value, '$.stageId')) IS NOT NULL`

// dealStateKnownSQL is true when the deal's open/closed state is known.
func dealStateKnownSQL(alias string) string {
	dataCol := dealDataCol(alias)
	stage := dealPropSQL(dataCol, "dealstage")
	return "(" + dealPropSQL(dataCol, "hs_is_closed") + " <> ''" +
		" OR " + stage + " IN (" + hsDefaultDealStageIDs + ")" +
		" OR " + stage + " IN (" + storedStageIDsSQL + "))"
}

// requireKnownDealStates fails command when any unarchived deal (in pipeline,
// when pipeline is not "") has an unknown open/closed state. A store with no
// deals table yet has nothing to check.
func requireKnownDealStates(ctx context.Context, db *store.Store, command, pipeline string) error {
	if n, err := hsTableCount(ctx, db, "hubspot_deals_crm"); err != nil || n == 0 {
		return err
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT id, `+dealPropSQL("data", "dealstage")+`
FROM hubspot_deals_crm
WHERE COALESCE(archived, 0) = 0
  AND (? = '' OR `+dealPropSQL("data", "pipeline")+` = ?)
  AND NOT `+dealStateKnownSQL(""), pipeline, pipeline)
	if err != nil {
		return fmt.Errorf("%s: checking deal stages: %w", command, err)
	}
	defer rows.Close()
	byStage := map[string][]string{}
	for rows.Next() {
		var id, stage string
		if err := rows.Scan(&id, &stage); err != nil {
			return fmt.Errorf("%s: checking deal stages: %w", command, err)
		}
		byStage[stage] = append(byStage[stage], "deal "+id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%s: checking deal stages: %w", command, err)
	}
	if len(byStage) == 0 {
		return nil
	}
	stages := make([]string, 0, len(byStage))
	for st := range byStage {
		stages = append(stages, st)
	}
	sort.Strings(stages)
	parts := make([]string, 0, len(stages))
	for _, st := range stages {
		refs := byStage[st]
		more := ""
		if len(refs) > 5 {
			more = fmt.Sprintf(" and %d more", len(refs)-5)
			refs = refs[:5]
		}
		label := st
		if label == "" {
			label = "(no dealstage)"
		}
		parts = append(parts, fmt.Sprintf("%q (%s%s)", label, strings.Join(refs, ", "), more))
	}
	return fmt.Errorf("%s: deals sit in stages no stored pipeline lists, so whether they are open or closed is unknown: %s. No open-deal figure is reported on a guess. To store the pipelines and each deal's hs_is_closed, %s, then rerun",
		command, strings.Join(parts, ", "), hsDealPipelinesRemedy)
}

// hsTableCount returns 0 for a table that does not exist yet.
func hsTableCount(ctx context.Context, db *store.Store, table string) (int, error) {
	var exists int
	if err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&exists); err != nil {
		return 0, err
	}
	if exists == 0 {
		return 0, nil
	}
	var n int
	err := db.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n)
	return n, err
}
