// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloudally-pp-cli/internal/cadate"
	"cloudally-pp-cli/internal/portal"
	"github.com/spf13/cobra"
)

// planFilePerm is the mode for --plan-out files.
const planFilePerm = 0o600

// errNoEntityExtID marks an entity whose backup calendar cannot be looked
// up because the task record carries no external id for it.
var errNoEntityExtID = errors.New("no external id on the task record, so the backup calendar cannot be read")

// resolveConcurrency is how many entities are resolved against the portal
// at once. The portal session's own rate limiter caps the request rate, so
// this only decides how much of the per-request latency overlaps.
const resolveConcurrency = 8

// snapshotResolver is the portal lookup a plan needs, kept as an interface
// so the resolution logic can be tested without a portal session.
type snapshotResolver interface {
	NewestSuccessfulAtOrBefore(ctx context.Context, ent portal.Entity, cutoff time.Time) (portal.Snapshot, bool, error)
}

// planRow is one (entity, requested date) resolution.
//
// EntityStatus is the entity's own state on the task (ACTIVE, ARCHIVED,
// ON_PREMISE). It is reported for every entity, including ones that are no
// longer actively backed up, so a plan never hides part of the task.
//
// DateSource records where Resolved came from: the portal's per-mailbox
// backup calendar, or the latest-backup dates on the task record. A row whose
// date could not be resolved keeps status NO_BACKUP and carries no date at
// all, so a restore can never quietly fall back to a different backup.
type planRow struct {
	Entity       string `json:"entity"`
	EntityStatus string `json:"entity_status,omitempty"`
	ExtID        string `json:"ext_id,omitempty"`
	Requested    string `json:"requested"`
	Resolved     string `json:"resolved,omitempty"`
	ResolvedRaw  int64  `json:"resolved_raw,omitempty"`
	DateSource   string `json:"date_source,omitempty"`
	BatchID      string `json:"batch_id,omitempty"`
	SubSource    string `json:"sub_source,omitempty"`
	Age          string `json:"age,omitempty"`
	Status       string `json:"status"`
	// Detail explains a NO_BACKUP row.
	Detail string `json:"detail,omitempty"`
}

// restorePlan is the document --plan-out writes and --plan-file reads.
type restorePlan struct {
	Scope     string   `json:"scope"`
	AccountID string   `json:"account_id,omitempty"`
	TaskID    string   `json:"task_id"`
	Source    string   `json:"source"`
	Dates     []string `json:"dates"`
	SubSource string   `json:"sub_source,omitempty"`
	// Resolve records which resolution strategy produced these rows.
	Resolve string    `json:"resolve,omitempty"`
	Rows    []planRow `json:"rows"`
}

// requestedDate pairs the user's text with its parsed cutoff.
type requestedDate struct {
	text   string
	cutoff time.Time
}

func parseRequestedDates(values []string) ([]requestedDate, error) {
	var out []requestedDate
	seen := map[string]bool{}
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			cutoff, _, err := cadate.ParseUserDate(part)
			if err != nil {
				return nil, usageErr(fmt.Errorf("--date %w", err))
			}
			if seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, requestedDate{text: part, cutoff: cutoff})
		}
	}
	if len(out) == 0 {
		return nil, usageErr(errors.New("--date is required (YYYY-MM-DD, RFC3339, or yyyyMMddHHmmss)"))
	}
	return out, nil
}

// buildPlan resolves one row per entity per requested date from the latest
// backup dates on the task record. filter is a lowercase entity set; nil or
// empty means every entity on the task.
func buildPlan(task *taskWithEntities, dates []requestedDate, filter map[string]bool, subSource string) []planRow {
	rows, _ := buildPlanWithResolver(context.Background(), task, dates, filter, subSource, nil)
	return rows
}

