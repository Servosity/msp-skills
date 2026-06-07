// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type triageAlert struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Severity       string `json:"severity"`
	DeviceHostname string `json:"device_hostname,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
	IsResolved     bool   `json:"is_resolved"`
}

type triageCluster struct {
	Key        string         `json:"key"`
	Count      int            `json:"count"`
	BySeverity map[string]int `json:"by_severity"`
	Samples    []triageAlert  `json:"samples"`
}

type triageResult struct {
	GroupBy         string          `json:"group_by"`
	SeverityFilter  string          `json:"severity_filter,omitempty"`
	IncludeResolved bool            `json:"include_resolved"`
	TotalAlerts     int             `json:"total_alerts"`
	Count           int             `json:"count"`
	Clusters        []triageCluster `json:"clusters"`
}

var triageGroupBys = map[string]bool{"group": true, "severity": true, "name": true}
var alertSeverities = map[string]bool{"information": true,
	"info": true, "warning": true, "critical": true, "emergency": true}

const triageSampleCap = 5

// lvlComputeAlertTriage clusters alerts (unresolved by default) by group,
// severity, or name, with per-cluster severity breakdowns and samples.
func lvlComputeAlertTriage(alerts []lvlAlert, devices []lvlDevice, groups []lvlGroup, severityFilter, groupBy string, includeResolved bool) triageResult {
	res := triageResult{GroupBy: groupBy, SeverityFilter: severityFilter, IncludeResolved: includeResolved}
	sf := strings.ToLower(strings.TrimSpace(severityFilter))
	if sf == "info" {
		// Advertised alias: the live API stores "information".
		sf = "information"
	}

	devGroup := map[string]string{}
	for _, d := range devices {
		devGroup[d.ID] = d.GroupID
	}
	idx := lvlBuildGroupIndex(groups)

	type bucket struct {
		count   int
		bySev   map[string]int
		samples []triageAlert
	}
	buckets := map[string]*bucket{}
	order := []string{}

	for _, a := range alerts {
		if !includeResolved && a.IsResolved {
			continue
		}
		if sf != "" && strings.ToLower(a.Severity) != sf {
			continue
		}
		res.TotalAlerts++

		var key string
		switch groupBy {
		case "severity":
			key = orUnknown(a.Severity)
		case "name":
			key = orUnknown(a.Name)
		default: // group
			key = idx.name(devGroup[a.DeviceID])
		}

		b, ok := buckets[key]
		if !ok {
			b = &bucket{bySev: map[string]int{}}
			buckets[key] = b
			order = append(order, key)
		}
		b.count++
		b.bySev[orUnknown(a.Severity)]++
		b.samples = append(b.samples, triageAlert{
			ID: a.ID, Name: a.Name, Severity: a.Severity,
			DeviceHostname: a.DeviceHostname, StartedAt: a.StartedAt, IsResolved: a.IsResolved,
		})
	}

	for _, k := range order {
		b := buckets[k]
		sort.SliceStable(b.samples, func(i, j int) bool {
			wi, wj := lvlSeverityWeight(b.samples[i].Severity), lvlSeverityWeight(b.samples[j].Severity)
			if wi != wj {
				return wi > wj
			}
			return b.samples[i].StartedAt > b.samples[j].StartedAt
		})
		samples := b.samples
		if len(samples) > triageSampleCap {
			samples = samples[:triageSampleCap]
		}
		res.Clusters = append(res.Clusters, triageCluster{Key: k, Count: b.count, BySeverity: b.bySev, Samples: samples})
	}
	sort.SliceStable(res.Clusters, func(i, j int) bool {
		if res.Clusters[i].Count != res.Clusters[j].Count {
			return res.Clusters[i].Count > res.Clusters[j].Count
		}
		return res.Clusters[i].Key < res.Clusters[j].Key
	})
	res.Count = len(res.Clusters)
	return res
}

// pp:data-source local
func newNovelAlertTriageCmd(flags *rootFlags) *cobra.Command {
	var severity string
	var groupBy string
	var includeResolved bool
	var dbPath string

	cmd := &cobra.Command{
		Use:         "alert-triage",
		Short:       "Cluster unresolved alerts by group, severity, or name with device context",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Cluster alerts so systemic fires surface above one-off noise, computed
offline from the local store. By default only unresolved alerts are included;
add --include-resolved to fold in resolved ones. Choose the clustering with
--group-by (group|severity|name) and narrow with --severity
(information|warning|critical|emergency).

Use this command to cluster active alerts by client GROUP and severity to find
systemic fires. Do NOT use it to find the chronically noisiest monitors
fleet-wide; use 'alert-recurrence' instead.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # Active alerts clustered by group
  levelio-cli alert-triage --group-by group

  # Critical fires only, JSON for agents
  levelio-cli alert-triage --severity critical --group-by group --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			groupBy = strings.ToLower(strings.TrimSpace(groupBy))
			if groupBy == "" {
				groupBy = "group"
			}
			if !triageGroupBys[groupBy] {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --group-by %q: choose one of group, severity, name", groupBy))
			}
			if severity != "" && !alertSeverities[strings.ToLower(strings.TrimSpace(severity))] {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("invalid --severity %q: choose one of information (or info), warning, critical, emergency", severity))
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("levelio-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local store: %w\nRun 'levelio-cli sync' first.", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "alerts") {
				hintIfStale(cmd, db, "alerts", flags.maxAge)
			}

			alerts, err := lvlAlerts(db)
			if err != nil {
				return fmt.Errorf("loading alerts: %w", err)
			}
			devices, err := lvlDevices(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			groups, err := lvlGroups(db)
			if err != nil {
				return fmt.Errorf("loading groups: %w", err)
			}
			res := lvlComputeAlertTriage(alerts, devices, groups, severity, groupBy, includeResolved)

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "%d alert(s) in %d cluster(s) by %s\n", res.TotalAlerts, res.Count, res.GroupBy)
			if res.Count == 0 {
				return nil
			}
			fmt.Fprintln(out, "COUNT\tCLUSTER\tSEVERITIES")
			for _, c := range res.Clusters {
				fmt.Fprintf(out, "%d\t%s\t%s\n", c.Count, c.Key, sevSummary(c.BySeverity))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&severity, "severity", "", "Filter to one severity: information|warning|critical|emergency")
	cmd.Flags().StringVar(&groupBy, "group-by", "group", "Cluster by: group|severity|name")
	cmd.Flags().BoolVar(&includeResolved, "include-resolved", false, "Include resolved alerts (default: unresolved only)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func sevSummary(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool { return lvlSeverityWeight(keys[i]) > lvlSeverityWeight(keys[j]) })
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}
