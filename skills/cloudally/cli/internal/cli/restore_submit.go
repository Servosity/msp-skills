// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/cliutil"
	"github.com/spf13/cobra"
)

const (
	defaultSubmitConcurrency = 3
	defaultSubmitRate        = 2.0
)

// restoreDateFlagHelp is the --date help shared by plan and submit. The
// cutoff is always interpreted in UTC.
const restoreDateFlagHelp = "Restore point, in UTC: YYYY-MM-DD means up to 23:59:59 UTC that day (2026-10-01 is up to 2026-10-01 23:59:59 UTC); also RFC3339, YYYY-MM-DDTHH:MM (UTC), or yyyyMMddHHmmss (UTC). Repeatable"

// submitOptions are the optional restore body fields shared by every row.
type submitOptions struct {
	includeDeleted   bool
	includeLegalHold bool
	restoreOption    string
	targetAccount    string
}

// Flag names of the submit options; a resumed batch checks the ones a run
// passed explicitly against what the batch recorded.
const (
	flagNameIncludeDeleted   = "include-deleted"
	flagNameIncludeLegalHold = "include-legal-hold"
	flagNameRestoreOption    = "restore-option"
	flagNameTargetAccount    = "target-account"
)

func (o submitOptions) asMap() map[string]any {
	return map[string]any{
		"include_deleted":    o.includeDeleted,
		"include_legal_hold": o.includeLegalHold,
		"restore_option":     o.restoreOption,
		"target_account":     o.targetAccount,
	}
}

// submitOptionsFromMap reads the options a batch recorded. ok is false when
// any of them is missing, in which case where the batch's restores went
// cannot be confirmed.
func submitOptionsFromMap(m map[string]any) (o submitOptions, ok bool) {
	var ok1, ok2, ok3, ok4 bool
	o.includeDeleted, ok1 = m["include_deleted"].(bool)
	o.includeLegalHold, ok2 = m["include_legal_hold"].(bool)
	o.restoreOption, ok3 = m["restore_option"].(string)
	o.targetAccount, ok4 = m["target_account"].(string)
	return o, ok1 && ok2 && ok3 && ok4
}

// restoreBody builds the POST body for one row. The date is the compact
// yyyyMMddHHmmss integer; see cadate.CompactLayout.
func restoreBody(taskID, source string, entity, extID string, dateRaw int64, opts submitOptions) map[string]any {
	body := map[string]any{
		"taskId":           taskID,
		"account":          entity,
		"source":           source,
		"date":             dateRaw,
		"includeDeleted":   opts.includeDeleted,
		"includeLegalHold": opts.includeLegalHold,
	}
	if extID != "" {
		body["extid"] = extID
	}
	if opts.restoreOption != "" {
		body["restoreOption"] = opts.restoreOption
	}
	if opts.targetAccount != "" {
		body["targetAccount"] = opts.targetAccount
	}
	return body
}

// restoreResponse mirrors the 200 body of POST .../recovery/restore.
type restoreResponse struct {
	NeedRedirect        bool   `json:"needRedirect"`
	RedirectURL         string `json:"redirectUrl"`
	ShowNotification    bool   `json:"showNotification"`
	NotificationMessage string `json:"notificationMessage"`
}

// submitResult is what a worker reports back for one row.
type submitResult struct {
	job         restoreJob
	status      string
	httpStatus  int
	response    string
	errText     string
	at          time.Time
	authFailure bool
	// notSent is set when the row could not be claimed in the ledger and
	// so was never sent; its ledger status is unchanged.
	notSent bool
}

// submitFailure is the JSON shape of one failed or unconfirmed row.
type submitFailure struct {
	Entity     string `json:"entity"`
	Requested  string `json:"requested"`
	HTTPStatus int    `json:"http_status,omitempty"`
	Status     string `json:"status"`
	Error      string `json:"error"`
}

