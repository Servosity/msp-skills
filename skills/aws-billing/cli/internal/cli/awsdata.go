// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Shared AWS read helpers for the cost / consolidated / compare / waste / ask
// commands: period resolution, store queries, and store-first-with-live-fallback
// data loading.
//
// pp:client-call
package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
	"aws-billing-pp-cli/internal/store"
)

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
	// The day-count presets are whole days ending before today (End is
	// exclusive): 7d is seven days, never eight (Codex r1 class
	// wrong-requested-time-window).
	case "7d", "7days", "week":
		return periodRange{day(now).AddDate(0, 0, -7).Format(layout), day(now).Format(layout), "last 7 days", "DAILY"}, nil
	case "30d", "30days":
		return periodRange{day(now).AddDate(0, 0, -30).Format(layout), day(now).Format(layout), "last 30 days", "DAILY"}, nil
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

	sess *awsSession // the command's AWS client and account (awsbilling_scope.go)
}

// awsReadOptsFromFlags builds read options from the shared --data-source flag
// plus the AWS-specific flags a command declares.
func awsReadOptsFromFlags(flags *rootFlags, dbPath, profile, region string) awsReadOpts {
	o := awsReadOpts{dbPath: dbPath, profile: profile, region: region, sess: &awsSession{}}
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

// canonicalCostLines returns account x service cost lines for the period
// through the one cost reader (costLines): the local store when it holds this
// request's answer for this account, else Cost Explorer. The returned source
// is "local" or "live".
func canonicalCostLines(ctx context.Context, o awsReadOpts, pr periodRange) ([]awsx.CostLine, string, error) {
	return costLines(ctx, o, pr, costByAccountService)
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

// ensureUsageTypeLines returns service x usage-type lines for the period
// through the one cost reader; like keeps the usage types containing any of
// its substrings.
func ensureUsageTypeLines(ctx context.Context, o awsReadOpts, pr periodRange, like []string) ([]awsx.CostLine, string, error) {
	lines, source, err := costLines(ctx, o, pr, costByServiceUsage)
	if err != nil || len(like) == 0 {
		return lines, source, err
	}
	var out []awsx.CostLine
	for _, l := range lines {
		for _, sub := range like {
			if strings.Contains(l.UsageType, sub) {
				out = append(out, l)
				break
			}
		}
	}
	return out, source, nil
}

// ensureInventory returns the account's waste inventory of one region (the
// one a live scan covers, inventoryRegion): from the store when the account's
// scan of that region was complete, else from a fresh live scan (stored).
// --data-source local never scans: an account with no complete scan of that
// region stored is an error naming why.
func ensureInventory(ctx context.Context, o awsReadOpts, typeFilter, account string, wasteOnly bool) ([]awsx.InventoryResource, string, error) {
	if !o.live {
		if cliutil.IsVerifyEnv() && !o.localOnly {
			return nil, "verify", nil
		}
		db, err := store.OpenWithContext(ctx, o.db())
		if err != nil && o.localOnly {
			return nil, "local", err
		}
		if err == nil {
			scope, serr := o.readScope(ctx, db)
			if serr == nil {
				serr = inventoryAccountMatches(scope, account)
			}
			// The region a live read would scan, resolved the same way
			// (r1b-inventory-region-resolved): the store answers for it alone.
			region := ""
			if serr == nil {
				region, serr = o.inventoryRegion(ctx)
			}
			if serr != nil {
				_ = db.Close()
				return nil, "", serr
			}
			inv, ok, incomplete, ierr := storedInventory(ctx, db, scope, region, typeFilter, account, wasteOnly)
			_ = db.Close()
			if ierr != nil {
				return nil, "", ierr
			}
			if ok {
				return inv, "local", nil
			}
			if o.localOnly {
				if incomplete != "" {
					return nil, "local", fmt.Errorf("%s; run 'aws-billing-cli sync' again, or drop --data-source local", incomplete)
				}
				return nil, "local", fmt.Errorf("the local store holds no complete inventory scan of AWS account %s%s (run 'aws-billing-cli sync --region %s' first, or name the region a stored scan covers with --region)", scope, regionSuffix(region), region)
			}
		}
	}
	if cliutil.IsVerifyEnv() {
		return nil, "verify", nil
	}
	client, err := o.awsClient(ctx)
	if err != nil {
		return nil, "", err
	}
	scope, err := o.callerAccount(ctx)
	if err == nil {
		err = inventoryAccountMatches(scope, account)
	}
	if err != nil {
		return nil, "", err
	}
	all, err := client.CollectInventory(ctx, awsx.InventoryOptions{SkipCPU: cliutil.IsDogfoodEnv()})
	if db, derr := store.OpenWithContext(ctx, o.db()); derr == nil {
		// The cache is best-effort: the live answer is returned (or its error).
		_, _ = storeInventoryScan(ctx, db, scope, client.Region, all, err)
		_ = db.RecordScopeLabel(o.profileLabel(), scope)
		_ = db.Close()
	}
	if err != nil {
		return nil, "", err
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

// inventoryAccountMatches refuses an --account filter naming another account
// than the one the inventory is read for: inventory is scanned per account
// (the credentials' account), so another account's resources are never there
// and "no waste" would be a wrong answer.
func inventoryAccountMatches(scope, account string) error {
	if account == "" || account == scope {
		return nil
	}
	return usageErr(fmt.Errorf("--account %s: the inventory is scanned per AWS account and these credentials read account %s; run with that account's --profile-aws", account, scope))
}

func regionSuffix(region string) string {
	if region == "" {
		return ""
	}
	return " in " + region
}
