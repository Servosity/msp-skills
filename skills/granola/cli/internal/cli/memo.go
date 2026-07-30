// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"granola-pp-cli/internal/granola"
)

func newMemoCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memo",
		Short: "MEMO pipeline: preflight + extract for one or many meetings",
	}
	cmd.AddCommand(newMemoRunCmd(flags))
	cmd.AddCommand(newMemoQueueCmd(flags))
	return cmd
}

func newMemoRunCmd(flags *rootFlags) *cobra.Command {
	var last, since, until, outDir, root string
	var limit int
	cmd := &cobra.Command{
		Use:   "run [<id>]",
		Short: "Run preflight + extract for one meeting, or every new meeting in --since",
		Long: `Without an id, iterates every meeting in --since/--last/--until and runs
the pipeline for those whose full_<id>.md does not yet exist in --to.

Emits ndjson one line per meeting: {id, status, files, unknown_sources, error}.

status values:
  new                   memo written
  duplicate             full_<id>.md already exists
  missing_transcript    no transcript, OR fewer than 5 microphone / 10 classified
                        segments (absent and too-thin are not distinguished here;
                        run preflight for the exit code that tells them apart)
  unknown_source_label  the upstream transcript grammar changed so the channels
                        could not be classified -- NOT the same as a missing
                        transcript; the text may well be there
  skipped               meeting deliberately passed over
  error                 anything else, with error set`,
		Example: strings.Trim(`
  granola-cli memo run not_06Yq6JtogRihEr --out ./memos
  granola-cli memo run --out ./memos --since 7d
  granola-cli memo run --to ./memos --last 30d --limit 50`, "\n"),
		Annotations: map[string]string{
			"pp:typed-exit-codes": "0,1,2,3",
			// not read-only — writes files
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if outDir == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			home, _ := os.UserHomeDir()
			if root == "" {
				root = filepath.Join(home, PreflightDefaultRoot)
			}
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return ioErr(err)
			}
			w := cmd.OutOrStdout()
			var ids []string
			if len(args) > 0 {
				ids = []string{args[0]}
			} else {
				from, to, err := parseTimeWindow(last, since, until)
				if err != nil {
					return usageErr(err)
				}
				c, err := openGranolaCache()
				if err != nil {
					return err
				}
				ids = selectDocsInWindow(c, from, to, limit)
			}
			for _, id := range ids {
				rec := runOneMemo(id, outDir, root, flags.dataSource != "local")
				_ = emitNDJSONLine(w, rec)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&last, "last", "", "Time window (e.g. 7d)")
	cmd.Flags().StringVar(&since, "since", "", "Start date")
	cmd.Flags().StringVar(&until, "until", "", "End date")
	cmd.Flags().StringVarP(&outDir, "out", "o", "", "Output directory (alias: --to)")
	cmd.Flags().StringVar(&outDir, "to", "", "Output directory alias")
	cmd.Flags().StringVar(&root, "root", "", "Existing transcripts root (for duplicate check)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Cap meetings processed")
	return cmd
}

type memoRecord struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Files  []string `json:"files,omitempty"`
	// UnknownSources carries unrecognized transcript source labels even when
	// the memo succeeded -- drift on a passing transcript still means upstream
	// changed its grammar, and a stderr warning alone is invisible to anything
	// consuming this JSON.
	UnknownSources []string `json:"unknown_sources,omitempty"`
	DuplicateOf    string   `json:"duplicate_of,omitempty"`
	Error          string   `json:"error,omitempty"`
}

func runOneMemo(id, outDir, root string, allowLive bool) memoRecord {
	rec := memoRecord{ID: id}
	c, err := openGranolaCache()
	if err != nil {
		rec.Status = "error"
		rec.Error = err.Error()
		return rec
	}
	d := c.DocumentByID(id)
	if d == nil {
		rec.Status = "error"
		rec.Error = "meeting not in cache"
		return rec
	}
	segs := c.TranscriptByID(id)
	mic, far, unknownSegs, unknown := countSources(segs)
	// Report drift regardless of outcome -- a passing transcript with a
	// relabelled channel is still a signal the caller needs.
	rec.UnknownSources = unknown
	if !transcriptUsable(mic, mic+far) || unknownIsMaterial(unknownSegs) {
		// Distinguish "upstream relabelled the channel" from "no transcript":
		// collapsing both into missing_transcript is what let this hide.
		if len(unknown) > 0 {
			rec.Status = "unknown_source_label"
			rec.Error = fmt.Sprintf("unrecognized transcript source label(s) on %d/%d segments: %s",
				unknownSegs, len(segs), strings.Join(unknown, ", "))
		} else {
			rec.Status = "missing_transcript"
		}
		return rec
	}
	if dup := findDuplicateFile(root, id, d.Title); dup != "" {
		rec.Status = "duplicate"
		rec.DuplicateOf = dup
		return rec
	}
	res, err := runExtract(id, outDir, "", allowLive)
	if err != nil {
		var ce *cliError
		if errors.As(err, &ce) {
			switch ce.code {
			case 2:
				rec.Status = "missing_transcript"
			case 3:
				rec.Status = "duplicate"
				rec.DuplicateOf = "full_" + id + ".md (in --out dir)"
			default:
				rec.Status = "error"
			}
		} else {
			rec.Status = "error"
		}
		rec.Error = err.Error()
		return rec
	}
	rec.Status = "new"
	rec.Files = res.Files
	return rec
}

