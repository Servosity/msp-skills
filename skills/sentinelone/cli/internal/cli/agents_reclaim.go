// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type reclaimAgent struct {
	ID           string   `json:"id"`
	UUID         string   `json:"uuid,omitempty"`
	ComputerName string   `json:"computer_name"`
	Site         string   `json:"site"`
	SiteID       string   `json:"site_id,omitempty"`
	Version      string   `json:"agent_version,omitempty"`
	LastActive   string   `json:"last_active_date"`
	DaysOffline  int      `json:"days_offline"`
	Reasons      []string `json:"reasons"`
	// NewerSameName names a newer record of the same endpoint name in the same
	// site. It is context for a seat already listed for being offline, never a
	// reason to list one: a shared hostname does not prove a replacement.
	NewerSameName string `json:"newer_record_same_name,omitempty"`
}

type reclaimSite struct {
	Site        string         `json:"site"`
	SiteID      string         `json:"site_id,omitempty"`
	Account     string         `json:"account,omitempty"`
	Reclaimable int            `json:"reclaimable"`
	Agents      []reclaimAgent `json:"agents"`
}

// newNovelAgentsReclaimCmd lists agents that hold a license but no live
// machine uses. An MSP decommissions what this prints, so it only names an
// agent it can prove is idle: SentinelOne reports it inactive (isActive
// explicitly false, see agentLiveness) AND its last check-in is known and at
// least --days ago. networkStatus plays no part: it is the network-isolation
// state, so an isolated agent that still checks in is never listed, and one
// that has also been silent --days or more is listed like any other. A newer
// record with the same endpoint name is shown as context on a listed seat,
// never as a reason to list one (two machines can share a hostname). An
// unknown active status or an unknown last check-in is counted, never listed.
// pp:data-source local
func newNovelAgentsReclaimCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var days int

	cmd := &cobra.Command{
		Use:   "reclaim",
		Short: "Agents still holding a license with no live machine behind them, per site",
		Long: `List the agents an MSP pays for that no live machine uses, grouped per site:
an agent is listed only when SentinelOne reports it inactive (isActive false:
it has stopped checking in to the console) and its last check-in
(lastActiveDate) is at least --days ago.

An active agent is never listed. The network status (connected or
disconnected) is not used: it is the "Disconnect from network" isolation
state, and an isolated agent keeps checking in. An agent with no readable
active status is counted as unknown_status, and one with no readable last
check-in as unknown_last_seen; neither is listed. When a newer record in the
same site carries the same endpoint name (a re-image or re-enrollment, or
simply two machines that share a hostname), a listed seat shows it as
newer_record_same_name: context to check, never a reason on its own.
Decommissioned agents are already off the license and are skipped. Sites are
grouped by site id, so two accounts' same-named sites stay apart.

Use this before a license true-up. Do NOT use it to rank endpoints by health
risk; use 'fleet-health stale' instead.`,
		Example: `  # Reclaimable seats, agents idle 45+ days
  sentinelone-cli agents reclaim --days 45 --agent

  # Default 30-day threshold, as a table
  sentinelone-cli agents reclaim`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if days < 1 {
				return usageErr(fmt.Errorf("--days must be at least 1, got %d", days))
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			db, err := openS1Store(cmd, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			if !hintIfUnsynced(cmd, db, "agents") {
				hintIfStale(cmd, db, "agents", flags.maxAge)
			}

			agents, err := loadAgents(cmd.Context(), db)
			if err != nil {
				return fmt.Errorf("loading agents: %w", err)
			}
			if len(agents) == 0 {
				return honestEmptyJSON(cmd, flags,
					"No agents in the local store. Run 'sentinelone-cli sync' first.", nil)
			}

			res := reclaimCandidates(agents, days, time.Now())
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"days":                   days,
					"agents_considered":      res.considered,
					"decommissioned_skipped": res.decommissioned,
					"unknown_status":         res.unknownStatus,
					"unknown_last_seen":      res.unknownLastSeen,
					"reclaimable_total":      res.total,
					"sites":                  res.sites,
				})
			}

			w := cmd.OutOrStdout()
			if res.total == 0 {
				fmt.Fprintf(w, "No reclaimable agents: none of %d agents is provably idle %d+ days.\n", res.considered, days)
				printReclaimUnjudged(w, res)
				return nil
			}
			fmt.Fprintf(w, "%d reclaimable seat(s) across %d site(s) (threshold %d days; %d agents considered):\n", res.total, len(res.sites), days, res.considered)
			for _, s := range res.sites {
				fmt.Fprintf(w, "\n%s: %d\n", s.Site, s.Reclaimable)
				for _, a := range s.Agents {
					note := strings.Join(a.Reasons, ", ")
					if a.NewerSameName != "" {
						note += "; newer record with this name: agent " + a.NewerSameName
					}
					fmt.Fprintf(w, "  %-28s %5dd  %s\n", clip(a.ComputerName, 28), a.DaysOffline, note)
				}
			}
			if res.unknownStatus > 0 || res.unknownLastSeen > 0 {
				fmt.Fprintln(w)
				printReclaimUnjudged(w, res)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local data directory)")
	cmd.Flags().IntVar(&days, "days", 30, "Minimum whole days since an inactive agent's last check-in (at least 1)")
	return cmd
}

