// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// insights mttr: mean time to acknowledge (MTTA) and resolve (MTTR), grouped by
// service, team, or priority, reconstructed offline from synced log-entry
// timestamps. Reproduces the gist of PagerDuty's paid Analytics product from
// the local incident + log-entry mirror with no POST-body analytics call.
//
// This file also owns pdLifecycle and pdBuildLifecycles — the shared
// per-incident timeline reconstruction reused by `insights noisy`.
package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// pdLifecycle is one incident's reconstructed lifecycle: when it triggered,
// was first acknowledged, and was resolved, plus grouping dimensions.
type pdLifecycle struct {
	IncidentID   string
	Service      string
	ServiceID    string
	Priority     string
	Team         string
	Urgency      string
	TriggerAt    time.Time
	hasTrigger   bool
	AckAt        time.Time
	hasAck       bool
	ResolveAt    time.Time
	hasResolve   bool
	autoResolved bool
	triggerCount int
}

// pdBuildLifecycles reconstructs per-incident lifecycles from the incident
// records (for grouping dimensions + the created_at trigger fallback) and the
// log entries (for ack/resolve timestamps and auto-resolve detection).
func pdBuildLifecycles(incidents, logs []map[string]any) map[string]*pdLifecycle {
	byID := map[string]*pdLifecycle{}
	get := func(id string) *pdLifecycle {
		lc := byID[id]
		if lc == nil {
			lc = &pdLifecycle{IncidentID: id}
			byID[id] = lc
		}
		return lc
	}

	for _, inc := range incidents {
		id := pdString(inc, "id")
		if id == "" {
			continue
		}
		lc := get(id)
		svc := pdMap(inc, "service")
		lc.Service = pdRefLabel(svc)
		lc.ServiceID = pdString(svc, "id")
		lc.Priority = pdRefLabel(pdMap(inc, "priority"))
		if teams := pdSlice(inc, "teams"); len(teams) > 0 {
			lc.Team = pdRefLabel(teams[0])
		}
		lc.Urgency = pdString(inc, "urgency")
		if t, ok := pdParseTime(pdString(inc, "created_at")); ok {
			lc.TriggerAt = t
			lc.hasTrigger = true
		}
	}

	for _, le := range logs {
		id := pdString(pdMap(le, "incident"), "id")
		if id == "" {
			continue
		}
		lc := get(id)
		if lc.Service == "" {
			if svc := pdMap(le, "service"); svc != nil {
				lc.Service = pdRefLabel(svc)
				lc.ServiceID = pdString(svc, "id")
			}
		}
		t, ok := pdParseTime(pdString(le, "created_at"))
		if !ok {
			continue
		}
		switch pdString(le, "type") {
		case "trigger_log_entry":
			lc.triggerCount++
			if !lc.hasTrigger || t.Before(lc.TriggerAt) {
				lc.TriggerAt = t
				lc.hasTrigger = true
			}
		case "acknowledge_log_entry":
			if !lc.hasAck || t.Before(lc.AckAt) {
				lc.AckAt = t
				lc.hasAck = true
			}
		case "resolve_log_entry":
			if !lc.hasResolve || t.After(lc.ResolveAt) {
				lc.ResolveAt = t
				lc.hasResolve = true
			}
			if pdString(pdMap(le, "agent"), "type") == "service_reference" {
				lc.autoResolved = true
			}
		}
	}
	return byID
}

type pdMttrGroup struct {
	Key          string `json:"key"`
	Incidents    int    `json:"incidents"`
	Acknowledged int    `json:"acknowledged"`
	Resolved     int    `json:"resolved"`
	MTTASeconds  int64  `json:"mtta_seconds"`
	MTTA         string `json:"mtta"`
	MTTRSeconds  int64  `json:"mttr_seconds"`
	MTTR         string `json:"mttr"`
}

type pdMttrResult struct {
	Window struct {
		Since string `json:"since"`
		Until string `json:"until"`
	} `json:"window"`
	By     string        `json:"by"`
	Groups []pdMttrGroup `json:"groups"`
}

