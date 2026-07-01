// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Shared AWS read helpers for the cost / consolidated / compare / waste / ask
// commands: period resolution, store queries, and store-first-with-live-fallback
// data loading.
//
// pp:client-call
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
	"aws-billing-pp-cli/internal/store"
)

// mustJSON marshals v to JSON, returning an empty object on error (never used
// in practice for our flat structs).
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// periodRange is a resolved [Start, End) date window (YYYY-MM-DD, End exclusive)
// plus a human label and the granularity it implies.
type periodRange struct {
	Start       string
	End         string
	Label       string
	Granularity string
}

// resolvePeriod turns a --period preset (or explicit "from:to") into a range.
// Presets: this-month (default), last-month, last-3-months, last-6-months,
// ytd, 7d, 30d, yesterday. An explicit range is "YYYY-MM-DD:YYYY-MM-DD".
func resolvePeriod(preset string) (periodRange, error) {
	const layout = "2006-01-02"
	now := time.Now().UTC()
	som := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC) }
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	thisMonth := som(now)

	p := strings.TrimSpace(strings.ToLower(preset))
	switch p {
	case "", "this-month", "month", "mtd":
		return periodRange{thisMonth.Format(layout), thisMonth.AddDate(0, 1, 0).Format(layout), "this month", "MONTHLY"}, nil
	case "last-month", "last":
		return periodRange{thisMonth.AddDate(0, -1, 0).Format(layout), thisMonth.Format(layout), "last month", "MONTHLY"}, nil
	case "last-3-months", "3m", "last-3":
		return periodRange{thisMonth.AddDate(0, -2, 0).Format(layout), thisMonth.AddDate(0, 1, 0).Format(layout), "last 3 months", "MONTHLY"}, nil
	case "last-6-months", "6m":
		return periodRange{thisMonth.AddDate(0, -5, 0).Format(layout), thisMonth.AddDate(0, 1, 0).Format(layout), "last 6 months", "MONTHLY"}, nil
	case "ytd":
		jan := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return periodRange{jan.Format(layout), thisMonth.AddDate(0, 1, 0).Format(layout), "year to date", "MONTHLY"}, nil
	case "7d", "7days", "week":
		return periodRange{day(now).AddDate(0, 0, -7).Format(layout), day(now).AddDate(0, 0, 1).Format(layout), "last 7 days", "DAILY"}, nil
	case "30d", "30days":
		return periodRange{day(now).AddDate(0, 0, -30).Format(layout), day(now).AddDate(0, 0, 1).Format(layout), "last 30 days", "DAILY"}, nil
	case "yesterday":
		return periodRange{day(now).AddDate(0, 0, -1).Format(layout), day(now).Format(layout), "yesterday", "DAILY"}, nil
	}
	if from, to, ok := strings.Cut(preset, ":"); ok {
		if _, err := time.Parse(layout, from); err != nil {
			return periodRange{}, fmt.Errorf("invalid period start %q (want YYYY-MM-DD)", from)
		}
		if _, err := time.Parse(layout, to); err != nil {
			return periodRange{}, fmt.Errorf("invalid period end %q (want YYYY-MM-DD)", to)
		}
		gran := "MONTHLY"
		return periodRange{from, to, from + " to " + to, gran}, nil
	}
	return periodRange{}, fmt.Errorf("unknown period %q (try this-month, last-month, last-3-months, ytd, 7d, 30d, yesterday, or YYYY-MM-DD:YYYY-MM-DD)", preset)
}

// previousPeriod returns the window immediately preceding pr: one calendar
// month earlier for MONTHLY ranges, or the same day-span earlier for DAILY.
func previousPeriod(pr periodRange) periodRange {
	const layout = "2006-01-02"
	start, _ := time.Parse(layout, pr.Start)
	end, _ := time.Parse(layout, pr.End)
	if pr.Granularity == "DAILY" {
		span := end.Sub(start)
		return periodRange{start.Add(-span).Format(layout), start.Format(layout), "prior " + pr.Label, "DAILY"}
	}
	months := monthsBetween(start, end)
	if months < 1 {
		months = 1
	}
	ps := start.AddDate(0, -months, 0)
	return periodRange{ps.Format(layout), start.Format(layout), "prior period", "MONTHLY"}
}

