// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence feature: multi-tenant hygiene rollup. Synthesizes
// the four per-dimension audits (completeness, stale passwords, expirations due,
// stale articles) into one worst-first scorecard per company from the local
// mirror — a cross-audit ranking no single audit or Hudu API call returns.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type hygieneSummaryRow struct {
	CompanyID        int     `json:"company_id"`
	CompanyName      string  `json:"company_name"`
	HygieneScore     float64 `json:"hygiene_score"`
	DimensionsScored int     `json:"dimensions_scored"`
	CompletenessPct  float64 `json:"completeness_pct,omitempty"`
	AssetsScored     int     `json:"assets_scored"`
	StalePasswords   int     `json:"stale_passwords"`
	TotalPasswords   int     `json:"total_passwords"`
	ExpiringSoon     int     `json:"expiring_soon"`
	TotalDated       int     `json:"total_dated"`
	StaleArticles    int     `json:"stale_articles"`
	TotalArticles    int     `json:"total_articles"`
}

// hygieneScore averages the available 0-100 subscores; companies are only
// scored on dimensions they have data for.
func hygieneScore(subs []float64) float64 {
	if len(subs) == 0 {
		return 0
	}
	var sum float64
	for _, s := range subs {
		sum += s
	}
	return round1(sum / float64(len(subs)))
}

// ratioScore converts a bad/total pair into a 0-100 "healthy" percentage.
func ratioScore(bad, total int) float64 {
	if total <= 0 {
		return 100
	}
	return float64(total-bad) / float64(total) * 100
}

