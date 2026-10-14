// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature: `tenants` command group (add/list/remove/scopes/test).

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/cliutil"
)

// pp:data-source computed
func newTenantsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenants",
		Short: "Register and verify client Google Workspace tenants (one service account per client)",
		Long: "Each client tenant is a service account with domain-wide delegation that impersonates\n" +
			"that client's super admin. Register it once with 'tenants add', authorize the printed\n" +
			"Client ID and scopes in the client's Admin console, then run 'tenants test'.\n" +
			"Select a tenant per command with --tenant <name> or fan out with --all-tenants.",
	}
	cmd.AddCommand(newTenantsAddCmd(flags), newTenantsListCmd(flags), newTenantsRemoveCmd(flags),
		newTenantsScopesCmd(flags), newTenantsTestCmd(flags))
	return cmd
}

// registryErr keeps typed CLI errors from the update callback and maps
// anything else (I/O, parse, lock) to a config error.
func registryErr(err error) error {
	var ce *cliError
	if errors.As(err, &ce) {
		return err
	}
	return configErr(err)
}

type saKeyIdentity struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	ClientID    string `json:"client_id"`
	ProjectID   string `json:"project_id"`
}

func readSAKeyIdentity(path string) (saKeyIdentity, []byte, error) {
	// #nosec G304 -- operator-supplied path to their own service-account key
	raw, err := os.ReadFile(expandTenantPath(path))
	if err != nil {
		return saKeyIdentity{}, nil, fmt.Errorf("reading service-account key: %w", err)
	}
	var id saKeyIdentity
	if err := json.Unmarshal(raw, &id); err != nil {
		return saKeyIdentity{}, nil, fmt.Errorf("service-account key is not JSON: %w", err)
	}
	if id.Type != "service_account" || id.ClientEmail == "" || id.ClientID == "" {
		return saKeyIdentity{}, nil, fmt.Errorf("not a service-account key (need type=service_account, client_email, client_id)")
	}
	return id, raw, nil
}

func newTenantsAddCmd(flags *rootFlags) *cobra.Command {
	var domain, admin, key, scopes, customerID, extraDomains string
	var reseller bool
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a client tenant (copies its service-account key into the config dir, mode 0600 on macOS/Linux)",
		// Registering a key reads an arbitrary local path; that is operator
		// setup, not an agent action, so it stays off the MCP surface.
		Annotations: map[string]string{"mcp:hidden": "true"},
		Example: "  google-workspace-cli tenants add acme --domain example.com --admin admin@example.com --key ./acme-sa.json\n" +
			"  google-workspace-cli tenants add acme --domain example.com --admin admin@example.com --key ./acme-sa.json --scopes full",
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "register a client tenant"); done {
				return err
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("tenants add takes exactly one tenant name"))
			}
			name := strings.ToLower(strings.TrimSpace(args[0]))
			if !tenantNamePattern.MatchString(name) {
				return usageErr(fmt.Errorf("tenant name %q must be lowercase letters, digits, '.', '_' or '-'", name))
			}
			if strings.TrimSpace(domain) == "" || admin == "" || key == "" {
				return usageErr(fmt.Errorf("--domain, --admin and --key are required"))
			}
			if !strings.Contains(admin, "@") {
				return usageErr(fmt.Errorf("--admin must be the super admin's email address"))
			}
			resolved, err := scopesForProfile(scopes)
			if err != nil {
				return usageErr(err)
			}
			id, raw, err := readSAKeyIdentity(key)
			if err != nil {
				return usageErr(err)
			}
			dir, err := tenantRegistryDir()
			if err != nil {
				return configErr(err)
			}
			keyDest := filepath.Join(dir, "keys", name+".json")
			t := gwsTenant{
				Name: name, Domain: strings.ToLower(strings.TrimSpace(domain)), Domains: splitTenantCSV(strings.ToLower(extraDomains)),
				Admin: admin, KeyFile: keyDest, CustomerID: customerID, Scopes: resolved, Reseller: reseller,
			}
			err = updateTenantRegistry(func(reg *tenantRegistry) error {
				if _, exists := reg.find(name); exists && !confirmed(cmd, flags) {
					return usageErr(fmt.Errorf("tenant %q already exists (pass --yes to replace it)", name))
				}
				if err := cliutil.AtomicWritePrivateFile(keyDest, raw, 0o600, 0o700); err != nil {
					return configErr(fmt.Errorf("storing service-account key: %w", err))
				}
				kept := reg.Tenants[:0]
				for _, existing := range reg.Tenants {
					if existing.Name != name {
						kept = append(kept, existing)
					}
				}
				reg.Tenants = append(kept, t)
				return nil
			})
			if err != nil {
				return registryErr(err)
			}
			return printTenantSetup(cmd, flags, t, id, "registered")
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "Client's primary Google Workspace domain")
	cmd.Flags().StringVar(&admin, "admin", "", "Super admin email the service account impersonates")
	cmd.Flags().StringVar(&key, "key", "", "Path to the service-account JSON key for this client")
	cmd.Flags().StringVar(&scopes, "scopes", "read", "Scope profile(s) or scopes, comma-separated: read (default), full (adds offboarding writes), policy (posture's Policy API columns), reseller")
	cmd.Flags().StringVar(&customerID, "customer-id", "", "Workspace customer ID (default: my_customer, filled in by 'tenants test')")
	cmd.Flags().StringVar(&extraDomains, "domains", "", "Additional domains owned by this client, comma-separated (filled in by 'tenants test')")
	cmd.Flags().BoolVar(&reseller, "reseller", false, "This is the MSP's own reseller domain (excluded from --all-tenants)")
	return cmd
}

