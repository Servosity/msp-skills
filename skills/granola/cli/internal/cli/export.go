// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

// meetingArtifacts is the three-stream payload for one meeting. The
// three fields are deliberately kept separate; the export/extract
// commands stitch them into labeled H2 sections at write time.
type meetingArtifacts struct {
	Doc          *granola.Document
	Metadata     *granola.MeetingMetadata
	NotesHuman   string
	PanelSummary string
	PanelMap     map[string]string
	// PanelFetchErr is set when the live summary fetch FAILED, so the export
	// can say so instead of claiming no summary exists.
	PanelFetchErr string
	Transcript    []granola.TranscriptSegment
}

// buildArtifacts gathers human notes + AI panels + transcript for one
// doc id, preferring the local cache and falling back to live API for
// panels and transcripts.
func buildArtifacts(id string, allowLive bool, panelTemplate string) (*meetingArtifacts, error) {
	c, err := openGranolaCache()
	if err != nil {
		return nil, err
	}
	d := c.DocumentByID(id)
	if d == nil {
		return nil, notFoundErr(fmt.Errorf("meeting %s not in cache", id))
	}
	a := &meetingArtifacts{Doc: d, Metadata: c.MeetingMetadataByID(id)}

	// Stream 1: human notes.
	if len(d.Notes) > 0 {
		if md, err := granola.Render(d.Notes); err == nil {
			a.NotesHuman = strings.TrimSpace(md)
		}
	}
	if a.NotesHuman == "" {
		a.NotesHuman = strings.TrimSpace(d.NotesMarkdown)
	}
	if a.NotesHuman == "" {
		a.NotesHuman = strings.TrimSpace(d.NotesPlain)
	}

	// Stream 2: AI panel/summary. On v7.4x+ the internal named-panel endpoint is
	// sealed; the public API exposes the AI summary, which we fetch for not_ ids.
	// Hand-wired (handfixes.json: export-summary-stored-locally): the summary
	// hydration already fetched is read from the store first, so a local
	// export carries it; a live fetch fills the gap, and a FAILED live fetch
	// is reported as a failure rather than rendered as "no AI panel".
	if md := storedSummary(id); md != "" {
		a.PanelMap = map[string]string{"summary": md}
	}
	if a.PanelMap == nil && allowLive && strings.HasPrefix(id, "not_") {
		panels, perr := restSummaryPanel(context.Background(), id)
		var ce *cliError
		if perr == nil {
			a.PanelMap = panels
		} else if !(errors.As(perr, &ce) && ce.code == 3) {
			// code 3 = the API answered and the meeting has no summary: an
			// authoritative absence, not a failure.
			a.PanelFetchErr = perr.Error()
			stderr("note: AI summary for %s could not be fetched: %v", id, perr)
		}
	}
	if a.PanelMap != nil {
		if panelTemplate != "" {
			a.PanelSummary = a.PanelMap[panelTemplate]
		} else {
			a.PanelSummary = bestPanel(a.PanelMap)
		}
	}

	// Stream 3: transcript — cache (REST-fed by enrich) first, then a live REST
	// fetch for not_ ids. The dead internal API is no longer attempted.
	a.Transcript = c.TranscriptByID(id)
	if len(a.Transcript) == 0 && allowLive && strings.HasPrefix(id, "not_") {
		if segs, terr := fetchTranscriptREST(id); terr == nil {
			a.Transcript = segs
		}
	}
	return a, nil
}

// bestPanel picks a panel for default "AI Summary" rendering. Priority:
// summary, executive_summary, action_items, then the first lex-sorted key.
func bestPanel(panels map[string]string) string {
	for _, k := range []string{"summary", "executive-summary", "executive_summary", "meeting-summary", "action-items", "action_items"} {
		if v, ok := panels[k]; ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	keys := make([]string, 0, len(panels))
	for k := range panels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.TrimSpace(panels[k]) != "" {
			return panels[k]
		}
	}
	return ""
}