// planEntities returns the task's entities, filtered and ordered by name.
func planEntities(task *taskWithEntities, filter map[string]bool) []taskEntity {
	entities := append([]taskEntity(nil), task.Entities...)
	sort.SliceStable(entities, func(i, j int) bool {
		return strings.ToLower(entities[i].Entity) < strings.ToLower(entities[j].Entity)
	})
	if len(filter) == 0 {
		return entities
	}
	kept := entities[:0:0]
	for _, ent := range entities {
		if filter[strings.ToLower(strings.TrimSpace(ent.Entity))] {
			kept = append(kept, ent)
		}
	}
	return kept
}

// buildPlanWithResolver resolves one row per entity per requested date.
//
// With a resolver, each row takes the newest successful backup at or before
// the requested date from the portal's per-mailbox backup calendar, which is
// the only source that knows about past runs. Without one, resolution falls
// back to the latest backup dates the task record carries, which can only
// satisfy a date at or after the most recent backup.
//
// A mailbox with no backup at or before the date stays in the plan as a
// NO_BACKUP row carrying no date, so it is visible and excluded from
// submission rather than quietly restored from some other backup.
//
// A resolver error for one entity is recorded on that entity's rows and
// returned as a warning; it never substitutes a different date and never
// aborts the other entities.
//
// Snapshot resolution runs several entities at a time, because a task with
// scores of mailboxes needs one portal round trip each and doing them in
// series overruns the command deadline. The portal's own rate limiter still
// governs the request rate, so concurrency shortens the wall clock without
// raising the load. Rows come back in entity order regardless.
func buildPlanWithResolver(ctx context.Context, task *taskWithEntities, dates []requestedDate, filter map[string]bool, subSource string, resolver snapshotResolver) ([]planRow, []error) {
	entities := planEntities(task, filter)
	perEntity := make([][]planRow, len(entities))
	perWarnings := make([][]error, len(entities))

	newRows := func(ent taskEntity) []planRow {
		rows := make([]planRow, 0, len(dates))
		for _, d := range dates {
			rows = append(rows, planRow{
				Entity:       ent.Entity,
				EntityStatus: ent.Status,
				ExtID:        string(ent.ExtID),
				Requested:    d.text,
				Status:       planNoBackup,
			})
		}
		return rows
	}

	if resolver == nil {
		for i, ent := range entities {
			rows := newRows(ent)
			for j := range rows {
				resolveFromLastBackup(&rows[j], ent, dates[j], subSource)
			}
			perEntity[i] = rows
		}
		return flattenPlanRows(perEntity), nil
	}

	// Each goroutine owns one entity: it writes only its own slots, and all
	// of that entity's dates share its cached months.
	sem := make(chan struct{}, resolveConcurrency)
	var wg sync.WaitGroup
	for i, ent := range entities {
		wg.Add(1)
		go func(i int, ent taskEntity) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
			}
			rows := newRows(ent)
			var warnings []error
			for j := range rows {
				if err := resolveFromSnapshots(ctx, &rows[j], ent, dates[j], task, resolver); err != nil {
					warnings = append(warnings, fmt.Errorf("%s: %w", ent.Entity, err))
				}
			}
			perEntity[i], perWarnings[i] = rows, warnings
		}(i, ent)
	}
	wg.Wait()

	var allWarnings []error
	for _, w := range perWarnings {
		allWarnings = append(allWarnings, w...)
	}
	return flattenPlanRows(perEntity), allWarnings
}

// flattenPlanRows concatenates per-entity rows in entity order.
func flattenPlanRows(perEntity [][]planRow) []planRow {
	total := 0
	for _, rows := range perEntity {
		total += len(rows)
	}
	if total == 0 {
		return nil
	}
	out := make([]planRow, 0, total)
	for _, rows := range perEntity {
		out = append(out, rows...)
	}
	return out
}

