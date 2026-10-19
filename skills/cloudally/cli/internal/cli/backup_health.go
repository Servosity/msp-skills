// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"github.com/spf13/cobra"
)

const (
	defaultStaleDays   = 2
	defaultHealthLimit = 500
	// defaultHealthMaxAge is the sync-staleness threshold when the root
	// flag carries none.
	defaultHealthMaxAge = 24 * time.Hour

	healthResourceDistributor = "distributor_accounts-backup-statuses"
	healthResourceReseller    = "reseller_accounts-backup-statuses"

	healthUnknownDate = "UNKNOWN_DATE"
	healthStateOK     = "OK"
	healthStateStale  = "STALE"
	healthStateFail   = "FAILING"

	groupByAccount = "account"
	groupByEntity  = "entity"
)

// failingStatusWords are backupStatus.status values treated as failures
// even when the error text is empty.
var failingStatusWords = map[string]bool{"FAILED": true, "FAILURE": true, "ERROR": true}

// successStatusWords are backupStatus.status values that mean the last
// attempt finished well.
var successStatusWords = map[string]bool{"SUCCESS": true, "SUCCEEDED": true, "SUCCESSFUL": true, "OK": true, "COMPLETED": true, "DONE": true}

// runningStatusWords are backupStatus.status values that mean an attempt
// is still in progress.
var runningStatusWords = map[string]bool{"PENDING": true, "QUEUED": true, "DISPATCHED": true, "INPROCESS": true, "IN_PROGRESS": true, "INPROGRESS": true, "RUNNING": true, "ACTIVE": true}

// healthAttemptGrace is how long an attempt newer than the last success may
// run before it counts as failing when it reports no error.
const healthAttemptGrace = 6 * time.Hour

