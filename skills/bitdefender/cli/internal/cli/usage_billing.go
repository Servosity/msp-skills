// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"bitdefender-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

type billingRow struct {
	CompanyID        string         `json:"company_id"`
	CompanyName      string         `json:"company_name"`
	CompanyType      int            `json:"company_type"`
	ParentID         string         `json:"parent_id,omitempty"`
	ProductType      int            `json:"product_type"`
	Usage            map[string]int `json:"usage"`
	PriorUsage       map[string]int `json:"prior_usage"`
	OwnShare         map[string]int `json:"own_share"` // null = not computed (see flags), {} = computed zero
	Delta            map[string]int `json:"delta"`
	Flags            []string       `json:"flags"`
	ManagedEndpoints *int           `json:"managed_endpoints,omitempty"`
	AgreementQty     *int           `json:"agreement_quantity,omitempty"`
}

type billingView struct {
	Month         string         `json:"month"`
	PriorMonth    string         `json:"prior_month"`
	Rows          []billingRow   `json:"rows"`
	Flagged       int            `json:"flagged"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
	Note          string         `json:"note,omitempty"`
}

// usageCounts maps product type -> counter -> seats for one company and month.
type usageCounts map[int]map[string]int

func newNovelUsageBillingCmd(flags *rootFlags) *cobra.Command {
	var month, agreements string
	var maxCompanies int
	cmd := &cobra.Command{
		Use:   "billing",
		Short: "Show per-company billable usage for --month with own share after the child rollup, seat swings and agreement mismatches",
		Long: `Calls getMonthlyUsagePerProductType for every company in the partner tree for --month
and the month before. For each company and product type it reports the non-zero usage
counters and, for partner companies, the own share (the partner's counters minus its direct
children's, since child usage rolls up into the parent). Flags:
  model_breakdown_exceeds_total  aLaCarte + mspSecure* protection-model counters (a breakdown
                    of endpointMonthlyUsage) add up to more than endpointMonthlyUsage
  negative_own_share:<c>  children report more than the parent: the rollup assumption failed
  seat_swing:<c>    counter c moved more than 50% AND more than 5 seats month over month
  agreement_mismatch  --agreements quantity differs from the Endpoint Security seats
  prior_month_unavailable / own_share_unavailable / agreement_unchecked
                    a needed fetch failed, so that comparison is withheld (never read as zero)
Managed endpoint counts come from the latest 'fleet health' snapshot when one exists.
--agreements reads a CSV with a company (id or name) column and a quantity column.

Use this command for billable usage counters per company for a month, own share after the
child rollup, and month-over-month seat changes. Do NOT use this command for endpoint
protection status; use 'fleet health' instead.`,
		Example:     "  bitdefender-cli usage billing --month 09/2026 --agent\n  bitdefender-cli usage billing --month 09/2026 --agreements psa-agreements.csv --csv",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "usage billing")
			}
			cur, prior, err := billingMonths(month, time.Now().UTC())
			if err != nil {
				return fleetUsageErr(cmd, flags, err)
			}
			var agreed map[string]int
			if agreements != "" {
				if agreed, err = readAgreements(agreements); err != nil {
					return fleetUsageErr(cmd, flags, err)
				}
			}
			// No whole-command deadline: a fan-out over many companies runs far
			// longer than --timeout, which the generated client applies per request.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			s, err := openGZStore(ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			tree, failures, err := companyTreeFor(ctx, c, s.DB(), flags.maxAge, maxCompanies)
			if err != nil && len(tree) == 0 {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view := billingView{Month: cur, PriorMonth: prior, Rows: make([]billingRow, 0), FetchFailures: make([]fetchFailure, 0)}
			view.FetchFailures = append(view.FetchFailures, failures...)
			if err != nil {
				view.Note = err.Error()
				view.FetchFailures = append(view.FetchFailures, fetchFailure{Scope: "company tree", Error: err.Error()})
			}
			limit := len(tree)
			if dogfoodLimit(&limit) {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" live dogfood: %d of %d companies.", limit, len(tree)))
			}
			curUsage := map[string]usageCounts{}
			priorUsage := map[string]usageCounts{}
			for _, co := range tree[:limit] {
				u, err := fetchUsage(ctx, c, co.ID, cur)
				if err != nil {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{Scope: co.Name + " " + cur, Error: err.Error()})
					continue
				}
				curUsage[co.ID] = u
				if p, err := fetchUsage(ctx, c, co.ID, prior); err == nil {
					priorUsage[co.ID] = p
				} else {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{Scope: co.Name + " " + prior, Error: err.Error()})
				}
			}
			managed := latestManagedCounts(ctx, s.DB())
			// Own share subtracts children; with an incomplete child list it would
			// credit missing children's seats to the parent.
			treeComplete := err == nil && len(failures) == 0 && limit == len(tree)
			view.Rows = buildBillingRows(tree[:limit], curUsage, priorUsage, managed, agreed, treeComplete)
			for _, r := range view.Rows {
				if len(r.Flags) > 0 {
					view.Flagged++
				}
			}
			if n := len(view.FetchFailures); n > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d usage fetches failed; own-share math skips companies without data\n", n)
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				humanNote(cmd, view.Note)
				if len(view.Rows) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "No usage returned for "+cur+".")
					return nil
				}
				rows := make([]map[string]any, 0, len(view.Rows))
				for _, r := range view.Rows {
					var own any = r.Usage["endpointMonthlyUsage"]
					if r.OwnShare != nil {
						own = r.OwnShare["endpointMonthlyUsage"]
					} else if slices.Contains(r.Flags, "own_share_unavailable") {
						own = "n/a"
					}
					rows = append(rows, map[string]any{"company": r.CompanyName, "product_type": r.ProductType,
						"endpoints": r.Usage["endpointMonthlyUsage"], "own_endpoints": own, "delta": r.Delta["endpointMonthlyUsage"], "flags": strings.Join(r.Flags, " ")})
				}
				return printAutoTable(cmd.OutOrStdout(), rows)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().StringVar(&month, "month", "", "Billing month as mm/yyyy (default: current month)")
	cmd.Flags().StringVar(&agreements, "agreements", "", "CSV of PSA agreement quantities: a company (id or name) column and a quantity column")
	cmd.Flags().IntVar(&maxCompanies, "max-companies", 5000, "Stop walking the company tree after this many companies")
	return cmd
}

func billingMonths(month string, now time.Time) (string, string, error) {
	t := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if month != "" {
		p, err := time.Parse("01/2006", month)
		if err != nil {
			return "", "", fmt.Errorf("--month must be mm/yyyy, got %q", month)
		}
		t = p
	}
	return t.Format("01/2006"), t.AddDate(0, -1, 0).Format("01/2006"), nil
}

func fetchUsage(ctx context.Context, c *client.Client, companyID, month string) (usageCounts, error) {
	raw, err := gzRead(ctx, c, gzPathUsageByProduct, map[string]any{"companyId": companyID, "targetMonth": month})
	if err != nil {
		return nil, err
	}
	// The licensing docs return 0 (or null) for a company without a monthly
	// licence: that is "no usage", not a failed fetch.
	if t := strings.TrimSpace(string(raw)); t == "" || t == "null" || t == "0" {
		return usageCounts{}, nil
	}
	var resp struct {
		Usages []map[string]json.RawMessage `json:"usages"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("unexpected getMonthlyUsagePerProductType response: %w", err)
	}
	out := usageCounts{}
	for _, u := range resp.Usages {
		pt := 0
		if v, ok := u["productType"]; ok {
			_ = json.Unmarshal(v, &pt)
		}
		counts := map[string]int{}
		for k, v := range u {
			if k == "productType" {
				continue
			}
			var n float64
			if json.Unmarshal(v, &n) == nil {
				counts[k] = int(n)
			}
		}
		out[pt] = counts
	}
	return out, nil
}

