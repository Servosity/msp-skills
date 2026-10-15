// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sophos-central-pp-cli/internal/client"
	"sophos-central-pp-cli/internal/cliutil"
	"sophos-central-pp-cli/internal/store"
)

var fleetSyncResources = []string{"tenants", "endpoints", "alerts", "health", "licenses"}

// fleetSyncSummary is the fleet refresh result envelope.
type fleetSyncSummary struct {
	DB              string           `json:"db"`
	TenantsTotal    int              `json:"tenants_total"`
	OK              int              `json:"ok"`
	Partial         int              `json:"partial"`
	Forbidden       int              `json:"forbidden"`
	NoAPIHost       int              `json:"no_api_host"`
	Errors          int              `json:"error"`
	Stored          map[string]int   `json:"stored"`
	Skipped         int              `json:"skipped,omitempty"`
	Unexpected      int              `json:"tenants_with_unexpected_errors"`
	DurationSeconds float64          `json:"duration_seconds"`
	Tenants         []fleetLedgerRow `json:"tenants"`
	Pruned          []string         `json:"pruned_tenants,omitempty"`
}

// fleetSyncPlan is the --plan envelope: what a sync would cost.
type fleetSyncPlan struct {
	Tenants          int      `json:"tenants"`
	Resources        []string `json:"resources"`
	CallsEstimate    int      `json:"calls_estimate"`
	RatePerSecond    float64  `json:"rate_per_second"`
	MinutesEstimate  float64  `json:"minutes_estimate"`
	CredentialType   string   `json:"credential_type"`
	SkippedTrialNote string   `json:"note,omitempty"`
}

