// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature command.
//
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"rocketcyber-pp-cli/internal/cliutil"
)

type scorePoint struct {
	Date  string
	Score float64
}

type trendView struct {
	StartDate    string  `json:"start_date,omitempty"`
	EndDate      string  `json:"end_date,omitempty"`
	FirstScore   float64 `json:"first_score"`
	LastScore    float64 `json:"last_score"`
	Delta        float64 `json:"delta"`
	Direction    string  `json:"direction"`
	MinScore     float64 `json:"min_score"`
	MaxScore     float64 `json:"max_score"`
	AverageScore float64 `json:"average_score"`
	DataPoints   int     `json:"data_points"`
	Note         string  `json:"note,omitempty"`
}

// computeScoreTrend derives first/last/delta/direction from a secure-score
// daily series, sorted by date. Direction is flat when |delta| < 0.5.
func computeScoreTrend(points []scorePoint) trendView {
	view := trendView{Direction: "flat"}
	if len(points) == 0 {
		return view
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Date < points[j].Date })
	view.DataPoints = len(points)
	view.FirstScore = points[0].Score
	view.LastScore = points[len(points)-1].Score
	view.Delta = math.Round((view.LastScore-view.FirstScore)*100) / 100
	switch {
	case view.Delta >= 0.5:
		view.Direction = "improving"
	case view.Delta <= -0.5:
		view.Direction = "declining"
	}
	view.MinScore = points[0].Score
	view.MaxScore = points[0].Score
	var sum float64
	for _, p := range points {
		if p.Score < view.MinScore {
			view.MinScore = p.Score
		}
		if p.Score > view.MaxScore {
			view.MaxScore = p.Score
		}
		sum += p.Score
	}
	view.AverageScore = math.Round(sum/float64(len(points))*100) / 100
	return view
}

func newNovelOfficeTrendCmd(flags *rootFlags) *cobra.Command {
	var flagAccountID int

	cmd := &cobra.Command{
		Use:   "trend",
		Short: "First/last/delta/direction computed over the Microsoft 365 secure-score daily series.",
		Long: strings.Trim(`
Turns the raw Microsoft 365 secure-score daily series from /office into a
trend: first and last score, delta, direction (improving, declining, flat),
min/max/average, and the number of data points.

Use this for the secure-score direction/delta over time. Do NOT use it to
dump the raw daily series; use 'office' instead.
`, "\n"),
		Example: strings.Trim(`
  rocketcyber-cli office trend --account-id 2 --json
  rocketcyber-cli office trend --agent
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would fetch /office and compute the secure-score trend")
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			params := map[string]string{}
			if flagAccountID != 0 {
				params["accountId"] = strconv.Itoa(flagAccountID)
			}
			data, err := c.Get(cmd.Context(), "/office", params)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			series := data
			var probe map[string]json.RawMessage
			startDate, endDate := "", ""
			if err := json.Unmarshal(data, &probe); err == nil {
				if nested, ok := probe["secureScoreProgress"]; ok {
					series = nested
					if err := json.Unmarshal(nested, &probe); err != nil {
						probe = nil
					}
				}
				if probe != nil {
					startDate = extractString(probe, "startDate")
					endDate = extractString(probe, "endDate")
					if d, ok := probe["data"]; ok {
						series = d
					}
				}
			}
			var rawPoints []json.RawMessage
			_ = json.Unmarshal(series, &rawPoints)
			points := make([]scorePoint, 0, len(rawPoints))
			for _, rp := range rawPoints {
				var pp map[string]json.RawMessage
				if err := json.Unmarshal(rp, &pp); err != nil {
					continue
				}
				score, ok := cliutil.ExtractNumber(pp, "secureScorePercentage")
				if !ok {
					continue
				}
				points = append(points, scorePoint{Date: extractString(pp, "detectionDate"), Score: score})
			}
			view := computeScoreTrend(points)
			view.StartDate = startDate
			view.EndDate = endDate
			if view.DataPoints == 0 {
				view.Note = "no secure-score data points returned by /office for this scope"
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&flagAccountID, "account-id", 0, "Account ID to scope the secure-score report")
	return cmd
}
