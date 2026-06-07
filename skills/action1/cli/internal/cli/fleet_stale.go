// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type staleRow struct {
	OrganizationID string  `json:"organization_id"`
	EndpointID     string  `json:"endpoint_id"`
	Name           string  `json:"name"`
	OS             string  `json:"os"`
	LastSeen       string  `json:"last_seen"`
	DaysSinceSeen  float64 `json:"days_since_seen"`
	OnlineStatus   string  `json:"online_status"`
	Reason         string  `json:"reason"`
}

// pp:data-source local
func newNovelFleetStaleCmd(flags *rootFlags) *cobra.Command {
	var dbPath, orgFilter string
	var days, limit int
	var includeOffline bool

	cmd := &cobra.Command{
		Use:         "stale",
		Short:       "Find endpoints that stopped checking in (or are offline) across all organizations.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Dark-agent detection across the whole fleet. Reads locally synced endpoints
from every organization and surfaces those whose last check-in is older than
--days, or that report an offline status. Action1 has no "show me dark agents"
query; this is a time-windowed scan over the local store.`,
		Example: `  action1-cli fleet stale --days 14 --agent
  action1-cli fleet stale --days 30 --org <org-uuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := fleetOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "endpoints", flags.maxAge)

			endpoints, err := fleetLoadAll(cmd.Context(), db, "endpoints")
			if err != nil {
				return err
			}

			now := time.Now()
			cutoff := time.Duration(days) * 24 * time.Hour
			rows := make([]staleRow, 0)
			for _, e := range endpoints {
				org := fleetOrgID(e)
				if orgFilter != "" && org != orgFilter {
					continue
				}
				lastSeenStr := fleetStrField(e, "last_seen")
				online := fleetStrField(e, "online_status")
				offline := includeOffline && online != "" && !isOnlineStatus(online)

				var daysSince float64
				stale := false
				reason := ""
				if t, ok := fleetParseTime(lastSeenStr); ok {
					age := now.Sub(t)
					daysSince = age.Hours() / 24
					if age >= cutoff {
						stale = true
						reason = "last seen > threshold"
					}
				} else if lastSeenStr == "" {
					// No last_seen recorded — treat as stale only when offline.
					daysSince = -1
				}
				if offline {
					stale = true
					if reason == "" {
						reason = "offline"
					} else {
						reason += " + offline"
					}
				}
				if !stale {
					continue
				}
				name := fleetStrField(e, "name")
				if name == "" {
					name = fleetStrField(e, "device_name")
				}
				rows = append(rows, staleRow{
					OrganizationID: org,
					EndpointID:     fleetStrField(e, "id"),
					Name:           name,
					OS:             fleetStrField(e, "OS"),
					LastSeen:       lastSeenStr,
					DaysSinceSeen:  round1(daysSince),
					OnlineStatus:   online,
					Reason:         reason,
				})
			}

			sort.SliceStable(rows, func(i, j int) bool {
				return rows[i].DaysSinceSeen > rows[j].DaysSinceSeen
			})
			if limit > 0 && len(rows) > limit {
				rows = rows[:limit]
			}

			header := []string{"ORG", "ENDPOINT", "NAME", "OS", "LAST_SEEN", "DAYS", "ONLINE", "REASON"}
			matrix := make([][]string, 0, len(rows))
			for _, r := range rows {
				matrix = append(matrix, []string{r.OrganizationID, r.EndpointID, r.Name, r.OS,
					r.LastSeen, fmtFloat(r.DaysSinceSeen), r.OnlineStatus, r.Reason})
			}
			return fleetEmit(cmd, flags, rows, header, matrix)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: standard cache location)")
	cmd.Flags().StringVar(&orgFilter, "org", fleetEnvOrg(), "Limit to one organization ID (default: $ACTION1_ORG_ID)")
	cmd.Flags().IntVar(&days, "days", 7, "Flag endpoints not seen within this many days")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum endpoints to return (0 = all)")
	cmd.Flags().BoolVar(&includeOffline, "include-offline", true, "Also flag endpoints reporting an offline status")
	return cmd
}

// isOnlineStatus reports whether an online_status string means the agent is up.
func isOnlineStatus(s string) bool {
	switch {
	case s == "":
		return true // unknown — don't flag on status alone
	default:
		ls := s
		for _, on := range []string{"Online", "online", "Connected", "connected", "Up"} {
			if ls == on {
				return true
			}
		}
		return false
	}
}

// round1 rounds to one decimal place.
func round1(f float64) float64 {
	if f < 0 {
		return f
	}
	return float64(int64(f*10+0.5)) / 10
}
