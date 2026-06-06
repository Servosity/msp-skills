// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature command (Printing Press transcendence): the
// cross-tenant remediation worklist. Groups synced Spotlight vulnerabilities by
// remediation action so one row answers "what single fix clears the most hosts
// and tenants" — a three-way join (vulns × hosts × remediations) the live API
// only exposes one CID at a time.

package cli

import (
	"slices"
	"sort"

	"github.com/spf13/cobra"
)

// remediationGroup is one remediation action with its cross-tenant blast radius.
type remediationGroup struct {
	Action     string         `json:"action"`
	Vulns      int            `json:"vulns"`
	Hosts      int            `json:"hosts"`
	CIDs       int            `json:"cids"`
	Severities map[string]int `json:"severities,omitempty"`
	SampleCVEs []string       `json:"sample_cves,omitempty"`
}

// fleetRemediateView is the `fleet remediate` response envelope.
type fleetRemediateView struct {
	Groups                  []remediationGroup `json:"groups"`
	TotalVulns              int                `json:"total_vulns"`
	VulnsWithoutRemediation int                `json:"vulns_without_remediation"`
	Note                    string             `json:"note,omitempty"`
}

const sampleCVECap = 5

// remediationActions extracts the remediation action strings from a vuln's raw
// payload. Falcon's remediation facet nests either entities ([{id, action}])
// or bare ids ({ids: [...]}).
func remediationActions(o map[string]any) []string {
	if o == nil {
		return nil
	}
	rem, ok := o["remediation"].(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	if entities, ok := rem["entities"].([]any); ok {
		for _, x := range entities {
			if e, ok := x.(map[string]any); ok {
				if a := firstString(e, "action", "id"); a != "" {
					out = append(out, a)
				}
			}
		}
	}
	if len(out) == 0 {
		out = append(out, rawStrings(rem, "ids")...)
	}
	return out
}

// vulnHostKey identifies the affected host from a vuln's raw payload.
func vulnHostKey(o map[string]any) string {
	if o == nil {
		return ""
	}
	if hi, ok := o["host_info"].(map[string]any); ok {
		if h := firstString(hi, "hostname", "local_ip"); h != "" {
			return h
		}
	}
	return firstString(o, "aid", "host_id")
}

// remediationWorklist groups vulns by remediation action. Pure function.
func remediationWorklist(ents []fleetEntity, severity string) fleetRemediateView {
	view := fleetRemediateView{}
	type agg struct {
		vulns      int
		hosts      map[string]bool
		cids       map[string]bool
		severities map[string]int
		cves       []string
	}
	groups := map[string]*agg{}
	order := []string{}

	for _, e := range ents {
		if e.Kind != kindVuln {
			continue
		}
		if severity != "" && e.Severity != severity {
			continue
		}
		view.TotalVulns++
		o := rawObj(e)
		actions := remediationActions(o)
		if len(actions) == 0 {
			view.VulnsWithoutRemediation++
			continue
		}
		host := vulnHostKey(o)
		for _, action := range dedupeStrings(actions) {
			g, ok := groups[action]
			if !ok {
				g = &agg{hosts: map[string]bool{}, cids: map[string]bool{}, severities: map[string]int{}}
				groups[action] = g
				order = append(order, action)
			}
			g.vulns++
			if host != "" {
				g.hosts[host] = true
			}
			g.cids[e.CID] = true
			if e.Severity != "" {
				g.severities[e.Severity]++
			}
			if e.Name != "" && len(g.cves) < sampleCVECap && !slices.Contains(g.cves, e.Name) {
				g.cves = append(g.cves, e.Name)
			}
		}
	}

	view.Groups = make([]remediationGroup, 0, len(order))
	for _, action := range order {
		g := groups[action]
		view.Groups = append(view.Groups, remediationGroup{
			Action:     action,
			Vulns:      g.vulns,
			Hosts:      len(g.hosts),
			CIDs:       len(g.cids),
			Severities: g.severities,
			SampleCVEs: g.cves,
		})
	}
	sort.SliceStable(view.Groups, func(i, j int) bool {
		if view.Groups[i].Hosts != view.Groups[j].Hosts {
			return view.Groups[i].Hosts > view.Groups[j].Hosts
		}
		if view.Groups[i].Vulns != view.Groups[j].Vulns {
			return view.Groups[i].Vulns > view.Groups[j].Vulns
		}
		return view.Groups[i].Action < view.Groups[j].Action
	})

	switch {
	case view.TotalVulns == 0:
		view.Note = "no synced vulnerabilities match; run 'fleet sync' first (vulns sync with remediation facets), or widen --severity"
	case len(view.Groups) == 0:
		view.Note = "synced vulnerabilities carry no remediation facets; re-run 'fleet sync' to refresh them with facet data"
	}
	return view
}

// pp:data-source local
func newNovelFleetRemediateCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var severity string
	cmd := &cobra.Command{
		Use:   "remediate",
		Short: "Group fleet-wide exposure by remediation action: which single fix clears the most hosts and tenants",
		Long: "Use this command to group exposure by REMEDIATION ACTION (what to fix, " +
			"across how many hosts and tenants), joined offline from synced Spotlight " +
			"vulnerabilities and their remediation facets. Run 'fleet sync' first.\n" +
			"Do NOT use this command for the raw severity-ranked vulnerability list; use " +
			"'fleet vulns' instead.",
		Example:     "  crowdstrike-cli fleet remediate --severity critical --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			st, err := openFleetStore(cmd.Context(), resolveFleetDB(dbPath))
			if err != nil {
				return configErr(err)
			}
			defer st.Close()
			if !hintIfFleetUnsynced(cmd, st) {
				hintIfFleetStale(cmd, st, flags.maxAge)
			}
			ents, err := loadFleetEntitiesFrom(cmd.Context(), st, kindVuln, true)
			if err != nil {
				return configErr(err)
			}
			return flags.printJSON(cmd, remediationWorklist(ents, severity))
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local SQLite store path (default: standard data dir)")
	cmd.Flags().StringVar(&severity, "severity", "", "Filter to one severity: critical|high|medium|low")
	return cmd
}
