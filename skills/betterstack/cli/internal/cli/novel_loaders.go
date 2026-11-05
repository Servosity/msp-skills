// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence layer (reprint 2026-06-06): loaders for the novel
// commands added this run (down, triage, statuspage-audit, group-health).
// Same contract as analytics.go: every loader reads the generic `resources`
// table via json_extract over the JSON:API element, CAST to TEXT, no
// per-type scan logic.
package cli

import (
	"context"
	"fmt"

	"betterstack-pp-cli/internal/store"
)

type statusPageRow struct {
	ID             string `json:"id"`
	CompanyName    string `json:"company_name"`
	Subdomain      string `json:"subdomain"`
	CustomDomain   string `json:"custom_domain,omitempty"`
	AggregateState string `json:"aggregate_state"`
}

func loadStatusPages(ctx context.Context, s *store.Store) ([]statusPageRow, error) {
	if err := requireFinishedSync(s, "status-pages"); err != nil { // betterstack handfix: novels-refuse-a-partial-store
		return nil, err
	}
	q := fmt.Sprintf(`SELECT %s,%s,%s,%s,%s FROM resources WHERE resource_type='status-pages'`,
		jx("id"), ja("company_name"), ja("subdomain"),
		ja("custom_domain"), ja("aggregate_state"))
	recs, err := queryTextRows(ctx, s, q, 5)
	if err != nil {
		return nil, err
	}
	out := make([]statusPageRow, 0, len(recs))
	for _, r := range recs {
		out = append(out, statusPageRow{
			ID: r[0], CompanyName: r[1], Subdomain: r[2], CustomDomain: r[3], AggregateState: r[4],
		})
	}
	return out, nil
}

type groupRow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Paused bool   `json:"paused"`
}

func loadGroupRows(ctx context.Context, s *store.Store, resourceType string) ([]groupRow, error) {
	if err := requireFinishedSync(s, resourceType); err != nil { // betterstack handfix: novels-refuse-a-partial-store
		return nil, err
	}
	q := fmt.Sprintf(`SELECT %s,%s,%s FROM resources WHERE resource_type='%s'`,
		jx("id"), ja("name"), ja("paused"), resourceType)
	recs, err := queryTextRows(ctx, s, q, 3)
	if err != nil {
		return nil, err
	}
	out := make([]groupRow, 0, len(recs))
	for _, r := range recs {
		out = append(out, groupRow{ID: r[0], Name: r[1], Paused: truthy(r[2])})
	}
	return out, nil
}

// loadHeartbeatGroupIDs returns heartbeat id -> heartbeat_group_id for the
// group-health rollup. Kept separate from loadHeartbeats so the ported
// analytics layer stays byte-compatible with the prior print.
func loadHeartbeatGroupIDs(ctx context.Context, s *store.Store) (map[string]string, error) {
	if err := requireFinishedSync(s, "heartbeats"); err != nil { // betterstack handfix: novels-refuse-a-partial-store
		return nil, err
	}
	q := fmt.Sprintf(`SELECT %s,%s FROM resources WHERE resource_type='heartbeats'`,
		jx("id"), ja("heartbeat_group_id"))
	recs, err := queryTextRows(ctx, s, q, 2)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(recs))
	for _, r := range recs {
		out[r[0]] = r[1]
	}
	return out, nil
}

// openIncidents indexes open incidents (no resolved_at) by the monitor and
// by the heartbeat they belong to (incidentRow.MonitorID / HeartbeatID). Codex
// r1 (betterstack 4.34.2 reprint) class heartbeat-incidents-omitted: the index
// was keyed by monitor only, so a heartbeat's open incident counted nowhere
// (group-health open_incidents 0, statuspage-audit never checked a heartbeat
// entry). Open counts every open incident whatever its source.
type openIncidents struct {
	byMonitor, byHeartbeat map[string][]incidentRow
	open                   int
}

func indexOpenIncidents(incidents []incidentRow) openIncidents {
	idx := openIncidents{byMonitor: map[string][]incidentRow{}, byHeartbeat: map[string][]incidentRow{}}
	for _, in := range incidents {
		if in.ResolvedAt != "" {
			continue
		}
		idx.open++
		if in.MonitorID != "" {
			idx.byMonitor[in.MonitorID] = append(idx.byMonitor[in.MonitorID], in)
		}
		if in.HeartbeatID != "" {
			idx.byHeartbeat[in.HeartbeatID] = append(idx.byHeartbeat[in.HeartbeatID], in)
		}
	}
	return idx
}
