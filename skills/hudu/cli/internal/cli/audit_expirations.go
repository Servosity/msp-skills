// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored transcendence feature: expiration radar. A windowed, typed view
// of expiring SSL certs, domains, warranties, and passwords, unioning the local
// expirations mirror with website SSL/domain dates — sorted by days remaining,
// at zero API cost. Hudu's UI rollup has no horizon lens.

// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type expirationRow struct {
	Type          string `json:"type"`
	Item          string `json:"item"`
	CompanyID     int    `json:"company_id,omitempty"`
	CompanyName   string `json:"company_name,omitempty"`
	ExpiresOn     string `json:"expires_on"`
	DaysRemaining int    `json:"days_remaining"`
	Expired       bool   `json:"expired"`
}

func newNovelAuditExpirationsCmd(flags *rootFlags) *cobra.Command {
	var within string
	var typeFilter string
	var flagCompany int
	var includeExpired bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "expirations",
		Short:       "Windowed, typed view of expiring SSL certs, domains, warranties, and passwords sorted by days remaining.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Show items expiring within a window, newest-deadline first, from the local
mirror (run 'sync' first). Unions the Hudu expirations rollup with website
SSL/domain expiry dates. Already-expired items are included by default (negative
days remaining) since they are the most urgent; pass --include-expired=false to
hide them.`,
		Example: `  # Everything expiring in the next 30 days
  hudu-cli audit expirations --within 30d

  # Only SSL certificates, as JSON
  hudu-cli audit expirations --within 60d --type ssl --agent`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			windowDays, err := parseAgeDays(within)
			if err != nil {
				return usageErr(err)
			}
			typeFilter = strings.ToLower(strings.TrimSpace(typeFilter))
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

			out := []expirationRow{}
			add := func(typ, item string, cid int, on string) {
				if typeFilter != "" && typeFilter != typ {
					return
				}
				if flagCompany > 0 && cid != flagCompany {
					return
				}
				days, ok := daysUntil(on, now)
				if !ok {
					return
				}
				if days > windowDays {
					return
				}
				if days < 0 && !includeExpired {
					return
				}
				out = append(out, expirationRow{
					Type: typ, Item: item, CompanyID: cid, CompanyName: companyNames[cid],
					ExpiresOn: on, DaysRemaining: days, Expired: days < 0,
				})
			}

			// Source 1: expirations rollup.
			expRows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM expirations`)
			if err == nil {
				for _, raw := range expRows {
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						continue
					}
					typ := normalizeExpType(asString(m["expiration_type"]))
					item := asString(m["resource_type"])
					if rid := intField(m, "resource_id"); rid > 0 {
						item = fmt.Sprintf("%s #%d", item, rid)
					}
					add(typ, item, intField(m, "company_id"), asString(m["date"]))
				}
			}

			// Source 2: website SSL + domain expiry.
			webRows, err := queryDataRows(cmd.Context(), db, `SELECT data FROM websites`)
			if err == nil {
				for _, raw := range webRows {
					var m map[string]any
					if json.Unmarshal(raw, &m) != nil {
						continue
					}
					name := asString(m["name"])
					cid := intField(m, "company_id")
					if ssl := asString(m["ssl_expiration_date"]); ssl != "" {
						add("ssl", name, cid, ssl)
					}
					if dom := asString(m["domain_expiration_date"]); dom != "" {
						add("domain", name, cid, dom)
					}
				}
			}

			sort.Slice(out, func(i, j int) bool { return out[i].DaysRemaining < out[j].DaysRemaining })

			return emitAudit(cmd, flags, out, func(w io.Writer) {
				if len(out) == 0 {
					fmt.Fprintf(w, "Nothing expiring within %d days. (Run 'hudu-cli sync' first if unexpected.)\n", windowDays)
					return
				}
				tw := newTabWriter(w)
				fmt.Fprintln(tw, "DAYS\tTYPE\tITEM\tCOMPANY\tEXPIRES")
				for _, r := range out {
					days := fmt.Sprintf("%d", r.DaysRemaining)
					if r.Expired {
						days += " (EXPIRED)"
					}
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", days, r.Type, r.Item, r.CompanyName, r.ExpiresOn)
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&within, "within", "30d", "Horizon window (e.g. 30d, 12w, 3m)")
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by type: ssl, domain, warranty, password")
	cmd.Flags().IntVar(&flagCompany, "company", 0, "Limit to a single company id")
	cmd.Flags().BoolVar(&includeExpired, "include-expired", true, "Include already-expired items (negative days)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// normalizeExpType maps Hudu's expiration_type values onto the four user-facing
// type buckets used by --type.
func normalizeExpType(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.Contains(s, "ssl") || strings.Contains(s, "certificate"):
		return "ssl"
	case strings.Contains(s, "domain") || strings.Contains(s, "whois"):
		return "domain"
	case strings.Contains(s, "warranty"):
		return "warranty"
	case strings.Contains(s, "password") || strings.Contains(s, "credential"):
		return "password"
	case s == "":
		return "other"
	default:
		return s
	}
}
