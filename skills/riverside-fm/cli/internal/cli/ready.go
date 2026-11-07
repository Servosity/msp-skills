// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// newReadyCmd checks every synced studio for fully-ready takes.
// A take is "ready" when: every track.status == "done" AND transcription has speakers/voice-activity.
// Live mode: walks studios -> projects -> takes via the API.
func newReadyCmd(flags *rootFlags) *cobra.Command {
	var studio string

	cmd := &cobra.Command{
		Use:         "ready",
		Short:       "List takes that are fully ready: backup done, transcription done, no participant uploading.",
		Long:        "Walks studios -> projects -> takes via the API and filters to takes whose every track is done AND whose transcription has content. Use --studio to scope to one studio (recommended for large workspaces).",
		Example:     "  riverside-fm-cli ready --studio damien-stevenss-studio --json --select session_id,project,duration_min",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if studio == "" {
				return usageErr(fmt.Errorf("--studio is required (no global studio inventory endpoint exposed by the API)"))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			projects, err := listStudioProjects(cmd.Context(), c, studio)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}

			type readyRow struct {
				Studio       string  `json:"studio"`
				Project      string  `json:"project"`
				ProjectID    string  `json:"project_id"`
				TakeID       string  `json:"take_id"`
				SessionID    string  `json:"session_id"`
				Title        string  `json:"title"`
				DurationMin  float64 `json:"duration_min"`
				Participants int     `json:"participants"`
			}
			rows := []readyRow{}

			// A take whose fetch failed is not "not ready": it is unknown. The
			// output is a bare list with nowhere to say so, so any failed fetch
			// exits non-zero (after the partial list) and names the first error.
			fetches, failures := 0, 0
			var firstErr error
			fail := func(e error) {
				failures++
				if firstErr == nil {
					firstErr = e
				}
			}

			for _, proj := range projects {
				fetches++
				takes, err := listProjectTakes(cmd.Context(), c, proj.ID)
				if err != nil {
					fail(fmt.Errorf("project %s takes: %w", proj.ID, err))
					continue
				}
				for _, take := range takes {
					fetches++
					adata, aerr := c.Get(cmd.Context(), "/api/v4/take/"+url.PathEscape(take.SessionID)+"/assets", nil)
					if aerr != nil {
						fail(fmt.Errorf("take %s assets: %w", take.SessionID, aerr))
						continue
					}
					if !allTracksDone(adata) {
						continue
					}
					fetches++
					ttData, tterr := c.Get(cmd.Context(), "/api/v4/transcriptions/editableWithVoiceActivity/"+url.PathEscape(take.SessionID), nil)
					if tterr != nil {
						fail(fmt.Errorf("take %s transcript: %w", take.SessionID, tterr))
						continue
					}
					if !hasTranscriptContent(ttData) {
						continue
					}
					rows = append(rows, readyRow{
						Studio:       studio,
						Project:      proj.Title,
						ProjectID:    proj.ID,
						TakeID:       take.ID,
						SessionID:    take.SessionID,
						Title:        take.Title,
						DurationMin:  takeDurationMin(adata),
						Participants: len(extractParticipantHandles(adata)),
					})
				}
			}

			incomplete := func() error {
				if failures == 0 {
					return nil
				}
				return classifyAPIErrorOnly(fmt.Errorf("ready is incomplete: %d of %d fetches failed, so the takes behind them are missing from this list. First: %w", failures, fetches, firstErr))
			}

			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				j, _ := json.MarshalIndent(rows, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(j))
				return incomplete()
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No fully-ready takes in studio %s.\n", studio)
				return incomplete()
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d ready takes in %s:\n", len(rows), studio)
			for _, r := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s | %s | %s | %.1f min | %d participants\n",
					r.SessionID, r.Title, r.Project, r.DurationMin, r.Participants)
			}
			return incomplete()
		},
	}

	cmd.Flags().StringVar(&studio, "studio", "", "Studio slug to scan (required)")
	return cmd
}

func allTracksDone(data json.RawMessage) bool {
	var probe struct {
		Take struct {
			Tracks []struct {
				Status string `json:"status"`
			} `json:"tracks"`
		} `json:"take"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return false
	}
	if len(probe.Take.Tracks) == 0 {
		return false
	}
	for _, t := range probe.Take.Tracks {
		s := strings.ToLower(t.Status)
		if s != "done" {
			return false
		}
	}
	return true
}

func takeDurationMin(data json.RawMessage) float64 {
	var probe struct {
		Take struct {
			Tracks []struct {
				Duration float64 `json:"duration"`
			} `json:"tracks"`
		} `json:"take"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return 0
	}
	var max float64
	for _, t := range probe.Take.Tracks {
		if t.Duration > max {
			max = t.Duration
		}
	}
	return max / 60.0
}