func newMemoQueueCmd(flags *rootFlags) *cobra.Command {
	var last, since, until, root string
	var limit int
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "List meetings whose transcript is cached but whose full_<id>.md is missing",
		Long: `Emits ndjson, one line per meeting that is READY to memo:
{id, title, started_at, mic_count, system_count, total_count, one_sided}.

The stream is a union: it ALSO emits diagnostic rows for meetings skipped
because their transcript source labels could not be classified. Those rows
carry status:"skipped" plus reason, and never carry a title. Consumers building
a work list must filter them out:

  granola-cli memo queue --last 7d | jq 'select(.status != "skipped")'

They exist so a fleet-wide upstream relabel cannot present as an empty queue --
which is exactly how a dead source-label grammar went unnoticed. Exit stays 0:
a partial-drift run should still hand back the meetings that ARE ready.`,
		Example: strings.Trim(`
  granola-cli memo queue --last 7d
  granola-cli memo queue --since 2026-07-01 --limit 20`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			home, _ := os.UserHomeDir()
			if root == "" {
				root = filepath.Join(home, PreflightDefaultRoot)
			}
			from, to, err := parseTimeWindow(last, since, until)
			if err != nil {
				return usageErr(err)
			}
			c, err := openGranolaCache()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			ids := selectDocsInWindow(c, from, to, 0)
			emitted := 0
			for _, id := range ids {
				segs := c.TranscriptByID(id)
				mic, far, unknownSegs, unknown := countSources(segs)
				if !transcriptUsable(mic, mic+far) || unknownIsMaterial(unknownSegs) {
					// A bare `continue` here is what made a fleet-wide label
					// drift look like "you have no meetings". A stderr warning
					// alone repeats that mistake for anything reading the
					// ndjson, so emit a machine-visible skip row too. Rows with
					// status:"skipped" are diagnostics, not queue entries --
					// consumers building a work list filter them out.
					if len(unknown) > 0 {
						stderr("warning: %s: skipped, unrecognized transcript source label(s) on %d/%d segments: %s", id, unknownSegs, len(segs), strings.Join(unknown, ", "))
						_ = emitNDJSONLine(w, map[string]any{
							"id":              id,
							"status":          "skipped",
							"reason":          "unknown_source_label",
							"unknown_sources": unknown,
							"unknown_count":   unknownSegs,
							"mic_count":       mic,
							"system_count":    far,
							"total_count":     len(segs),
						})
					}
					continue
				}
				if len(unknown) > 0 {
					stderr("warning: %s: unrecognized transcript source label(s) on %d/%d segments: %s", id, unknownSegs, len(segs), strings.Join(unknown, ", "))
				}
				d := c.DocumentByID(id)
				if d == nil {
					continue
				}
				if dup := findDuplicateFile(root, id, d.Title); dup != "" {
					continue
				}
				row := map[string]any{
					"id":           id,
					"title":        d.Title,
					"started_at":   d.CreatedAt,
					"mic_count":    mic,
					"system_count": far,
					"total_count":  len(segs),
				}
				if len(unknown) > 0 {
					row["unknown_sources"] = unknown
					row["unknown_count"] = unknownSegs
				} else {
					// Only assertable when every label classified; see preflight.
					row["one_sided"] = far == 0
				}
				_ = emitNDJSONLine(w, row)
				emitted++
				if limit > 0 && emitted >= limit {
					break
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&last, "last", "", "Time window (e.g. 7d)")
	cmd.Flags().StringVar(&since, "since", "", "Start date")
	cmd.Flags().StringVar(&until, "until", "", "End date")
	cmd.Flags().StringVar(&root, "root", "", "Existing transcripts root (for duplicate check)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Cap result count")
	return cmd
}

// Ensure time + granola references are present.
var (
	_ = time.Now
	_ = granola.ParseISO
)
