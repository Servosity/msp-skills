// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"cipp-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// gdapRelationshipResource is the store resource_type the live read of the
// partner-wide /ListGDAPRelationships writes through to (resolveReadWithStrategy).
const gdapRelationshipResource = "gdaprelationships"

// gdapRow is one delegated-admin relationship approaching its end date.
type gdapRow struct {
	Tenant       string `json:"tenant"`
	TenantID     string `json:"tenant_id,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	Status       string `json:"status,omitempty"`
	EndDateTime  string `json:"end_date_time,omitempty"`
	DaysToExpiry int    `json:"days_to_expiry"`
}

// gdapCoverageRow is a tenant with no active delegated-admin relationship at
// all, found by left-joining the stored tenant list against the relationship
// set.
type gdapCoverageRow struct {
	Tenant string `json:"tenant"`
	Reason string `json:"reason"`
}

// gdapReport is the command's full JSON envelope.
type gdapReport struct {
	WindowDays  int               `json:"window_days"`
	Expiring    []gdapRow         `json:"expiring"`
	Uncovered   []gdapCoverageRow `json:"uncovered_tenants"`
	TotalActive int               `json:"total_active_relationships"`
	// Note says why the report is empty when nothing was fetched, so an
	// empty result is never read as "this partner has no relationships".
	Note string `json:"note,omitempty"`
}

// gdapEndDateFormats are the shapes Microsoft Partner Center and Graph use for
// delegatedAdminRelationship.endDateTime across API versions.
var gdapEndDateFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05.9999999Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// parseGDAPDate parses a relationship end date, reporting ok=false when the
// value is absent or unparseable so the caller can surface it rather than
// treating an unknown date as "not expiring".
func parseGDAPDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range gdapEndDateFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// gdapRelationshipRows unwraps a CIPP response into relationship objects.
// ListGDAPRelationships is partner-wide (it carries no tenantFilter parameter),
// so one call returns the whole set. The payload arrives either as a bare array
// or wrapped in the standard {"Results": [...]} envelope, so it reuses
// cippListRows, which also refuses a list carrying a CIPP message.
func gdapRelationshipRows(data json.RawMessage) ([]map[string]any, error) {
	return cippListRows(data, "GDAP relationship list", nil, "id")
}

// gdapCustomer returns the customer object of a relationship: Graph's
// delegatedAdminRelationship nests it as {"customer": {"tenantId",
// "displayName"}}; flat CIPP spellings are read as a fallback.
func gdapCustomer(obj map[string]any) map[string]any {
	if v, ok := lookupField(obj, "customer"); ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return obj
}

// gdapRelationshipTenant returns the customer tenant id a relationship
// belongs to (a GUID; stored tenants carry it as customerId).
func gdapRelationshipTenant(obj map[string]any) string {
	if v := tenantFieldLookup(gdapCustomer(obj), "tenantId", "customerTenantId", "customerId"); v != "" {
		return v
	}
	if v := tenantFieldLookup(obj, "customerTenantId", "customerId", "tenantId", "_tenant", "defaultDomainName"); v != "" {
		return v
	}
	return "(unknown)"
}

// gdapRelationshipCustomerName is the customer's display name, or "".
func gdapRelationshipCustomerName(obj map[string]any) string {
	if v, ok := lookupField(obj, "customer"); ok {
		if c, ok := v.(map[string]any); ok {
			if name := tenantFieldLookup(c, "displayName", "defaultDomainName"); name != "" {
				return name
			}
		}
	}
	return tenantFieldLookup(obj, "customerName", "customerDisplayName")
}

// gdapIsActive reports whether a relationship is currently in force. Only an
// explicitly active status counts: a terminated or pending relationship grants
// no delegated access, and treating an unknown status as active would hide the
// coverage gap this command exists to surface.
func gdapIsActive(obj map[string]any) bool {
	return lookupString(obj, "status", "relationshipStatus", "state") == "active"
}

func newNovelGdapExpiringCmd(flags *rootFlags) *cobra.Command {
	var flagDays int
	var flagDB string

	cmd := &cobra.Command{
		Use:         "expiring",
		Short:       "List delegated-admin relationships expiring inside a window, plus tenants with no active relationship at all.",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		Long: `Report GDAP (Granular Delegated Admin Privileges) relationships approaching
their end date, and the tenants that have no active relationship at all.

GDAP relationships auto-terminate at their end date, and the first symptom is
usually a technician hitting a permission error on a customer tenant. CIPP ships
ExecAutoExtendGDAP because expiry bites; this puts the relationships ending
inside a window and the tenants with no active relationship in one result.

ListGDAPRelationships is partner-wide, so the expiry window needs a single call.
The coverage gap — tenants with no active relationship — comes from left-joining
that set against the stored tenant list, which requires
'cipp-cli fanout --endpoint /ListTenants --all-tenants --save' to have run.`,
		Example: "  cipp-cli gdap expiring --days 30 --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "gdap expiring")
			}
			if flagDays <= 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--days must be greater than 0, got %d", flagDays))
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			data, _, err := resolveReadWithStrategy(cmd.Context(), c, flags, "auto",
				gdapRelationshipResource, true, "/ListGDAPRelationships", nil, nil, cmd.ErrOrStderr())
			if err != nil {
				if c.Config != nil && !c.Config.CredentialConfigured() {
					// Same honest-empty shape as the local rollups: exit 0 and
					// name what produces the data.
					fmt.Fprintln(cmd.ErrOrStderr(),
						"hint: no GDAP relationship data: this command reads the partner-wide /ListGDAPRelationships live (results are cached locally); authenticate first with 'cipp-cli auth login'")
					if flags.asJSON {
						return printNovelJSON(cmd, flags, gdapReport{WindowDays: flagDays, Expiring: []gdapRow{}, Uncovered: []gdapCoverageRow{},
							Note: "not fetched: no CIPP credentials are configured; run 'cipp-cli auth login'"})
					}
					return nil
				}
				return fmt.Errorf("listing GDAP relationships: %w", err)
			}

			// A message in place of, or next to, relationship rows is CIPP
			// reporting a failed or partial list: coverage computed from it
			// would report covered tenants as uncovered.
			rows, err := gdapRelationshipRows(data)
			if err != nil {
				return apiErr(err)
			}
			if len(rows) == 0 {
				// Still compute coverage below: with no relationship at all,
				// every stored tenant is uncovered.
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: no GDAP relationships returned")
			}

			now := time.Now().UTC()
			cutoff := now.AddDate(0, 0, flagDays)

			report := gdapReport{
				WindowDays: flagDays,
				Expiring:   []gdapRow{},
				Uncovered:  []gdapCoverageRow{},
			}
			activeTenants := map[string]bool{}

			for _, obj := range rows {
				tenant := gdapRelationshipTenant(obj)
				active := gdapIsActive(obj)
				if active {
					activeTenants[strings.ToLower(tenant)] = true
					report.TotalActive++
				}

				endRaw := ""
				if v, ok := lookupField(obj, "endDateTime", "EndDateTime", "endDate", "expirationDateTime"); ok {
					if s, isStr := v.(string); isStr {
						endRaw = s
					}
				}
				end, ok := parseGDAPDate(endRaw)
				if !ok {
					continue
				}
				// Only relationships still in force are worth warning about;
				// an already-terminated one is a coverage gap, not an expiry.
				if !active {
					continue
				}
				if end.After(cutoff) {
					continue
				}
				name := gdapRelationshipCustomerName(obj)
				if name == "" {
					name = tenant
				}
				report.Expiring = append(report.Expiring, gdapRow{
					Tenant:       name,
					TenantID:     tenant,
					DisplayName:  strings.TrimSpace(fmt.Sprint(firstNonEmpty(obj, "displayName", "DisplayName", "name"))),
					Status:       lookupString(obj, "status", "relationshipStatus", "state"),
					EndDateTime:  endRaw,
					DaysToExpiry: int(end.Sub(now).Hours() / 24),
				})
			}

			sort.Slice(report.Expiring, func(i, j int) bool {
				if report.Expiring[i].DaysToExpiry != report.Expiring[j].DaysToExpiry {
					return report.Expiring[i].DaysToExpiry < report.Expiring[j].DaysToExpiry
				}
				return report.Expiring[i].Tenant < report.Expiring[j].Tenant
			})

			// Coverage gap: stored tenants with no active relationship. Skipped
			// silently when the tenant list has not been fanned out, because an
			// empty tenant table would otherwise report every tenant as covered.
			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("cipp-cli")
			}
			var db *store.Store
			if flagDB != "" {
				db, err = store.OpenReadOnlyContext(cmd.Context(), dbPath)
			} else {
				db, err = openStoreForRead(cmd.Context(), "cipp-cli")
			}
			if err == nil && db != nil {
				defer db.Close()
				if tenantRows, err := readResourceRows(db, "tenants"); err == nil && len(tenantRows) > 0 {
					for _, obj := range tenantRows {
						name := tenantFieldLookup(obj, "defaultDomainName", "displayName")
						id := tenantFieldLookup(obj, "customerId", "tenantId")
						if name == "" {
							name = id
						}
						if name == "" {
							continue
						}
						// Relationships carry the customer tenant id (a GUID);
						// a domain never equals one, so match on the id.
						if !activeTenants[strings.ToLower(id)] && !activeTenants[strings.ToLower(name)] {
							report.Uncovered = append(report.Uncovered, gdapCoverageRow{
								Tenant: name,
								Reason: "no active GDAP relationship",
							})
						}
					}
					sort.Slice(report.Uncovered, func(i, j int) bool { return report.Uncovered[i].Tenant < report.Uncovered[j].Tenant })
				} else {
					fmt.Fprintln(cmd.ErrOrStderr(),
						"hint: tenant coverage gap not computed: no stored tenant list; save it with: cipp-cli fanout --preset core --all-tenants --save")
				}
			}

			if flags.asJSON {
				return printNovelJSON(cmd, flags, report)
			}

			if len(report.Expiring) == 0 && len(report.Uncovered) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: no GDAP relationships expiring within %d days, and every synced tenant has an active relationship\n", flagDays)
				return nil
			}

			headers := []string{"TENANT", "STATUS", "END DATE", "DAYS"}
			tableRows := make([][]string, 0, len(report.Expiring)+len(report.Uncovered))
			for _, r := range report.Expiring {
				tableRows = append(tableRows, []string{r.Tenant, r.Status, r.EndDateTime, fmt.Sprintf("%d", r.DaysToExpiry)})
			}
			for _, r := range report.Uncovered {
				tableRows = append(tableRows, []string{r.Tenant, "NONE", "-", "-"})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().IntVar(&flagDays, "days", 30, "Report relationships ending within this many days")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}

// firstNonEmpty returns the first non-empty string value among keys.
func firstNonEmpty(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := lookupField(obj, key); ok {
			if s, isStr := v.(string); isStr && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
