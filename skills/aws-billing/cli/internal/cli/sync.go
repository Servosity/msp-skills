// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// AWS-backed sync: populates the local SQLite cache from the AWS SDK (Cost
// Explorer + Organizations + EC2/CloudWatch/S3 inventory). This is a synthetic
// CLI — the generated HTTP client is not used for AWS.
//
// pp:client-call
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"aws-billing-pp-cli/internal/awsx"
	"aws-billing-pp-cli/internal/cliutil"
	"aws-billing-pp-cli/internal/store"
)

type syncSummary struct {
	Accounts  int         `json:"accounts"`
	CostLines int         `json:"cost_lines"`
	Forecasts int         `json:"forecasts"`
	Inventory int         `json:"inventory"`
	Warnings  []string    `json:"warnings,omitempty"`
	Period    string      `json:"period"`
	Layers    []syncLayer `json:"layers"`
}

// syncLayer is the outcome of one AWS data layer a sync attempted (accounts,
// costs, forecasts, inventory). A layer fails when its AWS call failed or any
// row could not be stored, so "nothing synced" is never reported as "synced 0".
type syncLayer struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// record appends one layer outcome; the first error wins for that layer.
func (s *syncSummary) record(name string, errs ...error) {
	l := syncLayer{Name: name, OK: true}
	for _, err := range errs {
		if err != nil {
			l.OK, l.Error = false, err.Error()
			break
		}
	}
	s.Layers = append(s.Layers, l)
}

// succeeded counts the layers that synced.
func (s syncSummary) succeeded() int {
	n := 0
	for _, l := range s.Layers {
		if l.OK {
			n++
		}
	}
	return n
}

// allFailed reports a sync in which every attempted layer failed. Callers fail
// closed on it: an all-failed sync must not read as an empty account.
func (s syncSummary) allFailed() error {
	if len(s.Layers) == 0 || s.succeeded() > 0 {
		return nil
	}
	return fmt.Errorf("sync failed: 0 of %d AWS data layer(s) synced (%s)", len(s.Layers), s.Layers[0].Error)
}

type awsSyncOpts struct {
	dbPath        string
	profile       string
	region        string
	start         string
	end           string
	granularity   string
	skipInventory bool
	skipCost      bool
}

