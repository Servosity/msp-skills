// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: lifecycle-stage funnel snapshot with stage-to-stage
// conversion ratios.

package cli

import (
	"fmt"
	"math"
	"sort"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/store"
)

// canonicalLifecycleStages is HubSpot's default lifecycle-stage order. Stages
// are rendered in this order; any non-canonical value buckets after these
// under its own literal name, and empty/null lands last as "(unset)".
var canonicalLifecycleStages = []string{
	"subscriber",
	"lead",
	"marketingqualifiedlead",
	"salesqualifiedlead",
	"opportunity",
	"customer",
	"evangelist",
	"other",
}

// pp:data-source local
func newNovelContactsFunnelCmd(flags *rootFlags) *cobra.Command {
	var owner string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "funnel",
		Short:       "Lifecycle-stage funnel snapshot with stage-to-stage conversion ratios",
		Long:        "One-shot funnel table of contacts per lifecycle stage computed from the local mirror, with stage-to-stage conversion ratios.\n\nUse this command for top-of-funnel leak questions ('where do contacts stall'). It reads only synced local data; run 'hubspot-cli sync --resources hubspot-contacts-crm' first.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example: `  hubspot-cli contacts funnel
  hubspot-cli contacts funnel --owner me`,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			if !hintIfUnsynced(cmd, db, "hubspot-contacts-crm") {
				hintIfStale(cmd, db, "hubspot-contacts-crm", flags.maxAge)
			}
			ownerID, err := resolveOwnerArg(db, owner)
			if err != nil {
				return err
			}

			view, err := queryContactsFunnel(cmd, db, ownerID)
			if err != nil {
				return err
			}

			if flags.asJSON {
				return flags.printJSON(cmd, view)
			}
			headers := []string{"stage", "count", "conversion_pct"}
			rows := make([][]string, 0, len(view.Stages))
			for _, s := range view.Stages {
				conv := ""
				if s.ConversionPct != nil {
					conv = fmt.Sprintf("%.1f%%", *s.ConversionPct)
				}
				rows = append(rows, []string{s.Stage, fmt.Sprintf("%d", s.Count), conv})
			}
			return flags.printTabular(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "Filter by owner id, email, or 'me'")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

type funnelStage struct {
	Stage         string   `json:"stage"`
	Count         int64    `json:"count"`
	ConversionPct *float64 `json:"conversion_pct,omitempty"`
}

type funnelView struct {
	Stages []funnelStage `json:"stages"`
	Total  int64         `json:"total"`
}

func queryContactsFunnel(cmd *cobra.Command, db *store.Store, ownerID string) (funnelView, error) {
	view := funnelView{Stages: []funnelStage{}}

	q := `
SELECT COALESCE(json_extract(data, '$.properties.lifecyclestage'), '') AS stage,
       COUNT(*) AS cnt
FROM hubspot_contacts_crm
WHERE COALESCE(archived, 0) = 0
  AND (? = '' OR json_extract(data, '$.properties.hubspot_owner_id') = ?)
GROUP BY stage`
	rows, err := db.DB().QueryContext(cmd.Context(), q, ownerID, ownerID)
	if err != nil {
		return view, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	counts := map[string]int64{}
	var total int64
	for rows.Next() {
		var stage string
		var cnt int64
		if err := rows.Scan(&stage, &cnt); err != nil {
			return view, err
		}
		counts[stage] += cnt
		total += cnt
	}
	if err := rows.Err(); err != nil {
		return view, fmt.Errorf("iterating funnel rows: %w", err)
	}
	view.Total = total

	canonical := map[string]bool{}
	for _, s := range canonicalLifecycleStages {
		canonical[s] = true
	}

	// Canonical stages first, in order — emit even when zero so the funnel
	// shape is visible at a glance.
	ordered := make([]funnelStage, 0, len(counts)+len(canonicalLifecycleStages))
	for _, s := range canonicalLifecycleStages {
		ordered = append(ordered, funnelStage{Stage: s, Count: counts[s]})
	}

	// Non-canonical values (excluding empty/null), sorted by name, after the
	// canonical block.
	extras := make([]string, 0)
	for stage := range counts {
		if stage == "" || canonical[stage] {
			continue
		}
		extras = append(extras, stage)
	}
	sort.Strings(extras)
	for _, s := range extras {
		ordered = append(ordered, funnelStage{Stage: s, Count: counts[s]})
	}

	// Empty/null lifecyclestage last as "(unset)".
	if c, ok := counts[""]; ok {
		ordered = append(ordered, funnelStage{Stage: "(unset)", Count: c})
	}

	// conversion_pct for each canonical stage after the first = count / prev * 100.
	// Only computed across the canonical block (the funnel proper); extras and
	// (unset) carry no conversion ratio.
	for i := 1; i < len(canonicalLifecycleStages); i++ {
		prev := ordered[i-1].Count
		if prev == 0 {
			continue
		}
		pct := math.Round(float64(ordered[i].Count)/float64(prev)*1000) / 10
		ordered[i].ConversionPct = &pct
	}

	view.Stages = ordered
	return view, nil
}
