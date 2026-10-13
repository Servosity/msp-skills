// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/client"
	"granola-pp-cli/internal/config"
	"granola-pp-cli/internal/granola"
)

func newTranscriptCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transcript",
		Short: "Read a meeting transcript from the cache or live API",
	}
	cmd.AddCommand(newTranscriptGetCmd(flags))
	return cmd
}

func newTranscriptGetCmd(flags *rootFlags) *cobra.Command {
	var speaker bool
	var format, since string
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get the transcript for a meeting",
		Long: `Returns the cached transcript when available, falling back to the
live internal API. --format=json|text|srt. --speaker prefixes lines with
the source (microphone/system). --since 1:30 trims to segments after the
M:SS mark.`,
		Example: `  # Plain transcript text
  granola-cli transcript get ff1186df-593b-4ce5-bb1d-70e265f4a811 --format text

  # Speaker-labeled (microphone vs system)
  granola-cli transcript get ff1186df-593b-4ce5-bb1d-70e265f4a811 --format text --speaker

  # SRT for upload to a captioning tool
  granola-cli transcript get ff1186df-593b-4ce5-bb1d-70e265f4a811 --format srt

  # Skip the first 90 seconds (intros)
  granola-cli transcript get ff1186df-593b-4ce5-bb1d-70e265f4a811 --since 1:30 --format text`,
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
			id := args[0]
			segs, source, err := loadTranscript(id, flags.dataSource)
			if err != nil {
				return err
			}
			if since != "" {
				cut, err := parseClock(since)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --since %q: %w", since, err))
				}
				segs = trimSegmentsAfter(segs, cut)
			}
			switch format {
			case "json", "":
				if flags.asJSON || flags.agent || format == "json" {
					out := map[string]any{
						"document_id": id,
						"source":      source,
						"segments":    segs,
					}
					return emitJSON(cmd, flags, out)
				}
				fallthrough
			case "text":
				w := cmd.OutOrStdout()
				for _, s := range segs {
					if speaker {
						fmt.Fprintf(w, "[%s] %s\n", s.Source, s.Text)
					} else {
						fmt.Fprintf(w, "%s\n", s.Text)
					}
				}
				return nil
			case "srt":
				w := cmd.OutOrStdout()
				// SRT cues are RELATIVE to the meeting start, not wall-clock —
				// a 2pm meeting starts at 00:00:00, not 14:00:00.
				var base time.Time
				if len(segs) > 0 {
					base, _ = granola.ParseISO(segs[0].StartTimestamp)
				}
				for i, s := range segs {
					st, _ := granola.ParseISO(s.StartTimestamp)
					en, _ := granola.ParseISO(s.EndTimestamp)
					fmt.Fprintf(w, "%d\n%s --> %s\n", i+1, srtOffset(st, base), srtOffset(en, base))
					if speaker {
						fmt.Fprintf(w, "[%s] %s\n\n", s.Source, s.Text)
					} else {
						fmt.Fprintf(w, "%s\n\n", s.Text)
					}
				}
				return nil
			default:
				return usageErr(fmt.Errorf("invalid --format %q: must be json, text, or srt", format))
			}
		},
	}
	cmd.Flags().BoolVar(&speaker, "speaker", false, "Prefix each line with the source label")
	cmd.Flags().StringVar(&format, "format", "", "Output format: json | text | srt (default: json with --json, else text)")
	cmd.Flags().StringVar(&since, "since", "", "Trim to segments after M:SS from meeting start")
	return cmd
}

