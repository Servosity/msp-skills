// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/store"
)

// One reading of HubSpot pipelines and deal state for every novel command.
//
// HubSpot's PipelineStage (GET /crm/v3/pipelines/{objectType}) carries the
// stage key in "id" and string-valued metadata: "isClosed" ("true"/"false")
// and, for deal pipelines, "probability" ("0.0".."1.0"). Custom pipelines use
// numeric stage ids, so the default ids closedwon/closedlost alone cannot say
// whether a deal is closed.

type pipelineStageJSON struct {
	ID           string         `json:"id"`
	StageID      string         `json:"stageId"` // older local fixtures; "id" wins
	Label        string         `json:"label"`
	DisplayOrder int            `json:"displayOrder"`
	Metadata     map[string]any `json:"metadata"`
}

type pipelineStage struct {
	StageID     string
	Label       string
	Probability float64
	// HasProbability is false when the stage carries no readable
	// metadata.probability in 0..1. Probability is then 0 and means UNKNOWN,
	// never "this stage is worth nothing".
	HasProbability bool
	IsClosed       bool
}

// parsePipelineStages decodes one stored pipeline's stages.
func parsePipelineStages(data string) ([]pipelineStage, error) {
	var pl struct {
		Stages []pipelineStageJSON `json:"stages"`
	}
	if err := json.Unmarshal([]byte(data), &pl); err != nil {
		return nil, err
	}
	out := make([]pipelineStage, 0, len(pl.Stages))
	for _, s := range pl.Stages {
		id := s.ID
		if id == "" {
			id = s.StageID
		}
		// A missing or unreadable probability is unknown (HasProbability
		// false), not 0: Closed Lost reports "0.0" explicitly. Never invent a
		// midpoint either.
		prob, hasProb := 0.0, false
		switch x := s.Metadata["probability"].(type) {
		case float64:
			prob, hasProb = x, true
		case string:
			if v, err := strconv.ParseFloat(strings.TrimSpace(x), 64); err == nil {
				prob, hasProb = v, true
			}
		}
		if hasProb && (prob < 0 || prob > 1) {
			prob, hasProb = 0, false
		}
		closed := false
		switch x := s.Metadata["isClosed"].(type) {
		case bool:
			closed = x
		case string:
			closed = strings.EqualFold(strings.TrimSpace(x), "true")
		}
		out = append(out, pipelineStage{StageID: id, Label: s.Label, Probability: prob, HasProbability: hasProb, IsClosed: closed})
	}
	return out, nil
}

// hsDealPipelineIDsSQL selects the ids of stored pipelines declared deal
// pipelines (hand-fix handfixes.json pipeline-object-type-declared). HubSpot
// keeps pipelines per object type (GET /crm/v3/pipelines/{objectType},
// press/tools-manifest.json) and the store keeps every object type's under one
// resource, so each row's object type is recorded from the request path that
// fetched it (hsDeclarePipelineObjectType). An undeclared row (a write
// response, a stage, a ticket pipeline) is never a deal pipeline. "0-3" is
// HubSpot's object type id for deals.
const hsDealPipelineIDsSQL = `SELECT id FROM hubspot_pipeline_object_types WHERE object_type IN ('deals', '0-3')`

// hsPipelineRowsSQL is the one shared pipeline reader (hand-fix
// handfixes.json sync-collects-deal-pipelines): (id, data, archived) for every
// stored DEAL pipeline, from both places one can land: the typed
// hubspot_pipelines_crm table, which sync writes (hsDealPipelinesSyncCmd), and
// the generic resources table, where a live pipelines read writes through. A
// typed row wins over a generic row with the same id. Every pipeline reader
// (stage probabilities, closed and listed stage ids, pipeline-health's
// pipeline lookup) goes through it, directly or through hsStoredPipelines.
const hsPipelineRowsSQL = `SELECT id, data, COALESCE(archived, 0) AS archived FROM hubspot_pipelines_crm
  WHERE id IN (` + hsDealPipelineIDsSQL + `)
  UNION ALL
  SELECT r.id, r.data,
    CASE WHEN LOWER(CAST(json_extract(r.data, '$.archived') AS TEXT)) IN ('true', '1') THEN 1 ELSE 0 END
  FROM resources r
  WHERE r.resource_type IN ('hubspot-pipelines-crm', 'hubspot_pipelines_crm')
    AND r.id IN (` + hsDealPipelineIDsSQL + `)
    AND NOT EXISTS (SELECT 1 FROM hubspot_pipelines_crm t WHERE t.id = r.id)`