func newSyncCmd(flags *rootFlags) *cobra.Command {
	var dbPath, profile, region, fromStr, toStr, granularity string
	var months int
	var skipInventory, skipCost, full bool
	var resources []string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync AWS cost + inventory data into the local SQLite cache",
		Long: `Pull cost and resource data from AWS once into a local SQLite database so
later queries (cost, consolidated, compare, waste, ask) run offline and don't
re-bill the Cost Explorer API ($0.01 per request).

What it syncs:
  accounts    AWS Organizations member accounts (id -> name resolution)
  costs       Cost Explorer cost lines (account x service, plus service x usage-type detail)
  forecasts   Cost Explorer next-period forecast (best effort)
  inventory   EC2 / EBS / snapshots / Elastic IPs / S3 buckets for waste detection

Cost Explorer data is org-wide only from the management (payer) account; from a
member account you see that account's own costs. Resource inventory works in any
account. Credentials resolve from --profile-aws, the environment, or SSO (EC2
instance-metadata credentials are not used).`,
		Example: `  # Sync the last 3 months for the default profile
  aws-billing-cli sync

  # Sync a specific profile and date range
  aws-billing-cli sync --profile-aws prod --from 2026-01-01 --to 2026-06-01

  # Inventory only (works in a member account without Cost Explorer access)
  aws-billing-cli sync --profile-aws dev --skip-cost`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if months <= 0 {
				months = 3
			}
			_ = full // cost/inventory sync is always a full refresh; flag accepted for tooling compatibility
			// --resources narrows what gets synced. Unknown names are ignored
			// (e.g. tooling probes with --resources repos).
			if len(resources) > 0 {
				want := map[string]bool{}
				for _, r := range resources {
					want[strings.ToLower(strings.TrimSpace(r))] = true
				}
				if !want["costs"] && !want["forecasts"] {
					skipCost = true
				}
				if !want["inventory"] {
					skipInventory = true
				}
			}
			start, end, err := resolveSyncRange(fromStr, toStr, months)
			if err != nil {
				return usageErr(err)
			}
			if granularity == "" {
				granularity = "MONTHLY"
			}
			if dbPath == "" {
				dbPath = defaultDBPath("aws-billing-cli")
			}

			// Verify mode: never dial AWS; report a synthetic plan.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintf(cmd.OutOrStdout(), "verify: would sync AWS %s..%s (%s) into %s\n", start, end, granularity, dbPath)
				return nil
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would sync AWS cost+inventory %s..%s (%s) into %s\n", start, end, granularity, dbPath)
				return nil
			}

			// Under live dogfood, bound the work (single month, no per-instance
			// CloudWatch) so the 30s per-command cap isn't tripped.
			if cliutil.IsDogfoodEnv() {
				start, end, _ = resolveSyncRange("", "", 1)
			}

			sum, err := runAWSSync(cmd.Context(), awsSyncOpts{
				dbPath: dbPath, profile: profile, region: region,
				start: start, end: end, granularity: granularity,
				skipInventory: skipInventory, skipCost: skipCost,
			})
			if err != nil {
				return err
			}

			if flags.asJSON {
				if err := flags.printJSON(cmd, sum); err != nil {
					return err
				}
				return sum.allFailed()
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Synced %s (profile=%s)\n", sum.Period, profileOrDefault(profile))
			fmt.Fprintf(w, "  accounts:   %d\n", sum.Accounts)
			fmt.Fprintf(w, "  cost lines: %d\n", sum.CostLines)
			fmt.Fprintf(w, "  forecasts:  %d\n", sum.Forecasts)
			fmt.Fprintf(w, "  inventory:  %d\n", sum.Inventory)
			for _, warn := range sum.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warn)
			}
			return sum.allFailed()
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: per-user cache)")
	cmd.Flags().StringVar(&profile, "profile-aws", "", "AWS shared-config profile (e.g. prod); empty uses the default chain")
	cmd.Flags().StringVar(&region, "region", "", "AWS region for inventory (default: profile/env region)")
	cmd.Flags().StringVar(&fromStr, "from", "", "Start date YYYY-MM-DD (default: N months ago)")
	cmd.Flags().StringVar(&toStr, "to", "", "End date YYYY-MM-DD, exclusive (default: first of next month)")
	cmd.Flags().StringVar(&granularity, "granularity", "MONTHLY", "Cost granularity: MONTHLY or DAILY")
	cmd.Flags().IntVar(&months, "months", 3, "Months of history to sync when --from/--to are omitted")
	cmd.Flags().BoolVar(&skipInventory, "skip-inventory", false, "Skip the resource inventory / waste scan")
	cmd.Flags().BoolVar(&skipCost, "skip-cost", false, "Skip Cost Explorer (inventory only; works without billing access)")
	cmd.Flags().BoolVar(&full, "full", false, "Full re-sync (re-fetch all periods; cost/inventory sync is always a full refresh)")
	cmd.Flags().StringSliceVar(&resources, "resources", nil, "Limit to specific resources: accounts, costs, forecasts, inventory (default: all)")
	return cmd
}

