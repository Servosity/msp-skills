// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"sort"
)

// maxHistoryDays bounds stored per-day history: the union below would
// otherwise grow by a day per sync forever, and no report reads past 365 days.
const maxHistoryDays = 366

// radarSnapshotResources are Backup Radar list rows whose nulls carry meaning
// (device or company unmapped, no result yet), so a new row replaces the old
// one instead of the generic keep-the-richer-value merge.
var radarSnapshotResources = map[string]bool{
	"devices": true, "health": true, "backups": true, "backups-inactive": true, "backups-retired": true,
}

// mergeRadarSnapshot decides what one stored Backup Radar row becomes when a
// new copy arrives (sync, or a command's write-through):
//   - scalars come from incoming, so a field the API now nulls is cleared;
//   - per-day "history" arrays are unioned by date, incoming winning a date,
//     so a short read (health list = 7 days) keeps the days sync stored;
//   - v3 rows: a null history replaces (the backup has no SLA history now);
//   - v2 rows: a missing/null history keeps the stored one, because v2 only
//     returns history when HistoryDays is requested.
//
// Per-day reads (`backups get <id> --date`) cache under their own collection
// (HAND-FIX(v2-subresource-cache-names)), so they never reach this merge.
func mergeRadarSnapshot(resourceType string, existing, incoming json.RawMessage) json.RawMessage {
	// DecodeJSONObject keeps numbers exact (json.Number) so large ids survive.
	oldObj, errOld := DecodeJSONObject(existing)
	newObj, errNew := DecodeJSONObject(incoming)
	if errOld != nil || errNew != nil || oldObj == nil || newObj == nil {
		return incoming
	}
	v2 := resourceType != "devices" && resourceType != "health"
	newHist, okNew := newObj["history"].([]any)
	oldHist, okOld := oldObj["history"].([]any)
	switch {
	case okNew && okOld:
		newObj["history"] = unionHistory(newHist, oldHist)
	case !okNew && okOld && v2:
		newObj["history"] = oldHist
	}
	out, err := json.Marshal(newObj)
	if err != nil {
		return incoming
	}
	return out
}

// unionHistory merges two per-day arrays by "date" (primary wins), newest
// first, keeping at most maxHistoryDays entries.
func unionHistory(primary, secondary []any) []any {
	seen := map[string]bool{}
	for _, h := range primary {
		if d := historyDate(h); d != "" {
			seen[d] = true
		}
	}
	merged := append([]any{}, primary...)
	for _, h := range secondary {
		if d := historyDate(h); d != "" && !seen[d] {
			merged = append(merged, h)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return historyDate(merged[i]) > historyDate(merged[j]) })
	if len(merged) > maxHistoryDays {
		merged = merged[:maxHistoryDays]
	}
	return merged
}

// historyDate tolerates non-object entries (sorted last) instead of panicking.
func historyDate(h any) string {
	m, _ := h.(map[string]any)
	d, _ := m["date"].(string)
	return d
}