func newNovelFleetRefreshCmd(flags *rootFlags) *cobra.Command {
	var flagResources, flagSince, dbPath string
	var flagIncludeTrial, flagPlan bool

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh tenants, endpoints, alerts, account health and licences for every customer tenant in one run",
		Long: `Use this command to refresh the local store for every tenant the credential can see (tenants, endpoints, alerts, account health, licences), recording a per-tenant ok/forbidden/no_api_host/error status. Do NOT use this command to refresh one resource for the currently selected tenant; use 'sync' instead.

Per-tenant 403s are expected for trial tenants and term customers without Partner Assistance; they are recorded in the ledger and the run continues. Calls share the CLI rate limiter (1.5 req/s by default; Sophos enforces 100/minute). Trial tenants are skipped unless --include-trial is set. A refresh without --tenant also removes the stored data of tenants outside its scope (offboarded tenants, and trial tenants when --include-trial is not set), so the local store always mirrors the last full refresh. A tenant credential syncs just its own tenant.`,
		Example: strings.Trim(`
  sophos-central-cli fleet refresh --resources tenants,endpoints,alerts,health,licenses
  sophos-central-cli fleet refresh --plan
  sophos-central-cli fleet refresh --resources alerts --since 24h --tenant 6f3c4a2e-1b9d-4e8f-a7c2-5d0e9b1f3a64`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "false", "mcp:local-write": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet refresh")
			}
			resources, err := parseFleetResources(flagResources)
			if err != nil {
				return usageErr(err)
			}
			var since time.Duration
			if flagSince != "" {
				if since, err = cliutil.ParseDurationLoose(flagSince); err != nil || since <= 0 {
					return usageErr(fmt.Errorf("--since %q: want a duration like 24h or 7d", flagSince))
				}
			}
			// No boundCtx: these runs span many tenants under the rate limit;
			// --timeout bounds each request inside the generated client.
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// A refresh reads live: a cached tenant list could prune a tenant
			// that was added (or refreshed with --tenant) a minute ago.
			c.NoCache = true
			filter, err := fleetTenantFilter()
			if err != nil {
				return err
			}
			// The prune cutoff: rows another refresh writes after this point
			// are never pruned by this run.
			started := time.Now().UTC()
			// An unfiltered refresh lists trial tenants too, to record the
			// ones it skips (their stored rows are out of scope, not blind).
			listed, idType, err := fleetTenantsFiltered(ctx, c, filter, flagIncludeTrial || len(filter) == 0)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			tenants, trialSkipped := listed, []string(nil)
			if !flagIncludeTrial && len(filter) == 0 {
				tenants = make([]fleetTenant, 0, len(listed))
				for _, t := range listed {
					if strings.EqualFold(t.BillingType, "trial") {
						trialSkipped = append(trialSkipped, t.ID)
						continue
					}
					tenants = append(tenants, t)
				}
			}
			if cliutil.IsDogfoodEnv() && len(tenants) > 2 {
				tenants = tenants[:2] // live dogfood: stay inside the 30s matrix budget
			}
			if flagPlan {
				return flags.printJSON(cmd, planFleetSync(tenants, resources, idType, flags.rateLimit, flagIncludeTrial))
			}
			// A refresh over every tenant in scope also drops tenants that left
			// it (offboarded, or trial tenants without --include-trial) so their
			// stale rows cannot pin freshness or linger in triage. Only such a
			// refresh moves the fleet-wide sync stamps.
			fullScope := len(filter) == 0 && !cliutil.IsDogfoodEnv()
			return runFleetSync(ctx, cmd, flags, c, tenants, resources, since, fleetDBPath(dbPath), fullScope, trialSkipped, started)
		},
	}
	cmd.Flags().StringVar(&flagResources, "resources", strings.Join(fleetSyncResources, ","), "Comma-separated resources to sync: tenants,endpoints,alerts,health,licenses")
	cmd.Flags().StringVar(&flagSince, "since", "", "Only fetch alerts raised within this window (e.g. 24h, 7d); alerts are then added, not reconciled")
	cmd.Flags().BoolVar(&flagIncludeTrial, "include-trial", false, "Include trial tenants (they usually return 403 for partners)")
	cmd.Flags().BoolVar(&flagPlan, "plan", false, "Print the tenant count and estimated calls/time, then stop (lists tenants only)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

func parseFleetResources(raw string) ([]string, error) {
	valid := map[string]bool{}
	for _, r := range fleetSyncResources {
		valid[r] = true
	}
	var out []string
	for _, r := range strings.Split(raw, ",") {
		r = strings.TrimSpace(strings.ToLower(r))
		if r == "" {
			continue
		}
		if !valid[r] {
			return nil, fmt.Errorf("--resources: unknown resource %q (valid: %s)", r, strings.Join(fleetSyncResources, ","))
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--resources: name at least one of %s", strings.Join(fleetSyncResources, ","))
	}
	return out, nil
}

// fleetTenantsFiltered applies a multi-ID filter on top of fleetTenants.
func fleetTenantsFiltered(ctx context.Context, c *client.Client, filter map[string]bool, includeTrial bool) ([]fleetTenant, string, error) {
	single := ""
	if len(filter) == 1 {
		for id := range filter {
			single = id
		}
	}
	tenants, idType, err := fleetTenants(ctx, c, single, includeTrial || len(filter) > 0)
	if err != nil || len(filter) <= 1 {
		return tenants, idType, err
	}
	kept := tenants[:0]
	found := map[string]bool{}
	for _, t := range tenants {
		if filter[t.ID] {
			kept = append(kept, t)
			found[t.ID] = true
		}
	}
	var missing []string
	for id := range filter {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, idType, usageErr(fmt.Errorf("tenant(s) %s not reachable with this %s credential", strings.Join(missing, ", "), idType))
	}
	return kept, idType, nil
}

func planFleetSync(tenants []fleetTenant, resources []string, idType string, rate float64, includeTrial bool) fleetSyncPlan {
	perTenant := 0
	for _, r := range resources {
		if r != "tenants" {
			perTenant++ // one page each for typical tenants (endpoints page 500, alerts page 100)
		}
	}
	if rate <= 0 || rate == client.RateLimitAuto {
		rate = 1.5
	}
	calls := 2 + perTenant*len(tenants) // whoami + tenant list
	p := fleetSyncPlan{
		Tenants: len(tenants), Resources: resources, CallsEstimate: calls, RatePerSecond: rate,
		MinutesEstimate: float64(int(float64(calls)/rate/60*10+0.5)) / 10, CredentialType: idType,
	}
	if !includeTrial && idType != "tenant" {
		p.SkippedTrialNote = "trial tenants excluded; add --include-trial to count them"
	}
	return p
}

func runFleetSync(ctx context.Context, cmd *cobra.Command, flags *rootFlags, c *client.Client, tenants []fleetTenant, resources []string, since time.Duration, dbPath string, fullScope bool, trialSkipped []string, started time.Time) error {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("opening local store %s: %w", dbPath, err)
	}
	defer db.Close()

	prev := loadFleetMeta(db)
	summary := fleetSyncSummary{DB: dbPath, TenantsTotal: len(tenants), Stored: map[string]int{}, Tenants: make([]fleetLedgerRow, 0, len(tenants))}
	want := map[string]bool{}
	for _, r := range resources {
		want[r] = true
	}

	if want["tenants"] {
		items := make([]json.RawMessage, 0, len(tenants))
		for _, t := range tenants {
			raw, _ := json.Marshal(t)
			items = append(items, raw)
		}
		n, _, err := db.UpsertBatch("tenants", items)
		if err != nil {
			return fmt.Errorf("storing tenants: %w", err)
		}
		summary.Stored["tenants"] = n
	}

	// A tenants-only refresh stores the list and touches no tenant: an empty
	// per-tenant pass would mark every tenant "error".
	perTenant := tenants
	if len(want) == 1 && want["tenants"] {
		perTenant = nil
	}
	attempted := 0
	for i, t := range perTenant {
		if ctx.Err() != nil {
			break
		}
		fmt.Fprintf(cmd.ErrOrStderr(), `{"event":"fleet_tenant","index":%d,"of":%d,"tenant_id":%q}`+"\n", i+1, len(tenants), t.ID)
		row := syncOneTenant(ctx, c, db, t, want, since, summary.Stored)
		// Count this run's own outcome; the stored ledger row merges in
		// earlier outcomes so other resources' failures stay visible.
		currentStatus := row.Status
		if hasUnexpectedFailure(row.Resources) {
			summary.Unexpected++
		}
		if prevRaw, gerr := db.Get(fleetLedgerType, t.ID); gerr == nil && len(prevRaw) > 0 {
			var prevRow fleetLedgerRow
			if json.Unmarshal(prevRaw, &prevRow) == nil {
				row = mergeLedgerRow(prevRow, row)
			}
		}
		raw, _ := json.Marshal(row)
		if err := replaceFleetDoc(db, fleetLedgerType, raw); err != nil {
			return fmt.Errorf("storing fleet ledger for %s: %w", t.ID, err)
		}
		attempted++
		switch currentStatus {
		case "ok":
			summary.OK++
		case "partial":
			summary.Partial++
		case "forbidden":
			summary.Forbidden++
		case "no_api_host":
			summary.NoAPIHost++
		default:
			summary.Errors++
		}
		summary.Tenants = append(summary.Tenants, row)
	}
	summary.DurationSeconds = time.Since(started).Round(time.Second).Seconds()

	// Interrupted: keep what was stored, but do not stamp the store as freshly
	// and completely refreshed, and do not report success.
	if err := ctx.Err(); err != nil {
		summary.Skipped = len(perTenant) - attempted
		_ = flags.printJSON(cmd, summary)
		return fmt.Errorf("fleet refresh interrupted after %d of %d tenants (%d not refreshed): %w", attempted, len(perTenant), summary.Skipped, err)
	}

	if summary.OK+summary.Partial == 0 && summary.Errors > 0 {
		_ = flags.printJSON(cmd, summary)
		return apiErr(fmt.Errorf("fleet refresh: no tenant refreshed (%d errors); see tenants[].resources", summary.Errors))
	}
	if fullScope {
		// An empty listing is an answer too: every stored tenant left.
		pruned, err := pruneOutOfScopeTenants(db, tenants, started)
		if err != nil {
			return fmt.Errorf("removing tenants no longer in scope: %w", err)
		}
		summary.Pruned = pruned
		// A --tenant refresh must not date other tenants' local rows; local
		// reads with --tenant use that tenant's ledger row instead.
		for _, rt := range []string{"tenants", "endpoints", "alerts"} {
			if want[rt] {
				_ = db.SaveSyncState(rt, "", summary.Stored[rt])
			}
		}
	}
	meta := fleetSyncMeta{ID: "sync", StartedAt: started, FinishedAt: time.Now().UTC(), PrevStartedAt: prev.StartedAt, TenantsAttempts: attempted, TrialSkipped: trialSkipped}
	if !fullScope {
		meta.TrialSkipped = prev.TrialSkipped
	}
	if raw, err := json.Marshal(meta); err == nil {
		_ = replaceFleetDoc(db, fleetMetaType, raw)
	}

	// Expected per-tenant outcomes (403, no apiHost) are recorded and do not
	// fail the run; any other failure (5xx, timeout, incomplete listing)
	// prints the full summary and then exits non-zero.
	var runErr error
	if summary.Unexpected > 0 {
		runErr = apiErr(fmt.Errorf("fleet refresh: %d tenant(s) had unexpected errors; see tenants[].resources", summary.Unexpected))
	}
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		rows := make([]map[string]any, 0, len(summary.Tenants))
		for _, r := range summary.Tenants {
			rows = append(rows, map[string]any{"tenant": r.TenantName, "status": r.Status, "endpoints": r.Resources["endpoints"].Items, "alerts": r.Resources["alerts"].Items})
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Synced %d tenants into %s: %d ok, %d partial, %d forbidden, %d no api host, %d error\n", summary.TenantsTotal, dbPath, summary.OK, summary.Partial, summary.Forbidden, summary.NoAPIHost, summary.Errors)
		if len(summary.Pruned) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Removed stored data for %d tenant(s) outside this refresh's scope (offboarded, or trial without --include-trial): %s\n", len(summary.Pruned), strings.Join(summary.Pruned, ", "))
		}
		if err := printAutoTable(cmd.OutOrStdout(), rows); err != nil {
			return err
		}
		return runErr
	}
	if err := flags.printJSON(cmd, summary); err != nil {
		return err
	}
	return runErr
}