func monthsBetween(a, b time.Time) int {
	return int(b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
}

// awsReadOpts carries the common flags the AWS read commands accept.
type awsReadOpts struct {
	dbPath    string
	profile   string
	region    string
	live      bool // force a live Cost Explorer call (ignore cache)
	localOnly bool // never fall back to a live call (--data-source local)
}

// awsReadOptsFromFlags builds read options from the shared --data-source flag
// plus the AWS-specific flags a command declares.
func awsReadOptsFromFlags(flags *rootFlags, dbPath, profile, region string) awsReadOpts {
	o := awsReadOpts{dbPath: dbPath, profile: profile, region: region}
	switch flags.dataSource {
	case "live":
		o.live = true
	case "local":
		o.localOnly = true
	}
	return o
}

func (o awsReadOpts) db() string {
	if o.dbPath != "" {
		return o.dbPath
	}
	return defaultDBPath("aws-billing-cli")
}

// canonicalCostLines returns account×service cost lines for the period. It
// reads the local store first; if empty (or --live), it calls Cost Explorer,
// caches the result, and returns it. The returned source is "local" or "live".
func canonicalCostLines(ctx context.Context, o awsReadOpts, pr periodRange) ([]awsx.CostLine, string, error) {
	if !o.live {
		lines, err := queryCanonicalLines(ctx, o.db(), pr)
		// Only trust the cache when it actually covers the requested period;
		// a narrower previously-synced window must not masquerade as a wider
		// one (that would corrupt month-over-month comparisons).
		if err == nil && len(lines) > 0 && (o.localOnly || cachePeriodCovered(ctx, o.db(), pr, false)) {
			return lines, "local", nil
		}
		if o.localOnly {
			return lines, "local", err
		}
	}
	// Verify mode: never dial AWS; return empty so commands exit 0.
	if cliutil.IsVerifyEnv() {
		return nil, "verify", nil
	}
	// Live fetch + cache.
	client, err := awsx.New(ctx, o.profile, o.region)
	if err != nil {
		return nil, "", err
	}
	lines, err := client.GetCostAndUsageGrouped(ctx, pr.Start, pr.End, pr.Granularity, []string{"LINKED_ACCOUNT", "SERVICE"})
	if err != nil {
		return nil, "", err
	}
	nameByID := liveAccountNames(ctx, client)
	if db, derr := store.OpenWithContext(ctx, o.db()); derr == nil {
		defer db.Close()
		cacheCanonicalLines(db, lines, nameByID)
	}
	for i := range lines {
		lines[i].AccountName = nameByID[lines[i].AccountID]
	}
	return lines, "live", nil
}

func liveAccountNames(ctx context.Context, client *awsx.Client) map[string]string {
	m := map[string]string{}
	accts, err := client.ListAccounts(ctx)
	if err != nil {
		return m
	}
	for _, a := range accts {
		m[a.AccountID] = a.Name
	}
	return m
}

func cacheCanonicalLines(db *store.Store, lines []awsx.CostLine, nameByID map[string]string) {
	for _, l := range lines {
		l.AccountName = nameByID[l.AccountID]
		l.ID = fmt.Sprintf("as|%s|%s|%s|%s", l.Granularity, l.PeriodStart, l.AccountID, l.Service)
		data := mustJSON(l)
		_ = db.UpsertCosts(data)
	}
}

// expectedPeriodCount returns how many distinct period_start buckets a fully
// synced period should contain (months for MONTHLY, days for DAILY).
func expectedPeriodCount(pr periodRange) int {
	const layout = "2006-01-02"
	start, _ := time.Parse(layout, pr.Start)
	end, _ := time.Parse(layout, pr.End)
	if pr.Granularity == "DAILY" {
		d := int(end.Sub(start).Hours() / 24)
		if d < 1 {
			d = 1
		}
		return d
	}
	m := monthsBetween(start, end)
	if m < 1 {
		m = 1
	}
	return m
}

// cachePeriodCovered reports whether the local store holds at least as many
// distinct period buckets as the requested period expects — i.e. the cache
// fully covers it, not just a narrower previously-synced sub-window.
func cachePeriodCovered(ctx context.Context, dbPath string, pr periodRange, usageType bool) bool {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return false
	}
	defer db.Close()
	pred := "(usage_type IS NULL OR usage_type = '')"
	if usageType {
		pred = "usage_type IS NOT NULL AND usage_type <> ''"
	}
	q := `SELECT COUNT(DISTINCT period_start) FROM "costs" WHERE ` + pred + ` AND period_start >= ? AND period_start < ?`
	var n int
	if err := db.DB().QueryRowContext(ctx, q, pr.Start, pr.End).Scan(&n); err != nil {
		return false
	}
	return n >= expectedPeriodCount(pr)
}

