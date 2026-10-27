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

// caDiffRow is one tenant's deviation from the fleet's Conditional Access
// baseline.
type caDiffRow struct {
	Tenant  string   `json:"tenant"`
	Missing []string `json:"missing"`
	Extra   []string `json:"extra"`
	Total   int      `json:"total_policies"`
}

// caPolicyName returns the display name a Conditional Access policy is keyed
// by. Graph uses displayName; CIPP echoes it under several spellings depending
// on the entrypoint.
func caPolicyName(obj map[string]any) string {
	for _, key := range []string{"displayName", "DisplayName", "name", "Name", "policyName"} {
		if v, ok := lookupField(obj, key); ok {
			if s, isStr := v.(string); isStr {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

// caDefaultThreshold makes the baseline a strict majority of tenants. A fixed
// high share (the old 0.8) is a strict intersection on fleets of four or fewer:
// hardened tenants then read as "extra" and the gap tenant is never listed.
const caDefaultThreshold = 0.5

// caDiffReport is the ca diff result: the baseline used, so a reader can see
// what each tenant was compared against, plus the deviating tenants.
type caDiffReport struct {
	Tenants    int         `json:"tenants"`
	Threshold  float64     `json:"threshold"`
	Baseline   []string    `json:"baseline"`
	Deviations []caDiffRow `json:"deviations"`
}

// caBaselineSet computes the fleet baseline: every policy name held by MORE
// than `threshold` of tenants. A share rather than a strict intersection keeps
// one misconfigured tenant from emptying the baseline.
func caBaselineSet(byTenant map[string]map[string]bool, threshold float64) map[string]bool {
	counts := map[string]int{}
	for _, policies := range byTenant {
		for name := range policies {
			counts[name]++
		}
	}
	tenants := len(byTenant)
	if tenants == 0 {
		return map[string]bool{}
	}
	baseline := map[string]bool{}
	for name, n := range counts {
		if float64(n)/float64(tenants) > threshold {
			baseline[name] = true
		}
	}
	return baseline
}

// caPolicySets maps each tenant to its Conditional Access policy names. A
// tenant a fanout asked that has no policy at all is the most exposed one, so
// fetched tenants are seeded empty: they are compared, not skipped. ca diff
// and scorecard share this so both see the same fleet and baseline.
func caPolicySets(rows []map[string]any, fetched []string) map[string]map[string]bool {
	byTenant := map[string]map[string]bool{}
	for _, t := range fetched {
		byTenant[t] = map[string]bool{}
	}
	for _, obj := range rows {
		name := caPolicyName(obj)
		if name == "" {
			continue
		}
		t := rowTenant(obj)
		if byTenant[t] == nil {
			byTenant[t] = map[string]bool{}
		}
		byTenant[t][name] = true
	}
	return byTenant
}

// sortedKeys returns the set's members in stable order so JSON and table output
// do not churn between runs.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func newNovelCaDiffCmd(flags *rootFlags) *cobra.Command {
	var flagDB string
	var flagThreshold float64
	var flagAll bool

	cmd := &cobra.Command{
		Use:         "diff",
		Short:       "Find which tenants deviate from the Conditional Access baseline the rest of the fleet shares.",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		Long: `Compare every tenant's Conditional Access policy set against the baseline the
fleet shares, and report what each tenant is missing or carrying extra. Reads
the local store only (no live calls) — populate it first with
'cipp-cli fanout --endpoint /ListConditionalAccessPolicies --all-tenants --save'.

Use this command for which tenants deviate right now from the Conditional Access baseline most of the fleet shares.
Do NOT use this command for what changed over time; use 'changes' instead.
Do NOT use it for per-tenant policy counts; use 'posture --dimension ca' instead.

The policy one tenant is missing that most others have is computed offline as
a set difference across the stored policy lists.

The baseline is every policy name held by more than --threshold of tenants
(default 0.5, a strict majority), so one misconfigured tenant cannot empty it.`,
		Example: "  cipp-cli ca diff --agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "ca diff")
			}
			if flagThreshold < 0 || flagThreshold >= 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--threshold must be at least 0 and less than 1, got %v", flagThreshold))
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

			rows, err := readResourceRows(db, "conditionalaccesspolicies")
			if err != nil {
				return fmt.Errorf("reading conditionalaccesspolicies: %w", err)
			}

			// Honest-empty path: no synced data → stderr hint naming the
			// mechanism that works, empty result, exit 0 (NOT an error).
			// CIPP has no bulk-list endpoints; fanout --save populates this.
			fetched, err := fetchedTenants(db, "conditionalaccesspolicies")
			if err != nil {
				return fmt.Errorf("reading conditionalaccesspolicies snapshots: %w", err)
			}
			if len(rows) == 0 && len(fetched) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"hint: no synced conditionalaccesspolicies data; populate it with: cipp-cli fanout --endpoint /ListConditionalAccessPolicies --all-tenants --save")
				if flags.asJSON {
					return printNovelJSON(cmd, flags, caDiffReport{Threshold: flagThreshold, Baseline: []string{}, Deviations: []caDiffRow{}})
				}
				return nil
			}

			byTenant := caPolicySets(rows, fetched)

			if len(byTenant) < 2 {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"hint: only %d tenant(s) have synced Conditional Access policies; a cross-tenant diff needs at least 2. Populate more with: cipp-cli fanout --endpoint /ListConditionalAccessPolicies --all-tenants --save\n",
					len(byTenant))
				if flags.asJSON {
					return printNovelJSON(cmd, flags, caDiffReport{Tenants: len(byTenant), Threshold: flagThreshold, Baseline: []string{}, Deviations: []caDiffRow{}})
				}
				return nil
			}

			baseline := caBaselineSet(byTenant, flagThreshold)

			out := make([]caDiffRow, 0, len(byTenant))
			for tenant, policies := range byTenant {
				row := caDiffRow{Tenant: tenant, Missing: []string{}, Extra: []string{}, Total: len(policies)}
				for name := range baseline {
					if !policies[name] {
						row.Missing = append(row.Missing, name)
					}
				}
				for name := range policies {
					if !baseline[name] {
						row.Extra = append(row.Extra, name)
					}
				}
				sort.Strings(row.Missing)
				sort.Strings(row.Extra)
				if !flagAll && len(row.Missing) == 0 && len(row.Extra) == 0 {
					continue
				}
				out = append(out, row)
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Tenant < out[j].Tenant })

			if flags.asJSON {
				return printNovelJSON(cmd, flags, caDiffReport{Tenants: len(byTenant), Threshold: flagThreshold, Baseline: sortedKeys(baseline), Deviations: out})
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "hint: baseline: %d policies held by more than %.0f%% of %d tenants: %s\n",
				len(baseline), flagThreshold*100, len(byTenant), strings.Join(sortedKeys(baseline), ", "))
			if len(out) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"hint: all %d tenants match the Conditional Access baseline (%d policies); pass --all to list them anyway\n",
					len(byTenant), len(baseline))
				return nil
			}

			headers := []string{"TENANT", "MISSING", "EXTRA", "TOTAL"}
			tableRows := make([][]string, 0, len(out))
			for _, r := range out {
				tableRows = append(tableRows, []string{
					r.Tenant,
					strings.Join(r.Missing, ", "),
					strings.Join(r.Extra, ", "),
					fmt.Sprintf("%d", r.Total),
				})
			}
			return flags.printTable(cmd, headers, tableRows)
		},
	}
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	cmd.Flags().Float64Var(&flagThreshold, "threshold", caDefaultThreshold, "A policy is baseline when MORE than this share of tenants hold it [0-1); default 0.5 is a strict majority")
	cmd.Flags().BoolVar(&flagAll, "all", false, "Include tenants that fully match the baseline")
	return cmd
}
