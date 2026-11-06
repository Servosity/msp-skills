// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// aws-billing handfix (cost-cache-answers-only-its-request; Codex r1 classes
// cache-answer-not-the-request, requested-dimension-never-fetched and
// failure-read-as-an-empty-result). Every read of AWS billing data goes
// through one reader per kind of data, and the local store answers a read only
// with what AWS answered for the same request:
//
//   - the account (scope): rows are stored under the AWS account the
//     credentials resolve to (sts:GetCallerIdentity, free and permission-free).
//     An automatic read resolves the account first and reads only that
//     account's rows; --data-source local reads without AWS, so it takes the
//     account recorded for the AWS profile named (--profile-aws, AWS_PROFILE),
//     or the store's only account, and refuses to pick one of several;
//   - Cost Explorer: the dimension pair (costKind: account x service, service
//     x usage type, service x region; each command fetches the pair its
//     breakdown needs), the granularity and the exact time periods. A request
//     is answered locally only when every period it spans (costBuckets: the
//     calendar months or days of [start, end), cut at both ends) was stored
//     from a complete answer for that account, pair and granularity, and,
//     for an automatic read, AWS did not mark it estimated (the current
//     month's amounts change until it closes: an automatic read fetches it
//     live; --data-source local reads the stored estimate);
//   - inventory: rows of the account, from regions whose last scan was
//     complete (a stored incomplete scan is never "no waste");
//   - accounts: the account's complete Organizations list.
// A read failure is an error, never an empty answer.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
	"aws-billing-pp-cli/internal/store"
)

// costKind is the Cost Explorer dimension pair a cost read groups by.
type costKind struct {
	id   string
	dims []string
	what string
}

var (
	costByAccountService = costKind{"as", []string{"LINKED_ACCOUNT", "SERVICE"}, "cost by account and service"}
	costByServiceUsage   = costKind{"ut", []string{"SERVICE", "USAGE_TYPE"}, "cost by service and usage type"}
	costByServiceRegion  = costKind{"rg", []string{"SERVICE", "REGION"}, "cost by service and region"}
)

// awsSession is the AWS client and the account it resolves to, built once
// per command.
type awsSession struct {
	client     *awsx.Client
	clientErr  error
	built      bool
	account    string
	accountErr error
	resolved   bool
}

func (o awsReadOpts) session() *awsSession {
	if o.sess == nil {
		return &awsSession{}
	}
	return o.sess
}

// awsClient builds the AWS client of the command (once).
func (o awsReadOpts) awsClient(ctx context.Context) (*awsx.Client, error) {
	s := o.session()
	if !s.built {
		s.client, s.clientErr = awsx.New(ctx, o.profile, o.region)
		s.built = true
	}
	return s.client, s.clientErr
}

// callerAccount is the AWS account the command's credentials resolve to.
func (o awsReadOpts) callerAccount(ctx context.Context) (string, error) {
	s := o.session()
	if !s.resolved {
		s.resolved = true
		client, err := o.awsClient(ctx)
		if err != nil {
			s.accountErr = err
		} else if s.account, _, s.accountErr = client.CallerIdentity(ctx); s.accountErr == nil && s.account == "" {
			s.accountErr = fmt.Errorf("sts:GetCallerIdentity named no account")
		}
	}
	return s.account, s.accountErr
}

// profileLabel is the AWS profile the command names: --profile-aws, else
// AWS_PROFILE, else AWS_DEFAULT_PROFILE; "" when none is named.
func (o awsReadOpts) profileLabel() string {
	if o.profile != "" {
		return o.profile
	}
	for _, name := range []string{"AWS_PROFILE", "AWS_DEFAULT_PROFILE"} {
		if v := strings.TrimSpace(cliutil.EnvOverride(name)); v != "" {
			return v
		}
	}
	return ""
}

// readScope is the account whose stored data this read may use: the
// credentials' account (automatic reads), or for --data-source local the
// account recorded for the named profile, or the store's only account.
func (o awsReadOpts) readScope(ctx context.Context, db *store.Store) (string, error) {
	if o.localOnly {
		return localScope(db, o.profileLabel())
	}
	acct, err := o.callerAccount(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving the AWS account of these credentials (the local store is read only for that account; --data-source local reads it without AWS): %w", err)
	}
	_ = db.RecordScopeLabel(o.profileLabel(), acct)
	return acct, nil
}

