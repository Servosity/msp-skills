// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"cipp-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// pp:data-source local

// postureDimension describes one --dimension choice: the store resource_type it
// reads and the List<X> endpoint that populates it (used for the honest-empty
// hint).
//
// resourceType MUST match what resourceTypeForEndpoint(listEndpoint) produces,
// because that is the key fanout --save writes. A mismatch makes the dimension
// silently return zero rows against a healthy store; the pre-4.30 build shipped
// "bestpracticeanalyser" here while fanout wrote "bpa", so --dimension bpa could
// never return a row. postureDimensionsConsistent (see posture_test.go) locks
// the invariant.
type postureDimension struct {
	resourceType string
	listEndpoint string
}

var postureDimensions = map[string]postureDimension{
	// /ListMFAUsers carries the MFA registration fields; /ListUsers does not,
	// so reading users here counted every user as mfa_missing.
	"mfa":       {resourceType: "mfausers", listEndpoint: "/ListMFAUsers"},
	"ca":        {resourceType: "conditionalaccesspolicies", listEndpoint: "/ListConditionalAccessPolicies"},
	"standards": {resourceType: "standards", listEndpoint: "/ListStandards"},
	"bpa":       {resourceType: "bpa", listEndpoint: "/ListBPA"},
	"dlp":       {resourceType: "dlpcompliancepolicy", listEndpoint: "/ListDlpCompliancePolicy"},
	"retention": {resourceType: "retentioncompliancepolicy", listEndpoint: "/ListRetentionCompliancePolicy"},
	"labels":    {resourceType: "sensitivitylabel", listEndpoint: "/ListSensitivityLabel"},
}