// hsPipelineListObjectType returns the object type a pipelines list path is
// for: "/crm/v3/pipelines/{objectType}" only. Any other path (one pipeline,
// its stages or audit) declares nothing.
func hsPipelineListObjectType(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, "/crm/v3/pipelines/")
	if !ok || rest == "" || strings.ContainsAny(rest, "/?") {
		return "", false
	}
	return strings.ToLower(rest), true
}

// hsDeclarePipelineObjectType records, for every pipeline in items (HubSpot
// Pipeline objects) that a pipelines list read at path just stored, the
// object type the path names. Sync calls it with its deal pipelines path; a
// live read calls it through hsDeclareReadPipelines.
func hsDeclarePipelineObjectType(db *store.Store, resource, path string, items []json.RawMessage) error {
	objectType, ok := hsPipelineListObjectType(path)
	if resource != "hubspot-pipelines-crm" || !ok {
		return nil
	}
	for _, raw := range items {
		var p struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &p) != nil || p.ID == "" {
			continue
		}
		if _, err := db.DB().Exec(`INSERT OR REPLACE INTO hubspot_pipeline_object_types (id, object_type) VALUES (?, ?)`, p.ID, objectType); err != nil {
			return fmt.Errorf("recording pipeline %s object type: %w", p.ID, err)
		}
	}
	return nil
}

// hsRetireUnlistedPipelines undeclares every stored pipeline of the listed
// object type that a complete listing no longer returns: GET
// /crm/v3/pipelines/{objectType} returns every pipeline at once
// (CollectionResponsePipelineNoPaging, press/tools-manifest.json), so one it
// omits was deleted, and undeclared it never answers a deal reader. "deals"
// and "0-3" are one object type. An empty listing retires nothing (HubSpot
// keeps a default pipeline).
func hsRetireUnlistedPipelines(db *store.Store, resource, path string, items []json.RawMessage) error {
	objectType, ok := hsPipelineListObjectType(path)
	if resource != "hubspot-pipelines-crm" || !ok {
		return nil
	}
	types := []any{objectType, objectType}
	if objectType == "deals" || objectType == "0-3" {
		types = []any{"deals", "0-3"}
	}
	var listed []any
	for _, raw := range items {
		var p struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &p) == nil && p.ID != "" {
			listed = append(listed, p.ID)
		}
	}
	if len(listed) == 0 {
		return nil
	}
	q := `DELETE FROM hubspot_pipeline_object_types WHERE object_type IN (?, ?) AND id NOT IN (?` + strings.Repeat(", ?", len(listed)-1) + `)`
	if _, err := db.DB().Exec(q, append(types, listed...)...); err != nil {
		return fmt.Errorf("retiring deleted %s pipelines: %w", objectType, err)
	}
	return nil
}

// hsDeclareReadPipelines declares the pipelines a live list read stored
// through writeThroughCache ({"results": [Pipeline]}, HubSpot's
// CollectionResponsePipelineNoPaging). Best effort like the write-through: a
// pipeline left undeclared is only ever not a deal pipeline.
func hsDeclareReadPipelines(ctx context.Context, resource, path string, data json.RawMessage) {
	if resource != "hubspot-pipelines-crm" || localDataDisabled() {
		return
	}
	var env struct {
		Results []json.RawMessage `json:"results"`
	}
	if json.Unmarshal(data, &env) != nil || len(env.Results) == 0 {
		return
	}
	db, err := store.OpenWithContext(ctx, defaultDBPath("hubspot-cli"))
	if err != nil {
		return
	}
	defer db.Close()
	if hsDeclarePipelineObjectType(db, resource, path, env.Results) == nil {
		_ = hsRetireUnlistedPipelines(db, resource, path, env.Results)
	}
}

// closedStageIDsSQL selects every stage id a stored pipeline marks closed. It
// never yields NULL: "x IN (..., NULL)" is NULL for a non-member, and NOT NULL
// would silently drop every open deal.
const closedStageIDsSQL = `SELECT COALESCE(json_extract(st.value, '$.id'), json_extract(st.value, '$.stageId'))
  FROM (` + hsPipelineRowsSQL + `) p, json_each(p.data, '$.stages') st
  WHERE LOWER(CAST(json_extract(st.value, '$.metadata.isClosed') AS TEXT)) IN ('true', '1')
    AND COALESCE(json_extract(st.value, '$.id'), json_extract(st.value, '$.stageId')) IS NOT NULL`

