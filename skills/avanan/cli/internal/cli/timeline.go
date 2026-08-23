// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.

// `timeline` — one message's full history.
//
// pp:data-source local
//
// The API models a message as three unlinked objects: an event (the
// detection), an entity (the message), and a task (the outcome of an action).
// Nothing in the API ties a task back to the entity it acted on, so the CLI
// records that link itself when it submits an action (see remediate.go). This
// command is the reason that link is worth persisting.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"avanan-pp-cli/internal/avananmirror"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newNovelTimelineCmd(flags))
	})
}

type timelineEntry struct {
	At      string `json:"at,omitempty"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Source  string `json:"source"`
	Detail  string `json:"detail,omitempty"`
}

type timelineReport struct {
	ID      string          `json:"id"`
	Subject string          `json:"subject,omitempty"`
	Sender  string          `json:"sender,omitempty"`
	Entries []timelineEntry `json:"entries"`
	Note    string          `json:"note,omitempty"`
}

func newNovelTimelineCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "timeline [id]",
		Short: "Reconstruct one message's full history: detection, state changes, actions submitted, task outcomes",
		Long: strings.Trim(`
Assemble everything the local mirror knows about a single message, in order:
the detection event, the entity record, any actions this CLI submitted, and
the task outcomes those actions produced.

Accepts either an entity ID or an event ID.

The API returns no link between a task and the entity it acted on, so the
action and task rows come from what this CLI recorded when it ran
'remediate'. Actions taken in the web portal or by another tool will not
appear.

Use this command to see the full history of one message across detection,
action, and restore. Do NOT use this command to fetch the current record or
decoded body; use 'avanan-search get-saas-entity', 'soar get-entity', or
'event get' instead.
`, "\n"),
		Example: strings.Trim(`
  avanan-cli timeline f05b74da3ee859eea41aeac40aaad3c2
  avanan-cli timeline f05b74da3ee859eea41aeac40aaad3c2 --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:happy-args":       "<id>=f05b74da3ee859eea41aeac40aaad3c2",
			"pp:typed-exit-codes": "0,3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "timeline")
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("an entity ID or event ID is required"))
			}
			if flags.dataSource == "live" {
				return usageErr(fmt.Errorf("timeline has no live equivalent: it merges local event, entity, action, and task rows; run 'mirror' then retry without --data-source live"))
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			id := strings.TrimSpace(args[0])
			empty := timelineReport{ID: id, Entries: []timelineEntry{}}

			db, _, ok, err := openMirror(cmd, ctx, flags, dbPath, empty)
			if err != nil || !ok {
				return err
			}
			defer db.Close()

			hintMirrorFreshness(cmd, db, avananmirror.ResourceEvents, flags.maxAge)

			events, err := loadResources(ctx, db, avananmirror.ResourceEvents)
			if err != nil {
				return err
			}
			entities, err := loadResources(ctx, db, avananmirror.ResourceEntities)
			if err != nil {
				return err
			}
			actions, err := loadResources(ctx, db, avananActionResource)
			if err != nil {
				return err
			}

			report := empty

			for _, ev := range events {
				if ev.Obj == nil {
					continue
				}
				if !matchesID(ev, id) {
					continue
				}
				ts := avananTime(ev.Obj)
				report.Entries = append(report.Entries, timelineEntry{
					At:   formatTS(ts),
					Kind: "detection",
					Summary: fmt.Sprintf("%s detected (state: %s)",
						orDefault(str(ev.Obj, eventTypeKeys...), "event"),
						orDefault(str(ev.Obj, stateKeys...), "unknown")),
					Source: "event",
					Detail: str(ev.Obj, severityKeys...),
				})
				if report.Subject == "" {
					report.Subject = str(ev.Obj, subjectKeys...)
				}
				if report.Sender == "" {
					report.Sender = str(ev.Obj, senderKeys...)
				}
			}

			for _, en := range entities {
				if en.Obj == nil || !matchesID(en, id) {
					continue
				}
				ts := avananTime(en.Obj)
				report.Entries = append(report.Entries, timelineEntry{
					At:   formatTS(ts),
					Kind: "message",
					Summary: fmt.Sprintf("message on %s from %s",
						orDefault(str(en.Obj, saasKeys...), "unknown platform"),
						orDefault(str(en.Obj, senderKeys...), "unknown sender")),
					Source: "entity",
					Detail: str(en.Obj, subjectKeys...),
				})
				if report.Subject == "" {
					report.Subject = str(en.Obj, subjectKeys...)
				}
				if report.Sender == "" {
					report.Sender = str(en.Obj, senderKeys...)
				}
			}

			for _, act := range actions {
				if act.Obj == nil {
					continue
				}
				if !actionTouchesID(act.Obj, id) {
					continue
				}
				report.Entries = append(report.Entries, timelineEntry{
					At:   str(act.Obj, "submitted_at"),
					Kind: "action",
					Summary: fmt.Sprintf("%s submitted (task %s)",
						orDefault(str(act.Obj, "action"), "action"),
						orDefault(str(act.Obj, "task_id"), "unknown")),
					Source: "local action log",
					Detail: str(act.Obj, "scope"),
				})
				if outcome := str(act.Obj, "outcome"); outcome != "" {
					report.Entries = append(report.Entries, timelineEntry{
						At:      str(act.Obj, "completed_at"),
						Kind:    "task",
						Summary: fmt.Sprintf("task %s finished: %s", orDefault(str(act.Obj, "task_id"), "?"), outcome),
						Source:  "local action log",
					})
				}
			}

			// Undated entries sort last rather than first: an unknown time is
			// not the beginning of time.
			sort.SliceStable(report.Entries, func(i, j int) bool {
				ai, aj := report.Entries[i].At, report.Entries[j].At
				if (ai == "") != (aj == "") {
					return aj == ""
				}
				return ai < aj
			})

			if len(report.Entries) == 0 {
				report.Note = fmt.Sprintf(
					"no mirrored event, entity, or recorded action matches %q. Run 'avanan-cli mirror --since 30d' to widen the local window, or check the ID.", id)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), report, flags); err != nil {
					return err
				}
				if len(report.Entries) == 0 {
					return &cliError{code: 3, err: fmt.Errorf("no history found for %s", id)}
				}
				return nil
			}

			if len(report.Entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), report.Note)
				return &cliError{code: 3, err: fmt.Errorf("no history found for %s", id)}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Timeline for %s\n", id)
			if report.Subject != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Subject: %s\n", report.Subject)
			}
			if report.Sender != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Sender:  %s\n", report.Sender)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			for _, e := range report.Entries {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-22s %-10s %s\n", orDefault(e.At, "(undated)"), e.Kind, e.Summary)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func matchesID(row avananRow, id string) bool {
	if strings.EqualFold(row.ID, id) {
		return true
	}
	if row.Obj == nil {
		return false
	}
	for _, k := range append(append([]string{}, entityIDKeys...), eventIDKeys...) {
		if strings.EqualFold(str(row.Obj, k), id) {
			return true
		}
	}
	return false
}

func actionTouchesID(obj map[string]any, id string) bool {
	if strings.EqualFold(str(obj, "entity_id"), id) || strings.EqualFold(str(obj, "task_id"), id) {
		return true
	}
	if ids, ok := obj["entity_ids"].([]any); ok {
		for _, v := range ids {
			if s, ok := v.(string); ok && strings.EqualFold(s, id) {
				return true
			}
		}
	}
	return false
}

func formatTS(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
