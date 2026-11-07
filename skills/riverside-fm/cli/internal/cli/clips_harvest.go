// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// newClipsHarvestCmd combines:
//
//	GET /api/v4/projects/{projectId}/ai-generation-status  (gate)
//	GET /api/v4/projects/{projectId}/clips                 (list -- the Magic Clips)
//	GET /api/v4/clip/{clipId}                              (score + cut points + signed URL)
//
// Optionally downloads each clip body when --download is set.
//
// PATCH(magic-clips-list): the list step used to call
// GET /api/v4/projects/{projectId}/clips/exports, which lists RENDERED EXPORT JOBS,
// not clips. A project with 21 "Made for you" Magic Clips returns {"clips":[],"total":0}
// there until a human presses Export in the UI, so harvest printed []. The clips
// themselves live on GET /api/v4/projects/{projectId}/clips.
func newClipsHarvestCmd(flags *rootFlags) *cobra.Command {
	var projectID string
	var since string
	var download bool
	var outDir string
	var wait bool
	var timeout time.Duration
	var typeFilter string
	var takeFilter string
	var minScore float64
	var limit int

	cmd := &cobra.Command{
		Use:   "harvest",
		Short: "Harvest Magic Clips: gate on AI-status, list the project's Magic Clips with scores, refresh signed URLs, optionally download rendered MP4s.",
		Long: "Combines the AI-generation-status gate + the project clips list + per-clip score/cut-point/URL refresh\n" +
			"into a single command. Clips Riverside has not rendered come back with status=not-rendered plus the\n" +
			"take id and start_seconds/end_seconds needed to cut them locally.",
		Example:     `  riverside-fm-cli clips harvest --project 69fcda9fba030a19ae93a526 --download --out ./clips`,
		Annotations: map[string]string{"mcp:read-only": "true", "mcp:write-flags": "out,download"}, // --download writes files; MCP gets the listing only
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if projectID == "" {
				return usageErr(fmt.Errorf("--project is required"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// 1. Gate on AI generation status
			if wait {
				deadline := time.Now().Add(timeout)
				for {
					if time.Now().After(deadline) {
						return &cliError{code: 2, err: fmt.Errorf("AI generation did not finish before --timeout")}
					}
					data, err := c.Get(cmd.Context(), "/api/v4/projects/"+url.PathEscape(projectID)+"/ai-generation-status", nil)
					if err == nil && aiAllDone(data) {
						break
					}
					fmt.Fprintln(cmd.OutOrStdout(), "Waiting for AI generation...")
					time.Sleep(15 * time.Second)
				}
			} else {
				statusData, err := c.Get(cmd.Context(), "/api/v4/projects/"+url.PathEscape(projectID)+"/ai-generation-status", nil)
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				if !aiAllDone(statusData) {
					if flags.asJSON {
						fmt.Fprintln(cmd.OutOrStdout(), `{"ready":false,"reason":"ai_generation_in_progress"}`)
					} else {
						fmt.Fprintln(cmd.OutOrStdout(), "AI generation still in progress. Use --wait to block until ready.")
					}
					return nil
				}
			}

			// 2. List the project's clips (Magic Clips / Hooks / Episodes).
			clipRows, _, err := gatherMagicClips(cmd.Context(), c, projectID, typeFilter, takeFilter, minScore, true, limit)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}

			// 3. Refresh each clip + collect URL
			type harvestRow struct {
				magicClipRow
				URL       string `json:"url,omitempty"`
				LocalPath string `json:"local_path,omitempty"`
				Bytes     int64  `json:"bytes,omitempty"`
				Status    string `json:"status"`
				Reason    string `json:"reason,omitempty"`
			}
			rows := []harvestRow{}
			if download && outDir == "" {
				outDir = "./clips"
			}
			if download {
				if err := os.MkdirAll(outDir, 0o700); err != nil {
					return err
				}
			}

			_ = since // since-filter could be applied if exports list returns createdAt; payload was empty in browser-sniff so left for future enrichment.

			for _, meta := range clipRows {
				row := harvestRow{magicClipRow: meta, Status: "metadata-only"}
				clipData, cerr := c.Get(cmd.Context(), "/api/v4/clip/"+url.PathEscape(meta.ClipID), nil)
				if cerr != nil {
					row.Status = "fetch-failed"
					rows = append(rows, row)
					continue
				}
				urls := extractClipExportURLs(clipData)
				if len(urls) == 0 {
					// PATCH(magic-clips-list): a Magic Clip is an un-rendered edit decision
					// list. Riverside materialises no MP4 until somebody exports it, so
					// there is no signed URL to hand back -- say so instead of dropping the
					// clip on the floor, and hand back the take + in/out points so callers
					// can cut the clip locally from the take media they can already fetch.
					row.Status = "not-rendered"
					row.Reason = "Riverside has not rendered this clip (composedRecording.status=" +
						firstNonEmpty(meta.RenderStatus, "unknown") + "); cut it locally from take " +
						firstNonEmpty(meta.SessionID, meta.TakeID) + " using start_seconds/end_seconds"
					rows = append(rows, row)
					continue
				}
				// Pick the first concrete download URL
				row.URL = urls[0].URL
				row.Status = "ready"
				if download {
					fp := safeJoin(outDir, meta.Title+"-"+meta.ClipID+guessExt(row.URL))
					if fp == "" {
						// API-supplied name/id would have escaped --out: report the
						// skip rather than dropping the clip from the output.
						row.Status = "skipped: name/id would escape --out"
						rows = append(rows, row)
						continue
					}
					n, derr := streamDownload(row.URL, fp)
					if derr != nil {
						row.Status = "download-failed: " + derr.Error()
					} else {
						row.LocalPath = fp
						row.Bytes = n
						row.Status = "downloaded"
					}
				}
				rows = append(rows, row)
			}

			statuses := make([]string, len(rows))
			for i, r := range rows {
				statuses[i] = r.Status
			}
			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				j, _ := json.MarshalIndent(rows, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(j))
				return rowsIncomplete("clips harvest", statuses)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No clips to harvest in this project.")
				return nil
			}
			for _, r := range rows {
				if r.LocalPath != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s | %s -> %s (%d bytes)\n", r.ClipID, r.Title, r.LocalPath, r.Bytes)
					continue
				}
				score := "-"
				if r.CalculatedScore != nil {
					score = fmt.Sprintf("%.0f", *r.CalculatedScore)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s | %-7s | score %-3s | %s | %s %s\n",
					r.ClipID, r.MagicType, score, r.Status, r.Title, truncate(r.URL, 60))
			}
			return rowsIncomplete("clips harvest", statuses)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "Project ID (required)")
	cmd.Flags().StringVar(&since, "since", "", "Only include clips created after this date or duration (reserved; not enforced in v0.1)")
	cmd.Flags().BoolVar(&download, "download", false, "Download each clip body to --out before signed-URL TTL expires")
	cmd.Flags().StringVar(&outDir, "out", "", "Output directory when --download (default ./clips)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Block until ai-generation-status reports done before harvesting")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Max wait when --wait")
	cmd.Flags().StringVar(&typeFilter, "type", "all", "Filter by magic type: Clip, Hook, Episode, or all")
	cmd.Flags().StringVar(&takeFilter, "take", "", "Only clips cut from this take (take ObjectId or sessionId UUID)")
	cmd.Flags().Float64Var(&minScore, "min-score", 0, "Only clips whose calculatedScore (0-100) is >= this")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max clips to harvest (0 = all)")
	return cmd
}

func extractClipTitle(data json.RawMessage) string {
	var probe struct {
		Data struct {
			Take struct {
				Name string `json:"name"`
			} `json:"take"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return ""
	}
	if probe.Data.Title != "" {
		return probe.Data.Title
	}
	return probe.Data.Take.Name
}
