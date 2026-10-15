// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"cisco-duo-pp-cli/internal/client"
	"cisco-duo-pp-cli/internal/cliutil"
)

type fleetFailure struct {
	AccountID string `json:"account_id"`
	Name      string `json:"name"`
	Error     string `json:"error"`
}

type fleetSummaryRow struct {
	AccountID        string   `json:"account_id"`
	Name             string   `json:"name"`
	APIHostname      string   `json:"api_hostname"`
	Edition          string   `json:"edition"`
	UserCount        *int     `json:"user_count"`
	UserLimit        *int     `json:"user_limit"`
	UserLimitPct     *float64 `json:"user_limit_pct"`
	TelephonyCredits *int     `json:"telephony_credits"`
	AdminCount       *int     `json:"admin_count"`
	IntegrationCount *int     `json:"integration_count"`
	PendingDeletion  *int     `json:"users_pending_deletion"`
	Partial          []string `json:"partial,omitempty"`
}

type fleetSummaryView struct {
	Accounts      int               `json:"accounts"`
	Rows          []fleetSummaryRow `json:"rows"`
	FetchFailures []fleetFailure    `json:"fetch_failures,omitempty"`
	Note          string            `json:"note,omitempty"`
}

// parseUserLimit reads billing/user_limit, where user_limit is an integer or
// the string "Not set".
func parseUserLimit(raw json.RawMessage) (count, limit *int) {
	var probe map[string]json.RawMessage
	if json.Unmarshal(raw, &probe) != nil {
		return nil, nil
	}
	if n, ok := cliutil.ExtractInt(probe, "current_users_count"); ok {
		count = intPtr(int(n))
	}
	if n, ok := cliutil.ExtractInt(probe, "user_limit"); ok && n > 0 { // "Not set" = no limit
		limit = intPtr(int(n))
	}
	return count, limit
}

func intPtr(n int) *int { return &n }

