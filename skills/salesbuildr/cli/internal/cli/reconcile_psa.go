// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// psaResourceGap summarizes external-identifier completeness for one resource.
type psaResourceGap struct {
	Resource   string        `json:"resource"`
	Total      int           `json:"total"`
	MissingExt int           `json:"missingExt"`
	MissingPct float64       `json:"missingPct"`
	Sample     []extIDRecord `json:"sample,omitempty"` // up to --limit missing rows
}

type psaReconcileReport struct {
	Resources    []psaResourceGap `json:"resources"`
	TotalMissing int              `json:"totalMissing"`
	Note         string           `json:"note,omitempty"`
}

// psaResources are the PSA-synced resource types reconcile-psa inspects, in
// display order.
var psaResources = []string{"company", "contact", "product", "opportunity"}

// newNovelReconcilePsaCmd implements the "reconcile-psa" transcendence
// command: rows whose missing external identifier will silently skip PSA sync.
// pp:data-source local
func newNovelReconcilePsaCmd(flags *rootFlags) *cobra.Command {
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "reconcile-psa",
		Short: "Find companies, contacts, products, and opportunities missing the external identifier a PSA sync needs",
		Long: "Scans the local store's companies, contacts, products, and opportunities for rows whose\n" +
			"externalIdentifier is empty — the records an Autotask/ConnectWise/HaloPSA integration will\n" +
			"silently skip. The API is single-resource; only the local store can answer this in one pass.\n" +
			"Run `sync` first.",
		Example: "  salesbuildr-cli reconcile-psa\n" +
			"  salesbuildr-cli reconcile-psa --limit 10 --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if flagLimit < 0 {
				return usageErr(fmt.Errorf("--limit must be >= 0"))
			}
			db, err := openNovelStore(cmd, flags, dbPath, "")
			if err != nil {
				return err
			}
			defer db.Close()

			report := psaReconcileReport{Resources: make([]psaResourceGap, 0, len(psaResources))}
			totalRows := 0
			for _, resource := range psaResources {
				records, err := loadExtIDRecords(db, resource)
				if err != nil {
					return err
				}
				gap := psaResourceGap{Resource: resource, Total: len(records), Sample: make([]extIDRecord, 0)}
				for _, r := range records {
					if r.ExternalIdentifier != "" {
						continue
					}
					gap.MissingExt++
					if len(gap.Sample) < flagLimit {
						gap.Sample = append(gap.Sample, r)
					}
				}
				if gap.Total > 0 {
					gap.MissingPct = float64(gap.MissingExt) / float64(gap.Total) * 100
				}
				totalRows += gap.Total
				report.TotalMissing += gap.MissingExt
				report.Resources = append(report.Resources, gap)
			}
			if totalRows == 0 {
				report.Note = "local store is empty — run `salesbuildr-cli sync` first"
			} else if report.TotalMissing == 0 {
				report.Note = "every synced row carries an external identifier — PSA sync coverage is complete"
			}

			w := cmd.OutOrStdout()
			if flags.asJSON || flags.agent || !isTerminal(w) {
				return printJSONFiltered(w, report, flags)
			}
			tw := newTabWriter(w)
			fmt.Fprintln(tw, "RESOURCE\tTOTAL\tMISSING-EXT\tMISSING%")
			for _, g := range report.Resources {
				fmt.Fprintf(tw, "%s\t%d\t%d\t%.1f\n", g.Resource, g.Total, g.MissingExt, g.MissingPct)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if report.Note != "" {
				fmt.Fprintf(w, "\n%s\n", report.Note)
			}
			for _, g := range report.Resources {
				if len(g.Sample) == 0 {
					continue
				}
				fmt.Fprintf(w, "\n%s missing external id (first %d):\n", g.Resource, len(g.Sample))
				for _, r := range g.Sample {
					fmt.Fprintf(w, "  %s  %s\n", r.ID, r.Name)
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 5, "Sample rows to list per resource (0 hides samples)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}