func printTenantSetup(cmd *cobra.Command, flags *rootFlags, t gwsTenant, id saKeyIdentity, verb string) error {
	short := make([]string, len(t.Scopes))
	copy(short, t.Scopes)
	out := map[string]any{
		"status":              verb,
		"tenant":              t.Name,
		"domain":              t.Domain,
		"admin":               t.Admin,
		"service_account":     id.ClientEmail,
		"client_id":           id.ClientID,
		"scopes_csv":          strings.Join(short, ","),
		"admin_console_path":  "Admin console > Security > Access and data control > API controls > Domain-wide delegation > Add new",
		"next":                "google-workspace-cli tenants test " + t.Name,
		"key_file":            t.KeyFile,
		"delete_original_key": "the key was copied; delete the downloaded original",
	}
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Tenant %s %s (%s, impersonating %s).\n\n", t.Name, verb, t.Domain, t.Admin)
	fmt.Fprintf(w, "In %s's Admin console:\n  %s\n", t.Domain, out["admin_console_path"])
	fmt.Fprintf(w, "  Client ID:    %s\n  OAuth scopes: %s\n\n", id.ClientID, out["scopes_csv"])
	fmt.Fprintf(w, "Then verify: %s\n", out["next"])
	return nil
}

func newTenantsListCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List registered client tenants",
		Example:     "  google-workspace-cli tenants list --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "list registered tenants")
			}
			reg, err := loadTenantRegistry()
			if err != nil {
				return configErr(err)
			}
			type row struct {
				Name     string   `json:"name"`
				Domain   string   `json:"domain"`
				Domains  []string `json:"domains,omitempty"`
				Admin    string   `json:"admin"`
				Customer string   `json:"customer"`
				Scopes   int      `json:"scope_count"`
				Reseller bool     `json:"reseller,omitempty"`
				KeyOK    bool     `json:"key_present"`
				// Hand-fix (handfixes.json tenants-test-checks-domain).
				Quarantined string `json:"ownership_mismatch,omitempty"`
			}
			rows := []row{}
			for _, t := range reg.Tenants {
				_, statErr := os.Stat(expandTenantPath(t.KeyFile))
				rows = append(rows, row{t.Name, t.Domain, t.Domains, t.Admin, t.customer(), len(t.Scopes), t.Reseller, statErr == nil, t.OwnershipMismatch})
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No tenants registered. Add one: google-workspace-cli tenants add <name> --domain <d> --admin <email> --key <sa.json>")
				return nil
			}
			table := [][]string{}
			for _, r := range rows {
				kind := "client"
				if r.Reseller {
					kind = "reseller"
				}
				if r.Quarantined != "" {
					kind += " (QUARANTINED: run tenants test " + r.Name + ")"
				}
				table = append(table, []string{r.Name, r.Domain, r.Admin, kind, fmt.Sprint(r.Scopes), fmt.Sprint(r.KeyOK)})
			}
			return flags.printTable(cmd, []string{"NAME", "DOMAIN", "ADMIN", "KIND", "SCOPES", "KEY"}, table)
		},
	}
}

func newTenantsRemoveCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Short:   "Remove a tenant and delete its stored service-account key copy",
		Example: "  google-workspace-cli tenants remove acme --yes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "remove a tenant and its stored key"); done {
				return err
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("tenants remove takes exactly one tenant name"))
			}
			var t gwsTenant
			err := updateTenantRegistry(func(reg *tenantRegistry) error {
				var ok bool
				if t, ok = reg.find(args[0]); !ok {
					return notFoundErr(fmt.Errorf("unknown tenant %q", args[0]))
				}
				if !confirmed(cmd, flags) {
					return usageErr(fmt.Errorf("removing tenant %q deletes its stored key; re-run with --yes", t.Name))
				}
				kept := reg.Tenants[:0]
				for _, existing := range reg.Tenants {
					if existing.Name != t.Name {
						kept = append(kept, existing)
					}
				}
				reg.Tenants = kept
				if err := os.Remove(expandTenantPath(t.KeyFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
					return configErr(fmt.Errorf("removing key file: %w", err))
				}
				return nil
			})
			if err != nil {
				return registryErr(err)
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"status": "removed", "tenant": t.Name}, flags)
		},
	}
}

func newTenantsScopesCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "scopes <name>",
		Short:       "Print the Client ID and OAuth scopes to authorize in the client's Admin console",
		Example:     "  google-workspace-cli tenants scopes acme",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "print a tenant's Client ID and scopes"); done {
				return err
			}
			if len(args) != 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("tenants scopes takes exactly one tenant name"))
			}
			reg, err := loadTenantRegistry()
			if err != nil {
				return configErr(err)
			}
			t, ok := reg.find(args[0])
			if !ok {
				return notFoundErr(fmt.Errorf("unknown tenant %q", args[0]))
			}
			id, _, err := readSAKeyIdentity(t.KeyFile)
			if err != nil {
				return configErr(err)
			}
			return printTenantSetup(cmd, flags, t, id, "configured")
		},
	}
}

type scopeCheck struct {
	Scope      string `json:"scope"`
	Authorized bool   `json:"authorized"`
	Error      string `json:"error,omitempty"`
}

type tenantTestResult struct {
	Tenant     string       `json:"tenant"`
	Domain     string       `json:"domain"`
	OK         bool         `json:"ok"`
	CustomerID string       `json:"customer_id,omitempty"`
	Domains    []string     `json:"domains,omitempty"`
	Scopes     []scopeCheck `json:"scopes"`
	Error      string       `json:"error,omitempty"`
	tested     gwsTenant    // the registry entry this result was produced from
	// wrongCustomer: the admin and key resolve to an organization that does
	// not own the configured domain; the tenant is quarantined
	// (handfixes.json tenants-test-checks-domain).
	wrongCustomer bool
}

