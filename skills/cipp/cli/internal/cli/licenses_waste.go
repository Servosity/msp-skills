// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"cipp-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// wasteRow reports assigned-but-unused license seats per tenant + SKU.
type wasteRow struct {
	Tenant      string `json:"tenant"`
	LicenseName string `json:"licenseName"`
	SkuID       string `json:"skuId,omitempty"`
	Assigned    int    `json:"assigned"`
	Unused      int    `json:"unused"`
}

// userLicenseNames returns the set of license SKU/display names assigned to a
// stored user row. CIPP exposes assigned licenses under a few shapes
// (assignedLicenses array of objects, or a flat Licenses string).
func userLicenseNames(obj map[string]any) []string {
	var names []string
	for k, v := range obj {
		if !strings.EqualFold(k, "assignedLicenses") && !strings.EqualFold(k, "Licenses") && !strings.EqualFold(k, "LicenseAssignments") {
			continue
		}
		switch t := v.(type) {
		case []any:
			for _, el := range t {
				switch e := el.(type) {
				case map[string]any:
					if n := tenantFieldLookup(e, "skuPartNumber", "License", "name", "skuId", "Sku"); n != "" {
						names = append(names, n)
					}
				case string:
					if e != "" {
						names = append(names, e)
					}
				}
			}
		case string:
			for _, part := range strings.Split(t, ",") {
				part = strings.TrimSpace(part)
				if part != "" {
					names = append(names, part)
				}
			}
		}
	}
	return names
}

// skuDisplayNames maps a SKU id, as user rows carry it in assignedLicenses,
// to the readable name the stored /ListLicenses rows give it (the License
// friendly name, else skuPartNumber). Keys are lower-cased.
func skuDisplayNames(db *store.Store) map[string]string {
	names := map[string]string{}
	rows, err := readResourceRows(db, "licenses")
	if err != nil {
		return names
	}
	for _, r := range rows {
		id := strings.ToLower(tenantFieldLookup(r, "skuId", "SkuId"))
		if id == "" {
			continue
		}
		if n := tenantFieldLookup(r, "License", "skuPartNumber", "SkuPartNumber"); n != "" {
			names[id] = n
		}
	}
	return names
}

// nameSKUs replaces SKU ids with their stored display names where known.
func nameSKUs(skus []string, names map[string]string) []string {
	out := make([]string, 0, len(skus))
	for _, s := range skus {
		if n, ok := names[strings.ToLower(s)]; ok {
			s = n
		}
		out = append(out, s)
	}
	return out
}

// userHasSignInField reports whether a user row carries a sign-in field at
// all (possibly null). Without one, sign-in activity is unknown, which is not
// the same as "never signed in": /ListUsers omits it unless the fanout asked
// for IncludeLogonDetails.
func userHasSignInField(obj map[string]any) bool {
	for _, key := range []string{"LastSignInDateTime", "lastSignInDateTime", "lastSignIn", "LastSignIn", "lastLogon", "signInActivity"} {
		if _, ok := lookupField(obj, key); ok {
			return true
		}
	}
	return false
}

// userLastSignIn extracts a parseable last-sign-in time from a user row, if
// present. Returns (time, true) only when a value parses.
func userLastSignIn(obj map[string]any) (time.Time, bool) {
	for _, key := range []string{"LastSignInDateTime", "lastSignInDateTime", "lastSignIn", "LastSignIn", "lastLogon", "signInActivity"} {
		for k, v := range obj {
			if !strings.EqualFold(k, key) {
				continue
			}
			// Graph/CIPP's signInActivity is a nested OBJECT carrying the
			// actual timestamps; a bare string assertion would skip it and
			// silently misclassify users whose rows only have that shape.
			if m, ok := v.(map[string]any); ok {
				for _, nested := range []string{"lastSignInDateTime", "lastNonInteractiveSignInDateTime"} {
					for nk, nv := range m {
						if !strings.EqualFold(nk, nested) {
							continue
						}
						if ts, ok := parseSignInTime(nv); ok {
							return ts, true
						}
					}
				}
				continue
			}
			if ts, ok := parseSignInTime(v); ok {
				return ts, true
			}
		}
	}
	return time.Time{}, false
}

// parseSignInTime parses a sign-in timestamp in the two shapes CIPP emits:
// RFC3339 and timezone-less.
func parseSignInTime(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return ts, true
	}
	// CIPP sometimes emits without timezone.
	if ts, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

// userIsActive reports whether a user account is enabled/active.
func userIsActive(obj map[string]any) bool {
	for _, key := range []string{"accountEnabled", "AccountEnabled", "Enabled"} {
		for k, v := range obj {
			if strings.EqualFold(k, key) {
				return truthy(v)
			}
		}
	}
	// No explicit flag: treat as active so we don't over-report waste.
	return true
}