func buildBillingRows(tree []gzCompany, cur, prior map[string]usageCounts, managed map[string]int, agreed map[string]int, treeComplete bool) []billingRow {
	children := map[string][]string{}
	for _, co := range tree {
		if co.ParentID != "" {
			children[co.ParentID] = append(children[co.ParentID], co.ID)
		}
	}
	rows := make([]billingRow, 0)
	for _, co := range tree {
		u, ok := cur[co.ID]
		if !ok {
			continue
		}
		p, havePrior := prior[co.ID]
		// Own share needs every direct child's current usage; a failed child
		// fetch must not be read as zero seats.
		kidsKnown := treeComplete
		for _, kid := range children[co.ID] {
			if _, ok := cur[kid]; !ok {
				kidsKnown = false
			}
		}
		seenPT := map[int]bool{}
		for pt := range u {
			seenPT[pt] = true
		}
		for pt := range p {
			seenPT[pt] = true // a product that disappeared this month still gets a row
		}
		_, byID := agreed[strings.ToLower(co.ID)]
		_, byName := agreed[strings.ToLower(co.Name)]
		if byID || byName {
			seenPT[0] = true // compare confirmed zero usage against the agreement
		}
		pts := make([]int, 0, len(seenPT))
		for pt := range seenPT {
			pts = append(pts, pt)
		}
		sort.Ints(pts)
		for _, pt := range pts {
			r := billingRow{CompanyID: co.ID, CompanyName: co.Name, CompanyType: co.Type, ParentID: co.ParentID, ProductType: pt,
				Usage: nonZero(u[pt]), PriorUsage: map[string]int{}, Delta: map[string]int{}, Flags: make([]string, 0)}
			// With an incomplete tree a partner may have undiscovered children.
			hasChildren := co.Type == 0 && (len(children[co.ID]) > 0 || !treeComplete)
			if hasChildren && kidsKnown {
				r.OwnShare = map[string]int{}
				for k, v := range u[pt] {
					own := v
					for _, kid := range children[co.ID] {
						own -= cur[kid][pt][k]
					}
					if own < 0 {
						r.Flags = append(r.Flags, "negative_own_share:"+k)
					}
					if own != 0 {
						r.OwnShare[k] = own
					}
				}
			} else if hasChildren {
				r.Flags = append(r.Flags, "own_share_unavailable")
			}
			if !havePrior {
				r.Flags = append(r.Flags, "prior_month_unavailable")
			} else {
				r.PriorUsage = nonZero(p[pt])
				keys := map[string]bool{}
				for k := range r.Usage {
					keys[k] = true
				}
				for k := range r.PriorUsage {
					keys[k] = true
				}
				for k := range keys {
					d := r.Usage[k] - r.PriorUsage[k]
					if d == 0 {
						continue
					}
					r.Delta[k] = d
					if seatSwing(r.PriorUsage[k], r.Usage[k]) {
						r.Flags = append(r.Flags, "seat_swing:"+k)
					}
				}
			}
			// The protection-model counters are a breakdown of endpointMonthlyUsage;
			// flag a breakdown that adds up to more than the total.
			breakdown := 0
			for k, v := range r.Usage {
				if k == "aLaCarteMonthlyUsage" || strings.HasPrefix(k, "mspSecure") {
					breakdown += v
				}
			}
			if breakdown > r.Usage["endpointMonthlyUsage"] {
				r.Flags = append(r.Flags, "model_breakdown_exceeds_total")
			}
			if n, ok := managed[co.ID]; ok {
				r.ManagedEndpoints = &n
			}
			if pt == 0 && agreed != nil {
				q, ok := agreed[strings.ToLower(co.ID)]
				if !ok {
					q, ok = agreed[strings.ToLower(co.Name)]
				}
				if ok {
					r.AgreementQty = &q
					billed := r.Usage["endpointMonthlyUsage"]
					if r.OwnShare != nil {
						billed = r.OwnShare["endpointMonthlyUsage"]
					}
					switch {
					case hasChildren && r.OwnShare == nil:
						r.Flags = append(r.Flags, "agreement_unchecked")
					case billed != q:
						r.Flags = append(r.Flags, fmt.Sprintf("agreement_mismatch:%d_billed_vs_%d_agreed", billed, q))
					}
				}
			}
			sort.Strings(r.Flags)
			rows = append(rows, r)
		}
	}
	return rows
}

