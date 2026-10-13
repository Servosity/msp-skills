// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

func newExtractCmd(flags *rootFlags) *cobra.Command {
	var outDir string
	var panelTemplate string
	cmd := &cobra.Command{
		Use:   "extract <id>",
		Short: "Extract one meeting into the three MEMO files (full/summary/metadata)",
		Long: `Writes three files into the chosen directory, preserving the
granola.py MEMO contract:

  full_<id>.md      — title + metadata + Notes (Human) + AI Summary + Transcript
  summary_<id>.md   — title + metadata + Notes (Human) + AI Summary (no transcript)
  metadata_<id>.md  — YAML-shaped frontmatter only

Exit codes: 0 success, 1 IO error, 2 missing transcript, 3 duplicate.`,
		Example: strings.Trim(`
  granola-cli extract not_06Yq6JtogRihEr --out ./memos
  granola-cli extract not_06Yq6JtogRihEr -o ./memos --panel summary`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only":       "true",
			"pp:typed-exit-codes": "0,1,2,3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || outDir == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			id := args[0]
			res, err := runExtract(id, outDir, panelTemplate, flags.dataSource != "local")
			if err != nil {
				return err
			}
			return emitJSON(cmd, flags, res)
		},
	}
	cmd.Flags().StringVarP(&outDir, "out", "o", "", "Output directory (creates if missing)")
	cmd.Flags().StringVar(&panelTemplate, "panel", "", "Panel template slug for AI Summary (default: best available)")
	return cmd
}

type extractResult struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Files []string `json:"files"`
	Bytes int      `json:"bytes"`
}

// runExtract produces the three MEMO files. Used by extract.go and
// memo.go.
func runExtract(id, outDir, panelTemplate string, allowLive bool) (extractResult, error) {
	res := extractResult{ID: id}
	if err := validateNoteIDForFilename(id); err != nil {
		return res, err
	}
	if err := ensureSafeDir(outDir); err != nil {
		return res, ioErr(err)
	}
	fullPath, err := safeJoinContained(outDir, "full_"+id+".md")
	if err != nil {
		return res, err
	}
	if _, err := os.Stat(fullPath); err == nil {
		return res, &cliError{code: 3, err: fmt.Errorf("full_%s.md already exists in %s", id, outDir)}
	}
	a, err := buildArtifacts(id, allowLive, panelTemplate)
	if err != nil {
		return res, err
	}
	res.Title = a.Doc.Title
	if len(a.Transcript) == 0 {
		return res, &cliError{code: 2, err: fmt.Errorf("no transcript available for %s", id)}
	}

	full := composeFullMarkdown(a)
	summary := composeSummaryMarkdown(a)
	meta := composeMetadataMarkdown(a)
	summaryPath, err := safeJoinContained(outDir, "summary_"+id+".md")
	if err != nil {
		return res, err
	}
	metaPath, err := safeJoinContained(outDir, "metadata_"+id+".md")
	if err != nil {
		return res, err
	}

	outputs := []struct {
		path    string
		content string
	}{
		// full_<id>.md is the completion marker (preflight and memo look for
		// it), so it is listed LAST and published last.
		{summaryPath, summary},
		{metaPath, meta},
		{fullPath, full},
	}
	// Hand-wired (handfixes.json: extract-triple-all-or-nothing): the triple
	// is all-or-nothing. Every target is checked first; each file is written
	// to a private temporary name beside its target; only when all three
	// are fully written are they renamed into place, full_<id>.md last. A
	// failure at any step removes the temporaries and any target this run
	// created, so a half-written triple can never block the retry.
	for _, o := range outputs {
		if fi, err := os.Lstat(o.path); err == nil && (fi.IsDir() || fi.Mode()&os.ModeSymlink != 0) {
			return res, ioErr(fmt.Errorf("refusing to write %s: it is a directory or a symlink", o.path))
		}
	}
	temps := make([]string, len(outputs))
	cleanupTemps := func() {
		for _, tmp := range temps {
			if tmp != "" {
				_ = os.Remove(tmp)
			}
		}
	}
	for i, o := range outputs {
		suffix := make([]byte, 6)
		_, _ = rand.Read(suffix)
		tmp := filepath.Join(filepath.Dir(o.path), "."+filepath.Base(o.path)+".tmp-"+hex.EncodeToString(suffix))
		temps[i] = tmp
		if err := writeSafeFile(tmp, []byte(o.content)); err != nil {
			cleanupTemps()
			return extractResult{ID: id, Title: res.Title}, ioErr(err)
		}
	}
	var published []string
	for i, o := range outputs {
		_, statErr := os.Lstat(o.path)
		if err := os.Rename(temps[i], o.path); err != nil {
			cleanupTemps()
			for _, p := range published {
				_ = os.Remove(p)
			}
			return extractResult{ID: id, Title: res.Title}, ioErr(err)
		}
		temps[i] = ""
		if os.IsNotExist(statErr) {
			published = append(published, o.path)
		}
	}
	for _, o := range []struct {
		path    string
		content string
	}{{fullPath, full}, {summaryPath, summary}, {metaPath, meta}} {
		res.Files = append(res.Files, o.path)
		res.Bytes += len(o.content)
	}
	return res, nil
}

