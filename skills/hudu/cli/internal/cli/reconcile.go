// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence feature: integration reconciliation report. Bulk
// classifies PSA/RMM integration matchers from the local mirror into matched,
// potential, and orphaned — drift no per-record cards/lookup call can surface.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type matcherRow struct {
	ID                 int    `json:"id"`
	Integrator         string `json:"integrator,omitempty"`
	Identifier         string `json:"identifier,omitempty"`
	CompanyID          int    `json:"company_id,omitempty"`
	CompanyName        string `json:"company_name,omitempty"`
	PotentialCompanyID int    `json:"potential_company_id,omitempty"`
	State              string `json:"state"` // matched | potential | orphaned
}

type reconcileReport struct {
	Integrator string       `json:"integrator,omitempty"`
	Matched    int          `json:"matched"`
	Potential  int          `json:"potential"`
	Orphaned   int          `json:"orphaned"`
	Unresolved []matcherRow `json:"unresolved"`
}

func matcherIntegrator(m map[string]any) string {
	return firstString(m, "integrator_name", "sync_type", "name", "integration_name", "sync_name")
}

func newNovelReconcileCmd(flags *rootFlags) *cobra.Command {
	var flagIntegrator string
	var includeMatched bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "reconcile",
		Short:       "Bulk bidirectional check of which integrator (PSA/RMM) records resolve to a live Hudu asset and which are orphaned.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Classify integration matchers from the local mirror (run 'sync' first) into:
  - matched:   linked to a Hudu company (company_id set)
  - potential: a candidate company was suggested but not confirmed
  - orphaned:  no company link at all

By default only unresolved (potential + orphaned) matchers are listed, with a
summary count of all three states. Filter to one integrator with --integrator.`,
		Example: `  # All integrators
  hudu-cli reconcile

  # One PSA integrator, as JSON
  hudu-cli reconcile --integrator cw_manage --agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
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
			want := strings.ToLower(strings.TrimSpace(flagIntegrator))

			rows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM matchers`)
			if err != nil {
				return fmt.Errorf("reading matchers: %w", err)
			}

			report := reconcileReport{Integrator: flagIntegrator, Unresolved: []matcherRow{}}
			for _, raw := range rows {
				var m map[string]any
				if json.Unmarshal(raw, &m) != nil {
					continue
				}
				integ := matcherIntegrator(m)
				if want != "" && !strings.Contains(strings.ToLower(integ), want) {
					continue
				}
				cid := intField(m, "company_id")
				pid := intField(m, "potential_company_id")
				state := "orphaned"
				switch {
				case cid > 0:
					state = "matched"
					report.Matched++
				case pid > 0:
					state = "potential"
					report.Potential++
				default:
					report.Orphaned++
				}
				if state == "matched" && !includeMatched {
					continue
				}
				report.Unresolved = append(report.Unresolved, matcherRow{
					ID: intField(m, "id"), Integrator: integ,
					Identifier: firstString(m, "identifier", "sync_id"),
					CompanyID:  cid, CompanyName: companyNames[cid],
					PotentialCompanyID: pid, State: state,
				})
			}
			sort.Slice(report.Unresolved, func(i, j int) bool {
				if report.Unresolved[i].State != report.Unresolved[j].State {
					return report.Unresolved[i].State < report.Unresolved[j].State
				}
				return report.Unresolved[i].ID < report.Unresolved[j].ID
			})

			return emitAudit(cmd, flags, report, func(w io.Writer) {
				fmt.Fprintf(w, "Integration matchers — matched: %d  potential: %d  orphaned: %d\n",
					report.Matched, report.Potential, report.Orphaned)
				if report.Matched+report.Potential+report.Orphaned == 0 {
					fmt.Fprintln(w, "No matchers found. (Run 'hudu-cli sync' first if unexpected.)")
					return
				}
				if len(report.Unresolved) == 0 {
					fmt.Fprintln(w, "All matchers are resolved to a company.")
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "STATE\tINTEGRATOR\tIDENTIFIER\tCOMPANY")
				for _, r := range report.Unresolved {
					company := r.CompanyName
					if company == "" && r.State != "matched" {
						company = "-"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.State, r.Integrator, r.Identifier, company)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&flagIntegrator, "integrator", "", "Filter by integrator name/slug (substring match)")
	cmd.Flags().BoolVar(&includeMatched, "include-matched", false, "Also list already-matched records (default: only unresolved)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