// loadTranscript returns segments + a string describing the source
// ("cache" or "live"). Honors flags.dataSource. On v7.4x+ the internal API is
// sealed, so the live path is the PUBLIC REST detail endpoint
// (/v1/notes/{id}?include=transcript), not the old internal client.
func loadTranscript(id, dataSource string) ([]granola.TranscriptSegment, string, error) {
	if dataSource != "live" {
		// openGranolaCache reads the segments the REST `enrich`/`sync` populated.
		// A MISSING store is not an error (returns an empty, nil-safe cache); a
		// REAL store error (corrupt/locked DB) IS propagated now that
		// openGranolaCache no longer swallows it. Handle it explicitly rather
		// than dereferencing a possibly-nil/partial cache.
		c, err := openGranolaCache()
		if err != nil {
			// --data-source local has no live fallback → surface the real error.
			// In auto mode (dataSource == ""), fall through to the live REST path
			// deliberately instead of failing on a local store problem.
			if dataSource == "local" {
				return nil, "", apiErr(fmt.Errorf("read local transcript store for %s: %w", id, err))
			}
		} else {
			if segs := c.TranscriptByID(id); len(segs) > 0 {
				return segs, "cache", nil
			}
			if dataSource == "local" {
				return nil, "", notFoundErr(fmt.Errorf("transcript for %s not in local store; run 'granola-cli sync' (or 'enrich') first, or use --data-source live", id))
			}
		}
	}
	segs, err := fetchTranscriptREST(id)
	if err != nil {
		return nil, "", err
	}
	return segs, "live", nil
}

// fetchTranscriptREST pulls a transcript live from the public REST API and maps
// it to the in-memory segment shape.
func fetchTranscriptREST(id string) ([]granola.TranscriptSegment, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, configErr(err)
	}
	c := client.New(cfg, 30*time.Second, 0)
	// v7.4x leaf REST helper with its own 30s-timeout client; a background
	// context reproduces the reference's bounded, cancellation-free behavior.
	// id is an opaque single-segment note id; escape it into the path segment.
	raw, err := c.Get(context.Background(), "/v1/notes/"+url.PathEscape(id), map[string]string{"include": "transcript"})
	if err != nil {
		return nil, apiErr(fmt.Errorf("fetch transcript for %s from the public API: %w", id, err))
	}
	var d restNoteDetail
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, apiErr(fmt.Errorf("parse transcript for %s: %w", id, err))
	}
	if d.Transcript == nil || len(*d.Transcript) == 0 {
		return nil, notFoundErr(fmt.Errorf("no transcript for %s (the meeting may not have been recorded/transcribed)", id))
	}
	segs := make([]granola.TranscriptSegment, 0, len(*d.Transcript))
	for _, t := range *d.Transcript {
		segs = append(segs, granola.TranscriptSegment{
			Source:         t.Speaker.Source,
			Text:           t.Text,
			StartTimestamp: t.StartTime,
			EndTimestamp:   t.EndTime,
		})
	}
	return segs, nil
}

// parseClock parses "M:SS" or "H:MM:SS" into a Duration.
func parseClock(s string) (time.Duration, error) {
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2:
		var m, sec int
		if _, err := fmt.Sscanf(s, "%d:%d", &m, &sec); err != nil {
			return 0, err
		}
		return time.Duration(m)*time.Minute + time.Duration(sec)*time.Second, nil
	case 3:
		var h, m, sec int
		if _, err := fmt.Sscanf(s, "%d:%d:%d", &h, &m, &sec); err != nil {
			return 0, err
		}
		return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second, nil
	}
	return 0, fmt.Errorf("expected M:SS or H:MM:SS")
}

func trimSegmentsAfter(segs []granola.TranscriptSegment, cut time.Duration) []granola.TranscriptSegment {
	if len(segs) == 0 || cut == 0 {
		return segs
	}
	first, err := granola.ParseISO(segs[0].StartTimestamp)
	if err != nil {
		return segs
	}
	threshold := first.Add(cut)
	out := segs[:0:len(segs)]
	for _, s := range segs {
		t, err := granola.ParseISO(s.StartTimestamp)
		if err == nil && !t.Before(threshold) {
			out = append(out, s)
		}
	}
	return out
}

// srtOffset formats t as an SRT timestamp RELATIVE to base (the meeting start).
// A zero base or t falls back to 00:00:00,000.
func srtOffset(t, base time.Time) string {
	if t.IsZero() || base.IsZero() {
		return "00:00:00,000"
	}
	d := t.Sub(base)
	if d < 0 {
		d = 0
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	s := int((d % time.Minute) / time.Second)
	ms := int((d % time.Second) / time.Millisecond)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}