// hasUnexpectedFailure reports a resource failure other than the expected
// per-tenant 403 or missing apiHost.
func hasUnexpectedFailure(resources map[string]fleetOutcome) bool {
	for _, o := range resources {
		if o.Status != "ok" && o.Status != "forbidden" && o.Status != "no_api_host" {
			return true
		}
	}
	return false
}

// syncOneTenant fetches the requested resources for one tenant and returns its
// ledger row. A 403 is recorded per resource and the next resource is still
// tried (Sophos permissions differ per API); a missing apiHost is tenant-wide,
// so it stops the tenant.
func syncOneTenant(ctx context.Context, c *client.Client, db *store.Store, t fleetTenant, want map[string]bool, since time.Duration, stored map[string]int) fleetLedgerRow {
	row := fleetLedgerRow{ID: t.ID, TenantName: t.Name, SyncedAt: time.Now().UTC(), Resources: map[string]fleetOutcome{}}
	steps := []struct {
		name string
		run  func() (int, error)
	}{
		{"endpoints", func() (int, error) {
			max := 0
			if cliutil.IsDogfoodEnv() {
				max = 500
			}
			items, err := fetchKeyPaged(ctx, c, "/endpoint/v1/endpoints", map[string]string{"pageSize": "500", "view": "full"}, t.ID, max)
			if err != nil {
				return 0, err
			}
			return storeTenantRows(db, "endpoints", t.ID, items, max == 0)
		}},
		{"alerts", func() (int, error) {
			params := map[string]string{"pageSize": "100"}
			if since > 0 {
				params["from"] = time.Now().UTC().Add(-since).Format(time.RFC3339)
			}
			items, err := fetchKeyPaged(ctx, c, "/common/v1/alerts", params, t.ID, 0)
			if err != nil {
				return 0, err
			}
			return storeTenantRows(db, "alerts", t.ID, items, since == 0)
		}},
		{"health", func() (int, error) {
			raw, err := c.GetWithHeadersNoCache(ctx, "/account-health-check/v1/health-check", nil, tenantHeaders(t.ID))
			if err != nil {
				return 0, err
			}
			if _, err := tenantDocObject("health check", raw); err != nil {
				return 0, err
			}
			if prevRaw, gerr := db.Get(fleetHealthType, t.ID); gerr == nil && len(prevRaw) > 0 {
				_ = replaceFleetDoc(db, fleetHealthPrevType, prevRaw)
			}
			return 1, storeTenantDoc(db, fleetHealthType, t, raw)
		}},
		{"licenses", func() (int, error) {
			raw, err := c.GetWithHeadersNoCache(ctx, "/licenses/v1/licenses", nil, tenantHeaders(t.ID))
			if err != nil {
				return 0, err
			}
			doc, err := tenantDocObject("licences", raw)
			if err != nil {
				return 0, err
			}
			list, ok := doc["licenses"].([]any) // required by the licensing spec
			if !ok {
				return 0, fmt.Errorf("licences response has no licenses array")
			}
			for i, l := range list {
				if _, ok := l.(map[string]any); !ok {
					return 0, fmt.Errorf("licences response item %d is not a JSON object", i)
				}
			}
			return 1, storeTenantDoc(db, fleetLicensesType, t, raw)
		}},
	}
	for _, step := range steps {
		if !want[step.name] {
			continue
		}
		n, err := step.run()
		if err != nil {
			status := classifyTenantErr(err)
			row.Resources[step.name] = fleetOutcome{TenantID: t.ID, TenantName: t.Name, Status: status, Error: truncateErr(err)}
			if status == "no_api_host" || ctx.Err() != nil {
				break
			}
			continue
		}
		stored[step.name] += n
		now := time.Now().UTC()
		o := fleetOutcome{TenantID: t.ID, TenantName: t.Name, Status: "ok", Items: n, At: now, FullAt: now}
		if step.name == "alerts" && since > 0 {
			o.Window, o.FullAt = since.String(), time.Time{} // added, not reconciled
		}
		row.Resources[step.name] = o
	}
	row.Status = ledgerStatus(row.Resources)
	return row
}

