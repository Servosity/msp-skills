// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence feature for levelio-cli. Reads the local store.

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"levelio-pp-cli/internal/store"
)

type groupNode struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Depth         int          `json:"depth"`
	DirectDevices int          `json:"direct_devices"`
	TotalDevices  int          `json:"total_devices"`
	OpenAlerts    *int         `json:"open_alerts,omitempty"`
	Stale         *int         `json:"stale,omitempty"`
	AvgScore      *float64     `json:"avg_score,omitempty"`
	Children      []*groupNode `json:"children,omitempty"`
}

type groupTreeResult struct {
	With             []string     `json:"with,omitempty"`
	StaleDays        float64      `json:"stale_days,omitempty"`
	GroupCount       int          `json:"group_count"`
	UngroupedDevices int          `json:"ungrouped_devices"`
	Roots            []*groupNode `json:"roots"`
}

type gSubAgg struct{ devices, alerts, stale, scoreSum, scoreCount int }

// lvlComputeGroupTree builds the group hierarchy with rolled-up device, alert,
// stale, and average-score metrics per node.
func lvlComputeGroupTree(devices []lvlDevice, groups []lvlGroup, alerts []lvlAlert, with map[string]bool, staleDays float64, root string, now time.Time) groupTreeResult {
	res := groupTreeResult{StaleDays: staleDays, GroupCount: len(groups)}
	for k := range with {
		res.With = append(res.With, k)
	}
	sort.Strings(res.With)
	idx := lvlBuildGroupIndex(groups)

	// Open alerts per device.
	openByDev := map[string]int{}
	for _, a := range alerts {
		if !a.IsResolved && a.DeviceID != "" {
			openByDev[a.DeviceID]++
		}
	}

	// Direct per-group stats.
	type gstat struct{ devices, alerts, stale, scoreSum, scoreCount int }
	stat := map[string]*gstat{}
	st := func(gid string) *gstat {
		s, ok := stat[gid]
		if !ok {
			s = &gstat{}
			stat[gid] = s
		}
		return s
	}
	for _, d := range devices {
		_, known := idx.byID[d.GroupID]
		if d.GroupID == "" || !known {
			res.UngroupedDevices++
			continue
		}
		s := st(d.GroupID)
		s.devices++
		s.alerts += openByDev[d.ID]
		if dd, ok := lvlDaysDark(d, now); ok && dd >= staleDays {
			s.stale++
		}
		if d.SecurityScore != nil {
			s.scoreSum += *d.SecurityScore
			s.scoreCount++
		}
	}

	// Rolled subtree aggregates (memoized, cycle-safe).
	memo := map[string]gSubAgg{}
	var roll func(gid string) gSubAgg
	roll = func(gid string) gSubAgg {
		if v, ok := memo[gid]; ok {
			return v
		}
		memo[gid] = gSubAgg{} // cycle guard
		agg := gSubAgg{}
		if s := stat[gid]; s != nil {
			agg = gSubAgg{devices: s.devices, alerts: s.alerts, stale: s.stale, scoreSum: s.scoreSum, scoreCount: s.scoreCount}
		}
		children := append([]string(nil), idx.children[gid]...)
		for _, cid := range children {
			c := roll(cid)
			agg.devices += c.devices
			agg.alerts += c.alerts
			agg.stale += c.stale
			agg.scoreSum += c.scoreSum
			agg.scoreCount += c.scoreCount
		}
		memo[gid] = agg
		return agg
	}

	built := map[string]bool{}
	var build func(gid string, depth int) *groupNode
	build = func(gid string, depth int) *groupNode {
		built[gid] = true
		agg := roll(gid)
		n := &groupNode{ID: gid, Name: idx.name(gid), Depth: depth, TotalDevices: agg.devices}
		if s := stat[gid]; s != nil {
			n.DirectDevices = s.devices
		}
		if with["alerts"] {
			v := agg.alerts
			n.OpenAlerts = &v
		}
		if with["stale"] {
			v := agg.stale
			n.Stale = &v
		}
		if with["score"] && agg.scoreCount > 0 {
			v := round1(float64(agg.scoreSum) / float64(agg.scoreCount))
			n.AvgScore = &v
		}
		kids := append([]string(nil), idx.children[gid]...)
		sort.SliceStable(kids, func(i, j int) bool { return idx.name(kids[i]) < idx.name(kids[j]) })
		for _, cid := range kids {
			if built[cid] {
				// Corrupt parent_id data (cycle or duplicate wiring): never
				// re-emit a node already placed in the tree.
				continue
			}
			n.Children = append(n.Children, build(cid, depth+1))
		}
		return n
	}

	var rootIDs []string
	if root != "" {
		if _, ok := idx.byID[root]; ok {
			rootIDs = []string{root}
		}
	} else {
		for _, g := range groups {
			if _, hasParent := idx.byID[g.ParentID]; g.ParentID == "" || !hasParent {
				rootIDs = append(rootIDs, g.ID)
			}
		}
	}
	sort.SliceStable(rootIDs, func(i, j int) bool { return idx.name(rootIDs[i]) < idx.name(rootIDs[j]) })
	for _, rid := range rootIDs {
		res.Roots = append(res.Roots, build(rid, 0))
	}
	if root == "" {
		// Cycle-orphan rescue: groups whose parent chain never reaches a root
		// (corrupt parent_id cycles / self-parents) select no root above and
		// would otherwise vanish from the output silently — along with their
		// devices. Emit one root per orphaned component.
		var orphans []string
		for _, g := range groups {
			if !built[g.ID] {
				orphans = append(orphans, g.ID)
			}
		}
		sort.SliceStable(orphans, func(i, j int) bool { return idx.name(orphans[i]) < idx.name(orphans[j]) })
		for _, oid := range orphans {
			if !built[oid] {
				res.Roots = append(res.Roots, build(oid, 0))
			}
		}
	}
	return res
}