// seatSwing is the ERPNext review guard: a move of more than 50% and more than 5 seats.
func seatSwing(prev, cur int) bool {
	d := math.Abs(float64(cur - prev))
	if d <= 5 {
		return false
	}
	if prev == 0 {
		return true
	}
	return d/float64(prev) > 0.5
}

func nonZero(m map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range m {
		if v != 0 {
			out[k] = v
		}
	}
	return out
}

func latestManagedCounts(ctx context.Context, db *sql.DB) map[string]int {
	out := map[string]int{}
	rows, err := db.QueryContext(ctx, `SELECT company_id, SUM(is_managed) FROM gz_fleet_endpoints
		WHERE run_id = (SELECT MAX(run_id) FROM gz_fleet_runs) GROUP BY company_id`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if rows.Scan(&id, &n) == nil {
			out[id] = n
		}
	}
	return out
}

// readAgreements loads company -> quantity from a CSV with a header row. The
// company column is the first header named company, company_id, customer,
// account or name; the quantity column is quantity, qty, seats or licenses.
func readAgreements(path string) (map[string]int, error) {
	b, err := readBoundedFile("--agreements", path)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("--agreements: %w", err)
	}
	if len(recs) < 2 {
		return nil, fmt.Errorf("--agreements: need a header row and at least one data row")
	}
	ci, qi := -1, -1
	for i, h := range recs[0] {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "company", "company_id", "companyid", "customer", "account", "name":
			if ci < 0 {
				ci = i
			}
		case "quantity", "qty", "seats", "licenses", "licences":
			if qi < 0 {
				qi = i
			}
		}
	}
	if ci < 0 || qi < 0 {
		return nil, fmt.Errorf("--agreements: header must name a company column and a quantity column")
	}
	out := map[string]int{}
	for n, rec := range recs[1:] {
		if strings.TrimSpace(strings.Join(rec, "")) == "" {
			continue // blank line
		}
		if len(rec) <= ci || len(rec) <= qi || strings.TrimSpace(rec[ci]) == "" {
			return nil, fmt.Errorf("--agreements: record %d is missing the company or quantity", n+2)
		}
		q, err := strconv.Atoi(strings.TrimSpace(rec[qi]))
		if err != nil {
			return nil, fmt.Errorf("--agreements: record %d quantity %q is not a whole number", n+2, rec[qi])
		}
		out[strings.ToLower(strings.TrimSpace(rec[ci]))] += q // duplicate rows add up
	}
	return out, nil
}
