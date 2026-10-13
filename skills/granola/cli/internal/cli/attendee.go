// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

func newAttendeeCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attendee",
		Short: "Cross-meeting attendee queries",
	}
	cmd.AddCommand(newAttendeeTimelineCmd(flags))
	cmd.AddCommand(newAttendeeBriefCmd(flags))
	return cmd
}

func newAttendeeTimelineCmd(flags *rootFlags) *cobra.Command {
	var since, folder string
	var limit int
	cmd := &cobra.Command{
		Use:   "timeline <email-or-name>",
		Short: "Every meeting with a given attendee, ordered oldest -> newest",
		Example: strings.Trim(`
  granola-cli attendee timeline alice@example.com
  granola-cli attendee timeline "Alice Example" --since 90d --limit 20
  granola-cli attendee timeline alice@example.com --since 30d --json`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			key := args[0]
			s, err := openGranolaStoreRead(cmd.Context())
			if err != nil {
				return err
			}
			if s == nil {
				return notFoundErr(fmt.Errorf("no local data — run sync"))
			}
			defer s.Close()
			q := `SELECT m.id, m.title, m.started_at, m.calendar_event_id, m.creation_source
				FROM meetings m
				JOIN attendees a ON a.meeting_id = m.id
				WHERE (a.email LIKE ? OR a.name LIKE ?)`
			args2 := []any{"%" + strings.ToLower(key) + "%", "%" + key + "%"}
			if since != "" {
				t, err := parseSinceOrDate(since, timeNow())
				if err != nil {
					return usageErr(err)
				}
				q += " AND m.started_at >= ?"
				args2 = append(args2, t.UTC().Format("2006-01-02T15:04:05Z"))
			}
			if folder != "" {
				// The public REST /v1/folders endpoint exposes only
				// id/name/object/parent_folder_id - no note->folder
				// membership - so folder_memberships is never populated from
				// REST. Filtering silently against an empty table would return
				// zero rows and read as "this attendee has no meetings in that
				// folder", which is a lie. Fail honestly instead. (The corrected
				// query below still works if a future membership source lands.)
				var hasMembership int
				if err := s.DB().QueryRowContext(cmd.Context(),
					`SELECT EXISTS(SELECT 1 FROM folder_memberships)`).Scan(&hasMembership); err != nil {
					return err
				}
				if hasMembership == 0 {
					return apiErr(fmt.Errorf("folder filtering is unavailable: the public Granola REST API does not expose folder membership (only folder id/name), so --folder cannot be honored"))
				}
				// The REST-populated folders table uses `name` (not `title`).
				q += ` AND m.id IN (
					SELECT fm.meeting_id FROM folder_memberships fm
					JOIN folders f ON f.id = fm.folder_id
					WHERE f.name = ? OR f.id = ?
				)`
				args2 = append(args2, folder, folder)
			}
			q += " ORDER BY m.started_at ASC"
			if limit > 0 {
				q += fmt.Sprintf(" LIMIT %d", limit)
			}
			rows, err := s.DB().Query(q, args2...)
			if err != nil {
				return err
			}
			defer rows.Close()
			out := []map[string]any{}
			for rows.Next() {
				var id, title, started, cal, source string
				if err := rows.Scan(&id, &title, &started, &cal, &source); err != nil {
					return err
				}
				out = append(out, map[string]any{
					"id":                id,
					"title":             title,
					"started_at":        started,
					"calendar_event_id": cal,
					"creation_source":   source,
				})
			}
			return emitJSON(cmd, flags, out)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "Lower-bound date")
	cmd.Flags().StringVar(&folder, "folder", "", "Restrict to a folder (id or name); unavailable on the public REST API, which exposes no folder membership")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max rows")
	return cmd
}

func newAttendeeBriefCmd(flags *rootFlags) *cobra.Command {
	var last int
	var panel string
	cmd := &cobra.Command{
		Use:   "brief <email-or-name>",
		Short: "Last N meetings with an attendee, with real notes + AI panel inlined",
		Example: `  # Last 3 meetings with a teammate, full notes + AI summary
  granola-cli attendee brief alice@example.com

  # Override the default panel and bump the count
  granola-cli attendee brief "Alice Roe" --last 5 --panel exec-summary

  # JSON for downstream LLM context
  granola-cli attendee brief alice@example.com --json`,
		Annotations: map[string]string{
			"mcp:read-only": "true",
			// Free-text attendee lookup: an unknown identifier is a valid-but-empty
			// result (exit 0), indistinguishable from a bad argument without
			// inventing lookup semantics. Opt out of dogfood's invalid-arg probe.
			"pp:no-error-path-probe": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if last <= 0 {
				last = 3
			}
			key := args[0]
			s, err := openGranolaStoreRead(cmd.Context())
			if err != nil {
				return err
			}
			if s == nil {
				return notFoundErr(fmt.Errorf("no local data"))
			}
			defer s.Close()
			rows, err := s.DB().Query(`SELECT m.id, m.title, m.started_at
				FROM meetings m
				JOIN attendees a ON a.meeting_id = m.id
				WHERE (a.email LIKE ? OR a.name LIKE ?)
				ORDER BY m.started_at DESC
				LIMIT ?`,
				"%"+strings.ToLower(key)+"%", "%"+key+"%", last)
			if err != nil {
				return err
			}
			var ids []string
			titles := map[string]string{}
			starteds := map[string]string{}
			for rows.Next() {
				var id, title, started string
				if err := rows.Scan(&id, &title, &started); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
				titles[id] = title
				starteds[id] = started
			}
			rows.Close()
			w := cmd.OutOrStdout()
			for _, id := range ids {
				a, err := buildArtifacts(id, flags.dataSource != "local", panel)
				if err != nil {
					_ = emitNDJSONLine(w, map[string]any{"id": id, "title": titles[id], "started_at": starteds[id], "error": err.Error()})
					continue
				}
				_ = emitNDJSONLine(w, map[string]any{
					"id":                  id,
					"title":               titles[id],
					"started_at":          starteds[id],
					"notes_human":         a.NotesHuman,
					"panel_summary":       a.PanelSummary,
					"panel_template_used": panel,
				})
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&last, "last", 3, "Number of most recent meetings to include")
	cmd.Flags().StringVar(&panel, "panel", "", "Panel template slug to inline")
	return cmd
}

// Ensure granola/time imports used.
var (
	_ = time.Now
	_ = granola.ParseISO
)
