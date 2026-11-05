// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: composite-scored top-N deals over the local store.

package cli

import (
	"database/sql"
	"fmt"
	"math"
	"sort"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/cliutil"
	"hubspot-pp-cli/internal/store"
)

// pp:data-source local
func newNovelDealsTopCmd(flags *rootFlags) *cobra.Command {
	var topN int
	var pipeline string
	var owner string
	var dbPath string
	var filterFlags []string
	var filterDebug bool

	cmd := &cobra.Command{
		Use:         "top",
		Short:       "Top-N deals by composite score (signal x amount x stage x recency)",
		Long:        `Score = signal_score * 2 + log(amount) * 1.5 + stage_probability * 3 + recency_score * 2. Reads from the local SQLite mirror. A deal whose stage has no known probability (the deal pipelines are not in the local store) fails the command instead of scoring 0. signal_score needs note-to-deal associations, which this build's sync does not collect: without them it is reported in not_computed and every deal's signal_score is 0.`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  hubspot-cli deals top --top 5 --owner me
  hubspot-cli deals top --top 10 --filter 'pipeline=default'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			expr, err := cliutil.ParseFilters(filterFlags)
			if err != nil {
				return err
			}
			if filterDebug {
				fmt.Fprint(cmd.ErrOrStderr(), expr.DebugString())
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
			if err := hsRequireCollected(cmd.Context(), db, "deals top", "hubspot-deals-crm", expr.FieldsReferenced()...); err != nil { // hand-wired (handfixes.json: hubspot-reports-read-only-collected-properties)
				return err
			}
			ownerID, err := resolveOwnerArg(db, owner)
			if err != nil {
				return err
			}

			pricer, err := newDealStagePricer(db)
			if err != nil {
				return err
			}

			assocCount, err := hsAssociationCount(db)
			if err != nil {
				return err
			}
			gaps := newHSReportGaps()
			source := "local"
			signalByDeal := map[string]struct {
				Buying  int
				Lost    int
				TopBuy  string
				TopLost string
			}{}
			if assocCount > 0 {
				if err := hsRequireCollected(cmd.Context(), db, "deals top", "hubspot-notes-crm"); err != nil { // hand-wired (handfixes.json: hubspot-reports-read-only-collected-properties)
					return err
				}
				if signalByDeal, err = computeDealSignals(db); err != nil {
					return err
				}
			} else {
				source = "local-no-associations"
				gaps.associationGap(cmd, "note_signals", "signal_score is 0 for every deal, so the ranking uses amount, stage probability and recency only")
			}

			q := `
SELECT id,
  COALESCE(json_extract(data, '$.properties.dealname'), '') AS name,
  COALESCE(CAST(json_extract(data, '$.properties.amount') AS REAL), 0) AS amount,
  COALESCE(json_extract(data, '$.properties.dealstage'), '') AS stage,
  COALESCE(json_extract(data, '$.properties.hubspot_owner_id'), '') AS owner_id,
  COALESCE(json_extract(data, '$.properties.pipeline'), '') AS pipeline,
  CAST((julianday('now') - julianday(COALESCE(
    json_extract(data, '$.properties.notes_last_contacted'),
    json_extract(data, '$.properties.notes_last_updated'),
    json_extract(data, '$.properties.hs_lastmodifieddate'),
    created_at
  ))) AS INTEGER) AS idle_days
FROM hubspot_deals_crm
WHERE COALESCE(archived, 0) = 0
  AND NOT ` + dealClosedSQL("") + `
  AND (? = '' OR pipeline = ?)
  AND (? = '' OR owner_id = ?)`
			queryArgs := []interface{}{pipeline, pipeline, ownerID, ownerID}
			// SQLFragment uses `?` placeholders for every value; no value is
			// ever concatenated into the SQL string. Field names are inlined
			// (SQLite can't parameterize JSON paths) but the parser rejects
			// quote/whitespace chars in field tokens, and SQLFragment runs a
			// second sanitization pass on top.
			if !expr.IsEmpty() {
				frag, fargs := expr.SQLFragment("data")
				q += " AND " + frag
				queryArgs = append(queryArgs, fargs...)
			}
			rows, err := db.DB().QueryContext(cmd.Context(), q, queryArgs...)
			if err != nil {
				return fmt.Errorf("query: %w", err)
			}
			defer rows.Close()

			type ranked struct {
				Rank        int     `json:"rank"`
				ID          string  `json:"id"`
				Name        string  `json:"name"`
				Amount      float64 `json:"amount"`
				Stage       string  `json:"stage"`
				Probability float64 `json:"probability"`
				OwnerID     string  `json:"owner"`
				IdleDays    int64   `json:"idle_days"`
				Score       float64 `json:"score"`
				TopBuying   string  `json:"top_buying_signal,omitempty"`
				TopLost     string  `json:"top_lost_signal,omitempty"`
			}
			items := []ranked{}
			for rows.Next() {
				var id, name, stage, ownerS, pipe sql.NullString
				var amt sql.NullFloat64
				var idle sql.NullInt64
				if err := rows.Scan(&id, &name, &amt, &stage, &ownerS, &pipe, &idle); err != nil {
					return err
				}
				stageStr := nullStr(stage)
				probability := pricer.price(stageStr, "deal "+nullStr(id))
				amount := nullF(amt)
				logAmt := math.Log(math.Max(amount, 1))
				idleD := nullI(idle)
				recency := 1.0 / (1.0 + float64(idleD)/30.0)
				signalScore := 0.0
				topBuy, topLost := "", ""
				if s, ok := signalByDeal[nullStr(id)]; ok {
					signalScore = float64(s.Buying - s.Lost)
					topBuy = s.TopBuy
					topLost = s.TopLost
				}
				final := signalScore*2 + logAmt*1.5 + probability*3 + recency*2
				items = append(items, ranked{
					ID: nullStr(id), Name: nullStr(name), Amount: amount,
					Stage: stageStr, Probability: probability,
					OwnerID: nullStr(ownerS), IdleDays: idleD, Score: final,
					TopBuying: topBuy, TopLost: topLost,
				})
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating deal rows: %w", err)
			}
			// Every candidate is scored before --top, so one unpriced deal
			// makes the whole ranking unknown.
			if err := pricer.err("deals top"); err != nil {
				return err
			}
			sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })
			if topN > 0 && len(items) > topN {
				items = items[:topN]
			}
			for i := range items {
				items[i].Rank = i + 1
			}

			if flags.asJSON {
				return flags.printJSON(cmd, gaps.envelope(map[string]any{
					"data_source": source,
					"results":     items,
				}))
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "data_source: %s\n", source)
			headers := []string{"rank", "id", "name", "amount", "stage", "probability", "owner", "idle_days", "score", "top_buying_signal", "top_lost_signal"}
			out := make([][]string, 0, len(items))
			for _, it := range items {
				out = append(out, []string{
					fmt.Sprintf("%d", it.Rank), it.ID, it.Name,
					formatAmount(it.Amount), it.Stage,
					fmt.Sprintf("%.2f", it.Probability),
					it.OwnerID, fmt.Sprintf("%d", it.IdleDays),
					fmt.Sprintf("%.2f", it.Score),
					it.TopBuying, it.TopLost,
				})
			}
			return flags.printTabular(cmd, headers, out)
		},
	}
	cmd.Flags().IntVar(&topN, "top", 5, "Number of top deals to return")
	cmd.Flags().StringVar(&pipeline, "pipeline", "", "Restrict to a single pipeline id")
	cmd.Flags().StringVar(&owner, "owner", "", "Filter by owner id, email, or 'me'")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringSliceVar(&filterFlags, "filter", nil, filterDescription)
	cmd.Flags().BoolVar(&filterDebug, "filter-debug", false, "Print parsed --filter expression to stderr before applying it")
	return cmd
}