// backupStatusEntry mirrors one backupStatus element of the feed.
type backupStatusEntry struct {
	SubSource  string `json:"subSource"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	ErrFAQLink string `json:"errFAQLink"`
}

// backupStatusRow mirrors one synced row of the backup-statuses feed.
type backupStatusRow struct {
	UserID                flexString          `json:"userId"`
	TaskID                flexString          `json:"taskId"`
	Source                string              `json:"source"`
	EntityName            string              `json:"entityName"`
	LastBackupDate        flexString          `json:"lastBackupDate"`
	LastBackupAttemptDate flexString          `json:"lastBackupAttemptDate"`
	BackupStatus          []backupStatusEntry `json:"backupStatus"`
}

// healthRow is the per-entity classification.
type healthRow struct {
	Account     string   `json:"account"`
	TaskID      string   `json:"task_id,omitempty"`
	Source      string   `json:"source"`
	Entity      string   `json:"entity"`
	LastBackup  string   `json:"last_backup"`
	AgeDays     *float64 `json:"age_days,omitempty"`
	LastAttempt string   `json:"last_attempt,omitempty"`
	Stale       bool     `json:"stale"`
	Failed      bool     `json:"failed"`
	State       string   `json:"state"`
	Error       string   `json:"error,omitempty"`
	FAQLink     string   `json:"faq_link,omitempty"`
}

// healthGroup is the per-account aggregate.
type healthGroup struct {
	Account string `json:"account"`
	// Entities counts every synced entity of the account that passes the
	// --account and --source filters; Flagged counts the stale or failing
	// ones (or only failing with --failing).
	Entities   int      `json:"entities"`
	Flagged    int      `json:"flagged"`
	Stale      int      `json:"stale"`
	Failing    int      `json:"failing"`
	OldestDays *float64 `json:"oldest_age_days,omitempty"`
	Unknown    int      `json:"unknown_date"`
}

// classifyBackupHealth derives staleness and failure for one feed row.
func classifyBackupHealth(r backupStatusRow, now time.Time, staleDays int) healthRow {
	h := healthRow{Account: string(r.UserID), TaskID: string(r.TaskID), Source: r.Source, Entity: r.EntityName}
	var errs []string
	nonSuccess := false
	for _, s := range r.BackupStatus {
		word := strings.ToUpper(strings.TrimSpace(s.Status))
		if word != "" && !successStatusWords[word] && !runningStatusWords[word] {
			nonSuccess = true
		}
		if strings.TrimSpace(s.Error) != "" || failingStatusWords[word] {
			h.Failed = true
			if strings.TrimSpace(s.Error) != "" {
				errs = append(errs, strings.TrimSpace(s.SubSource+": "+s.Error))
			} else {
				errs = append(errs, strings.TrimSpace(s.SubSource+": "+s.Status))
			}
		}
		if h.FAQLink == "" && s.ErrFAQLink != "" {
			h.FAQLink = s.ErrFAQLink
		}
	}
	h.Error = strings.Join(errs, "; ")
	last, lerr := cadate.ParseLoose(string(r.LastBackupDate))
	attempt, aerr := cadate.ParseLoose(string(r.LastBackupAttemptDate))
	if aerr == nil {
		h.LastAttempt = isoUTC(attempt)
	} else if strings.TrimSpace(string(r.LastBackupAttemptDate)) != "" {
		h.LastAttempt = healthUnknownDate
	}
	if lerr != nil {
		h.LastBackup = healthUnknownDate
		h.Stale = true
	} else {
		h.LastBackup = isoUTC(last)
		age := math.Round(now.Sub(last).Hours()/24*10) / 10
		h.AgeDays = &age
		h.Stale = now.Sub(last) > time.Duration(staleDays)*24*time.Hour
		// An attempt newer than the last success is still running unless
		// it reports an error or a non-success status, or has been going
		// longer than healthAttemptGrace.
		if aerr == nil && attempt.After(last) && (h.Failed || nonSuccess || now.Sub(attempt) > healthAttemptGrace) {
			h.Failed = true
		}
	}
	switch {
	case h.LastBackup == healthUnknownDate && h.Failed:
		h.State = healthUnknownDate + "+" + healthStateFail
	case h.LastBackup == healthUnknownDate:
		h.State = healthUnknownDate
	case h.Stale && h.Failed:
		h.State = healthStateStale + "+" + healthStateFail
	case h.Stale:
		h.State = healthStateStale
	case h.Failed:
		h.State = healthStateFail
	default:
		h.State = healthStateOK
	}
	return h
}

// healthFilter narrows the classified rows.
type healthFilter struct {
	account     string
	source      string
	failingOnly bool
}

// inScope reports whether a row passes the --account and --source filters,
// regardless of its health.
func (f healthFilter) inScope(h healthRow) bool {
	if f.account != "" && h.Account != f.account {
		return false
	}
	return f.source == "" || strings.EqualFold(h.Source, f.source)
}

func (f healthFilter) keep(h healthRow) bool {
	if !f.inScope(h) {
		return false
	}
	if f.failingOnly {
		return h.Failed
	}
	return h.Stale || h.Failed
}

// sortHealthRows orders failing first, then oldest first, then by name.
func sortHealthRows(rows []healthRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Failed != b.Failed {
			return a.Failed
		}
		ai, bi := math.Inf(1), math.Inf(1)
		if a.AgeDays != nil {
			ai = *a.AgeDays
		}
		if b.AgeDays != nil {
			bi = *b.AgeDays
		}
		if ai != bi {
			return ai > bi
		}
		if a.Account != b.Account {
			return a.Account < b.Account
		}
		return strings.ToLower(a.Entity) < strings.ToLower(b.Entity)
	})
}

// groupHealthByAccount aggregates flagged rows per account. totals holds
// the number of in-scope entities per account (healthy ones included);
// an account missing from totals reports its flagged count as entities.
func groupHealthByAccount(rows []healthRow, totals map[string]int) []healthGroup {
	byAcct := map[string]*healthGroup{}
	for _, r := range rows {
		g := byAcct[r.Account]
		if g == nil {
			g = &healthGroup{Account: r.Account, Entities: totals[r.Account]}
			byAcct[r.Account] = g
		}
		g.Flagged++
		if g.Entities < g.Flagged {
			g.Entities = g.Flagged
		}
		if r.Stale {
			g.Stale++
		}
		if r.Failed {
			g.Failing++
		}
		if r.AgeDays == nil {
			g.Unknown++
		} else if g.OldestDays == nil || *r.AgeDays > *g.OldestDays {
			v := *r.AgeDays
			g.OldestDays = &v
		}
	}
	out := make([]healthGroup, 0, len(byAcct))
	for _, g := range byAcct {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Failing != out[j].Failing {
			return out[i].Failing > out[j].Failing
		}
		if out[i].Stale != out[j].Stale {
			return out[i].Stale > out[j].Stale
		}
		return out[i].Account < out[j].Account
	})
	return out
}

func fmtDays(v *float64) string {
	if v == nil {
		return "?"
	}
	return fmt.Sprintf("%.1f", *v)
}

// errHealthNotSynced keeps an empty store from reading as "every backup is
// healthy": an agent reports results [] as good news.
func errHealthNotSynced(resource string) error {
	return notFoundErr(fmt.Errorf("no synced %s rows: run 'cloudally-cli sync --resources %s' under a partner login (auth login --partner) first", resource, resource))
}

func newBackupHealthCmd(flags *rootFlags) *cobra.Command {
	var flagStaleDays, flagLimit int
	var flagFailing, flagReseller bool
	var flagAccount, flagSource, flagGroupBy string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Find stale or failing mailboxes and sites across all customer accounts",
		Long:  "Use this command to find stale or failing mailboxes and sites across all customer accounts. Do NOT use it for the raw paged feed; use 'distributor-accounts backup-statuses' instead. It reads the partner-wide statuses feed, so it needs data synced under a partner login (auth login --partner).",
		Example: strings.Trim(`
  cloudally-cli backup health --stale-days 2
  cloudally-cli backup health --failing --group-by account
  cloudally-cli backup health --account 5f1c2a --source MS365 --json
  cloudally-cli backup health --reseller --stale-days 7 --limit 50
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "local",
			"pp:happy-args":  "--stale-days=2",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return usageErr(err)
			}
			if flagStaleDays < 0 {
				return usageErr(fmt.Errorf("--stale-days must be 0 or more"))
			}
			if flagGroupBy != groupByAccount && flagGroupBy != groupByEntity {
				return usageErr(fmt.Errorf("--group-by must be %s or %s", groupByAccount, groupByEntity))
			}
			resource := healthResourceDistributor
			if flagReseller {
				resource = healthResourceReseller
			}
			if dryRunOK(flags) {
				would := fmt.Sprintf("read synced %s rows from the local store and flag entities older than %d day(s) or failing", resource, flagStaleDays)
				return writeWould(cmd.OutOrStdout(), flags, "backup health", would, map[string]any{"resource": resource})
			}
			ctx := cmd.Context()
			db, err := openStoreForRead(ctx, restoreCLIName)
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			human := wantsHumanTable(cmd.OutOrStdout(), flags)
			if db == nil {
				return errHealthNotSynced(resource)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, resource) {
				maxAge := flags.maxAge
				if maxAge <= 0 {
					maxAge = defaultHealthMaxAge
				}
				hintIfStale(cmd, db, resource, maxAge)
			}
			now := time.Now().UTC()
			filter := healthFilter{account: flagAccount, source: flagSource, failingOnly: flagFailing}
			var rows []healthRow
			totals := map[string]int{}
			inScope := 0
			skipped := 0
			scanned := 0
			err = db.ListScan(resource, func(_ string, data json.RawMessage) bool {
				scanned++
				var r backupStatusRow
				if json.Unmarshal(data, &r) != nil || r.EntityName == "" {
					skipped++
					return true
				}
				h := classifyBackupHealth(r, now, flagStaleDays)
				if filter.inScope(h) {
					totals[h.Account]++
					inScope++
				}
				if filter.keep(h) {
					rows = append(rows, h)
				}
				return true
			})
			if err != nil && !isMissingTableErr(err) {
				return fmt.Errorf("reading local store: %w", err)
			}
			if scanned == 0 {
				return errHealthNotSynced(resource)
			}
			if skipped > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipped %d row(s) that did not decode as backup statuses\n", skipped)
			}
			sortHealthRows(rows)
			if flagGroupBy == groupByAccount {
				groups := groupHealthByAccount(rows, totals)
				if flagLimit > 0 && len(groups) > flagLimit {
					groups = groups[:flagLimit]
				}
				if !human {
					if groups == nil {
						groups = []healthGroup{}
					}
					return printJSONFiltered(cmd.OutOrStdout(), groups, flags)
				}
				if len(groups) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "no stale or failing entities in %d synced row(s)\n", inScope)
					return nil
				}
				table := make([][]string, 0, len(groups))
				for _, g := range groups {
					table = append(table, []string{g.Account, fmt.Sprint(g.Entities), fmt.Sprint(g.Flagged), fmt.Sprint(g.Stale), fmt.Sprint(g.Failing), fmt.Sprint(g.Unknown), fmtDays(g.OldestDays)})
				}
				return printRestoreTable(cmd.OutOrStdout(), []string{"account", "entities", "flagged", "stale", "failing", "unknown_date", "oldest_days"}, table)
			}
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}
			if !human {
				if rows == nil {
					rows = []healthRow{}
				}
				return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no stale or failing entities found")
				return nil
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Account, r.Source, r.Entity, r.LastBackup, fmtDays(r.AgeDays), r.State, strings.TrimSpace(truncate(r.Error, 50))})
			}
			return printRestoreTable(cmd.OutOrStdout(), []string{"account", "source", "entity", "last_backup", "age_days", "state", "error"}, table)
		},
	}
	cmd.Flags().IntVar(&flagStaleDays, "stale-days", defaultStaleDays, "Flag entities whose newest backup is older than this many days")
	cmd.Flags().BoolVar(&flagFailing, "failing", false, "Only entities whose last attempt failed or that report an error")
	cmd.Flags().StringVar(&flagAccount, "account", "", "Only this customer account id (userId in the feed)")
	cmd.Flags().StringVar(&flagSource, "source", "", "Only this service source, for example MS365 or GMAIL")
	cmd.Flags().StringVar(&flagGroupBy, "group-by", groupByEntity, "account for per-account counts, entity for one row per mailbox or site")
	cmd.Flags().IntVar(&flagLimit, "limit", defaultHealthLimit, "Maximum rows to print (0 for all)")
	cmd.Flags().BoolVar(&flagReseller, "reseller", false, "Read the reseller feed (reseller_accounts-backup-statuses) instead of the distributor one")
	return cmd
}
