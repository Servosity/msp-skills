// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: lifecycle leftovers across every organization.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/cliutil"
)

type hygieneRow struct {
	OrgID    string   `json:"org_id"`
	OrgEmail string   `json:"org_email"`
	Kind     string   `json:"kind"`
	Email    string   `json:"email"`
	Since    string   `json:"since"`
	AgeDays  *float64 `json:"age_days"`
	Storage  int64    `json:"storage"`
	Detail   string   `json:"detail,omitempty"`
}

type hygieneOrg struct {
	OrgID        string `json:"org_id"`
	OrgEmail     string `json:"org_email"`
	Deactivated  int    `json:"deactivated"`
	Deleted      int    `json:"deleted"`
	StaleInvites int    `json:"stale_invites"`
	StorageHeld  int64  `json:"storage_held"`
}

type hygieneReport struct {
	GeneratedAt   string         `json:"generated_at"`
	OlderThan     string         `json:"older_than"`
	Orgs          []hygieneOrg   `json:"orgs"`
	Rows          []hygieneRow   `json:"rows"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
}

func daysPtr(t time.Time, now time.Time) *float64 {
	if t.IsZero() {
		return nil
	}
	v := float64(int64(now.Sub(t).Hours()/24*10)) / 10
	return &v
}

// hygieneForOrg classifies one organization's deactivated, deleted and
// invited mailboxes. Deactivated/deleted rows qualify when they still hold
// storage and (when a date is known) are older than olderThan; invites
// qualify when expired or last sent before the cutoff.
func hygieneForOrg(deactivated, deleted, invites []map[string]any, o dsOrg, now time.Time, olderThan time.Duration) ([]hygieneRow, hygieneOrg) {
	rows := make([]hygieneRow, 0)
	sum := hygieneOrg{OrgID: o.ID, OrgEmail: o.Email}
	cutoff := now.Add(-olderThan)
	for _, m := range deactivated {
		since, ok := timeField(m, "deactivated_since")
		storage := num(m, "storage")
		if storage <= 0 || (ok && since.After(cutoff)) {
			continue
		}
		rows = append(rows, hygieneRow{OrgID: o.ID, OrgEmail: o.Email, Kind: "deactivated", Email: str(m, "email"), Since: formatTime(since), AgeDays: daysPtr(since, now), Storage: storage})
		sum.Deactivated++
		sum.StorageHeld += storage
	}
	for _, m := range deleted {
		storage := num(m, "storage")
		if storage <= 0 {
			continue
		}
		since, _ := timeField(m, "deactivated_since")
		rows = append(rows, hygieneRow{OrgID: o.ID, OrgEmail: o.Email, Kind: "deleted", Email: str(m, "email"), Since: formatTime(since), AgeDays: daysPtr(since, now), Storage: storage, Detail: "flg_deleted with storage still held"})
		sum.Deleted++
		sum.StorageHeld += storage
	}
	for _, m := range invites {
		sent, ok := timeField(m, "last_sent_at")
		expire := str(m, "expire")
		expired := strings.EqualFold(expire, "expired")
		if !expired && (!ok || sent.After(cutoff)) {
			continue
		}
		detail := "requested by " + str(m, "requester_email")
		if expire != "" {
			detail += "; " + expire
		}
		rows = append(rows, hygieneRow{OrgID: o.ID, OrgEmail: o.Email, Kind: "invite", Email: str(m, "email"), Since: formatTime(sent), AgeDays: daysPtr(sent, now), Detail: detail})
		sum.StaleInvites++
	}
	return rows, sum
}

func newNovelHygieneCmd(flags *rootFlags) *cobra.Command {
	var olderThanRaw string
	var includeInactive bool
	var concurrency, limit int
	var sweepTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "hygiene",
		Short: "Find deactivated or deleted mailboxes still holding storage and invitations nobody accepted, across every client",
		Long: `Use this command for lifecycle leftovers: deactivated or deleted mailboxes still holding storage and invites never accepted. Do NOT use this command for seat counts or invoice reconciliation; use 'billing' instead. Do NOT use it for connection failures or failing backups; use 'health' instead.

For every organization it reads /accounts/deactivated_accounts,
/accounts/deleted_accounts and /accounts/invites with that organization's own
token. Deactivated mailboxes are listed when they still hold storage and were
deactivated more than --older-than ago; deleted mailboxes when they still
hold storage; invites when expired or last sent more than --older-than ago.
The storage unit is whatever the vendor reports (undocumented).`,
		Example: strings.Trim(`
  dropsuite-cli hygiene --older-than 30d --agent
  dropsuite-cli hygiene --org 26561-14 --older-than 90d`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := rejectArgs(args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "hygiene")
			}
			olderThan, err := parseDurationFlag("older-than", olderThanRaw, "30d")
			if err != nil {
				return err
			}
			ctx, cancel := sweepCtx(cmd, sweepTimeout)
			defer cancel()
			admin, err := adminClient(flags)
			if err != nil {
				return err
			}
			orgs, err := fetchOrgs(ctx, admin)
			if err != nil {
				return err
			}
			picked, _, err := selectOrgs(orgs, orgSelector, includeInactive)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			type orgResult struct {
				rows  []hygieneRow
				sum   hygieneOrg
				fails []fetchFailure
			}
			results, errs := cliutil.FanoutRun(ctx, picked,
				func(o dsOrg) string { return o.ID },
				func(ctx context.Context, o dsOrg) (orgResult, error) {
					c, err := orgClient(flags, o)
					if err != nil {
						return orgResult{}, err
					}
					fails := make([]fetchFailure, 0)
					get := func(path string, mode listMode) []map[string]any {
						items, err := fetchList(ctx, c, path, nil, mode, 1)
						if err != nil {
							fails = append(fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: path, Error: errText(err)})
						}
						return items
					}
					rows, sum := hygieneForOrg(get("/accounts/deactivated_accounts", pagedSingle), get("/accounts/deleted_accounts", pagedSingle), get("/accounts/invites", pagedObjectOrList), o, now, olderThan)
					return orgResult{rows: rows, sum: sum, fails: fails}, nil
				},
				cliutil.WithConcurrency(concurrency),
			)
			rep := hygieneReport{
				GeneratedAt:   now.Format(time.RFC3339),
				OlderThan:     fmtDuration(olderThan),
				Orgs:          make([]hygieneOrg, 0, len(results)),
				Rows:          make([]hygieneRow, 0),
				FetchFailures: make([]fetchFailure, 0),
			}
			for _, r := range results {
				rep.Orgs = append(rep.Orgs, r.Value.sum)
				rep.Rows = append(rep.Rows, r.Value.rows...)
				rep.FetchFailures = append(rep.FetchFailures, r.Value.fails...)
			}
			for _, e := range errs {
				rep.FetchFailures = append(rep.FetchFailures, fetchFailure{OrgID: e.Source, Scope: "organization", Error: errText(e.Err)})
			}
			sort.SliceStable(rep.Rows, func(i, j int) bool { return rep.Rows[i].Storage > rep.Rows[j].Storage })
			if limit > 0 && len(rep.Rows) > limit {
				rep.Rows = rep.Rows[:limit]
			}
			if len(rep.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d read(s) failed; see fetch_failures (those lists are incomplete)\n", len(rep.FetchFailures))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
			}
			if len(rep.Rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No lifecycle leftovers found.")
				return nil
			}
			rows := make([][]string, 0, len(rep.Rows))
			for _, r := range rep.Rows {
				age := "-"
				if r.AgeDays != nil {
					age = strconv.FormatFloat(*r.AgeDays, 'f', 1, 64)
				}
				rows = append(rows, []string{r.Kind, r.OrgEmail, r.Email, age, strconv.FormatInt(r.Storage, 10), r.Detail})
			}
			return flags.printTable(cmd, []string{"KIND", "ORG", "EMAIL", "AGE DAYS", "STORAGE", "DETAIL"}, rows)
		},
	}
	cmd.Flags().StringVar(&olderThanRaw, "older-than", "30d", "Only report deactivations and invites older than this (e.g. 30d, 90d)")
	cmd.Flags().BoolVar(&includeInactive, "include-inactive", false, "Also check organizations whose customer is deactivated or suspended")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Organizations read in parallel")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	cmd.Flags().DurationVar(&sweepTimeout, "sweep-timeout", defaultSweepTimeout, "Budget for the whole run across all organizations (each request still uses --timeout)")
	return cmd
}