// resolveFromLastBackup fills a row from the task record's latest dates.
func resolveFromLastBackup(row *planRow, ent taskEntity, d requestedDate, subSource string) {
	key, raw, t, ok := cadate.NewestAtOrBefore(ent.backupDates(), d.cutoff, subSource)
	if !ok {
		row.Detail = "the latest backup the API reports is after this date; use --resolve snapshots to search past backups"
		return
	}
	row.Resolved = isoUTC(t)
	row.ResolvedRaw = raw
	row.SubSource = key
	row.DateSource = dateSourceLastBackup
	row.Age = formatAge(d.cutoff.Sub(t))
	row.Status = planOK
}

// resolveFromSnapshots fills a row from the portal's backup calendar. The
// returned error is a per-entity warning, not a fatal failure.
func resolveFromSnapshots(ctx context.Context, row *planRow, ent taskEntity, d requestedDate, task *taskWithEntities, resolver snapshotResolver) error {
	// The calendar is keyed by the entity's external id, and it answers a
	// request that omits one with an empty list rather than an error. An
	// entity the task carries no extId for would therefore look exactly
	// like a mailbox with no backups, so it is refused instead: an
	// unreadable calendar must never read as a confident NO_BACKUP.
	if strings.TrimSpace(string(ent.ExtID)) == "" {
		row.Detail = "the task record carries no external id for this entity, so its backup calendar cannot be read; this is not a statement that no backup exists"
		return errNoEntityExtID
	}
	snap, found, err := resolver.NewestSuccessfulAtOrBefore(ctx, portal.Entity{
		TaskID:    string(task.ID),
		AccountID: ent.Entity,
		ExtID:     string(ent.ExtID),
	}, d.cutoff)
	switch {
	case err != nil:
		row.Detail = "the backup calendar could not be read: " + err.Error()
		return err
	case !found:
		row.Detail = "no successful backup at or before this date"
		return nil
	}
	t, err := snap.Time()
	if err != nil {
		row.Detail = "the backup calendar returned an undecodable date: " + err.Error()
		return err
	}
	row.Resolved = isoUTC(t)
	row.ResolvedRaw = snap.DateTime
	row.DateSource = dateSourceSnapshots
	row.BatchID = snap.BatchID
	row.Age = formatAge(d.cutoff.Sub(t))
	row.Status = planOK
	return nil
}

// findTaskEntity looks up one entity on a task by name, case-insensitively.
func findTaskEntity(task *taskWithEntities, name string) (taskEntity, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	for _, e := range task.Entities {
		if strings.ToLower(strings.TrimSpace(e.Entity)) == want {
			return e, true
		}
	}
	return taskEntity{}, false
}

// missingEntities lists requested entities the task does not carry.
func missingEntities(task *taskWithEntities, requested []string) []string {
	have := map[string]bool{}
	for _, e := range task.Entities {
		have[strings.ToLower(strings.TrimSpace(e.Entity))] = true
	}
	var missing []string
	for _, r := range requested {
		if !have[strings.ToLower(r)] {
			missing = append(missing, r)
		}
	}
	return missing
}

func planRowsAsMaps(rows []planRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		resolvedRaw := ""
		if r.ResolvedRaw != 0 {
			resolvedRaw = strconv.FormatInt(r.ResolvedRaw, 10)
		}
		out = append(out, []string{
			r.Entity, r.EntityStatus, r.Requested,
			r.Resolved, resolvedRaw, r.DateSource, r.Status,
		})
	}
	return out
}

// planTableHeaders names the columns a plan prints: the mailbox and its own
// status on the task, the date asked for, the backup that resolved to in
// both ISO and the raw yyyyMMddHHmmss integer the restore body carries, and
// where that date came from.
var planTableHeaders = []string{"entity", "entity_status", "requested", "resolved", "resolved_raw", "source", "status"}

func countPlanStatus(rows []planRow, status string) int {
	n := 0
	for _, r := range rows {
		if r.Status == status {
			n++
		}
	}
	return n
}

func writePlanFile(path string, plan restorePlan) error {
	b, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), planFilePerm)
}

