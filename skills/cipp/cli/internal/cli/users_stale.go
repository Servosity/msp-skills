// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cipp-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// staleRow reports one licensed account with no recent sign-in.
type staleRow struct {
	Tenant            string `json:"tenant"`
	UserPrincipalName string `json:"userPrincipalName"`
	DisplayName       string `json:"displayName"`
	LastSignIn        string `json:"lastSignIn"`
	DaysSince         int    `json:"daysSince"`
}

// userIsLicensed reports whether a stored user row carries any assigned
// license. Reuses userLicenseNames from licenses_waste.go.
func userIsLicensed(obj map[string]any) bool {
	return len(userLicenseNames(obj)) > 0
}

func newNovelUsersStaleCmd(flags *rootFlags) *cobra.Command {
	var flagDays string
	var flagDB string
	var flagAllTenants bool

	cmd := &cobra.Command{
		Use:         "stale",
		Short:       "Flag licensed accounts with no sign-in in N days across every tenant.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Scan synced users across every tenant and flag licensed accounts whose last
sign-in is older than N days (or null). Reads the local store only.

Populate the store first:
  cipp-cli fanout --endpoint /ListUsers --all-tenants --save`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && flagDays == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if !flagAllTenants {
				return usageErr(fmt.Errorf("--all-tenants=false is not supported; this command always reads the full local store across all synced tenants"))
			}
			days := 90
			if flagDays != "" {
				n, err := strconv.Atoi(strings.TrimSpace(flagDays))
				if err != nil || n < 0 {
					return usageErr(fmt.Errorf("invalid --days %q: must be a non-negative integer", flagDays))
				}
				days = n
			}

			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("cipp-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			users, err := readResourceRows(db, "users")
			if err != nil {
				return fmt.Errorf("reading users: %w", err)
			}
			if len(users) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"no synced users data; populate it with: cipp-cli fanout --endpoint /ListUsers --all-tenants --save")
				if flags.asJSON {
					return flags.printJSON(cmd, []staleRow{})
				}
				return nil
			}

			now := time.Now()
			cutoff := now.AddDate(0, 0, -days)

			out := make([]staleRow, 0)
			for _, u := range users {
				if !userIsLicensed(u) {
					continue
				}
				lastSignIn, has := userLastSignIn(u)
				stale := false
				lastStr := ""
				daysSince := -1
				if !has {
					// Null sign-in counts as stale.
					stale = true
				} else {
					lastStr = lastSignIn.UTC().Format(time.RFC3339)
					daysSince = int(now.Sub(lastSignIn).Hours() / 24)
					if lastSignIn.Before(cutoff) {
						stale = true
					}
				}
				if !stale {
					continue
				}
				out = append(out, staleRow{
					Tenant:            rowTenant(u),
					UserPrincipalName: tenantFieldLookup(u, "userPrincipalName", "UserPrincipalName", "upn", "mail"),
					DisplayName:       tenantFieldLookup(u, "displayName", "DisplayName"),
					LastSignIn:        lastStr,
					DaysSince:         daysSince,
				})
			}

			sort.Slice(out, func(i, j int) bool {
				if out[i].Tenant != out[j].Tenant {
					return out[i].Tenant < out[j].Tenant
				}
				return out[i].UserPrincipalName < out[j].UserPrincipalName
			})

			if flags.asJSON {
				return flags.printJSON(cmd, out)
			}
			headers := []string{"TENANT", "UPN", "NAME", "LAST SIGN-IN", "DAYS"}
			tableRows := make([][]string, 0, len(out))
			for _, r := range out {
				dayStr := ""
				if r.DaysSince >= 0 {
					dayStr = fmt.Sprintf("%d", r.DaysSince)
				} else {
					dayStr = "never"
				}
				lastDisplay := r.LastSignIn
				if lastDisplay == "" {
					lastDisplay = "(never)"
				}
				tableRows = append(tableRows, []string{r.Tenant, r.UserPrincipalName, r.DisplayName, lastDisplay, dayStr})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&flagDays, "days", "", "Flag accounts with no sign-in in this many days (default 90)")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", true, "Scan all synced tenants (default; this command always reads the full local store)")
	return cmd
}
