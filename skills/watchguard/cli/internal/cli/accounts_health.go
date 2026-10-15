// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"watchguard-pp-cli/internal/client"
	"watchguard-pp-cli/internal/cliutil"
)

type healthRow struct {
	AccountID        string         `json:"account_id"`
	AccountName      string         `json:"account_name,omitempty"`
	FireboxesTotal   int            `json:"fireboxes_total"`
	FireboxesOffline int            `json:"fireboxes_offline"`
	EndpointsTotal   int            `json:"endpoints_total"`
	EndpointsAtRisk  int            `json:"endpoints_at_risk"`
	IncidentsOpen    map[string]int `json:"incidents_open"`
	LicensesExpiring int            `json:"licenses_expiring"`
	Truncated        bool           `json:"truncated"`
	Errors           []string       `json:"errors,omitempty"`
}

// score ranks rows for attention: offline Fireboxes, at-risk endpoints, expiring
// licenses, and open incidents weighted by severity.
func (r healthRow) score() int {
	n := r.FireboxesOffline + r.EndpointsAtRisk + r.LicensesExpiring
	for sev, c := range r.IncidentsOpen {
		n += c * (1 + sevRank(sev))
	}
	return n
}

type healthView struct {
	Accounts          []healthRow    `json:"accounts"`
	IncidentsError    string         `json:"incidents_error,omitempty"`
	IncidentsTruncate bool           `json:"incidents_truncated"`
	LicensesError     string         `json:"licenses_error,omitempty"`
	LicensesTruncated bool           `json:"licenses_truncated"`
	FetchFailures     []fetchFailure `json:"fetch_failures,omitempty"`
	Note              string         `json:"note,omitempty"`
}

// openIncidentsByAccount counts new/read incidents per account and severity.
func openIncidentsByAccount(items []map[string]any) map[string]map[string]int {
	out := map[string]map[string]int{}
	for _, m := range items {
		if s := strings.ToLower(strField(m, "status")); s != "new" && s != "read" {
			continue
		}
		acct := strField(m, "account")
		if out[acct] == nil {
			out[acct] = map[string]int{}
		}
		out[acct][strings.ToLower(strField(m, "severity"))]++
	}
	return out
}

// fleetCounts returns total and offline/never-connected device counts.
func fleetCounts(devices []map[string]any) (total, offline int) {
	for _, d := range devices {
		total++
		if isOffline(strField(d, "state")) {
			offline++
		}
	}
	return total, offline
}

// accountHealth reads Fireboxes and endpoints for one account. A failure to reach
// the account at all (audience or token) fails the account; a product the
// account does not use shows up in the row's errors.
func accountHealth(ctx context.Context, c *client.Client, s subscriber, maxPages int) (healthRow, error) {
	row := healthRow{AccountID: s.ID, AccountName: s.Name, IncidentsOpen: map[string]int{}}
	if _, err := c.TenantHeaders(ctx, s.ID); err != nil {
		return row, client.ScrubScoped(err)
	}
	devData, err := tenantGet(ctx, c, s.ID, "/rest/firebox/management/v1/info/"+url.PathEscape(s.ID)+"/devices", map[string]string{"device_type": "FB"})
	if err != nil {
		row.Errors = append(row.Errors, "firebox: "+err.Error())
	} else {
		row.FireboxesTotal, row.FireboxesOffline = fleetCounts(asMaps(devData, "data"))
	}
	prot, trunc, err := pageOData(ctx, c, s.ID, "/rest/endpoint-security/management/api/v1/accounts/"+url.PathEscape(s.ID)+"/devicesprotectionstatus", maxPages)
	if err != nil {
		row.Errors = append(row.Errors, "endpoint: "+err.Error())
	} else {
		row.EndpointsTotal, row.Truncated = len(prot), trunc
		for _, m := range prot {
			if len(protectionIssues(m)) > 0 {
				row.EndpointsAtRisk++
			}
		}
	}
	return row, nil
}

