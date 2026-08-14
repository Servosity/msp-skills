// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command. Preserved across `generate --force`.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/spf13/cobra"
)

// corkScoreImpact is the full score-history point, including the four-way
// impact decomposition the embedded client payload does not carry.
type corkScoreImpact struct {
	Score               int     `json:"score"`
	CreatedAt           string  `json:"created_at"`
	ClaimsImpact        float64 `json:"claims_impact"`
	ComplianceImpact    float64 `json:"compliance_impact"`
	CoverageImpact      float64 `json:"coverage_impact"`
	VulnerabilityImpact float64 `json:"vulnerability_impact"`
}

type scoreComponentDelta struct {
	Component   string  `json:"component"`
	Then        float64 `json:"then"`
	Now         float64 `json:"now"`
	Delta       float64 `json:"delta"`
	ShareOfMove float64 `json:"share_of_move"`
}

type scoreAttributeView struct {
	ClientUUID string                `json:"client_uuid"`
	Client     string                `json:"client,omitempty"`
	Window     string                `json:"window"`
	ScoreThen  int                   `json:"score_then"`
	ScoreNow   int                   `json:"score_now"`
	Delta      int                   `json:"delta"`
	PointsUsed int                   `json:"points_used"`
	FirstAt    string                `json:"first_at"`
	LastAt     string                `json:"last_at"`
	Components []scoreComponentDelta `json:"components"`
	ScanCapHit bool                  `json:"scan_cap_hit"`
	Note       string                `json:"note,omitempty"`
}

func newNovelScoreAttributeCmd(flags *rootFlags) *cobra.Command {
	var flagSince string
	var flagMaxScanPages int

	cmd := &cobra.Command{
		Use:   "attribute [client-uuid]",
		Short: "Explain why a client's Cork score moved, broken out by claims, compliance, coverage, and vulnerability impact.",
		Long: "Explain why a client's Cork score moved, broken out by claims, compliance,\n" +
			"coverage, and vulnerability impact.\n\n" +
			"Cork's score-history endpoint returns those four impact components per\n" +
			"timestamp but never differences them, so the platform can show a trend line\n" +
			"and never a cause. This command differences each component across the window\n" +
			"and ranks them by how much of the move they account for.\n\n" +
			"Use this command to explain WHY one named client's Cork score moved. Do NOT\n" +
			"use this command to find which clients moved across the whole book of\n" +
			"business; use 'score regressions' instead.",
		Example: "  cork-cli score attribute 3f2a9c14-7b6d-4e21-9a8c-1d5e2f0b4c77 --since 30d --agent",
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "client-uuid=3f2a9c14-7b6d-4e21-9a8c-1d5e2f0b4c77",
			// A client uuid that does not exist yields HTTP 404 -> exit 3. That is
			// a correct not-found answer, not a command failure.
			"pp:typed-exit-codes": "0,3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "score attribute")
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a client uuid is required"))
			}
			clientUUID := args[0]

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			window, err := corkSince(flagSince, 30*24*time.Hour)
			if err != nil {
				return err
			}
			cutoff := time.Now().Add(-window)

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			path := "/clients/" + corkPathSeg(clientUUID) + "/score-history"
			raw, _, capHit, err := corkFetchPages(ctx, c, path, map[string]string{
				"created_after": cutoff.UTC().Format(time.RFC3339),
			}, flagMaxScanPages)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			points := make([]corkScoreImpact, 0, len(raw))
			undecoded := 0
			for _, r := range raw {
				var p corkScoreImpact
				if json.Unmarshal(r, &p) != nil {
					undecoded++
					continue
				}
				if _, ok := corkParseTime(p.CreatedAt); !ok {
					continue
				}
				points = append(points, p)
			}

			if len(points) == 0 && undecoded > 0 {
				return corkDecodeFailure("score-history point(s)", undecoded)
			}

			view := scoreAttributeView{
				ClientUUID: clientUUID,
				Window:     window.String(),
				PointsUsed: len(points),
				Components: make([]scoreComponentDelta, 0, 4),
				ScanCapHit: capHit,
			}

			if len(points) < 2 {
				view.Note = fmt.Sprintf("only %d score point(s) in the last %s; at least 2 are needed to attribute a move", len(points), window)
				if capHit {
					view.Note += "; the page scan cap was reached, so raise --max-scan-pages to widen the window"
				}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}

			corkSortStable(points, func(a, b corkScoreImpact) bool {
				at, _ := corkParseTime(a.CreatedAt)
				bt, _ := corkParseTime(b.CreatedAt)
				return at.Before(bt)
			})
			oldest := points[0]
			newest := points[len(points)-1]

			view.ScoreThen = oldest.Score
			view.ScoreNow = newest.Score
			view.Delta = newest.Score - oldest.Score
			view.FirstAt = oldest.CreatedAt
			view.LastAt = newest.CreatedAt

			comps := []struct {
				name      string
				then, now float64
			}{
				{"claims", oldest.ClaimsImpact, newest.ClaimsImpact},
				{"compliance", oldest.ComplianceImpact, newest.ComplianceImpact},
				{"coverage", oldest.CoverageImpact, newest.CoverageImpact},
				{"vulnerability", oldest.VulnerabilityImpact, newest.VulnerabilityImpact},
			}
			// Share of move is computed over the total absolute component movement,
			// not over the score delta: the components are impact magnitudes and do
			// not have to sum to the score change.
			var totalAbs float64
			for _, cm := range comps {
				totalAbs += math.Abs(cm.now - cm.then)
			}
			for _, cm := range comps {
				d := cm.now - cm.then
				share := 0.0
				if totalAbs > 0 {
					share = math.Round((math.Abs(d)/totalAbs)*1000) / 10
				}
				view.Components = append(view.Components, scoreComponentDelta{
					Component:   cm.name,
					Then:        cm.then,
					Now:         cm.now,
					Delta:       math.Round(d*100) / 100,
					ShareOfMove: share,
				})
			}
			corkSortStable(view.Components, func(a, b scoreComponentDelta) bool {
				return math.Abs(a.Delta) > math.Abs(b.Delta)
			})
			if totalAbs == 0 {
				view.Note = fmt.Sprintf("score moved %+d but no impact component changed across the window", view.Delta)
			}
			if capHit {
				// Points beyond the cap were never read, so score_then is the
				// oldest FETCHED point rather than the oldest in the window.
				if view.Note != "" {
					view.Note += "; "
				}
				view.Note += fmt.Sprintf("the page scan cap of %d was reached, so the reported window start is the oldest point fetched, not necessarily the oldest in %s", flagMaxScanPages, window)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "score %d -> %d (%+d) over %s, %d points\n\n", view.ScoreThen, view.ScoreNow, view.Delta, window, view.PointsUsed)
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "COMPONENT\tTHEN\tNOW\tDELTA\tSHARE")
			for _, cm := range view.Components {
				fmt.Fprintf(tw, "%s\t%.2f\t%.2f\t%+.2f\t%.1f%%\n", cm.Component, cm.Then, cm.Now, cm.Delta, cm.ShareOfMove)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "\n"+view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "30d", "Window to attribute the move over (30d, 7d, 1w)")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", corkDefaultScanPages, "Maximum score-history pages to read")
	return cmd
}