func renderGroupNode(out *strings.Builder, n *groupNode, with map[string]bool) {
	indent := strings.Repeat("  ", n.Depth)
	extras := []string{fmt.Sprintf("devices=%d", n.TotalDevices)}
	if with["alerts"] && n.OpenAlerts != nil {
		extras = append(extras, fmt.Sprintf("alerts=%d", *n.OpenAlerts))
	}
	if with["stale"] && n.Stale != nil {
		extras = append(extras, fmt.Sprintf("stale=%d", *n.Stale))
	}
	if with["score"] && n.AvgScore != nil {
		extras = append(extras, fmt.Sprintf("avg_score=%.1f", *n.AvgScore))
	}
	fmt.Fprintf(out, "%s- %s (%s)\n", indent, n.Name, strings.Join(extras, ", "))
	for _, c := range n.Children {
		renderGroupNode(out, c, with)
	}
}

var groupTreeWithTokens = map[string]bool{"alerts": true, "stale": true, "score": true}

// pp:data-source local
func newNovelGroupTreeCmd(flags *rootFlags) *cobra.Command {
	var withSpec string
	var staleDays float64
	var root string
	var dbPath string

	cmd := &cobra.Command{
		Use:         "group-tree",
		Short:       "Render the Level group hierarchy with rolled-up device, alert, stale, and score counts",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Render the Level group hierarchy as a tree, with each node showing its
rolled-up descendant device count and — when requested via --with — open alerts,
stale devices, and average security score. Computed offline from the local
store. --with takes a comma list of alerts,stale,score; --stale-days sets the
staleness window; --root renders a single subtree.

Use this command to render the full nested group HIERARCHY with rolled-up
health per node. Do NOT use it for a flat one-row-per-client scorecard; use
'client-scorecard' instead.

Run 'levelio-cli sync' first to populate the local store.`,
		Example: strings.Trim(`
  # The group tree with device counts
  levelio-cli group-tree

  # Annotate each node with alerts, stale, and score, JSON for agents
  levelio-cli group-tree --with alerts,stale,score --agent
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			with := map[string]bool{}
			for _, tok := range strings.Split(withSpec, ",") {
				tok = strings.ToLower(strings.TrimSpace(tok))
				if tok == "" {
					continue
				}
				if !groupTreeWithTokens[tok] {
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("invalid --with token %q: choose from alerts, stale, score", tok))
				}
				with[tok] = true
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
			if !hintIfUnsynced(cmd, db, "groups") {
				hintIfStale(cmd, db, "groups", flags.maxAge)
			}

			groups, err := lvlGroups(db)
			if err != nil {
				return fmt.Errorf("loading groups: %w", err)
			}
			devices, err := lvlDevices(db)
			if err != nil {
				return fmt.Errorf("loading devices: %w", err)
			}
			var alerts []lvlAlert
			if with["alerts"] {
				if alerts, err = lvlAlerts(db); err != nil {
					return fmt.Errorf("loading alerts: %w", err)
				}
			}
			res := lvlComputeGroupTree(devices, groups, alerts, with, staleDays, root, time.Now().UTC())

			if flags.asJSON {
				return printJSONFiltered(out, res, flags)
			}
			fmt.Fprintf(out, "%d group(s), %d ungrouped device(s)\n", res.GroupCount, res.UngroupedDevices)
			var b strings.Builder
			for _, n := range res.Roots {
				renderGroupNode(&b, n, with)
			}
			fmt.Fprint(out, b.String())
			return nil
		},
	}
	cmd.Flags().StringVar(&withSpec, "with", "", "Annotate nodes with a comma list: alerts,stale,score")
	cmd.Flags().Float64Var(&staleDays, "stale-days", 7, "Staleness window in days when --with stale is set")
	cmd.Flags().StringVar(&root, "root", "", "Render only this group id and its descendants")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