func fetchSummaryRow(ctx context.Context, c *client.Client, a client.DuoSubaccount) (fleetSummaryRow, error) {
	row := fleetSummaryRow{AccountID: a.AccountID, Name: a.Name, APIHostname: a.APIHostname}
	raw, _, err := duoGet(ctx, c, a.AccountID, "/admin/v1/info/summary", nil)
	if err != nil {
		return row, err // the core row; billing extras degrade to partial
	}
	var sum struct {
		AdminCount       *int   `json:"admin_count"`
		IntegrationCount *int   `json:"integration_count"`
		Credits          *int   `json:"telephony_credits_remaining"`
		UserCount        *int   `json:"user_count"`
		Pending          *int   `json:"user_pending_deletion_count"`
		Edition          string `json:"edition"`
	}
	if err := json.Unmarshal(raw, &sum); err != nil {
		return row, fmt.Errorf("parsing summary: %w", err)
	}
	row.AdminCount, row.IntegrationCount, row.TelephonyCredits = sum.AdminCount, sum.IntegrationCount, sum.Credits
	row.UserCount, row.PendingDeletion, row.Edition = sum.UserCount, sum.Pending, sum.Edition
	if raw, _, err := duoGet(ctx, c, a.AccountID, "/admin/v1/billing/edition", nil); err == nil {
		var e struct {
			Edition string `json:"edition"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Edition != "" {
			row.Edition = e.Edition
		}
	} else {
		row.Partial = append(row.Partial, "edition: "+firstLine(err))
	}
	if raw, _, err := duoGet(ctx, c, a.AccountID, "/admin/v1/billing/user_limit", nil); err == nil {
		count, limit := parseUserLimit(raw)
		if count != nil {
			row.UserCount = count
		}
		row.UserLimit = limit
		if limit != nil && row.UserCount != nil && *limit > 0 {
			pct := float64(*row.UserCount) * 100 / float64(*limit)
			pct = float64(int(pct*10+0.5)) / 10
			row.UserLimitPct = &pct
		}
	} else {
		row.Partial = append(row.Partial, "user_limit: "+firstLine(err))
	}
	if raw, _, err := duoGet(ctx, c, a.AccountID, "/admin/v1/billing/telephony_credits", nil); err == nil {
		var tc struct {
			Credits *int `json:"credits"`
		}
		if json.Unmarshal(raw, &tc) == nil && tc.Credits != nil {
			row.TelephonyCredits = tc.Credits
		}
	} else {
		row.Partial = append(row.Partial, "telephony_credits: "+firstLine(err))
	}
	return row, nil
}

// fanoutAccounts runs fn per subaccount (4 at a time) and splits successes
// from failures; failed accounts never appear as zero-valued rows.
func fanoutAccounts[T any](ctx context.Context, accounts []client.DuoSubaccount, fn func(context.Context, client.DuoSubaccount) (T, error)) ([]T, []fleetFailure) {
	results, errs := cliutil.FanoutRun(ctx, accounts, func(a client.DuoSubaccount) string { return a.AccountID }, fn, cliutil.WithConcurrency(4))
	ok := make([]T, 0, len(results))
	for _, r := range results {
		ok = append(ok, r.Value)
	}
	byID := map[string]string{}
	for _, a := range accounts {
		byID[a.AccountID] = a.Name
	}
	failures := make([]fleetFailure, 0, len(errs))
	for _, e := range errs {
		failures = append(failures, fleetFailure{AccountID: e.Source, Name: byID[e.Source], Error: firstLine(e.Err)})
	}
	return ok, failures
}

// allFleetFailed turns "every account fetch failed" into a non-zero exit so
// automation never mistakes an empty fleet report for a clean one.
func allFleetFailed(failed, attempted int) error {
	if attempted == 0 || failed < attempted {
		return nil
	}
	return apiErr(fmt.Errorf("all %d account fetches failed; see fetch_failures", failed))
}

// fleetAccounts lists the parent's subaccounts selected by --only, capped
// under live dogfood.
func fleetAccounts(ctx context.Context, c *client.Client, only string) ([]client.DuoSubaccount, error) {
	accounts, err := duoListAccounts(ctx, c)
	if err != nil {
		return nil, err
	}
	accounts, err = selectFleetAccounts(accounts, only)
	if err != nil {
		return nil, err
	}
	if cliutil.IsDogfoodEnv() && len(accounts) > 3 {
		accounts = accounts[:3]
	}
	return accounts, nil
}

// selectFleetAccounts applies --only and refuses (exit 2) any selector that
// matched no subaccount, so a misspelled customer is reported up front rather
// than silently missing from an otherwise successful fleet report.
func selectFleetAccounts(accounts []client.DuoSubaccount, only string) ([]client.DuoSubaccount, error) {
	selected, unmatched := duoSelectAccounts(accounts, only)
	if len(unmatched) > 0 {
		quoted := make([]string, len(unmatched))
		for i, u := range unmatched {
			quoted[i] = strconv.Quote(u)
		}
		return nil, usageErr(fmt.Errorf("--only matched no subaccount for %s (match an exact account_id or name, case-insensitive; 'accounts list' shows both)", strings.Join(quoted, ", ")))
	}
	return selected, nil
}

// rowMarkerKeepKeys are row keys that say a row is incomplete or flagged:
// fleet risk undecoded_rows, fleet summary partial, failures flag. Each exists
// only on the rows it describes, so the --agent/--compact 80%-frequency rule
// would drop it and an agent would read a partial or flagged row as clean.
var rowMarkerKeepKeys = []string{"undecoded_rows", "partial", "flag"}

// printMarkedRowsJSON prints a fleet or failures view keeping rowMarkerKeepKeys
// under --agent/--compact.
func printMarkedRowsJSON(w io.Writer, view any, flags *rootFlags) error {
	return printJSONFilteredKeep(w, view, flags, rowMarkerKeepKeys...)
}

// fleetNote explains an ignored --account-id pin or an empty fleet.
func fleetNote(pinned string, accounts int) string {
	switch {
	case pinned != "":
		return "--account-id/" + client.DuoAccountEnv + " is ignored here; fleet commands cover every subaccount"
	case accounts == 0:
		return "no subaccounts found (is this an MSP parent account?)"
	}
	return ""
}

func printFleetFailures(w io.Writer, failures []fleetFailure) {
	for _, f := range failures {
		fmt.Fprintf(w, "FAILED %s (%s): %s\n", f.Name, f.AccountID, f.Error)
	}
}

func warnFleetFailures(cmd *cobra.Command, failures []fleetFailure, total int) {
	if len(failures) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d subaccounts failed; results cover the remaining %d\n", len(failures), total, total-len(failures))
	}
}

func newNovelFleetSummaryCmd(flags *rootFlags) *cobra.Command {
	var only string
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "One row per subaccount: edition, users vs user limit, telephony credits, admins",
		Long: strings.Trim(`
Use this command for seats, user-limit pressure, edition and telephony credits
per customer subaccount. It lists the MSP parent's subaccounts, then calls
info/summary and the billing endpoints against each subaccount's own API host.
Accounts that fail are listed under fetch_failures and never counted as zeros.

Do NOT use it for which customers have users in bypass, locked out or
unenrolled; use 'fleet risk' instead.

Requires an Admin API application in the MSP parent account with subaccount
permissions ("Grant accounts - Read", "Grant read information").`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli fleet summary --agent
  cisco-duo-cli fleet summary --only "Acme Corp,DA9VZOC5X63I2W72NRP9"`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet summary")
			}
			pinned := duoFleetPrep()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			accounts, err := fleetAccounts(ctx, c, only)
			if err != nil {
				return err
			}
			rows, failures := fanoutAccounts(ctx, accounts, func(ctx context.Context, a client.DuoSubaccount) (fleetSummaryRow, error) {
				return fetchSummaryRow(ctx, c, a)
			})
			sort.Slice(rows, func(i, j int) bool {
				pi, pj := -1.0, -1.0
				if rows[i].UserLimitPct != nil {
					pi = *rows[i].UserLimitPct
				}
				if rows[j].UserLimitPct != nil {
					pj = *rows[j].UserLimitPct
				}
				if pi != pj {
					return pi > pj
				}
				return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
			})
			view := fleetSummaryView{Accounts: len(accounts), Rows: rows, FetchFailures: failures}
			view.Note = fleetNote(pinned, len(accounts))
			warnFleetFailures(cmd, failures, len(accounts))
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printMarkedRowsJSON(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
				return allFleetFailed(len(failures), len(accounts))
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-28s %-10s %8s %8s %6s %8s %6s\n", "SUBACCOUNT", "EDITION", "USERS", "LIMIT", "PCT", "CREDITS", "ADMINS")
			for _, r := range rows {
				fmt.Fprintf(w, "%-28s %-10s %8s %8s %6s %8s %6s\n", trunc(r.Name, 28), r.Edition, intStr(r.UserCount), intStr(r.UserLimit),
					pctStr(r.UserLimitPct), intStr(r.TelephonyCredits), intStr(r.AdminCount))
			}
			printFleetFailures(w, failures)
			if view.Note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), view.Note)
			}
			return allFleetFailed(len(failures), len(accounts))
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "Comma-separated subaccount IDs or names to include")
	return cmd
}

func intStr(p *int) string {
	if p == nil {
		return "-"
	}
	return strconv.Itoa(*p)
}

func pctStr(p *float64) string {
	if p == nil {
		return "-"
	}
	return strconv.FormatFloat(*p, 'f', 1, 64)
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "~"
}