// queryCanonicalLines reads account×service lines (usage_type empty) from the
// store for the period.
func queryCanonicalLines(ctx context.Context, dbPath string, pr periodRange) ([]awsx.CostLine, error) {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.DB().QueryContext(ctx,
		`SELECT period_start, period_end, granularity, account_id, account_name, service, region, usage_type, amount_usd, unit
		 FROM "costs"
		 WHERE (usage_type IS NULL OR usage_type = '')
		   AND period_start >= ? AND period_start < ?
		 ORDER BY amount_usd DESC`, pr.Start, pr.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCostLines(rows)
}

// ensureUsageTypeLines returns service×usage-type lines for the period,
// store-first with a live Cost Explorer fallback (cached). `like` filters the
// usage-type to the given substrings.
func ensureUsageTypeLines(ctx context.Context, o awsReadOpts, pr periodRange, like []string) ([]awsx.CostLine, string, error) {
	if !o.live {
		lines, err := queryUsageTypeLines(ctx, o.db(), pr, like)
		if err == nil && len(lines) > 0 && (o.localOnly || cachePeriodCovered(ctx, o.db(), pr, true)) {
			return lines, "local", nil
		}
		if o.localOnly {
			return lines, "local", err
		}
	}
	if cliutil.IsVerifyEnv() {
		return nil, "verify", nil
	}
	client, err := awsx.New(ctx, o.profile, o.region)
	if err != nil {
		return nil, "", err
	}
	lines, err := client.GetCostAndUsageGrouped(ctx, pr.Start, pr.End, pr.Granularity, []string{"SERVICE", "USAGE_TYPE"})
	if err != nil {
		return nil, "", err
	}
	if db, derr := store.OpenWithContext(ctx, o.db()); derr == nil {
		defer db.Close()
		for _, l := range lines {
			l.ID = fmt.Sprintf("ut|%s|%s|%s|%s", l.Granularity, l.PeriodStart, l.Service, l.UsageType)
			_ = db.UpsertCosts(mustJSON(l))
		}
	}
	// Apply the like filter in-memory for the live path.
	if len(like) > 0 {
		var out []awsx.CostLine
		for _, l := range lines {
			for _, sub := range like {
				if strings.Contains(l.UsageType, sub) {
					out = append(out, l)
					break
				}
			}
		}
		return out, "live", nil
	}
	return lines, "live", nil
}

// queryUsageTypeLines reads service×usage-type lines (usage_type non-empty)
// from the store, optionally filtered to usage-type substrings.
func queryUsageTypeLines(ctx context.Context, dbPath string, pr periodRange, like []string) ([]awsx.CostLine, error) {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	q := `SELECT period_start, period_end, granularity, account_id, account_name, service, region, usage_type, amount_usd, unit
	      FROM "costs"
	      WHERE usage_type IS NOT NULL AND usage_type <> ''
	        AND period_start >= ? AND period_start < ?`
	args := []any{pr.Start, pr.End}
	if len(like) > 0 {
		var ors []string
		for _, l := range like {
			ors = append(ors, "usage_type LIKE ?")
			args = append(args, "%"+l+"%")
		}
		q += " AND (" + strings.Join(ors, " OR ") + ")"
	}
	q += " ORDER BY amount_usd DESC"
	rows, err := db.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCostLines(rows)
}

func scanCostLines(rows interface {
	Next() bool
	Scan(...any) error
}) ([]awsx.CostLine, error) {
	var out []awsx.CostLine
	for rows.Next() {
		var l awsx.CostLine
		if err := rows.Scan(&l.PeriodStart, &l.PeriodEnd, &l.Granularity, &l.AccountID, &l.AccountName,
			&l.Service, &l.Region, &l.UsageType, &l.AmountUSD, &l.Unit); err != nil {
			return out, err
		}
		out = append(out, l)
	}
	return out, nil
}

// loadAccounts reads synced AWS Organizations accounts from the store,
// optionally filtered to one account ID. Returns an empty slice (no error)
// when the store hasn't been synced — the accounts table is only populated
// from a management-account sync, so empty is a normal state, not a failure.
func loadAccounts(ctx context.Context, dbPath, accountID string) ([]awsx.Account, error) {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	q := `SELECT account_id, name, status, email FROM "accounts" WHERE 1=1`
	var argv []any
	if accountID != "" {
		q += " AND account_id = ?"
		argv = append(argv, accountID)
	}
	q += " ORDER BY account_id"
	rows, err := db.DB().QueryContext(ctx, q, argv...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []awsx.Account
	for rows.Next() {
		var a awsx.Account
		if err := rows.Scan(&a.AccountID, &a.Name, &a.Status, &a.Email); err != nil {
			return out, err
		}
		a.ID = a.AccountID
		out = append(out, a)
	}
	return out, nil
}

// loadInventory reads waste inventory from the store, optionally filtered by
// resource type and/or account, and optionally only waste candidates.
func loadInventory(ctx context.Context, dbPath, typeFilter, account string, wasteOnly bool) ([]awsx.InventoryResource, error) {
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	q := `SELECT resource_type, resource_id, account_id, region, state, monthly_cost_usd, monthly_waste_usd, waste_reason, attrs
	      FROM "inventory" WHERE 1=1`
	var args []any
	if typeFilter != "" {
		q += " AND resource_type = ?"
		args = append(args, typeFilter)
	}
	if account != "" {
		q += " AND account_id = ?"
		args = append(args, account)
	}
	if wasteOnly {
		q += " AND monthly_waste_usd > 0"
	}
	q += " ORDER BY monthly_waste_usd DESC, monthly_cost_usd DESC"
	rows, err := db.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []awsx.InventoryResource
	for rows.Next() {
		var r awsx.InventoryResource
		if err := rows.Scan(&r.ResourceType, &r.ResourceID, &r.AccountID, &r.Region, &r.State,
			&r.MonthlyCostUSD, &r.MonthlyWasteUSD, &r.WasteReason, &r.Attrs); err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

// ensureInventory returns waste inventory from the store, falling back to a
// fresh live scan (cached) when the store is empty or --live is set.
func ensureInventory(ctx context.Context, o awsReadOpts, typeFilter, account string, wasteOnly bool) ([]awsx.InventoryResource, string, error) {
	if !o.live {
		inv, err := loadInventory(ctx, o.db(), typeFilter, account, wasteOnly)
		if err == nil && len(inv) > 0 {
			return inv, "local", nil
		}
		if o.localOnly {
			return inv, "local", err
		}
	}
	if cliutil.IsVerifyEnv() {
		return nil, "verify", nil
	}
	client, err := awsx.New(ctx, o.profile, o.region)
	if err != nil {
		return nil, "", err
	}
	all, err := client.CollectInventory(ctx, awsx.InventoryOptions{SkipCPU: cliutil.IsDogfoodEnv()})
	if err != nil {
		return nil, "", err
	}
	if db, derr := store.OpenWithContext(ctx, o.db()); derr == nil {
		defer db.Close()
		for _, r := range all {
			r.ID = r.ResourceType + "|" + r.ResourceID
			_ = db.UpsertInventory(mustJSON(r))
		}
	}
	// Apply the same filters in-memory for the live path.
	var out []awsx.InventoryResource
	for _, r := range all {
		if typeFilter != "" && r.ResourceType != typeFilter {
			continue
		}
		if account != "" && r.AccountID != account {
			continue
		}
		if wasteOnly && r.MonthlyWasteUSD <= 0 {
			continue
		}
		out = append(out, r)
	}
	return out, "live", nil
}
