// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloudally-pp-cli/internal/portal"
	"github.com/spf13/cobra"
)

// monthFlagLayouts are the forms --month accepts.
var monthFlagLayouts = []string{"2006-01", "200601", "2006-1"}

// snapshotRow is one backup run as this command reports it.
type snapshotRow struct {
	Entity     string            `json:"entity"`
	Date       string            `json:"date"`
	DateRaw    int64             `json:"date_raw"`
	Succeeded  bool              `json:"succeeded"`
	Status     string            `json:"status"`
	Source     string            `json:"source"`
	BatchID    string            `json:"batch_id,omitempty"`
	SubSources map[string]string `json:"sub_sources,omitempty"`
}

var snapshotTableHeaders = []string{"date", "date_raw", "status", "source", "batch_id"}

func snapshotRowsAsMaps(rows []snapshotRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{
			r.Date, strconv.FormatInt(r.DateRaw, 10), r.Status, r.Source, r.BatchID,
		})
	}
	return out
}

// parseMonthFlag parses --month into a year and month.
func parseMonthFlag(s string) (int, time.Month, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, usageErr(errors.New("--month <YYYY-MM> is required"))
	}
	for _, layout := range monthFlagLayouts {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.Year(), t.Month(), nil
		}
	}
	return 0, 0, usageErr(fmt.Errorf("--month %q: want YYYY-MM", s))
}

// snapshotRowsFrom converts calendar rows into report rows, newest first,
// keeping only the requested month unless all is set.
func snapshotRowsFrom(entity string, snaps []portal.Snapshot, year int, month time.Month, all bool) []snapshotRow {
	var rows []snapshotRow
	for _, s := range snaps {
		if !all && !portal.InMonth(s, year, month) {
			continue
		}
		t, err := s.Time()
		if err != nil {
			continue
		}
		row := snapshotRow{
			Entity:    entity,
			Date:      isoUTC(t),
			DateRaw:   s.DateTime,
			Succeeded: s.Succeeded(),
			Status:    s.Status,
			Source:    s.Source,
			BatchID:   s.BatchID,
		}
		if len(s.SubSources) > 0 {
			row.SubSources = map[string]string{}
			for _, sub := range s.SubSources {
				if st, serr := sub.Time(); serr == nil {
					row.SubSources[sub.Source] = isoUTC(st)
				}
			}
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].DateRaw > rows[j].DateRaw })
	return rows
}

func newBackupSnapshotsCmd(flags *rootFlags) *cobra.Command {
	var scope restoreScope
	var flagTask, flagEntity, flagExtID, flagMonth, flagPortalURL string
	var flagAll bool

	cmd := &cobra.Command{
		Use:   "snapshots",
		Short: "List one mailbox's backups for a month from the portal's backup calendar",
		Long: strings.Trim(`
Lists every backup run the customer portal's backup calendar holds for one
mailbox in one month, so a restore point can be checked before anything is
restored. This is the only source that knows about past backups: the
documented API reports just the most recent one per mailbox.

Reading it needs a partner login (auth login --partner) and --account <id>,
which is how a partner reaches a customer's portal. The command is read-only.

One request covers the named month and the month before it; --all shows both
instead of trimming to the month asked for.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli backup snapshots --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --month 2025-12
  cloudally-cli backup snapshots --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --month 2025-12 --json
  cloudally-cli backup snapshots --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --month 2025-12 --all
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":  "true",
			"pp:data-source": "live",
			"pp:happy-args":  "--account=5f1c2a;--task=64b0e9;--entity=jane.doe@contoso.com;--month=2025-12",
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
			if strings.TrimSpace(flagEntity) == "" {
				return usageErr(errors.New("--entity <mailbox> is required"))
			}
			if scope.account == "" {
				return usageErr(errPortalNeedsAccount)
			}
			year, month, err := parseMonthFlag(flagMonth)
			if err != nil {
				return err
			}
			if restoreShortCircuit(flags) {
				would := fmt.Sprintf("read the portal backup calendar for %s on task %s for %04d-%02d", flagEntity, flagTask, year, int(month))
				return writeWould(cmd.OutOrStdout(), flags, "backup snapshots", would, map[string]any{
					"task": flagTask, "entity": flagEntity, "month": fmt.Sprintf("%04d-%02d", year, int(month)),
				})
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			// The calendar is keyed by the entity's external id and answers
			// a request without one with an empty list, not an error, so a
			// missing id would read as "no backups". Look it up from the
			// task rather than make the operator supply it.
			extID := strings.TrimSpace(flagExtID)
			if extID == "" {
				task, terr := fetchTaskWithEntities(ctx, c, scope.taskPath(flagTask))
				if terr != nil {
					return classifyAPIError(cmd.OutOrStdout(), terr, flags)
				}
				ent, ok := findTaskEntity(task, flagEntity)
				if !ok {
					return notFoundErr(fmt.Errorf("entity %q is not on task %s; check the name with 'restore plan --task %s --date <date>'", flagEntity, flagTask, flagTask))
				}
				extID = strings.TrimSpace(string(ent.ExtID))
				if extID == "" {
					return notFoundErr(fmt.Errorf("task %s carries no external id for entity %q, so its backup calendar cannot be read; this is not a statement that no backup exists", flagTask, flagEntity))
				}
				// Use the task's own spelling of the entity name.
				flagEntity = ent.Entity
			}
			sess, err := openPortalSession(ctx, c, scope, flagPortalURL)
			if err != nil {
				return err
			}
			snaps, err := sess.SnapshotsCalendar(ctx, portal.Entity{
				TaskID:    flagTask,
				AccountID: flagEntity,
				ExtID:     extID,
			}, year, month)
			if err != nil {
				return err
			}
			rows := snapshotRowsFrom(flagEntity, snaps, year, month, flagAll)
			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				if len(rows) == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "no backups for %s in %04d-%02d\n", flagEntity, year, int(month))
					return nil
				}
				if err := printRestoreTable(cmd.OutOrStdout(), snapshotTableHeaders, snapshotRowsAsMaps(rows)); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d backup(s) for %s; %d succeeded\n",
					len(rows), flagEntity, countSucceededSnapshots(rows))
				return nil
			}
			return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
		},
	}
	addRestoreScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&flagTask, "task", "", "Backup task id the mailbox belongs to")
	cmd.Flags().StringVar(&flagEntity, "entity", "", "Mailbox (or site, or drive) to list backups for")
	cmd.Flags().StringVar(&flagExtID, "ext-id", "", "Entity external id; looked up from the task when omitted")
	cmd.Flags().StringVar(&flagMonth, "month", "", "Month to list, as YYYY-MM (UTC)")
	cmd.Flags().BoolVar(&flagAll, "all", false, "Keep every row the portal returns, including the month before the one asked for")
	cmd.Flags().StringVar(&flagPortalURL, "portal-url", portal.DefaultBaseURL, portalURLFlagHelp)
	_ = cmd.Flags().MarkHidden("portal-url")
	return cmd
}

func countSucceededSnapshots(rows []snapshotRow) int {
	n := 0
	for _, r := range rows {
		if r.Succeeded {
			n++
		}
	}
	return n
}