// submitOutput is the JSON document 'restore submit' prints.
type submitOutput struct {
	BatchID       string          `json:"batch_id"`
	BatchKey      string          `json:"batch_key"`
	Resumed       bool            `json:"resumed"`
	Scope         string          `json:"scope"`
	AccountID     string          `json:"account_id,omitempty"`
	AuthScope     string          `json:"auth_scope,omitempty"`
	TaskID        string          `json:"task_id"`
	Source        string          `json:"source"`
	Total         int             `json:"total"`
	Submitted     int             `json:"submitted"`
	Failed        int             `json:"failed"`
	Unknown       int             `json:"unknown"`
	NotSent       int             `json:"not_sent"`
	Skipped       int             `json:"skipped"`
	HeldUnknown   int             `json:"held_unknown"`
	AddedEntities []string        `json:"added_entities,omitempty"`
	Counts        map[string]int  `json:"counts"`
	FetchFailures []submitFailure `json:"fetch_failures"`
	Rows          []restoreJob    `json:"rows"`
}

// submitRequest carries everything runSubmit needs about one invocation.
type submitRequest struct {
	scope     restoreScope
	account   submitAccount
	taskID    string
	source    string
	dateTexts []string
	filter    []string
	rows      []planRow
	opts      submitOptions
	// optsSet names the option flags this run passed explicitly; on
	// --resume each must agree with the batch's recorded value.
	optsSet      map[string]bool
	label        string
	resume       bool
	retryUnknown bool
	newBatch     bool
	concurrency  int
	rate         float64
	restorePath  string
}

func newRestoreSubmitCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagTask, flagPlanFile, flagEntitiesFile, flagSubSource, flagLabel string
	var flagDates, flagEntities []string
	var flagConcurrency int
	var flagRate float64
	var flagResume, flagRetryUnknown, flagNewBatch bool
	var flagResolve, flagPortalURL string
	var flagLookback int
	var opts submitOptions

	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit one restore per mailbox per date and record the batch in the local ledger",
		Long: strings.Trim(`
Use this command to submit many restores for one task and record them in the ledger.
Do NOT use it for a single mailbox; use 'recovery restore' instead. Do NOT use it to
preview only; use 'restore plan' or --dry-run instead.

Every real submission needs --yes, also with --agent, --no-input, or no terminal.
Without --yes the command prints what it would submit (rows, entities, dates, scope,
account), sends nothing, writes nothing, and exits 2.

Each row moves PLANNED -> SUBMITTING (saved before the request leaves) -> SUBMITTED,
FAILED, FAILED_REDIRECT, or UNKNOWN. UNKNOWN means the API may have accepted the
restore (timeout, connection reset, 408, 5xx, interrupted run). --resume sends only
PLANNED and FAILED rows again; check UNKNOWN rows with 'restore watch --batch <id> --once'
and add --retry-unknown only for those that are missing. A row the API accepted is never
sent again by --resume, even when 'restore watch' later shows its RESTORE task FAILED.

--resume keeps the target account, restore option, and include flags recorded on the
batch; passing one of those flags with a different value is refused.

A machine-wide restore guard, one file in the CLI state directory shared by every login
and token on this machine, lets each restore (task, mailbox, resolved backup) be sent
once. A run that would send a restore already accepted, unknown, or in flight from any
batch, login, token, or process here is refused before anything is sent, whatever date
spelling, scope, or entity list it uses; only a restore the API refused before running
it can be sent again. To send only the mailboxes never sent, limit --entities to them.
--new-batch overrides the guard and sends every row again, accepted restores included;
use it only to restore mailboxes a second time on purpose.

--timeout bounds each API and portal call, not the run. A run that ends with rows still
PLANNED (interrupted, or stopped by a 401/403) exits non-zero and prints the --resume
command that sends them.

Without --account the active login decides the customer: the account from
'auth use-account' or 'auth login' is recorded on the batch. A partner login needs
--account <id>.

For a past date, each mailbox's backup is resolved from the customer portal's own
per-mailbox backup calendar, exactly as 'restore plan' does, and --resolve selects
the source. A mailbox with no successful backup at or before the date is a NO_BACKUP
row and is never submitted; no row is ever submitted against a backup other than the
one the plan resolved for it.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli restore submit --task 64b0e9 --date 2026-10-01 --dry-run
  cloudally-cli restore submit --task 64b0e9 --date 2026-10-01 --yes
  cloudally-cli restore submit --task 64b0e9 --date 2026-10-01 --entities jane.doe@contoso.com --concurrency 3 --rate 2 --yes
  cloudally-cli restore submit --plan-file plan.json --label "ticket 4821" --yes
  cloudally-cli restore submit --task 64b0e9 --date 2026-10-01 --resume --yes
  cloudally-cli restore submit --task 64b0e9 --date 2026-10-01 --resume --retry-unknown --yes
  # partner login (auth login --partner): name the customer account
  cloudally-cli restore submit --account 5f1c2a --task 64b0e9 --date 2026-10-01 --yes
  # end-of-year restore point, resolved from the portal's backup calendar
  cloudally-cli restore submit --account 5f1c2a --task 64b0e9 --date 2025-12-31 --yes
`, "\n"),
		Annotations: map[string]string{
			"mcp:write-flags": "plan-file entities-file",
			"pp:data-source":  "live",
			"pp:happy-args":   "--task=64b0e9;--date=2026-10-01;--yes",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hasChangedLocalFlags(cmd) && len(args) == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if err := requireBoundStoreScope(); err != nil {
				return err
			}
			if err := scope.validate(); err != nil {
				return err
			}
			if flagConcurrency < 1 {
				return usageErr(errors.New("--concurrency must be at least 1"))
			}
			if flagRate <= 0 {
				return usageErr(errors.New("--rate must be greater than 0"))
			}
			if flagRetryUnknown && !flagResume {
				return usageErr(errors.New("--retry-unknown only applies with --resume, and sends UNKNOWN rows that may already have been accepted: add --resume only after 'restore watch --batch <id> --once' found no RESTORE task for them"))
			}
			if flagNewBatch && flagResume {
				return usageErr(errors.New("--new-batch and --resume cannot be combined: --resume continues the earlier batch, --new-batch starts another that sends every restore again"))
			}
			var planDoc *restorePlan
			if flagPlanFile != "" {
				p, err := readPlanFile(flagPlanFile)
				if err != nil {
					return err
				}
				planDoc = p
				// The plan fixes the task and scope its rows were resolved
				// against; a conflicting --task or --account would post those
				// rows to a different task or customer.
				if t := strings.TrimSpace(flagTask); t != "" && t != p.TaskID {
					return usageErr(fmt.Errorf("--task %s does not match the plan's task_id %s; drop --task or write a new plan", t, p.TaskID))
				}
				flagTask = p.TaskID
				planScope := scopeFromKind(p.Scope, p.AccountID)
				if scope.account == "" {
					scope = planScope
				} else if scope.account != planScope.account || scope.kind() != planScope.kind() {
					return usageErr(fmt.Errorf("--account/--reseller (%s %s) do not match the plan's scope (%s %s); drop them or write a new plan", scope.kind(), scope.account, planScope.kind(), planScope.account))
				}
			}
			if strings.TrimSpace(flagTask) == "" {
				return usageErr(errors.New("--task <backup task id> is required (or --plan-file)"))
			}
			// A plan file already carries resolved dates, so --resolve only
			// applies when this run resolves them itself. Validate against
			// the final scope, which --plan-file may have supplied.
			resolveMode := resolveLastBackup
			if planDoc == nil {
				m, err := validateResolveMode(flagResolve, scope)
				if err != nil {
					return err
				}
				resolveMode = m
			}
			var dates []requestedDate
			if planDoc == nil {
				d, err := parseRequestedDates(flagDates)
				if err != nil {
					return err
				}
				dates = d
			}
			filter, requested, err := parseEntityFilter(flagEntities, flagEntitiesFile)
			if err != nil {
				return err
			}
			restorePath := scope.restorePath()
			// Submit never sends under --dry-run or any Printing Press
			// harness (verify or live dogfood): a restore overwrites a real
			// mailbox, so a curtailed run is still a real side effect.
			if dryRunOK(flags) || cliutil.IsAnyHarness() {
				return printSubmitDryRun(cmd, flagTask, restorePath, planDoc, dates, filter, requested, opts)
			}
			account, err := resolveSubmitAccount(scope)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// No overall deadline: --timeout bounds each API and portal
			// call (the clients apply it per request). A run that sends
			// hundreds of restores at --rate must not stop part way and
			// leave mailboxes unsent.
			ctx := cmd.Context()
			var rows []planRow
			source := ""
			var dateTexts []string
			if planDoc != nil {
				rows, source, dateTexts = planDoc.Rows, planDoc.Source, planDoc.Dates
				if len(filter) > 0 {
					rows = filterPlanRows(rows, filter)
				}
			} else {
				task, err := fetchTaskWithEntities(ctx, c, scope.taskPath(flagTask))
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				for _, m := range missingEntities(task, requested) {
					termf(cmd.ErrOrStderr(), "warning: entity %q is not on task %s\n", m, flagTask)
				}
				var resolver snapshotResolver
				if wantsSnapshotResolution(resolveMode, scope) {
					r, rerr := openPortalResolver(ctx, c, scope, flagPortalURL, flagLookback)
					if rerr != nil {
						// A submit resolves the dates it is about to act
						// on, so never downgrade silently: either the
						// caller asked for the calendar and it must work,
						// or the fallback is stated plainly.
						if resolveMode == resolveSnapshots {
							return fmt.Errorf("opening the portal backup calendar: %w", rerr)
						}
						termf(cmd.ErrOrStderr(), "warning: the portal backup calendar is unavailable (%v); resolving from the latest backup dates instead, which cannot find a backup before the most recent one\n", rerr)
					} else {
						resolver = r
					}
				}
				builtRows, warnings := buildPlanWithResolver(ctx, task, dates, filter, flagSubSource, resolver)
				for _, w := range warnings {
					termf(cmd.ErrOrStderr(), "warning: %v\n", w)
				}
				rows, source = builtRows, task.Source
				for _, d := range dates {
					dateTexts = append(dateTexts, d.text)
				}
			}
			ok := filterPlanRows(rows, nil)
			if len(ok) == 0 {
				return notFoundErr(fmt.Errorf("no entity on task %s has a backup at or before the requested date(s)", flagTask))
			}
			optsSet := map[string]bool{}
			for _, name := range []string{flagNameIncludeDeleted, flagNameIncludeLegalHold, flagNameRestoreOption, flagNameTargetAccount} {
				optsSet[name] = cmd.Flags().Changed(name)
			}
			req := submitRequest{
				scope: scope, account: account, taskID: flagTask, source: source, dateTexts: dateTexts,
				filter: sortedKeys(filter), rows: ok, opts: opts, optsSet: optsSet, label: flagLabel, resume: flagResume,
				retryUnknown: flagRetryUnknown, newBatch: flagNewBatch, concurrency: flagConcurrency, rate: flagRate, restorePath: restorePath,
			}
			if !flags.yes {
				printSubmitConfirmation(cmd.ErrOrStderr(), req)
				return usageErr(fmt.Errorf("add --yes to submit these %d restores", len(ok)))
			}
			return runSubmit(ctx, cmd, flags, c, req)
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagTask, "task", "", "Backup task id whose entities will be restored")
	cmd.Flags().StringArrayVar(&flagDates, "date", nil, restoreDateFlagHelp)
	cmd.Flags().StringSliceVar(&flagEntities, "entities", nil, "Comma-separated entity names to include; default is every entity on the task")
	cmd.Flags().StringVar(&flagEntitiesFile, "entities-file", "", "File with one entity per line, or a CSV whose first column is the entity")
	cmd.Flags().StringVar(&flagSubSource, "sub-source", "", "Restrict backup lookup to one lastBackupDates key (for example mail)")
	cmd.Flags().StringVar(&flagPlanFile, "plan-file", "", "Submit the rows of a plan written by 'restore plan --plan-out' instead of resolving again")
	cmd.Flags().IntVar(&flagConcurrency, "concurrency", defaultSubmitConcurrency, "Parallel restore requests")
	cmd.Flags().Float64Var(&flagRate, "rate", defaultSubmitRate, "Maximum restore requests per second across all workers")
	cmd.Flags().BoolVar(&opts.includeDeleted, flagNameIncludeDeleted, false, "Set includeDeleted on every restore")
	cmd.Flags().BoolVar(&opts.includeLegalHold, flagNameIncludeLegalHold, false, "Set includeLegalHold on every restore")
	cmd.Flags().StringVar(&opts.restoreOption, flagNameRestoreOption, "", "Optional restoreOption string passed through to the API")
	cmd.Flags().StringVar(&opts.targetAccount, flagNameTargetAccount, "", "Optional targetAccount to restore into")
	cmd.Flags().StringVar(&flagLabel, "label", "", "Free-text label stored on the batch")
	cmd.Flags().BoolVar(&flagResume, "resume", false, "Continue the batch with the same scope, account, task, entity filter, and dates; sends only PLANNED and FAILED rows that were never accepted, with the batch's recorded restore options")
	cmd.Flags().BoolVar(&flagRetryUnknown, "retry-unknown", false, "With --resume, also send UNKNOWN rows again; use only after 'restore watch' shows they are missing")
	cmd.Flags().BoolVar(&flagNewBatch, "new-batch", false, "Send every row again, including restores already accepted or unconfirmed in an earlier batch under any login on this machine; prefer --entities limited to the mailboxes never sent")
	addResolveFlags(cmd, &flagResolve, &flagPortalURL, &flagLookback)
	return cmd
}