// dormantUserRow is one licensed account that has gone idle — the `--by user`
// view. This is the capability the standalone `users stale` command used to
// provide; it was folded here because it runs the identical join over the
// identical rows. Pure inactivity regardless of licensing is available natively
// via `fanout --endpoint /ListInactiveAccounts --all-tenants`.
type dormantUserRow struct {
	Tenant      string   `json:"tenant"`
	User        string   `json:"user"`
	Licenses    []string `json:"licenses"`
	LastSignIn  string   `json:"last_sign_in,omitempty"`
	DaysIdle    int      `json:"days_idle,omitempty"`
	AccountOpen bool     `json:"account_enabled"`
	Reason      string   `json:"reason"`
}

func newNovelLicensesWasteCmd(flags *rootFlags) *cobra.Command {
	var flagDB string
	var flagAllTenants bool
	var flagBy string
	var flagStaleDays int

	cmd := &cobra.Command{
		Use:         "waste",
		Short:       "Surface assigned-but-unused licenses, and the dormant accounts holding them, across all tenants.",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		Long: `Join synced licenses against synced users (and sign-in activity when present)
to surface wasted seats per tenant. A seat is "unused" when its holder has not
signed in within --stale-days (when sign-in data is present), or when its holder
is a disabled account with no sign-in data.

Views:
  --by sku   (default) assigned vs unused seat counts per tenant and SKU
  --by user  the dormant licensed accounts behind those counts

Use this command for reclaimable licenses totalled by SKU or by holder.
Do NOT use this command for a per-account list of enabled accounts that stopped signing in; use 'users stale' instead.
Do NOT use it to verify offboarded accounts; use 'users offboard-check' instead.
For pure inactivity regardless of licensing, run
'cipp-cli fanout --endpoint /ListInactiveAccounts --all-tenants'.

Populate the store first:
  cipp-cli fanout --endpoint /ListLicenses --all-tenants --save
  cipp-cli fanout --endpoint /ListUsers --all-tenants --save`,
		Example: "  cipp-cli licenses waste --by sku --stale-days 90 --all-tenants --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "licenses waste")
			}
			if !flagAllTenants {
				return usageErr(fmt.Errorf("--all-tenants=false is not supported; this command always reads the full local store across all synced tenants"))
			}
			view := strings.ToLower(strings.TrimSpace(flagBy))
			if view != "sku" && view != "user" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --by %q: must be sku or user", flagBy))
			}
			if flagStaleDays <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--stale-days must be greater than 0, got %d", flagStaleDays))
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
					"hint: no synced users data; populate it with: cipp-cli fanout --endpoint /ListUsers --all-tenants --save")
				if flags.asJSON {
					if view == "user" {
						return printNovelJSON(cmd, flags, []dormantUserRow{})
					}
					return printNovelJSON(cmd, flags, []wasteRow{})
				}
				return nil
			}

			skuNames := skuDisplayNames(db)
			now := time.Now()
			cutoff := now.AddDate(0, 0, -flagStaleDays)

			// --by user: the dormant licensed accounts behind the seat counts.
			if view == "user" {
				// false: absent sign-in data is unknown, not idle. See the
				// contract note on computeDormantUsers.
				dormant := computeDormantUsers(users, now, cutoff, flagStaleDays, false)
				for i := range dormant {
					dormant[i].Licenses = nameSKUs(dormant[i].Licenses, skuNames)
				}
				if flags.asJSON {
					return printNovelJSON(cmd, flags, dormant)
				}
				return printDormantUsersTable(cmd, flags, dormant)
			}

			// tenant -> sku -> [assigned, unused]
			type counts struct{ assigned, unused int }
			byTenantSku := map[string]map[string]*counts{}
			ensure := func(tenant, sku string) *counts {
				if byTenantSku[tenant] == nil {
					byTenantSku[tenant] = map[string]*counts{}
				}
				if byTenantSku[tenant][sku] == nil {
					byTenantSku[tenant][sku] = &counts{}
				}
				return byTenantSku[tenant][sku]
			}

			for _, u := range users {
				tenant := rowTenant(u)
				// Aggregate by SKU id; two SKUs can share a display name.
				skus := userLicenseNames(u)
				if len(skus) == 0 {
					continue
				}
				lastSignIn, hasSignIn := userLastSignIn(u)
				active := userIsActive(u)
				for _, sku := range skus {
					c := ensure(tenant, sku)
					c.assigned++
					unused := false
					if hasSignIn {
						if lastSignIn.Before(cutoff) {
							unused = true
						}
					} else if !active {
						// No sign-in data: a disabled account still holding a
						// seat is the waste signal.
						unused = true
					}
					if unused {
						c.unused++
					}
				}
			}

			out := make([]wasteRow, 0)
			tenants := make([]string, 0, len(byTenantSku))
			for t := range byTenantSku {
				tenants = append(tenants, t)
			}
			sort.Strings(tenants)
			for _, t := range tenants {
				skus := make([]string, 0, len(byTenantSku[t]))
				for s := range byTenantSku[t] {
					skus = append(skus, s)
				}
				sort.Strings(skus)
				for _, s := range skus {
					c := byTenantSku[t][s]
					if c.unused == 0 {
						continue
					}
					row := wasteRow{Tenant: t, LicenseName: s, Assigned: c.assigned, Unused: c.unused}
					if n, ok := skuNames[strings.ToLower(s)]; ok {
						row.LicenseName, row.SkuID = n, s
					}
					out = append(out, row)
				}
			}

			if flags.asJSON {
				return printNovelJSON(cmd, flags, out)
			}
			headers := []string{"TENANT", "LICENSE", "ASSIGNED", "UNUSED"}
			tableRows := make([][]string, 0, len(out))
			for _, r := range out {
				tableRows = append(tableRows, []string{r.Tenant, r.LicenseName, fmt.Sprintf("%d", r.Assigned), fmt.Sprintf("%d", r.Unused)})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", true, "Scan all synced tenants (default; this command always reads the full local store)")
	cmd.Flags().StringVar(&flagBy, "by", "sku", "View: sku (assigned vs unused seats) or user (the dormant accounts holding them)")
	cmd.Flags().IntVar(&flagStaleDays, "stale-days", 30, "Days without a sign-in before a licensed seat counts as unused")
	return cmd
}

