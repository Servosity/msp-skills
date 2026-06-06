// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

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

func newNovelLicensesWasteCmd(flags *rootFlags) *cobra.Command {
	var flagDB string
	var flagAllTenants bool

	cmd := &cobra.Command{
		Use:         "waste",
		Short:       "Surface assigned-but-unused licenses and CSP billing mismatches across all tenants.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Join synced licenses against synced users (and sign-in activity when present)
to surface assigned-but-unused seats per tenant. A seat is "unused" when its
holder has not signed in within 30 days (when sign-in data is present), or when
its holder is a disabled account with no sign-in data.

Populate the store first:
  cipp-cli fanout --endpoint /ListLicenses --all-tenants --save
  cipp-cli fanout --endpoint /ListUsers --all-tenants --save`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if !flagAllTenants {
				return usageErr(fmt.Errorf("--all-tenants=false is not supported; this command always reads the full local store across all synced tenants"))
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
					return flags.printJSON(cmd, []wasteRow{})
				}
				return nil
			}

			cutoff := time.Now().AddDate(0, 0, -30)

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
					out = append(out, wasteRow{Tenant: t, LicenseName: s, Assigned: c.assigned, Unused: c.unused})
				}
			}

			if flags.asJSON {
				return flags.printJSON(cmd, out)
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
	return cmd
}