// ledgerStatus summarizes per-resource outcomes: ok when every resource is
// ok, the shared failure status when every resource failed the same way,
// partial when some succeeded, error otherwise.
func ledgerStatus(resources map[string]fleetOutcome) string {
	if len(resources) == 0 {
		return "error"
	}
	counts := map[string]int{}
	for _, o := range resources {
		counts[o.Status]++
	}
	switch {
	case counts["ok"] == len(resources):
		return "ok"
	case counts["ok"] > 0:
		return "partial"
	case counts["forbidden"] == len(resources):
		return "forbidden"
	case counts["no_api_host"] == len(resources):
		return "no_api_host"
	}
	return "error"
}

// mergeLedgerRow keeps outcomes for resources this run did not touch, so a
// health-only refresh cannot hide an earlier endpoint failure.
func mergeLedgerRow(prev, cur fleetLedgerRow) fleetLedgerRow {
	merged := map[string]fleetOutcome{}
	for k, v := range prev.Resources {
		merged[k] = v
	}
	for k, v := range cur.Resources {
		if p, ok := prev.Resources[k]; ok {
			if v.Status == "ok" {
				v.PrevAt = p.At
				if v.FullAt.IsZero() {
					v.FullAt = p.FullAt // a --since fetch on top of an earlier full one
				}
			} else {
				v.At, v.PrevAt, v.FullAt = p.At, p.PrevAt, p.FullAt
			}
		}
		merged[k] = v
	}
	cur.Resources = merged
	cur.Status = ledgerStatus(merged)
	return cur
}