// wonStageIDsSQL selects closed stages HubSpot weights at probability 1.
const wonStageIDsSQL = closedStageIDsSQL + `
    AND CAST(json_extract(st.value, '$.metadata.probability') AS REAL) >= 1`

func dealPropSQL(dataCol, prop string) string {
	return "COALESCE(json_extract(" + dataCol + ", '$.properties." + prop + "'), '')"
}

func sqlTrue(expr string) string {
	return "LOWER(CAST(" + expr + " AS TEXT)) IN ('true', '1')"
}

// dealDataCol names the data column of the deals table under alias ("" for
// none). The alias is joined here, not written as one literal, because press
// 4.34.0's novel-host gate reads a literal like "d.data" as a hostname.
func dealDataCol(alias string) string {
	if alias == "" {
		return "data"
	}
	return alias + ".data"
}

// dealClosedSQL is a SQL condition that is true when ANY signal says the deal
// (deals table under alias) is closed: HubSpot's hs_is_closed, a stage the
// stored pipeline marks isClosed, or a default closed stage id. Any closed
// signal wins.
func dealClosedSQL(alias string) string {
	dataCol := dealDataCol(alias)
	stage := dealPropSQL(dataCol, "dealstage")
	return "(" + sqlTrue(dealPropSQL(dataCol, "hs_is_closed")) +
		" OR " + sqlTrue(dealPropSQL(dataCol, "hs_is_closed_won")) +
		" OR " + stage + " IN ('closedwon', 'closedlost')" +
		" OR " + stage + " IN (" + closedStageIDsSQL + "))"
}

// dealWonSQL is true when any signal says the deal closed won.
func dealWonSQL(alias string) string {
	dataCol := dealDataCol(alias)
	stage := dealPropSQL(dataCol, "dealstage")
	return "(" + sqlTrue(dealPropSQL(dataCol, "hs_is_closed_won")) +
		" OR " + stage + " = 'closedwon'" +
		" OR " + stage + " IN (" + wonStageIDsSQL + "))"
}

// loadAllStageProbabilities flattens all pipelines' stages into a stage id ->
// probability map. A stage with no readable probability is left OUT of the
// map (unknown, never 0). A read or decode failure fails the command: a
// forecast built on a pipeline it could not read is not a forecast.
func loadAllStageProbabilities(db *store.Store) (map[string]float64, error) {
	out, _, err := loadStageProbabilities(db)
	return out, err
}

func loadStageProbabilities(db *store.Store) (map[string]float64, int, error) {
	out := map[string]float64{}
	pipes, err := hsStoredPipelines(db)
	if err != nil {
		return nil, 0, err
	}
	pipelines := 0
	for _, p := range pipes {
		if p.Archived {
			continue
		}
		stages, err := parsePipelineStages(p.Data)
		if err != nil {
			return nil, 0, fmt.Errorf("parsing pipeline %s stages: %w", p.ID, err)
		}
		pipelines++
		for _, s := range stages {
			if s.StageID != "" && s.HasProbability {
				out[s.StageID] = s.Probability
			}
		}
	}
	return out, pipelines, nil
}

// storedPipeline is one stored pipeline as hsPipelineRowsSQL returns it.
type storedPipeline struct {
	ID       string
	Data     string
	Archived bool
}

