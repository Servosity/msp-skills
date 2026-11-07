// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live
//
// PATCH(magic-clips-list): hand-written, not generated. Riverside's "Made for you"
// Magic Clips are NOT in /api/v4/projects/{id}/clips/exports (that route only lists
// RENDERED export jobs and is empty until a human hits Export). The real list is
// GET /api/v4/projects/{id}/clips?limit=N. See SKILL.md "Magic Clips".

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"riverside-fm-pp-cli/internal/client"
)

// magicClipRow is the flattened, agent-friendly view of one Riverside clip.
type magicClipRow struct {
	ClipID          string   `json:"clip_id"`
	Title           string   `json:"title"`
	MagicType       string   `json:"magic_type"`
	IsMagicEpisode  bool     `json:"is_magic_episode"`
	IsAIGenerated   bool     `json:"is_ai_generated"`
	CalculatedScore *float64 `json:"calculated_score,omitempty"`
	ScoreType       string   `json:"score_type,omitempty"`
	RawScore        *float64 `json:"raw_score,omitempty"`
	NormalizedScore *float64 `json:"normalized_score,omitempty"`
	BatchID         string   `json:"batch_id,omitempty"`
	AIVariation     string   `json:"ai_variation,omitempty"`
	MagicClipID     string   `json:"magic_clip_id,omitempty"`
	DurationMS      int64    `json:"duration_ms"`
	DurationSeconds float64  `json:"duration_seconds"`
	StartSeconds    *float64 `json:"start_seconds,omitempty"`
	EndSeconds      *float64 `json:"end_seconds,omitempty"`
	AspectRatio     string   `json:"aspect_ratio,omitempty"`
	TakeID          string   `json:"take_id,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	TakeName        string   `json:"take_name,omitempty"`
	RenderStatus    string   `json:"render_status,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
	ThumbnailPath   string   `json:"thumbnail_path,omitempty"`
}

// rawClip is the subset of the clip document this CLI reads.
type rawClip struct {
	ID             string   `json:"_id"`
	Title          string   `json:"title"`
	MagicType      string   `json:"magicType"`
	MagicClipID    string   `json:"magicClipId"`
	AIVariation    string   `json:"aiVariation"`
	Duration       int64    `json:"duration"`
	AspectRatio    string   `json:"aspectRatio"`
	IsMagicEpisode bool     `json:"isMagicEpisode"`
	IsAIGenerated  bool     `json:"isAiGenerated"`
	CreatedAt      string   `json:"createdAt"`
	Deleted        bool     `json:"deleted"`
	RSTLTimeline   string   `json:"rstlTimeline"`
	TimelineThumb  string   `json:"timelineThumbnailSrc"`
	CalcScore      *float64 `json:"calculatedScore"`
	ScoreData      struct {
		ScoreType       string   `json:"scoreType"`
		RawScore        *float64 `json:"rawScore"`
		NormalizedScore *float64 `json:"normalizedScore"`
		BatchID         string   `json:"batchId"`
	} `json:"scoreData"`
	Timeline json.RawMessage `json:"timeline"`
	Take     struct {
		ID        string `json:"_id"`
		Name      string `json:"name"`
		SessionID string `json:"sessionId"`
	} `json:"take"`
	ComposedRecording struct {
		Status    string `json:"status"`
		SessionID string `json:"sessionId"`
	} `json:"composedRecording"`
}

func (r rawClip) magicTypeLabel() string {
	if r.MagicType != "" {
		return r.MagicType
	}
	if r.IsMagicEpisode {
		return "Episode"
	}
	return "Clip"
}

// listProjectClips calls the real clips route and returns every clip document.
// Riverside defaults to 10 per page; ask for a big page and honour `total`.
func listProjectClips(ctx context.Context, c *client.Client, projectID string, pageSize int) ([]rawClip, int, error) {
	if pageSize <= 0 {
		pageSize = 500
	}
	total := 0
	var decodeErr error
	clips, err := listAll(ctx, c, "/api/v4/projects/"+url.PathEscape(projectID)+"/clips", nil, pageSize,
		func(data json.RawMessage) []rawClip {
			var payload struct {
				Clips []rawClip `json:"clips"`
				Total int       `json:"total"`
			}
			if err := json.Unmarshal(data, &payload); err != nil {
				decodeErr = fmt.Errorf("decoding clips list: %w", err)
				return nil
			}
			total = payload.Total
			return payload.Clips
		}, func(rc rawClip) string { return rc.ID })
	if err != nil {
		return nil, 0, err
	}
	if decodeErr != nil {
		return nil, 0, decodeErr
	}
	return clips, total, nil
}

