// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var s1VersionRe = regexp.MustCompile(`^\d+(\.\d+)*$`)

type planAgent struct {
	ID           string `json:"id"`
	ComputerName string `json:"computer_name"`
	Version      string `json:"agent_version"`
	LastActive   string `json:"last_active_date,omitempty"`
}

type planGroup struct {
	Site      string      `json:"site"`
	SiteID    string      `json:"site_id,omitempty"`
	OS        string      `json:"os"`
	Target    string      `json:"target_version"`
	Reachable []planAgent `json:"reachable"`
	Offline   []planAgent `json:"offline"`
	// UnknownStatus holds agents with no readable isActive: never listed as
	// reachable, since nothing shows a job could reach them.
	UnknownStatus []planAgent `json:"unknown_status"`
}

// newNovelVersionsPlanCmd turns the version spread into an upgrade worklist:
// per site and OS, the agents still behind the target version, with the
// offline ones set aside because an upgrade job cannot reach them. Reachable
// is agentLiveness online (isActive true), never the network-isolation
// status; unknown liveness is its own bucket, never reachable. Agent
// packages are per OS, so the default target is the newest version seen
// among agents of the same OS, never one OS's version applied to another.
// pp:data-source local
func newNovelVersionsPlanCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var target string
	var osName string

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Per-site upgrade worklist: agents behind the target version, reachable vs offline",
		Long: `List, per site and OS, the agents still behind the target agent version.
Reachable agents are the ones SentinelOne reports active (isActive true:
checking in to the console), which an upgrade job can take now; offline
agents (isActive false) are set aside so the wave does not stall on them.
Agents with no readable isActive are listed as unknown_status, never as
reachable. The network status is not used: it is the "Disconnect from
network" isolation state, and an isolated agent still checks in, so it is
listed as reachable.

The default target is the newest version seen among agents of the same OS:
Windows, macOS and Linux agents ship separate packages, so one OS's version
is never applied to another. --target names a version explicitly and then
needs --os. Decommissioned agents and agents with no reported version are
counted, not planned.

Use 'versions rollout' to watch a wave progress over time.`,
		Example: `  # Worklist toward the newest version per OS
  sentinelone-cli versions plan --agent

  # Windows agents behind a chosen version
  sentinelone-cli versions plan --os windows --target 24.1.2.6`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			osName = strings.ToLower(strings.TrimSpace(osName))
			target = strings.TrimSpace(target)
			if target != "" && osName == "" {
				return usageErr(fmt.Errorf("--target needs --os (agent packages are per OS)"))
			}
			if target != "" && !s1VersionRe.MatchString(target) {
				return usageErr(fmt.Errorf("--target %q is not a dotted numeric agent version (e.g. 24.1.2.6)", target))
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

			p := upgradePlan(agents, osName, target)
			if osName != "" && p.considered == 0 {
				return usageErr(fmt.Errorf("no agents with os %q in the local store (seen: %s)", osName, strings.Join(p.osSeen, ", ")))
			}
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{
					"target_by_os":           p.targets,
					"agents_considered":      p.considered,
					"behind_total":           p.reachable + p.offline + p.unknownStatus,
					"reachable_total":        p.reachable,
					"offline_total":          p.offline,
					"unknown_status_total":   p.unknownStatus,
					"unknown_version":        p.unknownVersion,
					"decommissioned_skipped": p.decommissioned,
					"groups":                 p.groups,
				})
			}
			w := cmd.OutOrStdout()
			if len(p.groups) == 0 && p.unknownVersion == 0 {
				fmt.Fprintf(w, "Nothing to upgrade: all %d agents are at or above the target %s.\n", p.considered, formatTargets(p.targets))
				return nil
			}
			fmt.Fprintf(w, "%d agent(s) behind %s: %d reachable now, %d offline, %d unknown status.\n", p.reachable+p.offline+p.unknownStatus, formatTargets(p.targets), p.reachable, p.offline, p.unknownStatus)
			for _, g := range p.groups {
				fmt.Fprintf(w, "\n%s (%s -> %s): %d reachable, %d offline, %d unknown status\n", g.Site, g.OS, g.Target, len(g.Reachable), len(g.Offline), len(g.UnknownStatus))
				for _, a := range g.Reachable {
					fmt.Fprintf(w, "  %-28s %s\n", clip(a.ComputerName, 28), a.Version)
				}
			}
			if p.unknownVersion > 0 {
				fmt.Fprintf(w, "\n%d agent(s) report no version and were not planned.\n", p.unknownVersion)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: the CLI's local data directory)")
	cmd.Flags().StringVar(&target, "target", "", "Target agent version (needs --os; default: newest version seen per OS)")
	cmd.Flags().StringVar(&osName, "os", "", "Only plan agents of this OS type as SentinelOne reports it (windows, macos, linux)")
	return cmd
}

