// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"sort"
	"strings"

	"cipp-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// offboardRow is one disabled account and whatever it still holds.
type offboardRow struct {
	Tenant            string   `json:"tenant"`
	UserPrincipalName string   `json:"userPrincipalName"`
	DisplayName       string   `json:"displayName"`
	Forwarding        string   `json:"forwarding,omitempty"`
	InboxRules        int      `json:"inbox_rules"`
	Roles             []string `json:"roles"`
	Licenses          []string `json:"licenses"`
	Leftovers         []string `json:"leftovers"`
	// Unchecked names the sources with no stored rows for this tenant, so a
	// clean result is never mistaken for a checked one.
	Unchecked []string `json:"unchecked,omitempty"`
}

// rowUPN returns a stored row's lower-cased user principal name.
func rowUPN(obj map[string]any) string {
	return lookupString(obj, "userPrincipalName", "UPN", "PrimarySmtpAddress", "MailboxOwnerUPN", "mail")
}

// forwardingTarget returns where a mailbox forwards to, or "".
func forwardingTarget(obj map[string]any) string {
	for _, k := range []string{"ForwardingSmtpAddress", "ForwardingAddress", "ForwardTo"} {
		if v, ok := lookupField(obj, k); ok && v != nil {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				return s
			}
		}
	}
	return ""
}

// ruleEnabled treats a rule with no Enabled field as enabled.
func ruleEnabled(obj map[string]any) bool {
	v, ok := lookupField(obj, "Enabled", "enabled")
	if !ok || v == nil {
		return true
	}
	return truthy(v)
}

// roleMemberUPNs returns the lower-cased UPN-like tokens a role row lists as
// members. CIPP renders Members as a string, a string list, or member objects.
func roleMemberUPNs(obj map[string]any) map[string]bool {
	out := map[string]bool{}
	addText := func(s string) {
		for _, tok := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
			return r == ',' || r == ';' || r == '(' || r == ')' || r == ' ' || r == '<' || r == '>'
		}) {
			if strings.Contains(tok, "@") {
				out[tok] = true
			}
		}
	}
	v, ok := lookupField(obj, "Members", "members")
	if !ok {
		return out
	}
	switch m := v.(type) {
	case string:
		addText(m)
	case []any:
		for _, e := range m {
			switch x := e.(type) {
			case string:
				addText(x)
			case map[string]any:
				if u := lookupString(x, "userPrincipalName", "mail", "UPN"); u != "" {
					out[u] = true
				}
			}
		}
	}
	return out
}