// localScope picks the account a --data-source local read uses.
func localScope(db *store.Store, label string) (string, error) {
	if label != "" {
		scope, ok, err := db.ScopeForLabel(label)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("the local store holds no data recorded for AWS profile %q (run 'aws-billing-cli sync --profile-aws %s' first)", label, label)
		}
		return scope, nil
	}
	scopes, err := db.DataScopes()
	if err != nil {
		return "", err
	}
	switch len(scopes) {
	case 0:
		return "", fmt.Errorf("the local store holds no AWS data yet (run 'aws-billing-cli sync' first)")
	case 1:
		return scopes[0], nil
	}
	return "", fmt.Errorf("the local store holds data for %d AWS accounts (%s): pass --profile-aws <profile> to read one, or drop --data-source local to read the account of the current credentials", len(scopes), strings.Join(scopes, ", "))
}

// costBuckets are the time periods Cost Explorer answers [start, end) in: the
// calendar months (MONTHLY) or days (DAILY) it spans, the first and last cut
// at start and end.
func costBuckets(pr periodRange) []store.CostBucket {
	const layout = "2006-01-02"
	s, err1 := time.Parse(layout, pr.Start)
	e, err2 := time.Parse(layout, pr.End)
	if err1 != nil || err2 != nil || !s.Before(e) {
		return nil
	}
	var out []store.CostBucket
	for cur := s; cur.Before(e); {
		next := time.Date(cur.Year(), cur.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		if pr.Granularity == "DAILY" {
			next = cur.AddDate(0, 0, 1)
		}
		if next.After(e) {
			next = e
		}
		out = append(out, store.CostBucket{Start: cur.Format(layout), End: next.Format(layout)})
		cur = next
	}
	return out
}

// costLines is the one reader of Cost Explorer cost lines: the local store
// when it holds this request's answer for this account (automatic reads and
// --data-source local), else a live call whose complete answer is stored.
func costLines(ctx context.Context, o awsReadOpts, pr periodRange, kind costKind) ([]awsx.CostLine, string, error) {
	if !o.live {
		if cliutil.IsVerifyEnv() && !o.localOnly {
			return nil, "verify", nil // verify mode never dials AWS
		}
		lines, scope, ok, err := storedCostLines(ctx, o, pr, kind)
		if err != nil {
			return nil, "", err
		}
		if ok {
			return lines, "local", nil
		}
		if o.localOnly {
			return nil, "local", fmt.Errorf("the local store does not hold the %s for %s (%s) of AWS account %s: run 'aws-billing-cli sync' for this period, or drop --data-source local", kind.what, pr.Label, strings.ToLower(pr.Granularity), scope)
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
	if err != nil {
		return nil, "", err
	}
	ans, err := client.GetCostAndUsageAnswer(ctx, pr.Start, pr.End, pr.Granularity, kind.dims)
	if err != nil {
		return nil, "", err
	}
	if kind.id == costByAccountService.id {
		names := liveAccountNames(ctx, client)
		for i := range ans.Lines {
			ans.Lines[i].AccountName = names[ans.Lines[i].AccountID]
		}
	}
	if db, derr := store.OpenWithContext(ctx, o.db()); derr == nil {
		// The cache is best-effort: the live answer is returned either way,
		// and a period whose answer was not stored whole stays uncovered.
		_ = storeCostAnswer(db, scope, kind, pr.Granularity, ans)
		_ = db.RecordScopeLabel(o.profileLabel(), scope)
		_ = db.Close()
	}
	return ans.Lines, "live", nil
}

// storedCostLines reads this request's answer from the local store: ok only
// when every period it spans is covered for the account, pair and
// granularity. scope is the account it read for.
func storedCostLines(ctx context.Context, o awsReadOpts, pr periodRange, kind costKind) ([]awsx.CostLine, string, bool, error) {
	db, err := store.OpenWithContext(ctx, o.db())
	if err != nil {
		if o.localOnly {
			return nil, "", false, err
		}
		return nil, "", false, nil // no store to read: a live call answers
	}
	defer db.Close()
	scope, err := o.readScope(ctx, db)
	if err != nil {
		return nil, "", false, err
	}
	// One read transaction: the coverage record and the rows together (Codex
	// r2 class cache-answer-not-the-request).
	rows, covered, err := db.CostRowsIfCovered(ctx, scope, kind.id, pr.Granularity, costBuckets(pr), o.localOnly)
	if err != nil || !covered {
		return nil, scope, false, err
	}
	lines := make([]awsx.CostLine, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, awsx.CostLine{PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd, Granularity: r.Granularity,
			AccountID: r.AccountID, AccountName: r.AccountName, Service: r.Service, Region: r.Region, UsageType: r.UsageType,
			AmountUSD: r.AmountUSD, Unit: r.Unit})
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].AmountUSD > lines[j].AmountUSD })
	return lines, scope, true, nil
}