// pp:data-source local
func newNovelInsightsMttrCmd(flags *rootFlags) *cobra.Command {
	var flagBy, flagSince, flagUntil string

	cmd := &cobra.Command{
		Use:         "mttr",
		Short:       "Mean time to acknowledge (MTTA) and resolve (MTTR), grouped by service, team, or priority",
		Long:        "Reconstructs MTTA (trigger→first ack) and MTTR (trigger→resolve) offline from synced incidents and log entries, grouped by --by (service|team|priority|overall). --since/--until accept a relative duration (30d, 24h) or RFC3339; --since defaults to 30 days ago. Run `sync` first; exits 0 with an empty result when nothing is synced.",
		Example:     "  pagerduty-cli insights mttr --by service --since 30d\n  pagerduty-cli insights mttr --by priority --agent\n  pagerduty-cli insights mttr",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			pdHintSync(cmd, flags, "incidents")
			by := flagBy
			if by == "" {
				by = "service"
			}
			switch by {
			case "service", "team", "priority", "overall":
			default:
				return fmt.Errorf("invalid --by %q: must be one of service, team, priority, overall", by)
			}
			win, err := pdParseWindow(flagSince, flagUntil, time.Now())
			if err != nil {
				return err
			}
			incidents, err := pdLoadResource(cmd.Context(), "incidents")
			if err != nil {
				return fmt.Errorf("reading incidents from local store: %w", err)
			}
			logs, err := pdLoadResource(cmd.Context(), "log_entries")
			if err != nil {
				return fmt.Errorf("reading log_entries from local store: %w", err)
			}
			res := buildMttr(incidents, logs, win, by)
			return pdEmit(cmd, flags, res, func(w io.Writer) {
				if len(res.Groups) == 0 {
					fmt.Fprintln(w, "No incidents with lifecycle data in the window (run `pagerduty-cli sync` first).")
					return
				}
				fmt.Fprintf(w, "MTTA/MTTR by %s, %s → %s\n\n", res.By, res.Window.Since, res.Window.Until)
				tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "GROUP\tINCIDENTS\tACK\tRESOLVED\tMTTA\tMTTR")
				for _, g := range res.Groups {
					fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%s\t%s\n", g.Key, g.Incidents, g.Acknowledged, g.Resolved, dash(g.MTTA), dash(g.MTTR))
				}
				_ = tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&flagBy, "by", "service", "Group by: service, team, priority, or overall")
	cmd.Flags().StringVar(&flagSince, "since", "", "Window start: relative (30d, 24h) or RFC3339 (default 30 days ago)")
	cmd.Flags().StringVar(&flagUntil, "until", "", "Window end: relative or RFC3339 (default now)")
	return cmd
}

// buildMttr is the pure split-out for tests.
func buildMttr(incidents, logs []map[string]any, win pdWindow, by string) pdMttrResult {
	var res pdMttrResult
	res.By = by
	res.Window.Since = win.Since.UTC().Format(time.RFC3339)
	res.Window.Until = win.Until.UTC().Format(time.RFC3339)
	res.Groups = []pdMttrGroup{}

	type acc struct {
		incidents int
		ackN      int
		resN      int
		ackSum    time.Duration
		resSum    time.Duration
	}
	groups := map[string]*acc{}
	order := []string{}

	for _, lc := range pdBuildLifecycles(incidents, logs) {
		if !lc.hasTrigger || !win.contains(lc.TriggerAt) {
			continue
		}
		var key string
		switch by {
		case "service":
			key = lc.Service
		case "team":
			key = lc.Team
		case "priority":
			key = lc.Priority
		default:
			key = "overall"
		}
		if key == "" {
			key = "(none)"
		}
		a := groups[key]
		if a == nil {
			a = &acc{}
			groups[key] = a
			order = append(order, key)
		}
		a.incidents++
		if lc.hasAck && !lc.AckAt.Before(lc.TriggerAt) {
			a.ackN++
			a.ackSum += lc.AckAt.Sub(lc.TriggerAt)
		}
		if lc.hasResolve && !lc.ResolveAt.Before(lc.TriggerAt) {
			a.resN++
			a.resSum += lc.ResolveAt.Sub(lc.TriggerAt)
		}
	}

	for _, key := range order {
		a := groups[key]
		g := pdMttrGroup{Key: key, Incidents: a.incidents, Acknowledged: a.ackN, Resolved: a.resN}
		if a.ackN > 0 {
			mtta := a.ackSum / time.Duration(a.ackN)
			g.MTTASeconds = int64(mtta.Seconds())
			g.MTTA = pdHumanDur(mtta)
		}
		if a.resN > 0 {
			mttr := a.resSum / time.Duration(a.resN)
			g.MTTRSeconds = int64(mttr.Seconds())
			g.MTTR = pdHumanDur(mttr)
		}
		res.Groups = append(res.Groups, g)
	}
	sort.SliceStable(res.Groups, func(i, j int) bool {
		return res.Groups[i].Incidents > res.Groups[j].Incidents
	})
	return res
}