// storeTenantRows upserts a tenant's list rows and, for a complete listing,
// deletes that tenant's rows that are no longer returned (resolved alerts,
// removed endpoints). Rows are partitioned by their tenant.id field.
func storeTenantRows(db *store.Store, resourceType, tenantID string, items []json.RawMessage, complete bool) (int, error) {
	// Only rows that carry their Sophos id are stored. The store can fall
	// back to other fields (a hostname) for an id-less row; that would both
	// store a pseudo-row and leave the real row out of the seen set.
	valid := make([]json.RawMessage, 0, len(items))
	seen := make([]string, 0, len(items))
	for _, it := range items {
		obj, derr := store.DecodeJSONObject(it)
		sophosID, _ := obj["id"].(string)
		if derr != nil || strings.TrimSpace(sophosID) == "" {
			continue
		}
		valid = append(valid, it)
		seen = append(seen, store.ExtractResourceID(resourceType, obj))
	}
	// A fleet fetch returns whole objects (view=full), so each row replaces
	// the stored one in the same transaction: UpsertBatch's keep-richer merge
	// would keep a cleared field, e.g. assignedProducts emptied by the API.
	// The rows go to the local mirror (for --data-source local reads) and to
	// the fleet copy the fleet views read.
	for _, rt := range []string{resourceType, fleetCopyType(resourceType)} {
		n, extractFailures, err := db.ReplaceBatch(rt, valid)
		if err != nil {
			return 0, fmt.Errorf("storing %s: %w", rt, err)
		}
		dropped := len(items) - len(valid)
		if dropped > 0 || extractFailures > 0 || n != len(valid) {
			return n, fmt.Errorf("storing %s for tenant %s: %d of %d rows had no usable id; skipped reconcile", resourceType, tenantID, dropped+extractFailures+len(valid)-n, len(items))
		}
		if !complete {
			continue
		}
		typed, _ := store.TypedListTable(rt)
		if _, err := db.ReconcilePartition(rt, "$.tenant.id", tenantID, seen, typed, nil); err != nil {
			return n, fmt.Errorf("reconciling %s for tenant %s: %w", rt, tenantID, err)
		}
	}
	return len(valid), nil
}