// sameTenantConfig reports whether a registry entry is still the one that
// was tested (a concurrent replace must not inherit stale results).
func sameTenantConfig(a, b gwsTenant) bool {
	return a.Name == b.Name && a.Admin == b.Admin && a.KeyFile == b.KeyFile &&
		strings.EqualFold(a.Domain, b.Domain) && strings.Join(a.Scopes, " ") == strings.Join(b.Scopes, " ")
}

func newTenantsTestCmd(flags *rootFlags) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "test [name]",
		Short: "Verify each tenant's domain-wide delegation scope by scope, and record its customer ID and domains",
		Long: "Mints one token per configured scope so a missing delegation is named precisely,\n" +
			"then reads the customer record and domain list to confirm impersonation works.\n" +
			"Use this after 'tenants add' and whenever a client's Admin console changes.",
		Example:     "  google-workspace-cli tenants test acme\n  google-workspace-cli tenants test --all-tenants --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "verify tenant delegation scope by scope"); done {
				return err
			}
			reg, err := loadTenantRegistry()
			if err != nil {
				return configErr(err)
			}
			var targets []gwsTenant
			switch {
			case all:
				targets = reg.Tenants
			case len(args) == 1:
				t, ok := reg.find(args[0])
				if !ok {
					return notFoundErr(fmt.Errorf("unknown tenant %q", args[0]))
				}
				targets = []gwsTenant{t}
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("name a tenant or pass --all-tenants"))
			}
			results, errs := cliutil.FanoutRun(cmd.Context(), targets, func(t gwsTenant) string { return t.Name },
				func(ctx context.Context, t gwsTenant) (tenantTestResult, error) {
					return testTenant(ctx, flags, t), nil
				}, cliutil.WithConcurrency(4))
			out := []tenantTestResult{}
			verified := map[string]tenantTestResult{}
			for _, r := range results {
				out = append(out, r.Value)
				if r.Value.OK || r.Value.wrongCustomer {
					verified[r.Value.Tenant] = r.Value
				}
			}
			// A worker that was cancelled or panicked returns no result; report
			// it as a failed tenant so the exit code cannot read as success.
			for _, e := range errs {
				out = append(out, tenantTestResult{Tenant: e.Source, Error: e.Err.Error()})
			}
			if len(verified) > 0 {
				// Re-read under the lock and touch only tenants that still
				// exist, so a concurrent remove is never undone.
				err := updateTenantRegistry(func(reg *tenantRegistry) error {
					for i := range reg.Tenants {
						v, ok := verified[reg.Tenants[i].Name]
						if !ok || !sameTenantConfig(reg.Tenants[i], v.tested) {
							continue
						}
						if v.wrongCustomer {
							// Keep what the operator recorded; refuse the tenant.
							reg.Tenants[i].OwnershipMismatch = v.Error
							continue
						}
						reg.Tenants[i].CustomerID = v.CustomerID
						reg.Tenants[i].Domains = v.Domains
						reg.Tenants[i].OwnershipMismatch = ""
					}
					return nil
				})
				if err != nil {
					return registryErr(err)
				}
			}
			failed := 0
			for _, r := range out {
				if !r.OK {
					failed++
				}
			}
			if err := printTenantTest(cmd, flags, out); err != nil {
				return err
			}
			if failed > 0 {
				return authErr(fmt.Errorf("%d of %d tenant(s) failed verification", failed, len(out)))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all-tenants", false, "Test every registered tenant")
	return cmd
}