// computeDormantUsers is the licensed-dormancy join shared by
// `licenses waste --by user` and the deprecated `users stale` alias. Keeping it
// in one place is why `users stale` was consolidated: both views ask the same
// question of the same rows.
//
// A user is dormant when they hold at least one license AND signed in before the
// cutoff. Users with no licenses are skipped entirely — this is LICENSED
// dormancy, not general inactivity (for that, fan out /ListInactiveAccounts).
//
// missingSignInIsDormant selects how a user with NO sign-in data is treated, and
// the two callers deliberately disagree:
//   - `licenses waste` passes false: absent sign-in data is treated as unknown,
//     not idle, so a seat is only called wasted when the account is also
//     disabled. This is the conservative rule that avoids over-reporting waste
//     against tenants whose payloads simply lack signInActivity.
//   - `users stale` passes true: "null sign-in counts as stale" is the published
//     behavior of that command, and flipping it would silently stop flagging
//     never-signed-in licensed accounts for anyone already using it.
func computeDormantUsers(users []map[string]any, now, cutoff time.Time, staleDays int, missingSignInIsDormant bool) []dormantUserRow {
	dormant := make([]dormantUserRow, 0)
	for _, u := range users {
		skus := userLicenseNames(u)
		if len(skus) == 0 {
			continue
		}
		lastSignIn, hasSignIn := userLastSignIn(u)
		active := userIsActive(u)
		row := dormantUserRow{
			Tenant:      rowTenant(u),
			User:        firstNonEmpty(u, "userPrincipalName", "UserPrincipalName", "mail", "displayName"),
			Licenses:    skus,
			AccountOpen: active,
		}
		switch {
		case hasSignIn && lastSignIn.Before(cutoff):
			row.LastSignIn = lastSignIn.UTC().Format(time.RFC3339)
			row.DaysIdle = int(now.Sub(lastSignIn).Hours() / 24)
			row.Reason = fmt.Sprintf("no sign-in in %d days", staleDays)
		case !hasSignIn && missingSignInIsDormant:
			row.Reason = "no recorded sign-in while holding a licensed seat"
		case !hasSignIn && !active:
			row.Reason = "disabled account still holding a licensed seat"
		default:
			continue
		}
		dormant = append(dormant, row)
	}
	sort.Slice(dormant, func(i, j int) bool {
		if dormant[i].Tenant != dormant[j].Tenant {
			return dormant[i].Tenant < dormant[j].Tenant
		}
		return dormant[i].User < dormant[j].User
	})
	return dormant
}

// printDormantUsersTable renders the human view shared by both entry points.
func printDormantUsersTable(cmd *cobra.Command, flags *rootFlags, dormant []dormantUserRow) error {
	headers := []string{"TENANT", "USER", "LICENSES", "LAST SIGN-IN", "DAYS IDLE", "REASON"}
	tableRows := make([][]string, 0, len(dormant))
	for _, r := range dormant {
		idle := "-"
		if r.DaysIdle > 0 {
			idle = fmt.Sprintf("%d", r.DaysIdle)
		}
		last := r.LastSignIn
		if last == "" {
			last = "-"
		}
		tableRows = append(tableRows, []string{r.Tenant, r.User, strings.Join(r.Licenses, ", "), last, idle, r.Reason})
	}
	return flags.printTable(cmd, headers, tableRows)
}