func newExportCmd(flags *rootFlags) *cobra.Command {
	var out string
	var panelTemplate string
	cmd := &cobra.Command{
		Use:   "export <id>",
		Short: "Export one meeting as a combined three-stream markdown file",
		Long: `Writes a single markdown file with three labeled H2 sections — human
notes, AI summary, and transcript — kept distinctly so downstream agents
can route each stream.`,
		Example: strings.Trim(`
  granola-cli export not_06Yq6JtogRihEr --out ./meeting.md
  granola-cli export not_06Yq6JtogRihEr -o ./meeting.md --panel summary`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || out == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			id := args[0]
			if err := validateNoteIDForFilename(id); err != nil {
				return err
			}
			a, err := buildArtifacts(id, flags.dataSource != "local", panelTemplate)
			if err != nil {
				return err
			}
			body := composeFullMarkdown(a)
			// --out is a user-chosen file path; write owner-only 0600 (and
			// chmod an existing file, which WriteFile would leave world-readable).
			if err := writeSafeFile(out, []byte(body)); err != nil {
				return ioErr(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), `{"exported":true,"id":%q,"path":%q,"bytes":%d}`+"\n", id, out, len(body))
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "Output markdown file path")
	cmd.Flags().StringVar(&panelTemplate, "panel", "", "Panel template slug to use for AI Summary (default: best available)")
	return cmd
}

// composeFullMarkdown stitches the three streams into the canonical
// MEMO file layout. The H2 section headings are stable and load-bearing
// for downstream agents.
func composeFullMarkdown(a *meetingArtifacts) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(a.Doc.Title)
	b.WriteString("\n\n")
	writeMetadataBlock(&b, a)
	b.WriteString("\n## Notes (Human)\n\n")
	if a.NotesHuman != "" {
		b.WriteString(a.NotesHuman)
		b.WriteString("\n")
	} else {
		b.WriteString("_(no human notes recorded)_\n")
	}
	b.WriteString("\n## AI Summary\n\n")
	if a.PanelSummary != "" {
		b.WriteString(strings.TrimSpace(a.PanelSummary))
		b.WriteString("\n")
	} else {
		b.WriteString(noSummaryLine(a))
	}
	b.WriteString("\n## Transcript\n\n")
	if len(a.Transcript) > 0 {
		for _, s := range a.Transcript {
			fmt.Fprintf(&b, "[%s] %s\n", s.Source, s.Text)
		}
	} else {
		b.WriteString("_(no transcript available)_\n")
	}
	return b.String()
}

func writeMetadataBlock(b *strings.Builder, a *meetingArtifacts) {
	fmt.Fprintf(b, "**Meeting ID:** %s\n\n", a.Doc.ID)
	if a.Doc.CreatedAt != "" {
		fmt.Fprintf(b, "**Created:** %s\n\n", a.Doc.CreatedAt)
	}
	// Attendees union: prefer meetingsMetadata; fall back to doc.people.
	var attendees []granola.CalendarInvitee
	if a.Metadata != nil {
		attendees = a.Metadata.Attendees
	} else if a.Doc.People != nil {
		for _, p := range a.Doc.People.Attendees {
			attendees = append(attendees, granola.CalendarInvitee{Name: p.Name, Email: p.Email, ResponseStatus: p.ResponseStatus})
		}
	}
	if len(attendees) > 0 {
		b.WriteString("**Attendees:**\n")
		for _, a := range attendees {
			if a.Email == "" && a.Name == "" {
				continue
			}
			if a.Name != "" {
				fmt.Fprintf(b, "- %s <%s>\n", a.Name, a.Email)
			} else {
				fmt.Fprintf(b, "- %s\n", a.Email)
			}
		}
		b.WriteString("\n")
	}
	if a.Doc.GoogleCalendarEvent != nil {
		if a.Doc.GoogleCalendarEvent.HtmlLink != "" {
			fmt.Fprintf(b, "**Calendar:** %s\n\n", a.Doc.GoogleCalendarEvent.HtmlLink)
		} else if a.Doc.GoogleCalendarEvent.ID != "" {
			fmt.Fprintf(b, "**Calendar Event:** %s\n\n", a.Doc.GoogleCalendarEvent.ID)
		}
	}
}

// ioErr wraps an OS error with the canonical I/O exit code 1.
func ioErr(err error) error {
	return &cliError{code: 1, err: fmt.Errorf("io: %w", err)}
}

// noSummaryLine renders the AI Summary placeholder, distinguishing a fetch
// that failed from a meeting that has no summary.
func noSummaryLine(a *meetingArtifacts) string {
	if a.PanelFetchErr != "" {
		return "_(AI summary could not be fetched: " + a.PanelFetchErr + ")_\n"
	}
	return "_(no AI panel available)_\n"
}

// storedSummary returns the AI summary hydration saved for id (markdown
// preferred), or "" when none is stored or the store predates the table.
func storedSummary(id string) string {
	s, err := openGranolaStoreRead(context.Background())
	if err != nil || s == nil {
		return ""
	}
	defer s.Close()
	var md, txt sql.NullString
	if err := s.DB().QueryRow(`SELECT summary_markdown, summary_text FROM meeting_summaries WHERE meeting_id = ?`, id).Scan(&md, &txt); err != nil {
		return ""
	}
	if strings.TrimSpace(md.String) != "" {
		return md.String
	}
	return txt.String
}
