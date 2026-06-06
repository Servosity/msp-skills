// Copyright 2026 damienstevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cipp-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// postureDimension describes one --dimension choice: the store resource_type it
// reads and the List<X> endpoint that populates it (used for the honest-empty
// hint).
type postureDimension struct {
	resourceType string
	listEndpoint string
}

var postureDimensions = map[string]postureDimension{
	"mfa":       {resourceType: "users", listEndpoint: "/ListUsers"},
	"ca":        {resourceType: "conditionalaccesspolicies", listEndpoint: "/ListConditionalAccessPolicies"},
	"standards": {resourceType: "standards", listEndpoint: "/ListStandards"},
	"bpa":       {resourceType: "bestpracticeanalyser", listEndpoint: "/ListBpa"},
}

// postureRow is one tenant's metric line. Metrics is an ordered-by-key map of
// metric name to count so the JSON and table outputs stay aligned across
// dimensions.
type postureRow struct {
	Tenant  string         `json:"tenant"`
	Metrics map[string]int `json:"metrics"`
}

// readResourceRows reads every row of a resource_type from the store. A large
// limit is used because posture is a fleet rollup, not a paged view.
func readResourceRows(db *store.Store, resourceType string) ([]map[string]any, error) {
	raw, err := db.List(resourceType, 1000000)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		var obj map[string]any
		if json.Unmarshal(r, &obj) == nil {
			out = append(out, obj)
		}
	}
	return out, nil
}

// rowTenant returns the tenant a stored row belongs to, reading the _tenant
// field stamped by fanout --save (falling back to common CIPP tenant fields).
func rowTenant(obj map[string]any) string {
	if v := tenantFieldLookup(obj, "_tenant"); v != "" {
		return v
	}
	if v := tenantFieldLookup(obj, "Tenant", "tenantFilter", "defaultDomainName"); v != "" {
		return v
	}
	return "(unknown)"
}

// truthy interprets a CIPP-ish value as a boolean. CIPP serializes booleans
// inconsistently (true, "true", "Enabled", "Yes").
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "yes", "enabled", "1", "on":
			return true
		}
	case float64:
		return t != 0
	}
	return false
}

// mfaEnabledForUser reports whether a stored user row indicates MFA is on. CIPP
// exposes this under several field names depending on the source endpoint.
func mfaEnabledForUser(obj map[string]any) bool {
	for _, key := range []string{"MFARegistration", "isMfaRegistered", "MFA", "PerUserMFAState", "mfaEnabled", "isMFACapable", "MFAEnabled"} {
		for k, v := range obj {
			if strings.EqualFold(k, key) {
				if s, ok := v.(string); ok && strings.EqualFold(strings.TrimSpace(s), "enforced") {
					return true
				}
				if truthy(v) {
					return true
				}
			}
		}
	}
	return false
}

func newNovelPostureCmd(flags *rootFlags) *cobra.Command {
	var flagDimension string
	var flagDB string

	cmd := &cobra.Command{
		Use:         "posture --dimension mfa|ca|standards|bpa",
		Short:       "One table of every tenant's MFA, Conditional Access, Standards, and BPA posture — the QBR rollup the UI never renders.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Roll up one security dimension across every synced tenant into a single
tenant×metric matrix. Reads the local store only (no live calls) — populate it
first with 'cipp-cli fanout --endpoint /List<X> --all-tenants --save'.

Dimensions:
  mfa        MFA registration counts per tenant (from synced users)
  ca         Conditional Access policy counts per tenant
  standards  Standards counts per tenant
  bpa        Best Practice Analyser findings per tenant`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && flagDimension == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			dim, ok := postureDimensions[strings.ToLower(flagDimension)]
			if !ok {
				return usageErr(fmt.Errorf("invalid --dimension %q: must be one of mfa, ca, standards, bpa", flagDimension))
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

			rows, err := readResourceRows(db, dim.resourceType)
			if err != nil {
				return fmt.Errorf("reading %s: %w", dim.resourceType, err)
			}

			// Honest-empty path: no synced data → message to stderr, empty
			// result, exit 0 (NOT an error).
			if len(rows) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"no synced %s data; populate it with: cipp-cli fanout --endpoint %s --all-tenants --save\n",
					strings.ToLower(flagDimension), dim.listEndpoint)
				if flags.asJSON {
					return flags.printJSON(cmd, []postureRow{})
				}
				return nil
			}

			// Group rows by tenant and compute per-dimension metrics.
			byTenant := map[string]map[string]int{}
			ensure := func(t string) map[string]int {
				if byTenant[t] == nil {
					byTenant[t] = map[string]int{}
				}
				return byTenant[t]
			}

			switch strings.ToLower(flagDimension) {
			case "mfa":
				for _, obj := range rows {
					m := ensure(rowTenant(obj))
					m["users"]++
					if mfaEnabledForUser(obj) {
						m["mfa_enabled"]++
					} else {
						m["mfa_missing"]++
					}
				}
			default:
				// ca / standards / bpa: count rows per tenant under a
				// dimension-named metric.
				metricName := strings.ToLower(flagDimension) + "_count"
				for _, obj := range rows {
					m := ensure(rowTenant(obj))
					m[metricName]++
				}
			}

			out := make([]postureRow, 0, len(byTenant))
			for t, m := range byTenant {
				out = append(out, postureRow{Tenant: t, Metrics: m})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Tenant < out[j].Tenant })

			if flags.asJSON {
				return flags.printJSON(cmd, out)
			}

			// Build a stable column set from all metric keys seen.
			metricKeys := map[string]bool{}
			for _, r := range out {
				for k := range r.Metrics {
					metricKeys[k] = true
				}
			}
			cols := make([]string, 0, len(metricKeys))
			for k := range metricKeys {
				cols = append(cols, k)
			}
			sort.Strings(cols)

			headers := append([]string{"TENANT"}, cols...)
			tableRows := make([][]string, 0, len(out))
			for _, r := range out {
				row := make([]string, 0, len(headers))
				row = append(row, r.Tenant)
				for _, c := range cols {
					row = append(row, fmt.Sprintf("%d", r.Metrics[c]))
				}
				tableRows = append(tableRows, row)
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&flagDimension, "dimension", "", "Posture dimension: mfa, ca, standards, or bpa")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}