// storeCostAnswer stores a complete answer: each line under its period's
// prefix and group keys. Fail closed: an answer with a line outside the
// periods it named is not stored at all (its periods stay uncovered).
func storeCostAnswer(db *store.Store, scope string, kind costKind, granularity string, ans awsx.CostAnswer) error {
	in := map[store.CostBucket]bool{}
	var buckets []store.CostBucket
	for _, b := range ans.Buckets {
		in[store.CostBucket{Start: b.Start, End: b.End}] = true
		buckets = append(buckets, store.CostBucket{Start: b.Start, End: b.End, Estimated: b.Estimated})
	}
	rows := make([]json.RawMessage, 0, len(ans.Lines))
	for _, l := range ans.Lines {
		b := store.CostBucket{Start: l.PeriodStart, End: l.PeriodEnd}
		if !in[b] {
			return fmt.Errorf("a cost line for %s..%s is outside the periods the answer named", l.PeriodStart, l.PeriodEnd)
		}
		l.ID = store.CostRowPrefix(kind.id, scope, granularity, b) + strings.Join(costGroupKeys(l, kind.dims), "|")
		data, err := json.Marshal(l)
		if err != nil {
			return err
		}
		rows = append(rows, data)
	}
	return db.ReplaceCostBuckets(scope, kind.id, granularity, buckets, rows)
}

// costGroupKeys are a line's values of the dimensions it was grouped by,
// each as written behind its length (n:value), so no value can run into the
// next; the value is never re-encoded, so the store's held-value check reads
// it as AWS sent it.
func costGroupKeys(l awsx.CostLine, dims []string) []string {
	out := make([]string, 0, len(dims))
	for _, d := range dims {
		v := ""
		switch d {
		case "LINKED_ACCOUNT":
			v = l.AccountID
		case "SERVICE":
			v = l.Service
		case "USAGE_TYPE":
			v = l.UsageType
		case "REGION":
			v = l.Region
		}
		out = append(out, fmt.Sprintf("%d:%s", len(v), v))
	}
	return out
}

// inventoryRegion is the one region an inventory read answers for, stored or
// live (aws-billing handfix r1b-inventory-region-resolved; Codex r1b class
// cache-answer-not-the-request, region): the region the command's AWS client
// scans, resolved by awsx.ResolveRegion exactly as awsx.New resolves it
// (--region, AWS_REGION, AWS_DEFAULT_REGION, the profile's region, else
// us-east-1). It needs no request, so --data-source local resolves it too.
func (o awsReadOpts) inventoryRegion(ctx context.Context) (string, error) {
	if s := o.session(); s.built && s.clientErr == nil {
		return s.client.Region, nil
	}
	region, err := awsx.ResolveRegion(ctx, o.profile, o.region)
	if err != nil {
		return "", fmt.Errorf("resolving the AWS region this inventory read answers for (a scan covers one region): %w; pass --region <region>", err)
	}
	return region, nil
}

// inventoryScanDetail is the reason a scan is incomplete, for its record.
func inventoryScanDetail(scanErr, storeErr error) string {
	switch {
	case scanErr != nil:
		return scanErr.Error()
	case storeErr != nil:
		return storeErr.Error()
	}
	return ""
}

// storedInventory reads the account's stored inventory of region in one
// transaction (store.InventoryIfScanned): ok only when the account has a
// recorded scan of that region and it was complete. incomplete names an
// incomplete scan when there is one. region is the one a live read would
// scan (awsReadOpts.inventoryRegion), never "any".
func storedInventory(ctx context.Context, db *store.Store, scope, region, typeFilter, account string, wasteOnly bool) (inv []awsx.InventoryResource, ok bool, incomplete string, err error) {
	rows, ok, incomplete, err := db.InventoryIfScanned(ctx, scope, region, typeFilter, account, wasteOnly)
	for _, r := range rows {
		inv = append(inv, awsx.InventoryResource{ResourceType: r.ResourceType, ResourceID: r.ResourceID, AccountID: r.AccountID,
			Region: r.Region, State: r.State, MonthlyCostUSD: r.MonthlyCostUSD, MonthlyWasteUSD: r.MonthlyWasteUSD,
			WasteReason: r.WasteReason, Attrs: r.Attrs})
	}
	return inv, ok, incomplete, err
}