// printReclaimUnjudged names the agents reclaim could not judge, so an empty
// or short list is never read as "every other seat is in use".
func printReclaimUnjudged(w io.Writer, res reclaimResult) {
	if res.unknownStatus > 0 {
		fmt.Fprintf(w, "%d agent(s) have no readable active status (isActive) and were not judged.\n", res.unknownStatus)
	}
	if res.unknownLastSeen > 0 {
		fmt.Fprintf(w, "%d agent(s) have no readable last check-in and were not judged.\n", res.unknownLastSeen)
	}
}

type reclaimResult struct {
	sites                                                             []reclaimSite
	total, considered, decommissioned, unknownStatus, unknownLastSeen int
}

func reclaimCandidates(agents []map[string]any, days int, now time.Time) reclaimResult {
	var res reclaimResult
	type rec struct {
		a        map[string]any
		last     time.Time
		lastOK   bool
		offline  bool
		site     siteRef
		nameKey  string
		daysIdle int
	}
	var recs []*rec
	newest := map[string]*rec{} // site key + name -> newest record with a known check-in
	for _, a := range agents {
		if agentDecommissioned(a) {
			res.decommissioned++
			continue
		}
		res.considered++
		live := agentLiveness(a)
		if live == livenessUnknown {
			res.unknownStatus++
		}
		r := &rec{a: a, offline: live == livenessOffline, site: agentSiteRef(a),
			nameKey: strings.ToLower(strings.TrimSpace(gstr(a, "computerName")))}
		r.last, r.lastOK = parseS1Time(gstr(a, "lastActiveDate"))
		if !r.lastOK {
			res.unknownLastSeen++
		} else {
			r.daysIdle = int(now.Sub(r.last).Hours() / 24)
		}
		recs = append(recs, r)
		if r.lastOK && r.nameKey != "" {
			k := r.site.Key + "\x00" + r.nameKey
			if cur, ok := newest[k]; !ok || r.last.After(cur.last) {
				newest[k] = r
			}
		}
	}
	bySite := map[string]*reclaimSite{}
	for _, r := range recs {
		// Only a provably idle seat is listed: explicitly inactive, with a
		// known check-in at least the threshold ago.
		if !r.offline || !r.lastOK || r.daysIdle < days {
			continue
		}
		ra := reclaimAgent{
			ID: gstr(r.a, "id"), UUID: gstr(r.a, "uuid"), ComputerName: gstrFirst(r.a, "computerName", "id"),
			Site: r.site.Label(), SiteID: r.site.ID, Version: gstr(r.a, "agentVersion"), LastActive: gstr(r.a, "lastActiveDate"),
			DaysOffline: r.daysIdle, Reasons: []string{fmt.Sprintf("offline %dd", r.daysIdle)},
		}
		if r.nameKey != "" {
			if keep := newest[r.site.Key+"\x00"+r.nameKey]; keep != nil && keep != r && keep.last.After(r.last) {
				ra.NewerSameName = gstrFirst(keep.a, "id", "uuid")
			}
		}
		s := bySite[r.site.Key]
		if s == nil {
			s = &reclaimSite{Site: r.site.Label(), SiteID: r.site.ID, Account: r.site.Account}
			bySite[r.site.Key] = s
		}
		s.Agents = append(s.Agents, ra)
		s.Reclaimable++
		res.total++
	}
	res.sites = []reclaimSite{}
	for _, s := range bySite {
		sort.SliceStable(s.Agents, func(i, j int) bool {
			if s.Agents[i].DaysOffline != s.Agents[j].DaysOffline {
				return s.Agents[i].DaysOffline > s.Agents[j].DaysOffline
			}
			return s.Agents[i].ID < s.Agents[j].ID
		})
		res.sites = append(res.sites, *s)
	}
	sort.SliceStable(res.sites, func(i, j int) bool {
		if res.sites[i].Reclaimable != res.sites[j].Reclaimable {
			return res.sites[i].Reclaimable > res.sites[j].Reclaimable
		}
		if res.sites[i].Site != res.sites[j].Site {
			return res.sites[i].Site < res.sites[j].Site
		}
		return res.sites[i].SiteID < res.sites[j].SiteID
	})
	return res
}