// filterPlanRows keeps OK rows, optionally restricted to a lowercase set.
func filterPlanRows(rows []planRow, filter map[string]bool) []planRow {
	var out []planRow
	for _, r := range rows {
		if r.Status != planOK {
			continue
		}
		if len(filter) > 0 && !filter[strings.ToLower(strings.TrimSpace(r.Entity))] {
			continue
		}
		out = append(out, r)
	}
	return out
}

// printSubmitDryRun prints one JSON body per planned request and writes
// nothing. Without a plan file the entity list is whatever was requested,
// or a placeholder when the task's entities would be fetched live.
func printSubmitDryRun(cmd *cobra.Command, taskID, path string, planDoc *restorePlan, dates []requestedDate, filter map[string]bool, requested []string, opts submitOptions) error {
	w := cmd.OutOrStdout()
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	mode := "dry-run"
	if h := cliutil.HarnessName(); h != "" {
		mode = h + " harness"
	}
	termf(cmd.ErrOrStderr(), "%s: would POST %s once per row below; nothing sent, ledger untouched\n", mode, path)
	if planDoc != nil {
		for _, r := range filterPlanRows(planDoc.Rows, filter) {
			if err := enc.Encode(restoreBody(planDoc.TaskID, planDoc.Source, r.Entity, r.ExtID, r.ResolvedRaw, opts)); err != nil {
				return err
			}
		}
		return nil
	}
	entities := requested
	if len(entities) == 0 {
		entities = []string{"<each entity on task " + taskID + ">"}
	}
	for _, e := range entities {
		for _, d := range dates {
			if err := enc.Encode(restoreBody(taskID, "<task source>", e, "", cadate.FormatCompact(d.cutoff), opts)); err != nil {
				return err
			}
		}
	}
	return nil
}