type upgradePlanResult struct {
	targets                                                                       map[string]string
	groups                                                                        []planGroup
	osSeen                                                                        []string
	considered, reachable, offline, unknownStatus, unknownVersion, decommissioned int
}

func upgradePlan(agents []map[string]any, osFilter, target string) upgradePlanResult {
	res := upgradePlanResult{targets: map[string]string{}}
	seen := map[string]bool{}
	var live []map[string]any
	for _, a := range agents {
		if agentDecommissioned(a) {
			res.decommissioned++
			continue
		}
		os := agentOS(a)
		if !seen[os] {
			seen[os] = true
			res.osSeen = append(res.osSeen, os)
		}
		if osFilter != "" && os != osFilter {
			continue
		}
		res.considered++
		v := strings.TrimSpace(gstr(a, "agentVersion"))
		if v == "" {
			res.unknownVersion++
			continue
		}
		live = append(live, a)
		if target == "" && (res.targets[os] == "" || compareS1Version(v, res.targets[os]) > 0) {
			res.targets[os] = v
		}
	}
	sort.Strings(res.osSeen)
	if target != "" {
		res.targets[osFilter] = target
	}
	byKey := map[string]*planGroup{}
	for _, a := range live {
		os := agentOS(a)
		goal := res.targets[os]
		v := strings.TrimSpace(gstr(a, "agentVersion"))
		if compareS1Version(v, goal) >= 0 {
			continue
		}
		site := agentSiteRef(a)
		k := site.Key + "\x00" + os
		g := byKey[k]
		if g == nil {
			g = &planGroup{Site: site.Label(), SiteID: site.ID, OS: os, Target: goal, Reachable: []planAgent{}, Offline: []planAgent{}, UnknownStatus: []planAgent{}}
			byKey[k] = g
		}
		pa := planAgent{ID: gstrFirst(a, "id", "uuid"), ComputerName: gstrFirst(a, "computerName", "id"), Version: v, LastActive: gstr(a, "lastActiveDate")}
		switch agentLiveness(a) {
		case livenessOnline:
			g.Reachable = append(g.Reachable, pa)
			res.reachable++
		case livenessOffline:
			g.Offline = append(g.Offline, pa)
			res.offline++
		default:
			g.UnknownStatus = append(g.UnknownStatus, pa)
			res.unknownStatus++
		}
	}
	res.groups = []planGroup{}
	for _, g := range byKey {
		res.groups = append(res.groups, *g)
	}
	sort.SliceStable(res.groups, func(i, j int) bool {
		ri, rj := len(res.groups[i].Reachable), len(res.groups[j].Reachable)
		if ri != rj {
			return ri > rj
		}
		if res.groups[i].Site != res.groups[j].Site {
			return res.groups[i].Site < res.groups[j].Site
		}
		if res.groups[i].SiteID != res.groups[j].SiteID {
			return res.groups[i].SiteID < res.groups[j].SiteID
		}
		return res.groups[i].OS < res.groups[j].OS
	})
	return res
}

func formatTargets(t map[string]string) string {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+" "+t[k])
	}
	return strings.Join(parts, ", ")
}
