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

// scorecardSignal is one section of a tenant scorecard. SyncedAt is when its
// newest stored row was saved; NoData means nothing is stored for it, which is
// reported instead of being read as healthy.
type scorecardSignal struct {
	Signal   string `json:"signal"`
	SyncedAt string `json:"synced_at,omitempty"`
	NoData   bool   `json:"no_data"`
	Result   any    `json:"result,omitempty"`
}

type scorecardReport struct {
	Tenant  string            `json:"tenant"`
	Signals []scorecardSignal `json:"signals"`
}

// resolveScorecardTenant maps a --tenant value (a defaultDomainName or a
// customer tenant id) to the defaultDomainName rows are stamped with, plus
// the customer id GDAP relationships are keyed by.
func resolveScorecardTenant(tenants []tenantRef, arg string) (domain, customerID string) {
	arg = strings.TrimSpace(arg)
	for _, t := range tenants {
		if strings.EqualFold(t.DefaultDomainName, arg) || strings.EqualFold(t.CustomerID, arg) {
			return t.DefaultDomainName, t.CustomerID
		}
	}
	return arg, arg
}

// tenantRows keeps the rows stamped with tenant (case-insensitive).
func tenantRows(rows []map[string]any, tenant string) []map[string]any {
	out := []map[string]any{}
	for _, r := range rows {
		if strings.EqualFold(rowTenant(r), tenant) {
			out = append(out, r)
		}
	}
	return out
}

// newestSyncedAt is the most recent synced_at of resourceType rows for tenant.
func newestSyncedAt(db *store.Store, resourceType, tenant string) string {
	var ts *string
	_ = db.DB().QueryRow(
		`SELECT MAX(synced_at) FROM resources WHERE resource_type = ? AND lower(json_extract(data, '$._tenant')) = lower(?)`,
		resourceType, tenant,
	).Scan(&ts)
	if ts == nil {
		return ""
	}
	return *ts
}