// computeDealSignals walks notes_signals semantics inline and returns buying/lost counts per deal.
func computeDealSignals(db *store.Store) (map[string]struct {
	Buying  int
	Lost    int
	TopBuy  string
	TopLost string
}, error) {
	out := map[string]struct {
		Buying  int
		Lost    int
		TopBuy  string
		TopLost string
	}{}
	rows, err := db.DB().Query(`
SELECT a.to_id,
  COALESCE(json_extract(n.data, '$.properties.hs_note_body'), '') AS body
FROM hubspot_associations a
JOIN hubspot_notes_crm n ON n.id = a.from_id
WHERE a.from_type = 'notes' AND a.to_type = 'deals'`)
	if err != nil {
		return nil, fmt.Errorf("reading note signals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dealID, body string
		if err := rows.Scan(&dealID, &body); err != nil {
			return nil, fmt.Errorf("reading note signals: %w", err)
		}
		text := stripHTML(body)
		b, bk := scanRegexHits(text, buyingSignals)
		l, lk := scanRegexHits(text, lostSignals)
		s := out[dealID]
		s.Buying += b
		s.Lost += l
		if s.TopBuy == "" && len(bk) > 0 {
			s.TopBuy = bk[0]
		}
		if s.TopLost == "" && len(lk) > 0 {
			s.TopLost = lk[0]
		}
		out[dealID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading note signals: %w", err)
	}
	return out, nil
}