// fleetCopyType names the fleet-owned copy of a mirrored resource.
func fleetCopyType(resourceType string) string {
	return map[string]string{"endpoints": fleetEndpointsType, "alerts": fleetAlertsType}[resourceType]
}

// storeTenantDoc stores a per-tenant document (health check, licences) keyed
// by tenant ID with the tenant name and fetch time alongside.
// tenantDocObject rejects a 200 body that is not a JSON object (for example
// null), which would otherwise be stored and read back as "no findings".
func tenantDocObject(what string, raw json.RawMessage) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, fmt.Errorf("%s response is not a JSON object", what)
	}
	return m, nil
}

func storeTenantDoc(db *store.Store, resourceType string, t fleetTenant, raw json.RawMessage) error {
	doc := map[string]any{"id": t.ID, "tenant_id": t.ID, "tenant_name": t.Name, "synced_at": time.Now().UTC(), "data": raw}
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := replaceFleetDoc(db, resourceType, b); err != nil {
		return fmt.Errorf("storing %s: %w", resourceType, err)
	}
	return nil
}

// tenantsWrittenSince returns the tenants whose fleet endpoint or alert rows
// were written after t (only fleet refresh writes them). updated_at is RFC3339 UTC with second precision, so a
// write in the same second as t is not counted.
// ponytail: second precision; a concurrent write within that second can still be pruned.
func tenantsWrittenSince(db *store.Store, t time.Time) (map[string]bool, error) {
	rows, err := db.DB().Query(`SELECT DISTINCT json_extract(data, '$.tenant.id') FROM resources
		WHERE resource_type IN ('fleet_endpoints', 'fleet_alerts') AND json_valid(data) AND updated_at > ?`, t.UTC().Truncate(time.Second).Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id sql.NullString
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id.Valid && id.String != "" {
			out[id.String] = true
		}
	}
	return out, rows.Err()
}

