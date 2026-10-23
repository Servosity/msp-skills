// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command: retention and recovery-guarantee audit.

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// retentionTiers are the Servosity retention enum values (Backup, ResticBackup
// and DRBackup all share it), shortest first.
var retentionTiers = []string{"1YEAR", "4YEAR", "7YEAR", "10YEAR"}

// retentionRank returns a tier's position in retentionTiers, or -1 when the
// value is not a known tier.
func retentionRank(tier string) int {
	tier = strings.ToUpper(strings.TrimSpace(tier))
	for i, t := range retentionTiers {
		if t == tier {
			return i
		}
	}
	return -1
}

type retentionFinding struct {
	CompanyID         *int64   `json:"company_id"`
	CompanyName       string   `json:"company_name"`
	Engine            string   `json:"engine"`
	ID                string   `json:"id"`
	Hostname          string   `json:"hostname"`
	Retention         string   `json:"retention"`
	GuaranteeEligible *bool    `json:"guarantee_eligible"`
	Reasons           []string `json:"reasons"`
}

// retentionReasons returns why one host fails the audit, or nil when it
// passes. minRank < 0 disables the retention check. A host with no recorded
// retention or guarantee flag fails that check: compliance is something the
// record has to show, not something an empty field is allowed to imply.
func retentionReasons(retention string, guarantee *bool, minRank int, checkGuarantee bool) []string {
	var reasons []string
	if minRank >= 0 {
		switch rank := retentionRank(retention); {
		case retention == "":
			reasons = append(reasons, "retention not recorded")
		case rank < 0:
			reasons = append(reasons, fmt.Sprintf("unknown retention %q", retention))
		case rank < minRank:
			reasons = append(reasons, fmt.Sprintf("retention %s below %s", strings.ToUpper(retention), retentionTiers[minRank]))
		}
	}
	if checkGuarantee {
		switch {
		case guarantee == nil:
			reasons = append(reasons, "guarantee eligibility not recorded")
		case !*guarantee:
			reasons = append(reasons, "not guarantee-eligible")
		}
	}
	return reasons
}