// buildScorecard composes every stored computation for one tenant. All rows
// are read (and their sql.Rows drained) before any per-signal query runs.
func buildScorecard(db *store.Store, domain, customerID string, now time.Time) (scorecardReport, error) {
	read := map[string][]map[string]any{}
	types := []string{"users", "licenses", "conditionalaccesspolicies", "mailboxforwarding", "mailboxrules", "roles", gdapRelationshipResource}
	for _, dim := range postureDimensionNames() {
		types = append(types, postureDimensions[dim].resourceType)
	}
	for _, rt := range types {
		if _, done := read[rt]; done {
			continue
		}
		rows, err := readResourceRows(db, rt)
		if err != nil {
			return scorecardReport{}, fmt.Errorf("reading %s: %w", rt, err)
		}
		read[rt] = rows
	}
	snaps, err := loadStandardsSnapshots(db)
	if err != nil {
		return scorecardReport{}, fmt.Errorf("reading standards snapshots: %w", err)
	}

	rep := scorecardReport{Tenant: domain, Signals: []scorecardSignal{}}
	add := func(name, resourceType string, rows []map[string]any, result any) {
		sig := scorecardSignal{Signal: name, NoData: len(rows) == 0}
		if !sig.NoData {
			sig.Result = result
			sig.SyncedAt = newestSyncedAt(db, resourceType, domain)
		}
		rep.Signals = append(rep.Signals, sig)
	}

	for _, dim := range postureDimensionNames() {
		rt := postureDimensions[dim].resourceType
		rows := tenantRows(read[rt], domain)
		m := map[string]int{}
		for _, r := range rows {
			postureMetrics(dim, r, m)
		}
		add("posture."+dim, rt, rows, m)
	}

	users := tenantRows(read["users"], domain)
	skuNames := skuDisplayNames(db)
	dormant := computeDormantUsers(users, now, now.AddDate(0, 0, -30), 30, false)
	for i := range dormant {
		dormant[i].Licenses = nameSKUs(dormant[i].Licenses, skuNames)
	}
	leftovers := computeOffboardLeftovers(users,
		tenantRows(read["mailboxforwarding"], domain), tenantRows(read["mailboxrules"], domain),
		tenantRows(read["roles"], domain), false)
	for i := range leftovers {
		leftovers[i].Licenses = nameSKUs(leftovers[i].Licenses, skuNames)
	}
	add("users.stale", "users", users, computeStaleUsers(users, now, 90))
	add("licenses.dormant", "users", users, dormant)
	add("users.offboard_leftovers", "users", users, leftovers)

	// Conditional Access: this tenant against the fleet baseline, computed
	// exactly as 'ca diff' computes it (fetched empty tenants included).
	caFetched, err := fetchedTenants(db, "conditionalaccesspolicies")
	if err != nil {
		return scorecardReport{}, fmt.Errorf("reading conditionalaccesspolicies snapshots: %w", err)
	}
	byTenant := caPolicySets(read["conditionalaccesspolicies"], caFetched)
	var mine map[string]bool
	for t, set := range byTenant {
		if strings.EqualFold(t, domain) {
			mine = set
		}
	}
	if len(byTenant) >= 2 && mine != nil {
		baseline := caBaselineSet(byTenant, caDefaultThreshold)
		missing, extra := []string{}, []string{}
		for name := range baseline {
			if !mine[name] {
				missing = append(missing, name)
			}
		}
		for name := range mine {
			if !baseline[name] {
				extra = append(extra, name)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		sig := scorecardSignal{Signal: "ca.baseline_diff",
			Result: caDiffRow{Tenant: domain, Missing: missing, Extra: extra, Total: len(mine)}}
		sig.SyncedAt = newestSyncedAt(db, "conditionalaccesspolicies", domain)
		rep.Signals = append(rep.Signals, sig)
	} else {
		// A diff needs this tenant and at least one other.
		add("ca.baseline_diff", "conditionalaccesspolicies", nil, nil)
	}

	var mySnaps []standardsSnapshotRow
	for _, s := range snaps {
		if strings.EqualFold(s.tenant, domain) {
			mySnaps = append(mySnaps, s)
		}
	}
	drift := scorecardSignal{Signal: "standards.drift", NoData: len(mySnaps) == 0}
	if !drift.NoData {
		drift.Result = computeStandardsDrift(mySnaps, time.Time{})
		times := make([]string, 0, len(mySnaps))
		for _, s := range mySnaps {
			times = append(times, s.syncedAt)
		}
		sortSnapshotTimes(times)
		drift.SyncedAt = times[len(times)-1]
	}
	rep.Signals = append(rep.Signals, drift)

	// GDAP rows are partner-wide and keyed by customer tenant id, not _tenant.
	var gdap []map[string]any
	for _, r := range read[gdapRelationshipResource] {
		if t := gdapRelationshipTenant(r); strings.EqualFold(t, customerID) || strings.EqualFold(t, domain) {
			gdap = append(gdap, r)
		}
	}
	active := 0
	for _, r := range gdap {
		if gdapIsActive(r) {
			active++
		}
	}
	gsig := scorecardSignal{Signal: "gdap.relationships", NoData: len(gdap) == 0}
	if !gsig.NoData {
		gsig.Result = map[string]int{"relationships": len(gdap), "active": active}
		var ts *string
		_ = db.DB().QueryRow(`SELECT MAX(synced_at) FROM resources WHERE resource_type = ?`, gdapRelationshipResource).Scan(&ts)
		if ts != nil {
			gsig.SyncedAt = *ts
		}
	}
	rep.Signals = append(rep.Signals, gsig)
	return rep, nil
}

func newNovelScorecardCmd(flags *rootFlags) *cobra.Command {
	var flagTenant string
	var flagDB string

	cmd := &cobra.Command{
		Use:   "scorecard",
		Short: "Every stored signal about one tenant in a single record, each with the time its data was saved.",
		Long: `Use this command for every stored signal about one tenant in a single record.
Do NOT use this command to compare one metric across all tenants; use 'posture' instead.

Composes the posture dimensions, stale accounts, dormant licenses, offboarding
leftovers, the Conditional Access baseline diff, Standards drift and GDAP
relationships for one tenant from the local store. Each signal carries the
time its newest row was saved; a signal with nothing stored says no_data
instead of reading as healthy. --tenant takes the tenant's default domain or
its customer tenant id.

Populate the store first, e.g.:
  cipp-cli fanout --preset posture --all-tenants --save`,
		Example:     "  cipp-cli scorecard --tenant 7d3c9f2e-1b4a-4c8e-9f6d-2a5b8c1e4f70 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--tenant=7d3c9f2e-1b4a-4c8e-9f6d-2a5b8c1e4f70"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagTenant == "" {
				if len(args) == 0 && cmd.Flags().NFlag() == 0 {
					return cmd.Help()
				}
				return usageErr(fmt.Errorf("--tenant is required (default domain or customer tenant id)"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "scorecard")
			}
			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("cipp-cli")
			}
			tenants, _ := loadTenantsFromStore(cmd.Context(), dbPath)
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()
			domain, customerID := resolveScorecardTenant(tenants, flagTenant)
			rep, err := buildScorecard(db, domain, customerID, time.Now())
			if err != nil {
				return err
			}
			empty := true
			for _, s := range rep.Signals {
				if !s.NoData {
					empty = false
				}
			}
			if empty {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: nothing stored for tenant %q; populate the store with: cipp-cli fanout --preset posture --all-tenants --save\n", flagTenant)
			}
			if flags.asJSON {
				return printNovelJSON(cmd, flags, rep)
			}
			headers := []string{"SIGNAL", "SYNCED AT", "RESULT"}
			table := make([][]string, 0, len(rep.Signals))
			for _, s := range rep.Signals {
				res := "no data"
				if !s.NoData {
					res = fmt.Sprintf("%v", s.Result)
					if len(res) > 80 {
						res = res[:77] + "..."
					}
				}
				table = append(table, []string{s.Signal, s.SyncedAt, res})
			}
			return flags.printTable(cmd, headers, table)
		},
	}
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Tenant default domain or customer tenant id")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}
