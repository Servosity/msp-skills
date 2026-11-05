// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: ranked daily contact list under `nurture queue`.

package cli

import (
	"database/sql"
	"fmt"
	"math"
	"sort"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/cliutil"
)

// pp:data-source local
func newNovelNurtureQueueCmd(flags *rootFlags) *cobra.Command {
	var owner string
	var top int
	var staleDays int
	var stageUnder string
	var dbPath string
	var filterFlags []string
	var filterDebug bool

	cmd := &cobra.Command{
		Use:         "queue",
		Short:       "Ranked 'who to contact today' list with scoring rationale",
		Long:        `Score = stale_days * 1.0 + log(deal_amount) * 0.5 + (1 - probability) * 100, sorted high to low. A contact with no open deal scores at probability 0.5. A contact whose open deal sits in a stage with no known probability (the deal pipelines are not in the local store) fails the command instead of scoring a guess. The deal terms need contact-to-deal associations, which this build's sync does not collect: without them not_computed names deal_ranking, the queue ranks by stale days alone, and top_deal_amount and top_deal_stage are null.`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  hubspot-cli nurture queue --top 20
  hubspot-cli nurture queue --owner me --filter 'lifecyclestage=opportunity'`,
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
			db, err := hsOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "hubspot-contacts-crm") {
				hintIfStale(cmd, db, "hubspot-contacts-crm", flags.maxAge)
			}
			ownerID, err := resolveOwnerArg(db, owner)
			if err != nil {
				return err
			}

			// Every candidate is scored: the queue ranks by stale days x amount
			// x stage probability, so cutting candidates by stale days alone
			// (or before --filter) would drop a higher-scoring contact. --top
			// is applied after filtering and scoring.
			candidates, source, err := queryNurtureMine(cmd, db, ownerID, staleDays, stageUnder, -1)
			if err != nil {
				return err
			}

			if !expr.IsEmpty() {
				filterFields := expr.FieldsReferenced()
				if err := hsRequireCollected(cmd.Context(), db, "nurture queue", "hubspot-contacts-crm", filterFields...); err != nil { // hand-wired (handfixes.json: hubspot-reports-read-only-collected-properties)
					return err
				}
				kept := candidates[:0]
				for _, c := range candidates {
					var raw sql.NullString
					if err := db.DB().QueryRowContext(cmd.Context(), `SELECT data FROM hubspot_contacts_crm WHERE id = ?`, c.ContactID).Scan(&raw); err != nil && err != sql.ErrNoRows {
						return fmt.Errorf("reading contact %s: %w", c.ContactID, err)
					}
					if !expr.Match(extractPropertiesRow(raw.String, filterFields)) {
						continue
					}
					kept = append(kept, c)
				}
				candidates = kept
			}

			pricer, err := newDealStagePricer(db)
			if err != nil {
				return err
			}
			gaps := newHSReportGaps()
			if source == nurtureNoAssociations {
				gaps.associationGap(cmd, "deal_ranking", "the queue ranks by stale days alone (every contact scores at probability 0.5 with no deal amount), and top_deal_amount and top_deal_stage are null")
			}

			type scored struct {
				Rank      int      `json:"rank"`
				ContactID string   `json:"contact_id"`
				Name      string   `json:"name"`
				Score     float64  `json:"score"`
				StaleDays int64    `json:"stale_days"`
				TopAmount *float64 `json:"top_deal_amount"`
				TopStage  *string  `json:"top_deal_stage"`
				Reason    string   `json:"reason"`
			}
			scoredList := make([]scored, 0, len(candidates))
			for _, c := range candidates {
				topAmount, topStage := 0.0, ""
				if c.TopDealAmount != nil {
					topAmount = *c.TopDealAmount
				}
				if c.LatestDealStage != nil {
					topStage = *c.LatestDealStage
				}
				amt := topAmount
				if amt < 1 {
					amt = 1
				}
				// Real per-stage probability from the stored pipeline
				// metadata (the deals top rule). 0.5 (mid-funnel) is the
				// designed score for a contact KNOWN to have no open deal; an
				// open deal in a stage with no known probability fails the
				// queue below rather than scoring a guess.
				probability := 0.5
				if c.DealCount != nil && *c.DealCount > 0 {
					probability = pricer.price(topStage, "contact "+c.ContactID+"'s open deal")
				}
				score := float64(c.DaysStale)*1.0 + math.Log(amt)*0.5 + (1.0-probability)*100.0
				reason := fmt.Sprintf("%d days idle", c.DaysStale)
				if topAmount > 0 {
					reason += fmt.Sprintf(", %s deal in %q", formatAmount(topAmount), firstNonEmptyString(topStage, "open"))
				}
				scoredList = append(scoredList, scored{
					ContactID: c.ContactID, Name: c.Name, Score: score,
					StaleDays: c.DaysStale, TopAmount: c.TopDealAmount,
					TopStage: c.LatestDealStage, Reason: reason,
				})
			}
			if err := pricer.err("nurture queue"); err != nil {
				return err
			}
			sort.Slice(scoredList, func(i, j int) bool { return scoredList[i].Score > scoredList[j].Score })
			if top > 0 && len(scoredList) > top {
				scoredList = scoredList[:top]
			}
			for i := range scoredList {
				scoredList[i].Rank = i + 1
			}

			if flags.asJSON {
				return flags.printJSON(cmd, gaps.envelope(map[string]any{
					"data_source": source,
					"results":     scoredList,
				}))
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "data_source: %s\n", source)
			headers := []string{"rank", "contact_id", "name", "score", "stale_days", "top_deal_amount", "top_deal_stage", "reason"}
			out := make([][]string, 0, len(scoredList))
			for _, s := range scoredList {
				amount, stage := "unknown", "unknown"
				if s.TopAmount != nil {
					amount = formatAmount(*s.TopAmount)
				}
				if s.TopStage != nil {
					stage = *s.TopStage
				}
				out = append(out, []string{
					fmt.Sprintf("%d", s.Rank), s.ContactID, s.Name,
					fmt.Sprintf("%.2f", s.Score),
					fmt.Sprintf("%d", s.StaleDays),
					amount, stage, s.Reason,
				})
			}
			return flags.printTabular(cmd, headers, out)
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "me", "Filter by owner id, email, or 'me'")
	cmd.Flags().IntVar(&top, "top", 20, "Maximum number of contacts to return")
	cmd.Flags().IntVar(&staleDays, "stale-days", 14, "Minimum days since last contact for candidate set")
	cmd.Flags().StringVar(&stageUnder, "stage-under", "closedwon", "Also exclude deals in exactly this stage id (closed deals are always excluded)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringSliceVar(&filterFlags, "filter", nil, filterDescription)
	cmd.Flags().BoolVar(&filterDebug, "filter-debug", false, "Print parsed --filter expression to stderr before applying it")
	return cmd
}