// hsStoredPipelines reads every stored pipeline through hsPipelineRowsSQL,
// ordered by id. A read or scan failure fails the caller.
func hsStoredPipelines(db *store.Store) ([]storedPipeline, error) {
	rows, err := db.DB().Query(`SELECT id, data, archived FROM (` + hsPipelineRowsSQL + `) ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("reading pipelines: %w", err)
	}
	defer rows.Close()
	var out []storedPipeline
	for rows.Next() {
		var p storedPipeline
		var archived int
		if err := rows.Scan(&p.ID, &p.Data, &archived); err != nil {
			return nil, fmt.Errorf("reading pipelines: %w", err)
		}
		p.Archived = archived != 0
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading pipelines: %w", err)
	}
	return out, nil
}

// hsDealPipelinesSyncCmd stores the deal pipelines and each deal's
// hs_is_closed, in the store the command reads (--db and --home included).
const hsDealPipelinesSyncCmd = "hubspot-cli sync --resources hubspot-pipelines-crm,hubspot-deals-crm"

// hsDealPipelinesRemedy names that remedy for both surfaces: the CLI command,
// and the MCP sync tool the same store is shared with.
const hsDealPipelinesRemedy = "run '" + hsDealPipelinesSyncCmd + "' (over MCP: the sync tool with resources hubspot-pipelines-crm,hubspot-deals-crm), or a full sync"

// dealStagePricer is the one rule for every report that weights a deal by
// its stage probability (deals forecast, deals top, nurture queue). A stage
// no stored deal pipeline gives a probability for (pipelines never fetched, a
// stage added since, a deleted pipeline) is UNKNOWN, never 0: price records
// the record that needed it and err fails the command, naming what is missing
// and the command that fetches it. No weighted answer is built on a guess.
type dealStagePricer struct {
	probs     map[string]float64
	pipelines int
	missing   map[string][]string // stage id -> records that needed it
}

func newDealStagePricer(db *store.Store) (*dealStagePricer, error) {
	probs, n, err := loadStageProbabilities(db)
	if err != nil {
		return nil, err
	}
	return &dealStagePricer{probs: probs, pipelines: n, missing: map[string][]string{}}, nil
}

// price returns the stage's HubSpot probability. When it is unknown it
// records ref (e.g. "deal 12") and returns 0, which the caller must never
// report: err is non-nil from then on.
func (p *dealStagePricer) price(stage, ref string) float64 {
	if prob, ok := p.probs[stage]; ok && stage != "" {
		return prob
	}
	p.missing[stage] = append(p.missing[stage], ref)
	return 0
}

// err fails command when any priced record had an unknown stage probability.
func (p *dealStagePricer) err(command string) error {
	if len(p.missing) == 0 {
		return nil
	}
	stages := make([]string, 0, len(p.missing))
	for st := range p.missing {
		stages = append(stages, st)
	}
	sort.Strings(stages)
	records := 0
	parts := make([]string, 0, len(stages))
	for _, st := range stages {
		refs := p.missing[st]
		records += len(refs)
		label := st
		if label == "" {
			label = "(no dealstage)"
		}
		shown := refs
		more := ""
		if len(shown) > 5 {
			shown = shown[:5]
			more = fmt.Sprintf(" and %d more", len(refs)-5)
		}
		parts = append(parts, fmt.Sprintf("%q (%s%s)", label, strings.Join(shown, ", "), more))
	}
	why := "no deal pipelines are stored"
	if p.pipelines > 0 {
		why = fmt.Sprintf("the %d stored pipeline(s) give no probability for these stages (a stage added since they were fetched, or a deleted pipeline)", p.pipelines)
	}
	return fmt.Errorf("%s: %d open deal record(s) sit in a stage with no known HubSpot probability: %s. An unknown probability is not 0, so no weighted result is reported; %s. To store them, %s, then rerun",
		command, records, strings.Join(parts, ", "), why, hsDealPipelinesRemedy)
}

// hsAssociationCount reports how many association rows the local store holds.
// No sync writes hubspot_associations yet, so a missing table is zero; any
// other read error fails the command instead of silently selecting the
// degraded path. Zero means "never collected", never "none exist": every
// caller either refuses or marks the part it could not compute (hsReportGaps).
func hsAssociationCount(db *store.Store) (int, error) {
	var n int
	err := db.DB().QueryRow(`SELECT COUNT(*) FROM hubspot_associations`).Scan(&n)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return 0, nil
	}
	return n, err
}

// hsAssociationsNotCollected is why an association-dependent answer is not
// computed in this build.
const hsAssociationsNotCollected = "this build's sync does not collect HubSpot associations (the local hubspot_associations table is empty)"

// hsReportGaps is the machine-readable marker a local report carries when a
// part of its answer depends on data that was never collected. Both fields
// are always present (empty when the report is complete) so a consumer can
// rely on them.
type hsReportGaps struct {
	NotComputed []string `json:"not_computed"`
	Warnings    []string `json:"warnings"`
}

func newHSReportGaps() *hsReportGaps {
	return &hsReportGaps{NotComputed: []string{}, Warnings: []string{}}
}

// associationGap records that part was not computed because associations
// were never collected, and says so on stderr in every output mode.
func (g *hsReportGaps) associationGap(cmd *cobra.Command, part, consequence string) {
	msg := part + " not computed: " + hsAssociationsNotCollected + "; " + consequence
	g.NotComputed = append(g.NotComputed, part)
	g.Warnings = append(g.Warnings, msg)
	fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+msg)
}

// envelope adds the gap marker to a report's JSON envelope.
func (g *hsReportGaps) envelope(v map[string]any) map[string]any {
	v["not_computed"] = g.NotComputed
	v["warnings"] = g.Warnings
	return v
}