func roleName(obj map[string]any) string {
	if v, ok := lookupField(obj, "DisplayName", "displayName", "RoleName", "name"); ok && v != nil {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return "(unnamed role)"
}

// computeOffboardLeftovers joins disabled users to the forwarding, inbox-rule
// and role rows of their own tenant, plus the licenses on the user row.
func computeOffboardLeftovers(users, forwarding, rules, roles []map[string]any, includeClean bool) []offboardRow {
	type key struct{ tenant, upn string }
	has := func(rows []map[string]any) map[string]bool {
		out := map[string]bool{}
		for _, r := range rows {
			out[rowTenant(r)] = true
		}
		return out
	}
	fwdTenants, ruleTenants, roleTenants := has(forwarding), has(rules), has(roles)

	fwd := map[key]string{}
	for _, r := range forwarding {
		if t := forwardingTarget(r); t != "" {
			fwd[key{rowTenant(r), rowUPN(r)}] = t
		}
	}
	ruleCount := map[key]int{}
	for _, r := range rules {
		if ruleEnabled(r) {
			ruleCount[key{rowTenant(r), rowUPN(r)}]++
		}
	}
	roleHolders := map[key][]string{}
	for _, r := range roles {
		t, name := rowTenant(r), roleName(r)
		for upn := range roleMemberUPNs(r) {
			roleHolders[key{t, upn}] = append(roleHolders[key{t, upn}], name)
		}
	}

	out := []offboardRow{}
	for _, u := range users {
		if userAccountEnabled(u) {
			continue
		}
		t, upn := rowTenant(u), rowUPN(u)
		if upn == "" {
			continue
		}
		k := key{t, upn}
		row := offboardRow{
			Tenant:            t,
			UserPrincipalName: upn,
			DisplayName:       tenantFieldLookup(u, "displayName", "DisplayName"),
			Forwarding:        fwd[k],
			InboxRules:        ruleCount[k],
			Roles:             append([]string{}, roleHolders[k]...),
			Licenses:          append([]string{}, userLicenseNames(u)...),
		}
		sort.Strings(row.Roles)
		if row.Forwarding != "" {
			row.Leftovers = append(row.Leftovers, "forwarding")
		}
		if row.InboxRules > 0 {
			row.Leftovers = append(row.Leftovers, "inbox_rules")
		}
		if len(row.Roles) > 0 {
			row.Leftovers = append(row.Leftovers, "roles")
		}
		if len(row.Licenses) > 0 {
			row.Leftovers = append(row.Leftovers, "licenses")
		}
		if !fwdTenants[t] {
			row.Unchecked = append(row.Unchecked, "forwarding")
		}
		if !ruleTenants[t] {
			row.Unchecked = append(row.Unchecked, "inbox_rules")
		}
		if !roleTenants[t] {
			row.Unchecked = append(row.Unchecked, "roles")
		}
		if row.Leftovers == nil {
			row.Leftovers = []string{}
		}
		if len(row.Leftovers) == 0 && !includeClean {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tenant != out[j].Tenant {
			return out[i].Tenant < out[j].Tenant
		}
		return out[i].UserPrincipalName < out[j].UserPrincipalName
	})
	return out
}

// loadOffboardInputs reads the four stored resources the check joins.
func loadOffboardInputs(db *store.Store) (users, fwd, rules, roles []map[string]any, err error) {
	for _, x := range []struct {
		rt  string
		dst *[]map[string]any
	}{{"users", &users}, {"mailboxforwarding", &fwd}, {"mailboxrules", &rules}, {"roles", &roles}} {
		if *x.dst, err = readResourceRows(db, x.rt); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("reading %s: %w", x.rt, err)
		}
	}
	return users, fwd, rules, roles, nil
}

func newNovelUsersOffboardCheckCmd(flags *rootFlags) *cobra.Command {
	var flagAll bool
	var flagDB string

	cmd := &cobra.Command{
		Use:   "offboard-check",
		Short: "Find disabled accounts that still hold mail forwarding, inbox rules, directory roles or licenses.",
		Long: `Use this command for disabled accounts still holding mail forwarding, inbox rules, directory roles, or licenses.
Do NOT use this command for enabled accounts that stopped signing in; use 'users stale' instead.
Do NOT use it for license reclaim totals by SKU; use 'licenses waste' instead.

Offboarding entrypoints can report a failed step as text inside an HTTP 200, so
this checks the end state instead: every disabled account in the local store,
joined by UPN to its own tenant's stored mailbox forwarding, inbox rules and
directory roles, plus the licenses still on the account. A source with no
stored rows for a tenant is listed under "unchecked" rather than read as clean.

Populate the store first:
  cipp-cli fanout --preset offboarding --all-tenants --save`,
		Example:     "  cipp-cli users offboard-check --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "users offboard-check")
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

			users, fwd, rules, roles, err := loadOffboardInputs(db)
			if err != nil {
				return err
			}
			if len(users) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"hint: no stored users; populate the store with: cipp-cli fanout --preset offboarding --all-tenants --save")
			}
			out := computeOffboardLeftovers(users, fwd, rules, roles, flagAll)
			skuNames := skuDisplayNames(db)
			for i := range out {
				out[i].Licenses = nameSKUs(out[i].Licenses, skuNames)
			}
			if flags.asJSON {
				return printNovelJSON(cmd, flags, out)
			}
			headers := []string{"TENANT", "UPN", "LEFTOVERS", "UNCHECKED"}
			table := make([][]string, 0, len(out))
			for _, r := range out {
				table = append(table, []string{r.Tenant, r.UserPrincipalName, strings.Join(r.Leftovers, ","), strings.Join(r.Unchecked, ",")})
			}
			return flags.printTable(cmd, headers, table)
		},
	}
	cmd.Flags().BoolVar(&flagAll, "all", false, "Include disabled accounts that hold nothing")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}