// clipRow flattens one clip document into a row, optionally enriched by the
// per-clip detail call (which is the ONLY place calculatedScore and the
// millisecond cut points for Hook/Episode clips live).
func clipRow(rc rawClip, detail *rawClip) magicClipRow {
	row := magicClipRow{
		ClipID:          rc.ID,
		Title:           rc.Title,
		MagicType:       rc.magicTypeLabel(),
		IsMagicEpisode:  rc.IsMagicEpisode,
		IsAIGenerated:   rc.IsAIGenerated,
		ScoreType:       rc.ScoreData.ScoreType,
		RawScore:        rc.ScoreData.RawScore,
		NormalizedScore: rc.ScoreData.NormalizedScore,
		BatchID:         rc.ScoreData.BatchID,
		AIVariation:     rc.AIVariation,
		MagicClipID:     rc.MagicClipID,
		DurationMS:      rc.Duration,
		DurationSeconds: round3(float64(rc.Duration) / 1000),
		AspectRatio:     rc.AspectRatio,
		TakeID:          rc.Take.ID,
		SessionID:       firstNonEmpty(rc.Take.SessionID, rc.ComposedRecording.SessionID),
		TakeName:        rc.Take.Name,
		RenderStatus:    rc.ComposedRecording.Status,
		CreatedAt:       rc.CreatedAt,
		ThumbnailPath:   rc.TimelineThumb,
		CalculatedScore: rc.CalcScore,
	}
	src := rc
	if detail != nil {
		if detail.CalcScore != nil {
			row.CalculatedScore = detail.CalcScore
		}
		if row.ScoreType == "" {
			row.ScoreType = detail.ScoreData.ScoreType
			row.RawScore = detail.ScoreData.RawScore
			row.NormalizedScore = detail.ScoreData.NormalizedScore
			row.BatchID = detail.ScoreData.BatchID
		}
		if detail.RSTLTimeline != "" || len(detail.Timeline) > 0 {
			src = *detail
		}
	}
	if start, end, ok := clipCutWindow(src); ok {
		row.StartSeconds = &start
		row.EndSeconds = &end
	}
	return row
}

// clipCutWindow returns the clip's in/out points RELATIVE TO THE TAKE, in seconds.
//
// Riverside stores this two different ways depending on the workflow engine:
//   - workflowEngineVersion 1 ("Clip"): rstlTimeline, a JSON *string* whose
//     syncedCuts are in timebase ticks (timebase ticks == 1 second).
//   - the newer editor ("Hook"/"Episode"): timeline.sections[].effects entries of
//     type "cut", in MILLISECONDS, only present on GET /api/v4/clip/{id}.
//
// In both shapes the clip is what survives after a leading cut from 0 and a
// trailing cut that runs to the end of the section.
func clipCutWindow(rc rawClip) (float64, float64, bool) {
	if rc.RSTLTimeline != "" {
		var tl struct {
			Timebase   float64 `json:"timebase"`
			Duration   float64 `json:"duration"`
			SyncedCuts []struct {
				Range struct {
					Start    float64 `json:"start"`
					Duration float64 `json:"duration"`
				} `json:"range"`
			} `json:"syncedCuts"`
		}
		if json.Unmarshal([]byte(rc.RSTLTimeline), &tl) == nil && tl.Timebase > 0 && tl.Duration > 0 {
			cuts := make([][2]float64, 0, len(tl.SyncedCuts))
			for _, sc := range tl.SyncedCuts {
				cuts = append(cuts, [2]float64{sc.Range.Start, sc.Range.Start + sc.Range.Duration})
			}
			if s, e, ok := windowFromCuts(cuts, tl.Duration); ok {
				return round3(s / tl.Timebase), round3(e / tl.Timebase), true
			}
		}
	}
	if len(rc.Timeline) > 0 {
		var tl struct {
			Sections []struct {
				Duration float64 `json:"duration"`
				Effects  []struct {
					Type  string  `json:"type"`
					Start float64 `json:"start"`
					End   float64 `json:"end"`
				} `json:"effects"`
			} `json:"sections"`
		}
		if json.Unmarshal(rc.Timeline, &tl) == nil && len(tl.Sections) > 0 {
			sec := tl.Sections[0]
			cuts := make([][2]float64, 0, len(sec.Effects))
			for _, ef := range sec.Effects {
				if ef.Type == "cut" {
					cuts = append(cuts, [2]float64{ef.Start, ef.End})
				}
			}
			if s, e, ok := windowFromCuts(cuts, sec.Duration); ok {
				return round3(s / 1000), round3(e / 1000), true
			}
		}
	}
	return 0, 0, false
}

// windowFromCuts collapses a cut list into the surviving [start,end] window.
func windowFromCuts(cuts [][2]float64, total float64) (float64, float64, bool) {
	if total <= 0 || len(cuts) == 0 {
		return 0, 0, false
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i][0] < cuts[j][0] })
	start, end := 0.0, total
	if cuts[0][0] <= 0.001 {
		start = cuts[0][1]
	}
	last := cuts[len(cuts)-1]
	if last[1] >= total-0.001 {
		end = last[0]
	}
	if end <= start {
		return 0, 0, false
	}
	return start, end, true
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// matchesClipType applies the --type filter (case-insensitive, "all" = no filter).
func matchesClipType(row magicClipRow, want string) bool {
	if want == "" || strings.EqualFold(want, "all") {
		return true
	}
	return strings.EqualFold(row.MagicType, want)
}