func readPlanFile(path string) (*restorePlan, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- user-supplied plan path.
	if err != nil {
		return nil, usageErr(fmt.Errorf("--plan-file: %w", err))
	}
	var plan restorePlan
	if err := json.Unmarshal(b, &plan); err != nil {
		return nil, usageErr(fmt.Errorf("--plan-file: %w", err))
	}
	if plan.TaskID == "" || plan.Source == "" {
		return nil, usageErr(errors.New("--plan-file: task_id and source are required; write the file with 'restore plan --plan-out'"))
	}
	for i, r := range plan.Rows {
		if r.Status != planOK {
			continue
		}
		if strings.TrimSpace(r.Entity) == "" {
			return nil, usageErr(fmt.Errorf("--plan-file: row %d is OK but has no entity", i+1))
		}
		if _, err := cadate.ParseCompact(r.ResolvedRaw); err != nil {
			return nil, usageErr(fmt.Errorf("--plan-file: row %d (%s) has an invalid resolved_raw: %w", i+1, r.Entity, err))
		}
	}
	return &plan, nil
}

func newRestorePlanCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagTask string
	var flagDates []string
	var flagEntities []string
	var flagEntitiesFile string
	var flagSubSource string
	var flagPlanOut string
	var flagResolve, flagPortalURL string
	var flagLookback int

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview which backup each mailbox would be restored from for the given dates",
		Long: strings.Trim(`
Use this command to preview which (mailbox, date) restores a batch would
submit. Do NOT use it to submit; use 'restore submit' instead.

For a past date, each mailbox's backup is resolved from the customer portal's
own per-mailbox backup calendar, which lists every past run. The documented
API reports only the most recent backup per mailbox, so it cannot answer
"which backup was taken on or before 2025-12-31" by itself. Reading the
calendar needs a partner login and --account <id>; --resolve chooses the
source and the plan records it per row.

A mailbox with no successful backup at or before the date stays in the plan
as a NO_BACKUP row with no date, and 'restore submit' excludes it. Nothing
is ever silently resolved to a different backup. ARCHIVED and ON_PREMISE
entities stay listed with their own status.

--timeout bounds each API and portal call, not the plan, so a task with
hundreds of mailboxes resolves in full.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli restore plan --task 64b0e9 --date 2026-10-01
  cloudally-cli restore plan --task 64b0e9 --date 2026-10-01 --date 2026-10-03 --entities jane.doe@contoso.com
  cloudally-cli restore plan --task 64b0e9 --date 2026-10-01T08:00 --sub-source mail --plan-out plan.json
  cloudally-cli restore plan --task 64b0e9 --date 2026-10-01 --json
  # partner login (auth login --partner): name the customer account
  cloudally-cli restore plan --account 5f1c2a --task 64b0e9 --date 2026-10-01
  # end-of-year restore point, resolved from the portal's backup calendar
  cloudally-cli restore plan --account 5f1c2a --task 64b0e9 --date 2025-12-31
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":   "true",
			"mcp:write-flags": "plan-out entities-file",
			"pp:data-source":  "live",
			"pp:happy-args":   "--task=64b0e9;--date=2026-10-01",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hasChangedLocalFlags(cmd) && len(args) == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if err := scope.validate(); err != nil {
				return err
			}
			if strings.TrimSpace(flagTask) == "" {
				return usageErr(errors.New("--task <backup task id> is required"))
			}
			resolveMode, err := validateResolveMode(flagResolve, scope)
			if err != nil {
				return err
			}
			dates, err := parseRequestedDates(flagDates)
			if err != nil {
				return err
			}
			filter, requested, err := parseEntityFilter(flagEntities, flagEntitiesFile)
			if err != nil {
				return err
			}
			taskPath := scope.taskPath(flagTask)
			if restoreShortCircuit(flags) {
				would := fmt.Sprintf("GET %s and resolve %d date(s) for %s", taskPath, len(dates), describeEntities(requested))
				return writeWould(cmd.OutOrStdout(), flags, "restore plan", would, map[string]any{"path": taskPath, "dates": flagDates})
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// No overall deadline: --timeout bounds each API and portal
			// call (the clients apply it per request), so resolving a large
			// task's mailboxes against the backup calendar is not cut off
			// part way.
			ctx := cmd.Context()
			task, err := fetchTaskWithEntities(ctx, c, taskPath)
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
					// Under --resolve snapshots the portal is the point of
					// the run, so a failure is fatal rather than a quiet
					// downgrade to the latest-backup dates.
					if resolveMode == resolveSnapshots {
						return fmt.Errorf("opening the portal backup calendar: %w", rerr)
					}
					termf(cmd.ErrOrStderr(), "warning: the portal backup calendar is unavailable (%v); resolving from the latest backup dates instead, which cannot find a backup before the most recent one\n", rerr)
				} else {
					resolver = r
				}
			}
			rows, warnings := buildPlanWithResolver(ctx, task, dates, filter, flagSubSource, resolver)
			for _, w := range warnings {
				termf(cmd.ErrOrStderr(), "warning: %v\n", w)
			}
			plan := restorePlan{Scope: scope.kind(), AccountID: scope.account, TaskID: flagTask, Source: task.Source, SubSource: flagSubSource, Resolve: resolveMode, Rows: rows}
			for _, d := range dates {
				plan.Dates = append(plan.Dates, d.text)
			}
			if flagPlanOut != "" {
				if err := writePlanFile(flagPlanOut, plan); err != nil {
					return fmt.Errorf("writing --plan-out: %w", err)
				}
				termf(cmd.ErrOrStderr(), "plan written to %s\n", flagPlanOut)
			}
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				if len(rows) == 0 {
					termf(cmd.OutOrStdout(), "no entities matched on task %s (%s)\n", flagTask, task.Source)
					return nil
				}
				if err := printRestoreTable(cmd.OutOrStdout(), planTableHeaders, planRowsAsMaps(rows)); err != nil {
					return err
				}
				termf(cmd.OutOrStdout(), "\n%d row(s): %d restorable, %d with no backup at or before the date. Submit with: cloudally-cli restore submit%s --task %s --date %s --yes\n",
					len(rows), countPlanStatus(rows, planOK), countPlanStatus(rows, planNoBackup), scopeFlagText(scope), flagTask, strings.Join(plan.Dates, " --date "))
				if n := countPlanStatus(rows, planNoBackup); n > 0 {
					termf(cmd.OutOrStdout(), "%d NO_BACKUP row(s) carry no date and are excluded from submission; run with --json to see why each one failed to resolve.\n", n)
				}
				return nil
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagTask, "task", "", "Backup task id whose entities will be restored")
	cmd.Flags().StringArrayVar(&flagDates, "date", nil, restoreDateFlagHelp)
	cmd.Flags().StringSliceVar(&flagEntities, "entities", nil, "Comma-separated entity names (mailbox, site, drive) to include; default is every entity on the task")
	cmd.Flags().StringVar(&flagEntitiesFile, "entities-file", "", "File with one entity per line, or a CSV whose first column is the entity")
	cmd.Flags().StringVar(&flagSubSource, "sub-source", "", "Restrict backup lookup to one lastBackupDates key (for example mail); default is the newest across all")
	cmd.Flags().StringVar(&flagPlanOut, "plan-out", "", "Write the plan JSON to this path for 'restore submit --plan-file'")
	addResolveFlags(cmd, &flagResolve, &flagPortalURL, &flagLookback)
	return cmd
}

func describeEntities(requested []string) string {
	if len(requested) == 0 {
		return "every entity on the task"
	}
	return fmt.Sprintf("%d listed entit(ies)", len(requested))
}

func scopeFlagText(s restoreScope) string {
	if s.account == "" {
		return ""
	}
	out := " --account " + s.account
	if s.reseller {
		out += " --reseller"
	}
	return out
}
