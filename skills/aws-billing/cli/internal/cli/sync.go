// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
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
	Accounts  int      `json:"accounts"`
	CostLines int      `json:"cost_lines"`
	Forecasts int      `json:"forecasts"`
	Inventory int      `json:"inventory"`
	Warnings  []string `json:"warnings,omitempty"`
	Period    string   `json:"period"`
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
account. Credentials resolve from --profile-aws, the environment, SSO, or instance metadata.`,
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
				return flags.printJSON(cmd, sum)
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
			return nil
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
	if o.granularity == "" {
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

	// 1. Accounts (name resolution); access-denied is non-fatal.
	nameByID := map[string]string{}
	accts, aerr := client.ListAccounts(ctx)
	if aerr != nil {
		if awsx.IsAccessDenied(aerr) {
			sum.Warnings = append(sum.Warnings, "organizations:ListAccounts denied — account names unavailable (run from the management account or grant the permission); using account IDs")
		} else {
			sum.Warnings = append(sum.Warnings, "list accounts: "+aerr.Error())
		}
	}
	for _, a := range accts {
		a.ID = a.AccountID
		nameByID[a.AccountID] = a.Name
		data, _ := json.Marshal(a)
		if err := db.UpsertAccounts(data); err == nil {
			sum.Accounts++
		}
	}

	// 2. Cost lines: canonical account x service + service x usage-type.
	if !o.skipCost {
		canon, cerr := client.GetCostAndUsageGrouped(ctx, o.start, o.end, o.granularity, []string{"LINKED_ACCOUNT", "SERVICE"})
		if cerr != nil {
			if awsx.IsAccessDenied(cerr) {
				sum.Warnings = append(sum.Warnings, "ce:GetCostAndUsage denied — no cost data synced (run 'aws-billing-cli iam-setup --tier core' and use a management-account profile)")
			} else {
				sum.Warnings = append(sum.Warnings, "get cost and usage: "+cerr.Error())
			}
		}
		for _, l := range canon {
			l.AccountName = nameByID[l.AccountID]
			l.ID = fmt.Sprintf("as|%s|%s|%s|%s", l.Granularity, l.PeriodStart, l.AccountID, l.Service)
			data, _ := json.Marshal(l)
			if err := db.UpsertCosts(data); err == nil {
				sum.CostLines++
			}
		}
		if cerr == nil {
			if ut, uerr := client.GetCostAndUsageGrouped(ctx, o.start, o.end, o.granularity, []string{"SERVICE", "USAGE_TYPE"}); uerr == nil {
				for _, l := range ut {
					l.ID = fmt.Sprintf("ut|%s|%s|%s|%s", l.Granularity, l.PeriodStart, l.Service, l.UsageType)
					data, _ := json.Marshal(l)
					if err := db.UpsertCosts(data); err == nil {
						sum.CostLines++
					}
				}
			}
		}

		// 3. Forecast (best-effort).
		fStart, fEnd := nextMonthRange()
		if f, ferr := client.GetCostForecast(ctx, fStart, fEnd, "MONTHLY"); ferr == nil {
			f.ID = "fc|" + f.PeriodStart
			data, _ := json.Marshal(f)
			if err := db.UpsertForecasts(data); err == nil {
				sum.Forecasts++
			}
		} else if !awsx.IsAccessDenied(ferr) {
			sum.Warnings = append(sum.Warnings, "forecast unavailable: "+ferr.Error())
		}
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
		for _, r := range inv {
			r.ID = r.ResourceType + "|" + r.ResourceID
			data, _ := json.Marshal(r)
			if err := db.UpsertInventory(data); err == nil {
				sum.Inventory++
			}
		}
	}
	return sum, nil
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