// runAWSSync performs the actual AWS-to-store sync and returns a summary with
// non-fatal warnings collected along the way. Shared by `sync` and
// `workflow archive`.
func runAWSSync(ctx context.Context, o awsSyncOpts) (syncSummary, error) {
	sum := syncSummary{Period: o.start + " to " + o.end}
	// The granularity Cost Explorer is asked for, and the one the answer is
	// stored under: DAILY or MONTHLY, nothing else.
	if strings.EqualFold(strings.TrimSpace(o.granularity), "DAILY") {
		o.granularity = "DAILY"
	} else {
		o.granularity = "MONTHLY"
	}
	if o.dbPath == "" {
		o.dbPath = defaultDBPath("aws-billing-cli")
	}

	client, err := awsx.New(ctx, o.profile, o.region)
	if err != nil {
		return sum, err
	}
	db, err := store.OpenWithContext(ctx, o.dbPath)
	if err != nil {
		return sum, fmt.Errorf("opening local database: %w", err)
	}
	defer db.Close()

	// 0. The account every row is stored under (aws-billing handfix
	// cost-cache-answers-only-its-request): the account the credentials
	// resolve to. Without it nothing can be stored for the right account, so
	// every layer fails.
	scope, _, serr := client.CallerIdentity(ctx)
	if serr == nil && scope == "" {
		serr = fmt.Errorf("sts:GetCallerIdentity named no account")
	}
	if serr != nil {
		serr = fmt.Errorf("resolving the AWS account of these credentials: %w", serr)
		sum.Warnings = append(sum.Warnings, serr.Error())
		sum.record("accounts", serr)
		if !o.skipCost {
			sum.record("costs", serr)
			sum.record("forecasts", serr)
		}
		if !o.skipInventory {
			sum.record("inventory", serr)
		}
		return sum, nil
	}
	_ = db.RecordScopeLabel(awsReadOpts{profile: o.profile}.profileLabel(), scope) // for --data-source local

	// 1. Accounts (name resolution); access-denied is non-fatal. Only a
	// complete list is stored, in place of the account's stored one.
	nameByID := map[string]string{}
	accts, aerr := client.ListAccounts(ctx)
	if aerr != nil {
		if awsx.IsAccessDenied(aerr) {
			sum.Warnings = append(sum.Warnings, "organizations:ListAccounts denied — account names unavailable (run from the management account or grant the permission); using account IDs")
		} else {
			sum.Warnings = append(sum.Warnings, "list accounts: "+aerr.Error())
		}
	}
	var astoreErr error
	if aerr == nil {
		rows := make([]json.RawMessage, 0, len(accts))
		for _, a := range accts {
			nameByID[a.AccountID] = a.Name
			a.ID = store.AccountRowPrefix(scope) + a.AccountID
			data, merr := json.Marshal(a)
			if merr != nil {
				astoreErr = firstErr(astoreErr, fmt.Errorf("encoding row: %w", merr))
				continue
			}
			rows = append(rows, data)
		}
		if astoreErr == nil {
			if astoreErr = db.ReplaceAccountList(scope, rows); astoreErr == nil {
				sum.Accounts = len(rows)
			}
		}
	}
	sum.record("accounts", aerr, astoreErr)

	// 2. Cost lines: canonical account x service + service x usage-type, each
	// answer stored whole by period (storeCostAnswer).
	if !o.skipCost {
		canon, cerr := client.GetCostAndUsageAnswer(ctx, o.start, o.end, o.granularity, costByAccountService.dims)
		if cerr != nil {
			if awsx.IsAccessDenied(cerr) {
				sum.Warnings = append(sum.Warnings, "ce:GetCostAndUsage denied — no cost data synced (run 'aws-billing-cli iam-setup --tier core' and use a management-account profile)")
			} else {
				sum.Warnings = append(sum.Warnings, "get cost and usage: "+cerr.Error())
			}
		}
		var cstoreErr error
		if cerr == nil {
			for i := range canon.Lines {
				canon.Lines[i].AccountName = nameByID[canon.Lines[i].AccountID]
			}
			if cstoreErr = storeCostAnswer(db, scope, costByAccountService, o.granularity, canon); cstoreErr == nil {
				sum.CostLines += len(canon.Lines)
			}
			// The usage-type detail is secondary: the commands that need it
			// check its coverage and fetch it live, so a failure here is a
			// warning, never silent.
			ut, uerr := client.GetCostAndUsageAnswer(ctx, o.start, o.end, o.granularity, costByServiceUsage.dims)
			if uerr != nil {
				sum.Warnings = append(sum.Warnings, "usage-type cost detail unavailable: "+uerr.Error())
			} else if err := storeCostAnswer(db, scope, costByServiceUsage, o.granularity, ut); err != nil {
				cstoreErr = firstErr(cstoreErr, err)
			} else {
				sum.CostLines += len(ut.Lines)
			}
		}
		sum.record("costs", cerr, cstoreErr)

		// 3. Forecast (best-effort, but its outcome is reported).
		fStart, fEnd := nextMonthRange()
		f, ferr := client.GetCostForecast(ctx, fStart, fEnd, "MONTHLY")
		if ferr == nil {
			f.ID = "fc|" + scope + "|" + f.PeriodStart
			f.AccountID = scope
			ferr = upsertSynced(db.UpsertForecasts, f)
			if ferr == nil {
				sum.Forecasts++
			}
		}
		if ferr != nil {
			sum.Warnings = append(sum.Warnings, "forecast unavailable: "+ferr.Error())
		}
		sum.record("forecasts", ferr)
	}

	// 4. Inventory (waste detection); works in any account.
	if !o.skipInventory {
		inv, ierr := client.CollectInventory(ctx, awsx.InventoryOptions{SkipCPU: cliutil.IsDogfoodEnv()})
		if ierr != nil {
			if awsx.IsAccessDenied(ierr) {
				sum.Warnings = append(sum.Warnings, "resource inventory partially denied — grant ec2:Describe*/cloudwatch:GetMetricStatistics for waste detection")
			} else {
				sum.Warnings = append(sum.Warnings, "inventory: "+ierr.Error())
			}
		}
		n, istoreErr := storeInventoryScan(ctx, db, scope, client.Region, inv, ierr)
		sum.Inventory += n
		sum.record("inventory", ierr, istoreErr)
	}
	return sum, nil
}