// gatherMagicClips is the shared engine behind `clips list` and `clips harvest`.
func gatherMagicClips(ctx context.Context, c *client.Client, projectID, typeFilter, takeFilter string, minScore float64, withDetail bool, limit int) ([]magicClipRow, int, error) {
	clips, total, err := listProjectClips(ctx, c, projectID, 500)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]magicClipRow, 0, len(clips))
	for i := range clips {
		rc := clips[i]
		if rc.Deleted {
			continue
		}
		var detail *rawClip
		if withDetail {
			// The detail call is the only source of calculatedScore and the
			// Hook/Episode cut points. A clip whose detail fetch failed would be
			// filtered out by --min-score (or listed without them) as if Riverside
			// had not scored it, so a failed fetch fails the listing instead.
			d, derr := getClipDetail(ctx, c, rc.ID)
			if derr != nil {
				return nil, 0, fmt.Errorf("clip %s detail: %w", rc.ID, derr)
			}
			detail = d
		}
		row := clipRow(rc, detail)
		if !matchesClipType(row, typeFilter) {
			continue
		}
		if takeFilter != "" && row.TakeID != takeFilter && row.SessionID != takeFilter {
			continue
		}
		if minScore > 0 && (row.CalculatedScore == nil || *row.CalculatedScore < minScore) {
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		si, sj := -1.0, -1.0
		if rows[i].CalculatedScore != nil {
			si = *rows[i].CalculatedScore
		}
		if rows[j].CalculatedScore != nil {
			sj = *rows[j].CalculatedScore
		}
		return si > sj
	})
	// Limit AFTER the score sort so --limit 1 means "the clip Riverside rated highest",
	// not "whatever the API happened to return first".
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, total, nil
}

func getClipDetail(ctx context.Context, c *client.Client, clipID string) (*rawClip, error) {
	data, err := c.Get(ctx, "/api/v4/clip/"+url.PathEscape(clipID), nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data rawClip `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

func newClipsListCmd(flags *rootFlags) *cobra.Command {
	var projectID string
	var typeFilter string
	var takeFilter string
	var minScore float64
	var limit int
	var noDetail bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the \"Made for you\" Magic Clips for a project with AI scores and take in/out points.",
		Long: "Lists every clip Riverside generated for a project (Magic Clips, Magic Hooks, Magic Episodes),\n" +
			"including each clip's AI rating (calculatedScore 0-100), the raw model score, and the clip's\n" +
			"start/end offsets inside the source take -- so agents can compare Riverside's picks to their own.\n\n" +
			"Reads GET /api/v4/projects/{projectId}/clips (NOT .../clips/exports, which only lists RENDERED\n" +
			"export jobs and stays empty until somebody presses Export in the Riverside UI).",
		Example: "  riverside-fm-cli clips list --project 6aa84b6e11ceb0f1f3b6de53 --agent\n" +
			"  riverside-fm-cli clips list --project 6aa84b6e11ceb0f1f3b6de53 --type Clip --min-score 70 --agent",
		Annotations: map[string]string{"pp:endpoint": "projects.list_clips", "pp:method": "GET", "pp:path": "/api/v4/projects/{projectId}/clips", "mcp:read-only": "true"},
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
			rows, total, err := gatherMagicClips(cmd.Context(), c, projectID, typeFilter, takeFilter, minScore, !noDetail, limit)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				out, mErr := json.MarshalIndent(rows, "", "  ")
				if mErr != nil {
					return mErr
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No clips matched (project reports %d clips).\n", total)
				return nil
			}
			for _, r := range rows {
				score := "-"
				if r.CalculatedScore != nil {
					score = fmt.Sprintf("%.0f", *r.CalculatedScore)
				}
				window := "-"
				if r.StartSeconds != nil && r.EndSeconds != nil {
					window = fmt.Sprintf("%.1f-%.1fs", *r.StartSeconds, *r.EndSeconds)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s | %-7s | score %-3s | %6.1fs | %-14s | %s\n",
					r.ClipID, r.MagicType, score, r.DurationSeconds, window, r.Title)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d of %d clips.\n", len(rows), total)
			return nil
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "Project ID (required)")
	cmd.Flags().StringVar(&typeFilter, "type", "all", "Filter by magic type: Clip, Hook, Episode, or all")
	cmd.Flags().StringVar(&takeFilter, "take", "", "Only clips cut from this take (take ObjectId or take sessionId UUID)")
	cmd.Flags().Float64Var(&minScore, "min-score", 0, "Only clips whose calculatedScore (0-100) is >= this")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max clips to return (0 = all)")
	cmd.Flags().BoolVar(&noDetail, "no-detail", false, "Skip the per-clip detail call (faster, but drops calculatedScore and Hook/Episode in-out points)")
	return cmd
}