// postureDimensionNames returns the --dimension choices in a stable order for
// help text and error messages.
func postureDimensionNames() []string {
	names := make([]string, 0, len(postureDimensions))
	for k := range postureDimensions {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
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

// lookupField does a case-insensitive lookup of the first matching key. CIPP
// and Graph disagree on casing across endpoints, so every field read in this
// file goes through it rather than indexing the map directly.
func lookupField(obj map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		for k, v := range obj {
			if strings.EqualFold(k, key) {
				return v, true
			}
		}
	}
	return nil, false
}

// lookupString returns the trimmed lower-cased string value of the first
// matching key, or "" when absent or non-string.
func lookupString(obj map[string]any, keys ...string) string {
	v, ok := lookupField(obj, keys...)
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// mfaEnabledForUser reports whether a stored user row indicates MFA is on. CIPP
// exposes this under several field names depending on the source endpoint.
func mfaEnabledForUser(obj map[string]any) bool {
	for _, key := range []string{"MFARegistration", "isMfaRegistered", "MFA", "PerUserMFAState", "mfaEnabled", "isMFACapable", "MFAEnabled"} {
		if v, ok := lookupField(obj, key); ok {
			if s, isStr := v.(string); isStr && strings.EqualFold(strings.TrimSpace(s), "enforced") {
				return true
			}
			if truthy(v) {
				return true
			}
		}
	}
	return false
}

// caPolicyState buckets a Conditional Access policy by Microsoft Graph's
// documented conditionalAccessPolicy.state enum: "enabled",
// "enabledForReportingButNotEnforced", "disabled". Anything unrecognized
// (including a missing field) buckets as "unknown" rather than being silently
// counted as enforced — a report-only policy provides no protection, and
// conflating the two is the failure this dimension exists to prevent.
func caPolicyState(obj map[string]any) string {
	switch lookupString(obj, "state", "policyState") {
	case "enabled":
		return "enabled"
	case "enabledforreportingbutnotenforced":
		return "report_only"
	case "disabled":
		return "disabled"
	default:
		return "unknown"
	}
}

// complianceBucket buckets a Standards or BPA row into pass/fail using the
// boolean-ish compliance fields CIPP emits, or "unknown" when none is present.
// Returning "unknown" instead of guessing keeps an un-inspectable payload from
// being reported as compliant.
func complianceBucket(obj map[string]any, passKeys, failKeys []string) string {
	if v, ok := lookupField(obj, passKeys...); ok {
		if truthy(v) {
			return "pass"
		}
		return "fail"
	}
	if v, ok := lookupField(obj, failKeys...); ok {
		if truthy(v) {
			return "fail"
		}
		return "pass"
	}
	return "unknown"
}

// enabledBucket buckets a policy-ish row (DLP, retention, sensitivity label) by
// whether it is switched on, defaulting to "unknown" when no known field exists.
func enabledBucket(obj map[string]any) string {
	if v, ok := lookupField(obj, "enabled", "isEnabled", "Enabled", "Mode", "State"); ok {
		if s, isStr := v.(string); isStr {
			switch strings.ToLower(strings.TrimSpace(s)) {
			case "enable", "enabled", "on", "true", "active", "testwithnotifications", "testwithoutnotifications":
				return "enabled"
			case "disable", "disabled", "off", "false", "inactive":
				return "disabled"
			default:
				return "unknown"
			}
		}
		if truthy(v) {
			return "enabled"
		}
		return "disabled"
	}
	return "unknown"
}

// postureMetrics computes the per-row metric increments for one dimension.
// Every dimension emits a total so a tenant with data is never rendered as an
// empty row, plus dimension-specific signal buckets.
func postureMetrics(dimension string, obj map[string]any, m map[string]int) {
	switch dimension {
	case "mfa":
		m["users"]++
		if mfaEnabledForUser(obj) {
			m["mfa_enabled"]++
		} else {
			m["mfa_missing"]++
		}
	case "ca":
		m["policies"]++
		m[caPolicyState(obj)]++
	case "standards":
		m["standards"]++
		switch complianceBucket(obj,
			[]string{"Compliant", "isCompliant", "compliance"},
			[]string{"Deviated", "isDeviated", "drift", "Remediated"}) {
		case "pass":
			m["compliant"]++
		case "fail":
			m["deviated"]++
		default:
			m["unknown"]++
		}
	case "bpa":
		m["checks"]++
		switch complianceBucket(obj,
			[]string{"Passed", "isPassing", "Result", "Compliant"},
			[]string{"Failed", "isFailing", "FailureReason"}) {
		case "pass":
			m["passing"]++
		case "fail":
			m["failing"]++
		default:
			m["unknown"]++
		}
	case "dlp", "retention", "labels":
		m["policies"]++
		m[enabledBucket(obj)]++
	}
}

func newNovelPostureCmd(flags *rootFlags) *cobra.Command {
	var flagDimension string
	var flagDB string

	cmd := &cobra.Command{
		Use:         "posture",
		Short:       "One table of every tenant's MFA, Conditional Access, Standards, BPA, DLP, retention and label posture from the local store.",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--dimension=mfa"},
		Long: `Roll up one security dimension across every synced tenant into a single
tenant×metric matrix. Reads the local store only (no live calls) — populate it
first with 'cipp-cli fanout --endpoint /List<X> --all-tenants --save'.

Use this command to compare one posture dimension across every stored tenant.
Do NOT use this command for every signal about a single tenant; use 'scorecard' instead.
Do NOT use it to find which specific Conditional Access policy a tenant is
missing; use 'ca diff' instead, or what changed since last week; use
'standards drift' or 'changes' instead.

Dimensions:
  mfa        MFA registration per tenant: users, mfa_enabled, mfa_missing
  ca         Conditional Access per tenant: policies, enabled, report_only, disabled
  standards  Standards per tenant: standards, compliant, deviated
  bpa        Best Practice Analyser per tenant: checks, passing, failing
  dlp        DLP compliance policies per tenant: policies, enabled, disabled
  retention  Retention compliance policies per tenant: policies, enabled, disabled
  labels     Sensitivity labels per tenant: policies, enabled, disabled

A metric bucket of "unknown" means the stored payload carried no field this
command recognizes — it is reported rather than being counted as healthy.`,
		Example: "  cipp-cli posture --dimension mfa --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "posture")
			}
			if flagDimension == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--dimension is required: must be one of %s", strings.Join(postureDimensionNames(), ", ")))
			}
			dimension := strings.ToLower(strings.TrimSpace(flagDimension))
			dim, ok := postureDimensions[dimension]
			if !ok {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --dimension %q: must be one of %s", flagDimension, strings.Join(postureDimensionNames(), ", ")))
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

			fetched, err := fetchedTenants(db, dim.resourceType)
			if err != nil {
				return fmt.Errorf("reading %s snapshots: %w", dim.resourceType, err)
			}
			// Honest-empty path: no synced data → message to stderr, empty
			// result, exit 0 (NOT an error).
			if len(rows) == 0 && len(fetched) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"hint: no synced %s data; populate it with: cipp-cli fanout --endpoint %s --all-tenants --save\n",
					dimension, dim.listEndpoint)
				if flags.asJSON {
					return printNovelJSON(cmd, flags, []postureRow{})
				}
				return nil
			}

			out := computePosture(dimension, rows, fetched)
			if flags.asJSON {
				return printNovelJSON(cmd, flags, out)
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
	cmd.Flags().StringVar(&flagDimension, "dimension", "", "Posture dimension: "+strings.Join(postureDimensionNames(), ", "))
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}

// computePosture groups rows into one metric line per tenant. A tenant a
// fanout asked that returned zero rows still gets a line (its total is 0), so
// a tenant with no policies at all is visible instead of absent.
func computePosture(dimension string, rows []map[string]any, fetched []string) []postureRow {
	byTenant := map[string]map[string]int{}
	for _, t := range fetched {
		byTenant[t] = map[string]int{}
	}
	for _, obj := range rows {
		t := rowTenant(obj)
		if byTenant[t] == nil {
			byTenant[t] = map[string]int{}
		}
		postureMetrics(dimension, obj, byTenant[t])
	}
	out := make([]postureRow, 0, len(byTenant))
	for t, m := range byTenant {
		out = append(out, postureRow{Tenant: t, Metrics: m})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tenant < out[j].Tenant })
	return out
}