// storeInventoryScan stores every row of an inventory scan of scope and
// region and records the scan's outcome: complete only when the scan had no
// failure, every row stored and the prune succeeded. A complete scan prunes
// the rows of the scanned account and region it did not return
// (inventory-scan-replaces-scope). It returns how many rows it stored.
func storeInventoryScan(ctx context.Context, db *store.Store, scope, region string, inv []awsx.InventoryResource, scanErr error) (int, error) {
	seen := map[string]bool{}
	rows := make([]json.RawMessage, 0, len(inv))
	for _, r := range inv {
		r.ID = r.ResourceType + "|" + r.ResourceID
		seen[r.ID] = true
		if r.AccountID != scope {
			err := fmt.Errorf("an inventory row of account %q in a scan of account %s", r.AccountID, scope)
			_ = db.RecordInventoryScan(scope, region, false, err.Error())
			return 0, err
		}
		data, err := json.Marshal(r)
		if err != nil {
			_ = db.RecordInventoryScan(scope, region, false, "encoding row: "+err.Error())
			return 0, fmt.Errorf("encoding row: %w", err)
		}
		rows = append(rows, data)
	}
	// One transaction (Codex r2 class cache-answer-not-the-request): the
	// rows, the prune of a complete scan and the scan's record together.
	return db.ReplaceInventoryScan(scope, region, rows, seen, scanErr == nil, inventoryScanDetail(scanErr, nil))
}

// upsertSynced stores one synced row; a marshal or store failure is returned,
// never dropped.
func upsertSynced(upsert func(json.RawMessage) error, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding row: %w", err)
	}
	if err := upsert(data); err != nil {
		return fmt.Errorf("storing row: %w", err)
	}
	return nil
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

func profileOrDefault(p string) string {
	if p == "" {
		return "default"
	}
	return p
}

// resolveSyncRange computes the [start,end) date strings (YYYY-MM-DD) for a
// sync. Explicit from/to win; otherwise it spans the last `months` calendar
// months through the first of next month (so the current month-to-date is
// included).
func resolveSyncRange(fromStr, toStr string, months int) (string, string, error) {
	const layout = "2006-01-02"
	if fromStr != "" || toStr != "" {
		if fromStr == "" || toStr == "" {
			return "", "", fmt.Errorf("--from and --to must be given together")
		}
		if _, err := time.Parse(layout, fromStr); err != nil {
			return "", "", fmt.Errorf("invalid --from %q (want YYYY-MM-DD)", fromStr)
		}
		if _, err := time.Parse(layout, toStr); err != nil {
			return "", "", fmt.Errorf("invalid --to %q (want YYYY-MM-DD)", toStr)
		}
		return fromStr, toStr, nil
	}
	now := time.Now().UTC()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	start := firstOfThisMonth.AddDate(0, -(months - 1), 0)
	end := firstOfThisMonth.AddDate(0, 1, 0)
	return start.Format(layout), end.Format(layout), nil
}

// nextMonthRange returns [first-of-next-month, first-of-month-after) for the
// forecast window.
func nextMonthRange() (string, string) {
	const layout = "2006-01-02"
	now := time.Now().UTC()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	start := firstOfThisMonth.AddDate(0, 1, 0)
	end := firstOfThisMonth.AddDate(0, 2, 0)
	return start.Format(layout), end.Format(layout)
}

// upsertSingleObject stores a non-array response as a single record, routing to
// the typed store tables when the resource matches one. Retained for the
// generated sync test and any single-object store path.
func upsertSingleObject(db *store.Store, resource string, data json.RawMessage) error {
	obj, err := store.DecodeJSONObject(data)
	if err != nil {
		return db.Upsert(resource, resource, data)
	}
	switch resource {
	case "accounts":
		return db.UpsertAccounts(data)
	case "costs":
		return db.UpsertCosts(data)
	case "forecasts":
		return db.UpsertForecasts(data)
	case "inventory":
		return db.UpsertInventory(data)
	}
	id := syncObjectID(obj)
	if id == "" {
		id = resource
	}
	return db.Upsert(resource, id, data)
}

// syncObjectID extracts a stable id from a decoded object, mirroring the
// store's own fallback order.
func syncObjectID(obj map[string]any) string {
	for _, key := range []string{"id", "ID", "uuid", "guid", "name", "slug", "key"} {
		if v := store.LookupFieldValue(obj, key); v != nil {
			s := store.ResourceIDString(v)
			if s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}