// pp:data-source local
func newNovelRetentionAuditCmd(flags *rootFlags) *cobra.Command {
	var minTier string
	var companyID int
	var ignoreGuarantee bool

	cmd := &cobra.Command{
		Use:   "retention-audit",
		Short: "Hosts below a required retention tier or not covered by the recovery guarantee, across all three engines",
		Long: `List every protected host across the classic (/backups/), restic
(/restic-backups/) and DR (/dr-backups/) engines that fails the audit, grouped
by client. A host fails when its retention tier is below --min, or when it is
not eligible for the Servosity recovery guarantee. A host whose record does not
show a retention tier or guarantee flag fails that check too.

Reads the local store only; run 'sync' first. Prints nothing when every host
passes.`,
		Example: `  # Hosts below 7-year retention or not guarantee-eligible
  servosity-cli retention-audit --min 7YEAR

  # One regulated client, retention only
  servosity-cli retention-audit --min 7YEAR --company 4421 --ignore-guarantee --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "retention-audit")
			}
			minRank := -1
			if strings.TrimSpace(minTier) != "" {
				if minRank = retentionRank(minTier); minRank < 0 {
					return usageErr(fmt.Errorf("invalid --min %q (want %s)", minTier, strings.Join(retentionTiers, "|")))
				}
			}
			if minRank < 0 && ignoreGuarantee {
				return usageErr(fmt.Errorf("nothing to audit: pass --min, or drop --ignore-guarantee"))
			}

			db, err := openStoreForRead(cmd.Context(), "servosity-cli")
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			if db == nil {
				return fmt.Errorf("no local database yet; run 'servosity-cli sync' first")
			}
			defer db.Close()

			// Same per-engine company expressions as backup-facts: classic nests
			// the company object, restic and DR store a ".../companies/<id>/" URL.
			classicCID := `CAST(json_extract(b.data, '$.company.id') AS INTEGER)`
			urlCID := `CAST(substr(json_extract(b.data, '$.company'), instr(json_extract(b.data, '$.company'), '/companies/') + 11) AS INTEGER)`
			leg := func(engine, table, cid, nameFallback string) string {
				return `
SELECT ` + cid + ` AS company_id,
  COALESCE(c.name, ` + nameFallback + `, '') AS company_name,
  '` + engine + `' AS engine,
  b.id AS id,
  COALESCE(json_extract(b.data, '$.device_name'), json_extract(b.data, '$.display_name'),
           json_extract(b.data, '$.login'), '') AS hostname,
  COALESCE(json_extract(b.data, '$.retention'), '') AS retention,
  json_extract(b.data, '$.guarantee_eligible') AS guarantee
FROM ` + table + ` b
LEFT JOIN companies c ON CAST(c.id AS INTEGER) = ` + cid
			}
			query := "SELECT * FROM (" + strings.Join([]string{
				leg("classic", "backups", classicCID, "json_extract(b.data, '$.company.name')"),
				leg("restic", "restic_backups", urlCID, "NULL"),
				leg("dr", drBackupsTable, urlCID, "NULL"),
			}, "\nUNION ALL\n") + ") AS u"
			var bind []any
			if companyID > 0 {
				query += " WHERE u.company_id = ?"
				bind = append(bind, int64(companyID))
			}
			query += " ORDER BY u.company_name, u.engine, u.hostname"

			rows, err := db.DB().QueryContext(cmd.Context(), query, bind...)
			if err != nil {
				return fmt.Errorf("querying backups: %w", err)
			}
			defer rows.Close()

			findings := []retentionFinding{}
			total := 0
			for rows.Next() {
				var (
					cid       sql.NullInt64
					cname     sql.NullString
					eng       sql.NullString
					id        sql.NullString
					host      sql.NullString
					retention sql.NullString
					guarantee sql.NullInt64
				)
				if err := rows.Scan(&cid, &cname, &eng, &id, &host, &retention, &guarantee); err != nil {
					return fmt.Errorf("scanning backup row: %w", err)
				}
				total++
				var g *bool
				if guarantee.Valid {
					v := guarantee.Int64 != 0
					g = &v
				}
				reasons := retentionReasons(retention.String, g, minRank, !ignoreGuarantee)
				if len(reasons) == 0 {
					continue
				}
				f := retentionFinding{
					CompanyName:       cname.String,
					Engine:            eng.String,
					ID:                id.String,
					Hostname:          host.String,
					Retention:         strings.ToUpper(retention.String),
					GuaranteeEligible: g,
					Reasons:           reasons,
				}
				if cid.Valid {
					v := cid.Int64
					f.CompanyID = &v
				}
				findings = append(findings, f)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating backup rows: %w", err)
			}
			if total == 0 {
				fmt.Fprintln(os.Stderr, "no backups in the local store; run 'servosity-cli sync' first")
			} else {
				fmt.Fprintf(os.Stderr, "%d of %d hosts fail the audit\n", len(findings), total)
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(findings)
			}
			if len(findings) == 0 {
				return nil
			}
			headers := []string{"COMPANY", "ENGINE", "ID", "HOSTNAME", "RETENTION", "GUARANTEE", "REASONS"}
			out := make([][]string, 0, len(findings))
			for _, f := range findings {
				company := f.CompanyName
				if f.CompanyID != nil {
					company = strings.TrimSpace(fmt.Sprintf("%s (%d)", company, *f.CompanyID))
				}
				guarantee := "-"
				if f.GuaranteeEligible != nil {
					guarantee = fmt.Sprintf("%t", *f.GuaranteeEligible)
				}
				out = append(out, []string{company, f.Engine, f.ID, dashIfEmpty(f.Hostname), dashIfEmpty(f.Retention), guarantee, strings.Join(f.Reasons, "; ")})
			}
			return flags.printTable(cmd, headers, out)
		},
	}
	cmd.Flags().StringVar(&minTier, "min", "", "Required retention tier: 1YEAR|4YEAR|7YEAR|10YEAR (empty skips the retention check)")
	cmd.Flags().IntVar(&companyID, "company", 0, "Audit one company ID (0 = all)")
	cmd.Flags().BoolVar(&ignoreGuarantee, "ignore-guarantee", false, "Do not flag hosts that are not eligible for the recovery guarantee")
	return cmd
}

func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