func testTenant(ctx context.Context, flags *rootFlags, t gwsTenant) tenantTestResult {
	res := tenantTestResult{Tenant: t.Name, Domain: t.Domain, OK: true, tested: t}
	// A quarantined tenant is re-tested with its own credentials; passing
	// lifts the quarantine.
	t.OwnershipMismatch = ""
	for _, s := range t.Scopes {
		chk := scopeCheck{Scope: strings.TrimPrefix(s, scopePrefix), Authorized: true}
		if _, err := mintTenantToken(ctx, t, []string{s}); err != nil {
			chk.Authorized, chk.Error, res.OK = false, err.Error(), false
			if !strings.Contains(err.Error(), "unauthorized_client") {
				// Not a per-scope gap (bad admin, revoked key, network):
				// every other scope would fail identically.
				res.Error = err.Error()
				chk.Error = ""
				res.Scopes = append(res.Scopes, chk)
				return res
			}
		}
		res.Scopes = append(res.Scopes, chk)
	}
	if !res.OK || t.Reseller {
		return res
	}
	c, err := flags.newTenantClient(ctx, t)
	if err != nil {
		res.OK, res.Error = false, err.Error()
		return res
	}
	raw, err := c.Get(ctx, "/admin/directory/v1/customers/my_customer", nil)
	if err != nil {
		res.OK, res.Error = false, "reading customer record: "+err.Error()
		return res
	}
	var cust struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &cust); err != nil || cust.ID == "" {
		res.OK, res.Error = false, "reading customer record: no customer id in response"
		return res
	}
	res.CustomerID = cust.ID
	// A failed domain lookup must fail verification, not erase the tenant's
	// recorded secondary domains (offboard resolves tenants by domain).
	raw, err = c.Get(ctx, "/admin/directory/v1/customer/my_customer/domains", nil)
	if err != nil {
		res.OK, res.Error = false, "listing domains: "+err.Error()
		return res
	}
	domains, err := tenantDomains(t, raw)
	if err != nil {
		res.OK, res.Error, res.CustomerID = false, err.Error(), ""
		res.wrongCustomer = errors.Is(err, errWrongCustomer)
		res.Domains = nil
		return res
	}
	res.Domains = domains
	return res
}

var errWrongCustomer = errors.New("the admin or service-account key belongs to a different Google Workspace customer")

// tenantDomains decodes a domains.list response into the tenant's secondary
// domains and aliases (offboard resolves tenants by any of them). Hand-fix
// (handfixes.json tenants-test-checks-domain): it fails when the configured
// domain is not among them, because then the admin and key resolve to another
// organization, whose data would be reported (and stored) as this tenant's.
func tenantDomains(t gwsTenant, raw []byte) ([]string, error) {
	var doms struct {
		Domains []struct {
			DomainName    string `json:"domainName"`
			DomainAliases []struct {
				Name string `json:"domainAliasName"`
			} `json:"domainAliases"`
		} `json:"domains"`
	}
	if err := json.Unmarshal(raw, &doms); err != nil {
		return nil, fmt.Errorf("decoding domains: %w", err)
	}
	want := strings.ToLower(strings.TrimSpace(t.Domain))
	owned, seen, out := false, map[string]bool{want: true}, []string{}
	add := func(d string) {
		d = strings.ToLower(strings.TrimSpace(d))
		owned = owned || d == want
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, d := range doms.Domains {
		add(d.DomainName)
		for _, a := range d.DomainAliases {
			add(a.Name)
		}
	}
	if !owned {
		return nil, fmt.Errorf("%w: domain %s is not one of this organization's domains (%s); the tenant is quarantined (every command refuses it) until 'tenants add %s --yes' re-registers it with the right --domain, --admin and --key and tenants test passes", errWrongCustomer, want, strings.Join(out, ", "), t.Name)
	}
	return out, nil
}

func printTenantTest(cmd *cobra.Command, flags *rootFlags, out []tenantTestResult) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), out, flags)
	}
	w := cmd.OutOrStdout()
	for _, r := range out {
		status := "OK"
		if !r.OK {
			status = "FAIL"
		}
		fmt.Fprintf(w, "%s  %s (%s)\n", status, r.Tenant, r.Domain)
		for _, s := range r.Scopes {
			mark := "  ok  "
			if !s.Authorized {
				mark = "  MISSING"
			}
			fmt.Fprintf(w, "%s %s\n", mark, s.Scope)
		}
		if r.Error != "" {
			fmt.Fprintf(w, "  error: %s\n", r.Error)
		}
	}
	return nil
}