func composeSummaryMarkdown(a *meetingArtifacts) string {
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
	return b.String()
}

func composeMetadataMarkdown(a *meetingArtifacts) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", a.Doc.ID)
	fmt.Fprintf(&b, "title: %s\n", yamlEscape(a.Doc.Title))
	startedAt, endedAt := docWindow(a.Doc, a.Transcript)
	fmt.Fprintf(&b, "started_at: %s\n", startedAt)
	fmt.Fprintf(&b, "ended_at: %s\n", endedAt)
	if dur := durationMinutes(startedAt, endedAt); dur > 0 {
		fmt.Fprintf(&b, "duration_min: %d\n", dur)
	}
	if a.Doc.WorkspaceID != "" {
		fmt.Fprintf(&b, "workspace_id: %s\n", a.Doc.WorkspaceID)
	}
	if a.Doc.GoogleCalendarEvent != nil {
		if link := a.Doc.GoogleCalendarEvent.HtmlLink; link != "" {
			fmt.Fprintf(&b, "calendar_event_url: %s\n", link)
		}
	}
	if a.Doc.CreationSource != "" {
		fmt.Fprintf(&b, "creation_source: %s\n", a.Doc.CreationSource)
	}
	// Attendees
	b.WriteString("attendees:\n")
	var atts []granola.CalendarInvitee
	if a.Metadata != nil {
		atts = a.Metadata.Attendees
	} else if a.Doc.People != nil {
		for _, p := range a.Doc.People.Attendees {
			atts = append(atts, granola.CalendarInvitee{Name: p.Name, Email: p.Email})
		}
	}
	for _, p := range atts {
		fmt.Fprintf(&b, "  - name: %s\n    email: %s\n", yamlEscape(p.Name), p.Email)
	}
	b.WriteString("recipes_applied: []\n")
	b.WriteString("---\n")
	return b.String()
}

func yamlEscape(s string) string {
	// Quote anything that could change how YAML reads the scalar. Inside a
	// double-quoted scalar a backslash starts an escape, so backslashes are
	// escaped first, then quotes and control characters (a title such as
	// `QBR: C:\Users\Alice` used to produce invalid frontmatter).
	if strings.ContainsAny(s, ":#@\"'\\") || strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range s {
			switch {
			case r == '\\':
				b.WriteString(`\\`)
			case r == '"':
				b.WriteString(`\"`)
			case r == '\n':
				b.WriteString(`\n`)
			case r == '\t':
				b.WriteString(`\t`)
			case r < 0x20 || r == 0x7f:
				fmt.Fprintf(&b, `\x%02x`, r)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		return b.String()
	}
	return s
}

// docWindow derives a meeting's REAL start/end bounds for the metadata block.
// Source precedence is real-sources-only: the meeting's stored calendar window
// (reconstructed into GoogleCalendarEvent.Start/End from meetings.started_at/
// ended_at) first, else the transcript span, else empty. Note created_at/
// updated_at are EDIT times, never the meeting duration — using them would
// re-introduce the fabricated-duration bug the storage layer took care to
// avoid. Date-range FILTERING still uses created_at elsewhere; this function is
// strictly the start/end duration window.
func docWindow(d *granola.Document, segs []granola.TranscriptSegment) (string, string) {
	startedAt, endedAt := "", ""
	if d.GoogleCalendarEvent != nil {
		if s := extractCalTimeRaw(d.GoogleCalendarEvent.Start); s != "" {
			startedAt = s
		}
		if s := extractCalTimeRaw(d.GoogleCalendarEvent.End); s != "" {
			endedAt = s
		}
	}
	if len(segs) > 0 {
		if startedAt == "" {
			startedAt = segs[0].StartTimestamp
		}
		if endedAt == "" {
			endedAt = segs[len(segs)-1].EndTimestamp
		}
	}
	return startedAt, endedAt
}

func extractCalTimeRaw(raw []byte) string {
	// Tiny re-decoder that mirrors granola.extractCalTime; we duplicate
	// instead of exporting to keep the granola package's API surface tight.
	s := string(raw)
	if idx := strings.Index(s, `"dateTime":"`); idx >= 0 {
		rest := s[idx+len(`"dateTime":"`):]
		if end := strings.Index(rest, `"`); end >= 0 {
			return rest[:end]
		}
	}
	if idx := strings.Index(s, `"date":"`); idx >= 0 {
		rest := s[idx+len(`"date":"`):]
		if end := strings.Index(rest, `"`); end >= 0 {
			return rest[:end]
		}
	}
	return ""
}

func durationMinutes(start, end string) int {
	s, err1 := granola.ParseISO(start)
	e, err2 := granola.ParseISO(end)
	if err1 != nil || err2 != nil || s.IsZero() || e.IsZero() {
		return 0
	}
	d := e.Sub(s)
	if d < 0 {
		return 0
	}
	return int(d / time.Minute)
}