func newNovelAccountsHealthCmd(flags *rootFlags) *cobra.Command {
	var tf tenantFlags
	var duration, expiring, licenseTypes string
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Summarize each subscriber's health in one row: Fireboxes offline, endpoints at risk, open incidents by severity and expiring licenses",
		Long: "Use this command for one row per subscriber account with cross-product counts. " +
			"Do NOT use it to list individual endpoints; use 'endpoint-devices posture' instead. " +
			"Do NOT use it to list individual incidents; use 'threatsync-incidents queue' instead. " +
			"Do NOT use it for license line items or serial matching; use 'allocations reconcile' instead.\n\n" +
			"Built for a WatchGuard Service Provider account. Fireboxes (Access Points excluded) and endpoints are read per account with audience-scoped tokens; " +
			"a product the account does not have shows in that row's errors. Incidents come from one ThreatSync call across tenants and license expiry from the " +
			"Service Provider's allocation lists; failures of those two show at the top level (incidents_error, licenses_error). " +
			"Rows are sorted by attention score: offline Fireboxes + at-risk endpoints + expiring licenses + open incidents weighted by severity.\n\n" +
			"Exit codes: 0 all accounts reached, 5 none reached, 6 some could not be reached (--allow-partial-failure exits 0).",
		Example: strings.Trim(`
  watchguard-cli accounts health --agent
  watchguard-cli accounts health --expiring-within 60d --csv
  watchguard-cli accounts health --accounts ACC-1234567 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "accounts health")
			}
			window, err := cliutil.ParseDurationLoose(expiring)
			if err != nil || window < 0 {
				return usageErr(fmt.Errorf("--expiring-within: %q is not a duration like 30d", expiring))
			}
			dur, err := incidentDuration(duration)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			subs, accTrunc, err := resolveSubscribers(ctx, c, tf)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			own, _ := ownAccount(c)
			view := healthView{Accounts: make([]healthRow, 0)}

			incidents := map[string]map[string]int{}
			if items, trunc, err := fetchIncidents(ctx, c, own, dur, true, tf.pages()); err != nil {
				view.IncidentsError = err.Error()
			} else {
				incidents, view.IncidentsTruncate = openIncidentsByAccount(items), trunc
			}
			expiringBy := map[string]int{}
			now := time.Now()
			var licErrs []string
			for _, rt := range strings.Split(licenseTypes, ",") {
				if rt = strings.TrimSpace(rt); rt == "" {
					continue
				}
				rows, trunc, err := pageLimitOffset(ctx, c, "/rest/platform/allocation/v2/"+url.PathEscape(own)+"/assets/allocations/"+url.PathEscape(rt),
					map[string]string{"accountType": "2"}, nil, "data", 100, tf.pages())
				if err != nil {
					licErrs = append(licErrs, rt+": "+err.Error())
					continue
				}
				view.LicensesTruncated = view.LicensesTruncated || trunc
				for _, e := range expiringWithin(rt, rows, window, now) {
					expiringBy[e.AccountID]++
				}
			}
			view.LicensesError = strings.Join(licErrs, "; ")

			results, errs := cliutil.FanoutRun(ctx, subs, func(s subscriber) string { return s.ID },
				func(ctx context.Context, s subscriber) (healthRow, error) {
					return accountHealth(ctx, c, s, tf.pages())
				}, cliutil.WithConcurrency(tf.workers()))
			view.FetchFailures = fanoutFailures(errs)
			for _, r := range results {
				row := r.Value
				if inc := incidents[row.AccountID]; inc != nil {
					row.IncidentsOpen = inc
				}
				row.LicensesExpiring = expiringBy[row.AccountID]
				view.Accounts = append(view.Accounts, row)
			}
			sort.SliceStable(view.Accounts, func(i, j int) bool { return view.Accounts[i].score() > view.Accounts[j].score() })
			view.Note = accountsTruncatedNote(cmd.ErrOrStderr(), accTrunc)
			if err := renderHealth(cmd, flags, view); err != nil {
				return err
			}
			return fanoutVerdict(cmd.ErrOrStderr(), flags, len(errs), len(subs))
		},
	}
	tf.register(cmd.Flags())
	cmd.Flags().StringVar(&duration, "duration", "30D", "Days of ThreatSync incidents to count, a number followed by D")
	cmd.Flags().StringVar(&expiring, "expiring-within", "30d", "Count allocations that expire within this window (already-expired ones included)")
	cmd.Flags().StringVar(&licenseTypes, "license-types", "firebox,endPoint,authPoint,threatSync,ndr,fireCloud,accessPoint", "Allocation resource types checked for expiry")
	return cmd
}

func renderHealth(cmd *cobra.Command, flags *rootFlags, view healthView) error {
	w := cmd.ErrOrStderr()
	if view.IncidentsError != "" {
		fmt.Fprintln(w, "warning: incident counts unavailable (shown as 0): "+view.IncidentsError)
	}
	if view.LicensesError != "" {
		fmt.Fprintln(w, "warning: some license expiry counts unavailable: "+view.LicensesError)
	}
	if view.IncidentsTruncate || view.LicensesTruncated {
		fmt.Fprintln(w, "warning: --max-pages cut incident or license lists short; counts may be low")
	}
	for _, r := range view.Accounts {
		if r.Truncated {
			fmt.Fprintf(w, "warning: --max-pages cut the endpoint list short for %s; endpoint counts may be low\n", subscriber{ID: r.AccountID, Name: r.AccountName}.label())
		}
	}
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), view, flags)
	}
	if len(view.Accounts) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No accounts to report.")
		return nil
	}
	tw := newTabWriter(cmd.OutOrStdout())
	fmt.Fprintln(tw, "ACCOUNT\tFIREBOX OFF/TOTAL\tENDPOINT RISK/TOTAL\tINCIDENTS C/H/M/L\tEXPIRING\tERRORS")
	for _, r := range view.Accounts {
		i := r.IncidentsOpen
		fmt.Fprintf(tw, "%s\t%d/%d\t%d/%d\t%d/%d/%d/%d\t%d\t%d\n", subscriber{ID: r.AccountID, Name: r.AccountName}.label(), r.FireboxesOffline, r.FireboxesTotal,
			r.EndpointsAtRisk, r.EndpointsTotal, i["critical"], i["high"], i["medium"], i["low"], r.LicensesExpiring, len(r.Errors))
	}
	return tw.Flush()
}
