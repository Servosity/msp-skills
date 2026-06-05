// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. Not generated.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"n-central-pp-cli/internal/cliutil"
	"n-central-pp-cli/internal/store"
)

// triageGroup is an aggregated rollup of active issues sharing a group key.
type triageGroup struct {
	Group       string            `json:"group"`
	Count       int               `json:"count"`
	TopSeverity int               `json:"topSeverity"`
	Issues      []json.RawMessage `json:"issues"`
}

const triageOrgUnitCap = 50

func newNovelTriageCmd(flags *rootFlags) *cobra.Command {
	var flagBy string
	var flagOrgUnit int
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Group active monitoring issues by customer, device, or monitor — a live rollup of what's red right now.",
		Long: `Fetch active issues live from N-central and roll them up by customer, device,
or monitor, sorted within each group by severity (notificationState).

Scope to one org unit with --org-unit, or omit it to sweep every org unit in
the local mirror (capped at 50; run 'sync' first to populate org units).

triage requires live API access. Under --data-source local it returns an
honest error because active issues are not cached locally.`,
		Example: `  n-central-cli triage --by customer
  n-central-cli triage --by monitor --org-unit 100 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}

			by := flagBy
			if by == "" {
				by = "customer"
			}
			switch by {
			case "customer", "device", "monitor":
			default:
				return usageErr(fmt.Errorf("invalid --by %q: must be customer, device, or monitor", by))
			}

			if flags.dataSource == "local" {
				return fmt.Errorf("triage requires live API access: active issues are not cached locally; re-run with --data-source live or auto")
			}

			// Verify-mode: skip all network/store IO, return a clean empty rollup.
			if cliutil.IsVerifyEnv() {
				return flags.printJSON(cmd, []triageGroup{})
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Resolve the list of org units to scan.
			var orgUnitIDs []string
			if flagOrgUnit > 0 {
				orgUnitIDs = []string{fmt.Sprintf("%d", flagOrgUnit)}
			} else {
				ids, derr := triageOrgUnitIDs(cmd, flags)
				if derr != nil {
					return derr
				}
				orgUnitIDs = ids
			}
			if len(orgUnitIDs) == 0 {
				return fmt.Errorf("no org units to scan: pass --org-unit <id> or run 'n-central-cli sync' to populate org units")
			}

			// Fetch and aggregate.
			groups := map[string]*triageGroup{}
			for _, ouID := range orgUnitIDs {
				path := "/org-units/" + ouID + "/active-issues"
				raw, gerr := c.Get(cmd.Context(), path, nil)
				if gerr != nil {
					// Per-org-unit failures (e.g. 403 on an inaccessible unit)
					// shouldn't sink the whole sweep.
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: active-issues fetch failed for org unit %s: %v\n", ouID, classifyAPIError(gerr, flags))
					continue
				}
				for _, issue := range unwrapData(raw) {
					triageAggregate(groups, by, issue)
				}
			}

			out := triageSort(groups)
			if len(out) == 0 {
				if wantsHumanTable(cmd.OutOrStdout(), flags) {
					fmt.Fprintln(cmd.ErrOrStderr(), "No active issues found.")
					return nil
				}
				return flags.printJSON(cmd, []triageGroup{})
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				tw := newTabWriter(cmd.OutOrStdout())
				fmt.Fprintln(tw, bold("GROUP")+"\t"+bold("ISSUES")+"\t"+bold("TOP SEVERITY"))
				for _, g := range out {
					fmt.Fprintf(tw, "%s\t%d\t%d\n", g.Group, g.Count, g.TopSeverity)
				}
				return tw.Flush()
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&flagBy, "by", "customer", "Group by: customer, device, or monitor")
	cmd.Flags().IntVar(&flagOrgUnit, "org-unit", 0, "Scope to a single org unit ID (default: sweep all org units in the local mirror)")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum org units to sweep (0 = default cap)")
	return cmd
}

// triageOrgUnitIDs reads org unit IDs from the local mirror, capped.
func triageOrgUnitIDs(cmd *cobra.Command, flags *rootFlags) ([]string, error) {
	dbPath := defaultDBPath("n-central-cli")
	db, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'n-central-cli sync' first, or pass --org-unit.", err)
	}
	defer db.Close()

	rows, err := db.DB().QueryContext(cmd.Context(),
		`SELECT org_unit_id FROM org_units WHERE org_unit_id IS NOT NULL ORDER BY org_unit_id`)
	if err != nil {
		return nil, fmt.Errorf("reading org units: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning org unit: %w", err)
		}
		ids = append(ids, fmt.Sprintf("%d", id))
		if len(ids) >= triageOrgUnitCap {
			fmt.Fprintf(cmd.ErrOrStderr(), "note: capping org-unit sweep at %d; pass --org-unit to target a specific unit\n", triageOrgUnitCap)
			break
		}
	}
	return ids, rows.Err()
}

// triageAggregate folds one active-issue row into the group map per the --by key.
func triageAggregate(groups map[string]*triageGroup, by string, issue json.RawMessage) {
	obj := decodeObj(issue)
	if obj == nil {
		return
	}
	var key string
	switch by {
	case "device":
		key = asString(firstField(obj, "deviceId", "device_id"))
		if name := asString(extraField(obj, "deviceName")); name != "" {
			key = name
		}
	case "monitor":
		key = asString(firstField(obj, "serviceName", "service_name"))
	default: // customer
		key = asString(extraField(obj, "customerName"))
		if key == "" {
			key = asString(firstField(obj, "soCustomerID", "customerId", "customer_id"))
		}
	}
	if key == "" {
		key = "(unknown)"
	}

	sev := 0
	if v := firstField(obj, "notificationState", "notification_state"); v != nil {
		if n, ok := asInt(v); ok {
			sev = n
		}
	}

	g := groups[key]
	if g == nil {
		g = &triageGroup{Group: key}
		groups[key] = g
	}
	g.Count++
	if sev > g.TopSeverity {
		g.TopSeverity = sev
	}
	g.Issues = append(g.Issues, issue)
}

// triageSort sorts issues within each group by severity desc, then sorts the
// groups by top severity desc, then count desc, then name.
func triageSort(groups map[string]*triageGroup) []triageGroup {
	out := make([]triageGroup, 0, len(groups))
	for _, g := range groups {
		sort.SliceStable(g.Issues, func(i, j int) bool {
			return triageSeverity(g.Issues[i]) > triageSeverity(g.Issues[j])
		})
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TopSeverity != out[j].TopSeverity {
			return out[i].TopSeverity > out[j].TopSeverity
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Group < out[j].Group
	})
	return out
}

func triageSeverity(issue json.RawMessage) int {
	obj := decodeObj(issue)
	if obj == nil {
		return 0
	}
	if v := firstField(obj, "notificationState", "notification_state"); v != nil {
		if n, ok := asInt(v); ok {
			return n
		}
	}
	return 0
}