func newNovelAuditSummaryCmd(flags *rootFlags) *cobra.Command {
	var passwordAge string
	var articleAge string
	var expireWithin string
	var flagCompany int
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "summary",
		Short:       "One worst-first hygiene scorecard per company combining completeness, stale passwords, expirations due, and stale articles.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Rank every company by overall documentation hygiene from the local mirror
(run 'sync' first), worst-first.

Use this command for a single cross-client hygiene scorecard ranking every
company worst-first. Do NOT use it to drill into one hygiene dimension; use
'audit completeness', 'audit stale-passwords', 'audit expirations', or
'audit stale-articles' for the per-dimension detail.

The hygiene score (0-100) averages the dimensions each company has data for:
documentation completeness, password rotation health, expirations due within
the window, and knowledge-base freshness. Companies with no synced data in any
dimension are skipped (a count is reported on the human surface).`,
		Example: `  # Worst clients across every hygiene dimension
  hudu-cli audit summary

  # Bottom five as JSON, just the score
  hudu-cli audit summary --limit 5 --agent --select company_name,hygiene_score`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			pwThreshold, err := parseAgeDays(passwordAge)
			if err != nil {
				return usageErr(fmt.Errorf("--password-age: %w", err))
			}
			kbThreshold, err := parseAgeDays(articleAge)
			if err != nil {
				return usageErr(fmt.Errorf("--article-age: %w", err))
			}
			expWindow, err := parseAgeDays(expireWithin)
			if err != nil {
				return usageErr(fmt.Errorf("--expire-within: %w", err))
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openAuditStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}
			companyNames := loadCompanyNames(cmd.Context(), db)
			now := time.Now()

			type acc struct {
				pctSum     float64
				assets     int
				stalePw    int
				totalPw    int
				due        int
				totalDated int
				staleKb    int
				totalKb    int
			}
			byCompany := map[int]*acc{}
			get := func(cid int) *acc {
				a := byCompany[cid]
				if a == nil {
					a = &acc{}
					byCompany[cid] = a
				}
				return a
			}

			// Dimension 1: documentation completeness (layout schema vs custom_fields).
			// Layout-read failures degrade this dimension to unscored rather than
			// failing the whole rollup (the other three dimensions still report).
			schemas, schemaErr := loadLayoutSchemas(cmd.Context(), db)
			if schemaErr != nil {
				schemas = map[int]layoutSchema{}
			}
			if assetRows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM assets`); err == nil {
				for _, raw := range assetRows {
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						continue
					}
					sc, ok := schemas[intField(m, "asset_layout_id")]
					if !ok || len(sc.All) == 0 {
						continue
					}
					denom := sc.Required
					if len(denom) == 0 {
						denom = sc.All
					}
					cf, _ := json.Marshal(m["custom_fields"])
					filled := filledLabels(cf)
					have := 0
					for _, label := range denom {
						if filled[label] {
							have++
						}
					}
					pct := 100.0
					if len(denom) > 0 {
						pct = float64(have) / float64(len(denom)) * 100
					}
					a := get(intField(m, "company_id"))
					a.assets++
					a.pctSum += pct
				}
			}

			// Dimension 2: password rotation health.
			if pwRows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM asset_passwords`); err == nil {
				for _, raw := range pwRows {
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						continue
					}
					a := get(intField(m, "company_id"))
					a.totalPw++
					if days, ok := ageDays(asString(m["updated_at"]), now); ok && days >= pwThreshold {
						a.stalePw++
					}
				}
			}

			// Dimension 3: expirations due within the window (rollup + website dates).
			addDated := func(cid int, on string) {
				days, ok := daysUntil(on, now)
				if !ok {
					return
				}
				a := get(cid)
				a.totalDated++
				if days <= expWindow {
					a.due++
				}
			}
			if expRows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM expirations`); err == nil {
				for _, raw := range expRows {
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						continue
					}
					addDated(intField(m, "company_id"), asString(m["date"]))
				}
			}
			if webRows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM websites`); err == nil {
				for _, raw := range webRows {
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						continue
					}
					cid := intField(m, "company_id")
					if ssl := asString(m["ssl_expiration_date"]); ssl != "" {
						addDated(cid, ssl)
					}
					if dom := asString(m["domain_expiration_date"]); dom != "" {
						addDated(cid, dom)
					}
				}
			}

			// Dimension 4: knowledge-base freshness.
			if kbRows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM articles`); err == nil {
				for _, raw := range kbRows {
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						continue
					}
					a := get(intField(m, "company_id"))
					a.totalKb++
					if days, ok := ageDays(asString(m["updated_at"]), now); ok && days >= kbThreshold {
						a.staleKb++
					}
				}
			}

			skipped := 0
			out := []hygieneSummaryRow{}
			for cid, a := range byCompany {
				if flagCompany > 0 && cid != flagCompany {
					continue
				}
				var subs []float64
				row := hygieneSummaryRow{
					CompanyID: cid, CompanyName: companyNames[cid],
					AssetsScored: a.assets, StalePasswords: a.stalePw, TotalPasswords: a.totalPw,
					ExpiringSoon: a.due, TotalDated: a.totalDated,
					StaleArticles: a.staleKb, TotalArticles: a.totalKb,
				}
				if a.assets > 0 {
					row.CompletenessPct = round1(a.pctSum / float64(a.assets))
					subs = append(subs, row.CompletenessPct)
				}
				if a.totalPw > 0 {
					subs = append(subs, ratioScore(a.stalePw, a.totalPw))
				}
				if a.totalDated > 0 {
					subs = append(subs, ratioScore(a.due, a.totalDated))
				}
				if a.totalKb > 0 {
					subs = append(subs, ratioScore(a.staleKb, a.totalKb))
				}
				if len(subs) == 0 {
					skipped++
					continue
				}
				row.DimensionsScored = len(subs)
				row.HygieneScore = hygieneScore(subs)
				out = append(out, row)
			}
			sort.Slice(out, func(i, j int) bool {
				if out[i].HygieneScore != out[j].HygieneScore {
					return out[i].HygieneScore < out[j].HygieneScore
				}
				return out[i].StalePasswords > out[j].StalePasswords
			})
			if flagLimit > 0 && len(out) > flagLimit {
				out = out[:flagLimit]
			}

			return emitAudit(cmd, flags, out, func(w io.Writer) {
				if len(out) == 0 {
					fmt.Fprintln(w, "No companies with scoreable data. Run 'hudu-cli sync' first.")
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "SCORE\tCOMPANY\tCOMPLETE%\tSTALE-PW\tEXPIRING\tSTALE-KB\tDIMS")
				for _, r := range out {
					comp := "-"
					if r.AssetsScored > 0 {
						comp = fmt.Sprintf("%.1f", r.CompletenessPct)
					}
					fmt.Fprintf(tw, "%.1f\t%s\t%s\t%d/%d\t%d/%d\t%d/%d\t%d\n",
						r.HygieneScore, r.CompanyName, comp,
						r.StalePasswords, r.TotalPasswords,
						r.ExpiringSoon, r.TotalDated,
						r.StaleArticles, r.TotalArticles,
						r.DimensionsScored)
				}
				_ = tw.Flush()
				if skipped > 0 {
					fmt.Fprintf(w, "(%d compan%s with no scoreable data skipped)\n", skipped, pluralIes(skipped))
				}
			})
		},
	}
	cmd.Flags().StringVar(&passwordAge, "password-age", "180d", "Rotation-age threshold for the password dimension (e.g. 180d, 6m)")
	cmd.Flags().StringVar(&articleAge, "article-age", "365d", "Staleness threshold for the knowledge-base dimension (e.g. 365d, 1y)")
	cmd.Flags().StringVar(&expireWithin, "expire-within", "30d", "Horizon window for the expirations dimension (e.g. 30d, 12w)")
	cmd.Flags().IntVar(&flagCompany, "company", 0, "Limit to a single company id")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Return only the worst N companies (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func pluralIes(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
