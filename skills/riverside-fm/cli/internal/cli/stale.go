// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// staleTrack is one participant track that has not finished.
type staleTrack struct {
	Speaker      string `json:"speaker"`
	Status       string `json:"status"`
	ClientStatus string `json:"client_status"`
}

// staleRow is a take stuck past the threshold.
type staleRow struct {
	Studio    string       `json:"studio"`
	Project   string       `json:"project"`
	ProjectID string       `json:"project_id"`
	TakeID    string       `json:"take_id"`
	SessionID string       `json:"session_id"`
	Title     string       `json:"title"`
	CreatedAt string       `json:"created_at"`
	AgeHours  float64      `json:"age_hours"`
	Stage     string       `json:"stage"`
	Pending   []staleTrack `json:"pending_tracks"`
}

// takeAssetsState is what the stale check reads from GET /api/v4/take/{sessionId}/assets.
type takeAssetsState struct {
	Take struct {
		CreatedAt string `json:"createdAt"`
		Tracks    []struct {
			SpeakerName  string `json:"speakerName"`
			Status       string `json:"status"`
			ClientStatus string `json:"clientStatus"`
		} `json:"tracks"`
	} `json:"take"`
}

// classifyStaleTake decides whether a take is stuck. It returns the stage and the
// unfinished tracks, created (zero when createdAt is missing or unparsable) and
// whether the tracks are all done (the caller then checks the transcript).
// A track is uploading until the guest's client reports "uploaded", and
// processing until Riverside marks it "done".
func classifyStaleTake(data json.RawMessage) (stage string, pending []staleTrack, created time.Time, tracksDone bool, err error) {
	var st takeAssetsState
	if err = json.Unmarshal(data, &st); err != nil {
		return "", nil, time.Time{}, false, err
	}
	created, _ = time.Parse(time.RFC3339Nano, st.Take.CreatedAt)
	pending = []staleTrack{}
	uploading := false
	for _, t := range st.Take.Tracks {
		status, client := strings.ToLower(t.Status), strings.ToLower(t.ClientStatus)
		if status == "done" {
			continue
		}
		if client != "uploaded" {
			uploading = true
		}
		pending = append(pending, staleTrack{Speaker: t.SpeakerName, Status: t.Status, ClientStatus: t.ClientStatus})
	}
	switch {
	case len(st.Take.Tracks) == 0:
		return "no-tracks", pending, created, false, nil
	case uploading:
		return "uploading", pending, created, false, nil
	case len(pending) > 0:
		return "processing", pending, created, false, nil
	}
	return "", pending, created, true, nil
}

func newStaleCmd(flags *rootFlags) *cobra.Command {
	var studios []string
	var hours, days float64

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Cross-studio query: which recordings have been stuck in uploading / processing / transcribing past N hours?",
		Long: "Walks each --studio's projects -> takes via the API and lists every take older than the threshold\n" +
			"that is still uploading (a guest's client never finished), processing (Riverside has not marked a\n" +
			"track done) or transcribing (tracks done, transcript still empty). Riverside exposes no studio\n" +
			"inventory endpoint, so name each studio with --studio (repeatable).",
		Example: `  riverside-fm-cli stale --studio damien-stevenss-studio --days 1 --json
  riverside-fm-cli stale --studio show-a --studio show-b --hours 6`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(studios) == 0 {
				return usageErr(fmt.Errorf("--studio is required (no global studio inventory endpoint exposed by the API)"))
			}
			threshold := hours
			if days > 0 {
				threshold = days * 24
			}
			if threshold <= 0 {
				return usageErr(fmt.Errorf("--hours / --days must be positive"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "stale")
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			type result struct {
				ThresholdHours float64    `json:"threshold_hours"`
				Studios        []string   `json:"studios"`
				TakesChecked   int        `json:"takes_checked"`
				Undated        int        `json:"undated"`
				FetchFailures  int        `json:"fetch_failures"`
				FirstError     string     `json:"first_error,omitempty"`
				Stale          []staleRow `json:"stale"`
			}
			res := result{ThresholdHours: threshold, Studios: studios, Stale: []staleRow{}}
			fetches := 0
			fail := func(e error) {
				res.FetchFailures++
				if res.FirstError == "" {
					res.FirstError = e.Error()
				}
			}
			now := time.Now()

			for _, studio := range studios {
				fetches++
				projects, err := listStudioProjects(cmd.Context(), c, studio)
				if err != nil {
					fail(fmt.Errorf("studio %s: %w", studio, err))
					continue
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
						adata, err := c.Get(cmd.Context(), "/api/v4/take/"+url.PathEscape(take.SessionID)+"/assets", nil)
						if err != nil {
							fail(fmt.Errorf("take %s assets: %w", take.SessionID, err))
							continue
						}
						stage, pending, created, tracksDone, perr := classifyStaleTake(adata)
						if perr != nil {
							fail(fmt.Errorf("take %s assets: %w", take.SessionID, perr))
							continue
						}
						res.TakesChecked++
						if created.IsZero() {
							res.Undated++
							continue
						}
						age := now.Sub(created).Hours()
						if age < threshold {
							continue
						}
						if tracksDone {
							fetches++
							tt, terr := c.Get(cmd.Context(), "/api/v4/transcriptions/editableWithVoiceActivity/"+url.PathEscape(take.SessionID), nil)
							if terr != nil {
								fail(fmt.Errorf("take %s transcript: %w", take.SessionID, terr))
								continue
							}
							if hasTranscriptContent(tt) {
								continue
							}
							stage = "transcribing"
						}
						res.Stale = append(res.Stale, staleRow{
							Studio: studio, Project: proj.Title, ProjectID: proj.ID,
							TakeID: take.ID, SessionID: take.SessionID, Title: take.Title,
							CreatedAt: created.UTC().Format(time.RFC3339), AgeHours: round3(age),
							Stage: stage, Pending: pending,
						})
					}
				}
			}

			// "All fetches failed" must never read as "nothing is stuck".
			if fetches > 0 && res.FetchFailures == fetches {
				return classifyAPIError(cmd.OutOrStdout(), fmt.Errorf("every fetch failed (%d); first error: %s", fetches, res.FirstError), flags)
			}
			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				j, _ := json.MarshalIndent(res, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(j))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d stale takes (older than %.1fh) across %d takes checked", len(res.Stale), threshold, res.TakesChecked)
			if res.FetchFailures > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "; %d fetches failed (first: %s)", res.FetchFailures, res.FirstError)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			for _, r := range res.Stale {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s | %-12s | %6.1fh | %s | %s\n", r.SessionID, r.Stage, r.AgeHours, r.Title, r.Project)
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&studios, "studio", nil, "Studio slug to scan (repeatable; required)")
	cmd.Flags().Float64Var(&hours, "hours", 6, "Report takes older than this many hours")
	cmd.Flags().Float64Var(&days, "days", 0, "Threshold in days (overrides --hours when set)")
	return cmd
}