// replaceFleetDoc stores a fleet snapshot exactly as given, in one
// transaction. UpsertBatch keeps richer existing JSON (an omitted key or an
// empty array keeps the old value), which suits API rows but not a snapshot:
// a refresh that returns no licences, or a ledger outcome with no window or
// error, must replace.
func replaceFleetDoc(db *store.Store, resourceType string, raw json.RawMessage) error {
	n, _, err := db.ReplaceBatch(resourceType, []json.RawMessage{raw})
	if err == nil && n != 1 {
		err = fmt.Errorf("%s document has no id", resourceType)
	}
	return err
}

func truncateErr(err error) string {
	s := err.Error()
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

// pruneOutOfScopeTenants removes every stored trace of tenants that a full
// listing no longer returns: their tenant and ledger rows, health and licence
// documents, and endpoint and alert rows (including rows plain sync stored
// for a tenant fleet refresh never recorded). It returns the removed tenant IDs.
func pruneOutOfScopeTenants(db *store.Store, tenants []fleetTenant, started time.Time) ([]string, error) {
	scope := make(map[string]bool, len(tenants))
	ids := make([]string, 0, len(tenants))
	for _, t := range tenants {
		scope[t.ID] = true
		ids = append(ids, t.ID)
	}
	ledger, err := loadFleetLedger(db)
	if err != nil {
		return nil, err
	}
	stored := make(map[string]bool, len(ledger))
	for id := range ledger {
		stored[id] = true
	}
	// A tenants-only refresh stores tenant rows with no ledger row yet.
	// Rows without a name are not tenants (a mutation response cached under
	// the tenants group); the reconcile below still clears them.
	if err := db.ListScan("tenants", func(key string, raw json.RawMessage) bool {
		var t fleetTenant
		if json.Unmarshal(raw, &t) == nil && t.ID != "" && t.Name != "" {
			stored[t.ID] = true
		}
		return true
	}); err != nil {
		return nil, err
	}
	// A run cut off after storing a tenant's rows but before its ledger row.
	for _, rt := range []string{fleetEndpointsType, fleetAlertsType} {
		if err := db.ListScan(rt, func(_ string, raw json.RawMessage) bool {
			var row struct {
				Tenant struct {
					ID string `json:"id"`
				} `json:"tenant"`
			}
			if json.Unmarshal(raw, &row) == nil && row.Tenant.ID != "" {
				stored[row.Tenant.ID] = true
			}
			return true
		}); err != nil {
			return nil, err
		}
	}
	recent, err := tenantsWrittenSince(db, started)
	if err != nil {
		return nil, err
	}
	keep := append([]string(nil), ids...)
	gone := make([]string, 0)
	for id := range stored {
		if scope[id] {
			continue
		}
		// A --tenant refresh that ran alongside this one (a customer
		// onboarded mid-run) wrote rows after this run listed tenants.
		if ledger[id].SyncedAt.After(started) || recent[id] {
			keep = append(keep, id)
			continue
		}
		gone = append(gone, id)
	}
	sort.Strings(gone)
	for _, id := range gone {
		for _, rt := range []string{"endpoints", "alerts", fleetEndpointsType, fleetAlertsType} {
			typed, _ := store.TypedListTable(rt)
			if _, err := db.ReconcilePartition(rt, "$.tenant.id", id, nil, typed, nil); err != nil {
				return nil, err
			}
			// Its checkpoints go too, or a local read would date an empty
			// list with the last plain sync of a tenant no longer stored.
			if _, err := db.DB().Exec(`DELETE FROM sync_state WHERE resource_type = ?`, rt+"@"+id); err != nil {
				return nil, err
			}
		}
	}
	for _, rt := range []string{"tenants", fleetLedgerType, fleetHealthType, fleetHealthPrevType, fleetLicensesType} {
		typed, _ := store.TypedListTable(rt)
		if _, err := db.ReconcileAll(rt, keep, typed, nil); err != nil {
			return nil, err
		}
	}
	return gone, nil
}
