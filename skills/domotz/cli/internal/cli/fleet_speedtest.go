// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature. WAN speed-test trends across the fleet: per-site
// latest download/upload plus a fleet min/avg/max and worst-site ranking. The
// API returns speed history one agent at a time. Live fan-out across agents.

package cli

import (
	"context"
	"domotz-pp-cli/internal/cliutil"
	"encoding/json"
	"math"
	"net/url"

	"github.com/spf13/cobra"
)

type speedSample struct {
	Timestamp string  `json:"timestamp"`
	Values    []int64 `json:"values"` // [download_bps, upload_bps]
}

type speedSiteRow struct {
	Site         string  `json:"site"`
	AgentID      string  `json:"agent_id"`
	Timestamp    string  `json:"timestamp"`
	DownloadMbps float64 `json:"download_mbps"`
	UploadMbps   float64 `json:"upload_mbps"`
}

type speedRange struct {
	Min float64 `json:"min"`
	Avg float64 `json:"avg"`
	Max float64 `json:"max"`
}

type speedReport struct {
	SitesWithData int            `json:"sites_with_data"`
	DownloadMbps  speedRange     `json:"download_mbps"`
	WorstSite     string         `json:"worst_site"`
	Sites         []speedSiteRow `json:"sites"`
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// pp:data-source live
func newNovelFleetSpeedtestCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "speedtest",
		Short: "WAN speed-test trends across the fleet, with worst-site ranking",
		Long: "Aggregate each site's latest WAN speed test into a fleet view — per-site download/" +
			"upload in Mbps plus fleet min/avg/max download and the worst-performing site. Fetches " +
			"live from the API (needs DOMOTZ_API_KEY).",
		Example:     "  domotz-cli fleet speedtest --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := novelRequireLive(flags); err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			agents, err := fleetAgentsLive(cmd.Context(), c)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			results, errs := cliutil.FanoutRun(cmd.Context(), agents,
				func(a fleetAgentRef) string { return a.Name },
				func(ctx context.Context, a fleetAgentRef) (*speedSiteRow, error) {
					data, err := c.Get(ctx, "/agent/"+url.PathEscape(a.ID)+"/history/network/speed", map[string]string{})
					if err != nil {
						return nil, err
					}
					var samples []speedSample
					if err := json.Unmarshal(data, &samples); err != nil || len(samples) == 0 {
						return nil, nil //nolint:nilnil // no data for this site is not an error
					}
					latest := samples[0]
					for _, s := range samples[1:] {
						if s.Timestamp > latest.Timestamp {
							latest = s
						}
					}
					row := &speedSiteRow{
						Site:      a.site(),
						AgentID:   a.ID,
						Timestamp: latest.Timestamp,
					}
					if len(latest.Values) > 0 {
						row.DownloadMbps = round2(float64(latest.Values[0]) / 1e6)
					}
					if len(latest.Values) > 1 {
						row.UploadMbps = round2(float64(latest.Values[1]) / 1e6)
					}
					return row, nil
				},
				cliutil.WithConcurrency(6),
			)
			cliutil.FanoutReportErrors(cmd.ErrOrStderr(), errs)

			report := speedReport{Sites: make([]speedSiteRow, 0)}
			minDL, maxDL, sumDL := math.MaxFloat64, 0.0, 0.0
			worst := ""
			for _, r := range results {
				if r.Value == nil {
					continue
				}
				row := *r.Value
				report.Sites = append(report.Sites, row)
				dl := row.DownloadMbps
				sumDL += dl
				if dl < minDL {
					minDL = dl
					worst = row.Site
				}
				if dl > maxDL {
					maxDL = dl
				}
			}
			report.SitesWithData = len(report.Sites)
			if report.SitesWithData > 0 {
				report.DownloadMbps = speedRange{
					Min: round2(minDL),
					Avg: round2(sumDL / float64(report.SitesWithData)),
					Max: round2(maxDL),
				}
				report.WorstSite = worst
			}
			return printJSONFiltered(cmd.OutOrStdout(), report, flags)
		},
	}
	return cmd
}
