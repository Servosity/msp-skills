// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Shared pure-join helpers for the reprint-added transcendence commands
// (systems history, detections failures, inspectors coverage, health). Same
// defensive-extraction philosophy as novel_liongard.go: PascalCase-first key
// candidates with case-insensitive fallback, honest empties over fabricated
// rows. The commands load synced objects then call these testable functions.

package cli

import (
	"sort"
	"strings"
	"time"
)

// rtInspectors is the resources-table key for synced inspector definitions
// (the probe catalog), written by the generated UpsertInspectors path.
const rtInspectors = "inspectors"

// historyEvent is one entry in a single system's chronological change story:
// either an inspection run (timeline) or a detected change (detection).
type historyEvent struct {
	Kind          string `json:"kind"` // "inspection" | "detection"
	Timestamp     string `json:"timestamp,omitempty"`
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Status        string `json:"status,omitempty"`
	LaunchpointID string `json:"launchpoint_id,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Environment   string `json:"environment,omitempty"`
}

// filterSystemHistory merges timeline entries and detections that reference
// one system into a single newest-first event list, optionally windowed by
// since (0 = all history) and capped by limit (0 = uncapped).
func filterSystemHistory(timeline, detections, envs []map[string]any, systemID string, since time.Duration, now time.Time, limit int) []historyEvent {
	envByID, _ := envNameIndex(envs)
	var cutoff time.Time
	if since > 0 {
		cutoff = now.Add(-since)
	}
	events := []historyEvent{}

	appendEvent := func(obj map[string]any, kind string, tsKeys, nameKeys []string) {
		if lgRefID(obj, "System", "SystemID") != systemID {
			return
		}
		ev := historyEvent{
			Kind:          kind,
			ID:            lgID(obj),
			Name:          lgStr(obj, nameKeys...),
			Status:        lgStr(obj, "Status", "State"),
			LaunchpointID: lgRefID(obj, "Launchpoint", "LaunchpointID"),
			EnvironmentID: lgRefID(obj, "Environment", "EnvironmentID"),
			Environment:   lgRefName(obj, "Environment"),
		}
		if ev.Environment == "" {
			ev.Environment = envByID[ev.EnvironmentID]
		}
		if ts, ok := lgTime(obj, tsKeys...); ok {
			if !cutoff.IsZero() && ts.Before(cutoff) {
				return
			}
			ev.Timestamp = ts.Format(time.RFC3339)
		} else if !cutoff.IsZero() {
			// Windowed query: drop undated events rather than guessing.
			return
		}
		events = append(events, ev)
	}

	for _, t := range timeline {
		appendEvent(t, "inspection",
			tsKeysInspection,
			[]string{"Name", "name", "Description"})
	}
	for _, d := range detections {
		appendEvent(d, "detection",
			tsKeysDetection,
			[]string{"Name", "name", "Detection", "Description", "Title"})
	}

	sort.SliceStable(events, func(i, j int) bool { return events[i].Timestamp > events[j].Timestamp })
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events
}

// failedInspectionRow is one inspection that ran but did not complete cleanly.
type failedInspectionRow struct {
	TimelineID    string `json:"timeline_id"`
	Status        string `json:"status,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
	LaunchpointID string `json:"launchpoint_id,omitempty"`
	Launchpoint   string `json:"launchpoint,omitempty"`
	Inspector     string `json:"inspector,omitempty"`
	SystemID      string `json:"system_id,omitempty"`
	System        string `json:"system,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	Environment   string `json:"environment,omitempty"`
}

// inspectionFailed reports whether a timeline status string indicates a
// failed/errored run (vs Completed/Success/Running).
func inspectionFailed(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	if s == "" {
		return false
	}
	return strings.Contains(s, "fail") || strings.Contains(s, "error")
}

// filterFailedInspections returns timeline entries whose status indicates a
// failed or errored run within the window, attributed to their owning
// environment (directly, or via the referenced launchpoint when the timeline
// entry does not carry an Environment).
func filterFailedInspections(timeline, launchpoints, envs []map[string]any, since time.Duration, now time.Time, envFilter string, limit int) []failedInspectionRow {
	envByID, _ := envNameIndex(envs)
	type lpInfo struct{ alias, inspector, envID, envName string }
	lpByID := map[string]lpInfo{}
	for _, lp := range launchpoints {
		id := lgID(lp)
		if id == "" {
			continue
		}
		envID := lgRefID(lp, "Environment", "EnvironmentID")
		envName := lgRefName(lp, "Environment")
		if envName == "" {
			envName = envByID[envID]
		}
		lpByID[id] = lpInfo{
			alias:     lgStr(lp, "Alias", "Name", "name"),
			inspector: lgRefName(lp, "Inspector"),
			envID:     envID,
			envName:   envName,
		}
	}

	var cutoff time.Time
	if since > 0 {
		cutoff = now.Add(-since)
	}
	rows := []failedInspectionRow{}
	for _, t := range timeline {
		status := lgStr(t, "Status", "State", "Result")
		if !inspectionFailed(status) {
			continue
		}
		row := failedInspectionRow{
			TimelineID:    lgID(t),
			Status:        status,
			LaunchpointID: lgRefID(t, "Launchpoint", "LaunchpointID"),
			SystemID:      lgRefID(t, "System", "SystemID"),
			System:        lgRefName(t, "System"),
			EnvironmentID: lgRefID(t, "Environment", "EnvironmentID"),
			Environment:   lgRefName(t, "Environment"),
		}
		if ts, ok := lgTime(t, tsKeysInspection...); ok {
			if !cutoff.IsZero() && ts.Before(cutoff) {
				continue
			}
			row.FinishedAt = ts.Format(time.RFC3339)
		} else if !cutoff.IsZero() {
			continue
		}
		if lp, ok := lpByID[row.LaunchpointID]; ok {
			row.Launchpoint = lp.alias
			row.Inspector = lp.inspector
			if row.EnvironmentID == "" {
				row.EnvironmentID = lp.envID
			}
			if row.Environment == "" {
				row.Environment = lp.envName
			}
		}
		if row.Environment == "" {
			row.Environment = envByID[row.EnvironmentID]
		}
		if envFilter != "" && row.EnvironmentID != envFilter {
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].FinishedAt > rows[j].FinishedAt })
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

type envRef struct {
	EnvironmentID string `json:"environment_id"`
	Environment   string `json:"environment,omitempty"`
}

// inspectorCoverageRow summarizes one inspector's estate-wide adoption.
type inspectorCoverageRow struct {
	InspectorID         string   `json:"inspector_id,omitempty"`
	Inspector           string   `json:"inspector"`
	EnvironmentsCovered int      `json:"environments_covered"`
	EnvironmentsMissing int      `json:"environments_missing"`
	MissingEnvironments []envRef `json:"missing_environments"`
}

// computeInspectorCoverage anti-joins inspectors x launchpoints x environments:
// for each inspector (optionally filtered by case-insensitive substring on
// name), which environments have NO launchpoint bound to it. Inspectors are
// keyed by ID when launchpoint references carry IDs, with a name fallback.
func computeInspectorCoverage(inspectors, launchpoints, envs []map[string]any, inspectorQuery string) []inspectorCoverageRow {
	// covered[inspectorKey] = set of envIDs with a launchpoint for it
	coveredByID := map[string]map[string]bool{}
	coveredByName := map[string]map[string]bool{}
	mark := func(m map[string]map[string]bool, key, envID string) {
		if key == "" || envID == "" {
			return
		}
		if m[key] == nil {
			m[key] = map[string]bool{}
		}
		m[key][envID] = true
	}
	for _, lp := range launchpoints {
		envID := lgRefID(lp, "Environment", "EnvironmentID")
		mark(coveredByID, lgRefID(lp, "Inspector", "InspectorID"), envID)
		mark(coveredByName, strings.ToLower(lgRefName(lp, "Inspector")), envID)
	}

	q := strings.ToLower(strings.TrimSpace(inspectorQuery))
	rows := []inspectorCoverageRow{}
	for _, ins := range inspectors {
		name := lgStr(ins, "Name", "name", "Alias")
		if q != "" && !strings.Contains(strings.ToLower(name), q) {
			continue
		}
		id := lgID(ins)
		covered := coveredByID[id]
		if len(covered) == 0 {
			covered = coveredByName[strings.ToLower(name)]
		}
		row := inspectorCoverageRow{
			InspectorID:         id,
			Inspector:           name,
			EnvironmentsCovered: len(covered),
			MissingEnvironments: []envRef{},
		}
		for _, e := range envs {
			envID := lgID(e)
			if envID == "" || covered[envID] {
				continue
			}
			row.MissingEnvironments = append(row.MissingEnvironments, envRef{
				EnvironmentID: envID,
				Environment:   lgStr(e, "Name", "name"),
			})
		}
		row.EnvironmentsMissing = len(row.MissingEnvironments)
		sort.SliceStable(row.MissingEnvironments, func(i, j int) bool {
			return row.MissingEnvironments[i].Environment < row.MissingEnvironments[j].Environment
		})
		rows = append(rows, row)
	}
	// Largest adoption gap first; ties alphabetical.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].EnvironmentsMissing != rows[j].EnvironmentsMissing {
			return rows[i].EnvironmentsMissing > rows[j].EnvironmentsMissing
		}
		return rows[i].Inspector < rows[j].Inspector
	})
	return rows
}

// healthSummary is the one-shot estate scorecard the `health` command emits.
type healthSummary struct {
	StaleLaunchpoints  int    `json:"stale_launchpoints"`
	OfflineAgents      int    `json:"offline_agents"`
	FailedInspections  int    `json:"failed_inspections"`
	UninspectedSystems int    `json:"uninspected_systems"`
	EmptyEnvironments  int    `json:"empty_environments"`
	TotalIssues        int    `json:"total_issues"`
	Status             string `json:"status"` // "healthy" | "issues"
	StaleOlderThan     string `json:"stale_older_than"`
	FailuresSince      string `json:"failures_since"`
}

// summarizeHealth folds the individual sweep counts into one scorecard.
func summarizeHealth(stale, offline, failed, uninspected, emptyEnvs int, olderThan, failSince string) healthSummary {
	total := stale + offline + failed + uninspected + emptyEnvs
	status := "healthy"
	if total > 0 {
		status = "issues"
	}
	return healthSummary{
		StaleLaunchpoints:  stale,
		OfflineAgents:      offline,
		FailedInspections:  failed,
		UninspectedSystems: uninspected,
		EmptyEnvironments:  emptyEnvs,
		TotalIssues:        total,
		Status:             status,
		StaleOlderThan:     olderThan,
		FailuresSince:      failSince,
	}
}
