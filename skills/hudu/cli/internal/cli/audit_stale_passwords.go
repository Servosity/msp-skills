// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence feature: stale-password audit. Fills Hudu's #1
// unmet feature request (native password expiration) by computing rotation age
// from the local mirror. Never reads or stores the secret — name/username only.

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

type stalePasswordRow struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Username    string `json:"username,omitempty"`
	CompanyID   int    `json:"company_id"`
	CompanyName string `json:"company_name,omitempty"`
	UpdatedAt   string `json:"updated_at"`
	DaysStale   int    `json:"days_stale"`
}

func newNovelAuditStalePasswordsCmd(flags *rootFlags) *cobra.Command {
	var olderThan string
	var flagCompany int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "stale-passwords",
		Short:       "Find vault passwords not rotated within a threshold, grouped by company.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Report password-vault entries whose last update predates a threshold.

Hudu has no native password-expiration tracking; this computes rotation age
locally from asset_passwords.updated_at (run 'sync' first). The secret value is
never read or stored — only the entry name, username, and last-updated date.`,
		Example: `  # Credentials untouched for 6 months
  hudu-cli audit stale-passwords --older-than 180d

  # For one company, as JSON
  hudu-cli audit stale-passwords --older-than 1y --company 42 --agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			threshold, err := parseAgeDays(olderThan)
			if err != nil {
				return usageErr(err)
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openAuditStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "asset_passwords") {
				hintIfStale(cmd, db, "asset_passwords", flags.maxAge)
			}

			companyNames := loadCompanyNames(cmd.Context(), db)

			q := `SELECT data FROM asset_passwords`
			var qargs []any
			if flagCompany > 0 {
				q += ` WHERE company_id = ?`
				qargs = append(qargs, flagCompany)
			}
			rows, err := queryDataRows(cmd.Context(), db, q, qargs...)
			if err != nil {
				return fmt.Errorf("reading asset passwords: %w", err)
			}

			now := time.Now()
			out := []stalePasswordRow{}
			for _, raw := range rows {
				var m map[string]any
				if json.Unmarshal(raw, &m) != nil {
					continue
				}
				updated := asString(m["updated_at"])
				days, ok := ageDays(updated, now)
				if !ok || days < threshold {
					continue
				}
				cid := intField(m, "company_id")
				out = append(out, stalePasswordRow{
					ID:          intField(m, "id"),
					Name:        asString(m["name"]),
					Username:    asString(m["username"]),
					CompanyID:   cid,
					CompanyName: companyNames[cid],
					UpdatedAt:   updated,
					DaysStale:   days,
				})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].DaysStale > out[j].DaysStale })

			return emitAudit(cmd, flags, out, func(w io.Writer) {
				if len(out) == 0 {
					fmt.Fprintf(w, "No passwords older than %d days. (Run 'hudu-cli sync' first if unexpected.)\n", threshold)
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "DAYS\tNAME\tUSERNAME\tCOMPANY\tUPDATED")
				for _, r := range out {
					fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", r.DaysStale, r.Name, r.Username, r.CompanyName, r.UpdatedAt)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "180d", "Staleness threshold (e.g. 180d, 26w, 6m, 1y)")
	cmd.Flags().IntVar(&flagCompany, "company", 0, "Limit to a single company id")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
