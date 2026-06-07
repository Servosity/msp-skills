// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"sort"

	"sherweb-pp-cli/internal/insights"
	"sherweb-pp-cli/internal/store"
)

// Snapshot resource types appended by deep-sync alongside the payable
// snapshots, so the reprint transcendence commands (margin-trend, sub-changes)
// can diff fleet state across runs.
const (
	rtSubsSnapshot       = "subscription-snapshot" // {items:[Subscription]} per deep-sync run
	rtReceivableSnapshot = "receivable-snapshot"   // {items:[Charge]} per deep-sync run
)

// loadSnapshotRows returns the raw snapshot documents for a resource type,
// keyed and sorted by snapshot id (an RFC3339 timestamp written by deep-sync).
func loadSnapshotRows(s *store.Store, resourceType string) (ids []string, byID map[string]json.RawMessage, err error) {
	rows, err := s.Query(`SELECT id, data FROM resources WHERE resource_type = ? ORDER BY id ASC`, resourceType)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	byID = map[string]json.RawMessage{}
	for rows.Next() {
		var id string
		var data []byte
		if err := rows.Scan(&id, &data); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		byID[id] = json.RawMessage(data)
	}
	sort.Strings(ids)
	return ids, byID, rows.Err()
}

// loadSubscriptionSnapshots returns subscription snapshots ordered
// oldest→newest by snapshot id.
func loadSubscriptionSnapshots(s *store.Store) (ids []string, byID map[string][]insights.Subscription, err error) {
	rawIDs, raws, err := loadSnapshotRows(s, rtSubsSnapshot)
	if err != nil {
		return nil, nil, err
	}
	byID = map[string][]insights.Subscription{}
	for _, id := range rawIDs {
		byID[id] = subscriptionsFromSnapshot(raws[id])
	}
	return rawIDs, byID, nil
}

// subscriptionsFromSnapshot parses a {items:[...]} snapshot document into
// typed subscriptions using the same defensive field access as the live
// loaders.
func subscriptionsFromSnapshot(raw json.RawMessage) []insights.Subscription {
	m := asMap(raw)
	out := []insights.Subscription{}
	if m == nil {
		return out
	}
	if arr, ok := pick(m, "items").([]any); ok {
		for _, it := range arr {
			if im, ok := it.(map[string]any); ok {
				out = append(out, subscriptionFromMap(im))
			}
		}
	}
	return out
}

// loadReceivableSnapshots returns receivable-charge snapshots ordered
// oldest→newest by snapshot id.
func loadReceivableSnapshots(s *store.Store) (ids []string, byID map[string][]insights.Charge, err error) {
	rawIDs, raws, err := loadSnapshotRows(s, rtReceivableSnapshot)
	if err != nil {
		return nil, nil, err
	}
	byID = map[string][]insights.Charge{}
	for _, id := range rawIDs {
		byID[id] = flattenChargeRows([]json.RawMessage{raws[id]})
	}
	return rawIDs, byID, nil
}

// latestPerMonth reduces snapshot ids (RFC3339 timestamps) to the most recent
// snapshot per YYYY-MM month and re-keys the values by month. Precondition:
// ids are sorted ascending and lexicographic order == chronological order
// (true for RFC3339 UTC ids); the reduction is positional, not parsed.
func latestPerMonth[T any](ids []string, byID map[string]T) map[string]T {
	out := map[string]T{}
	for _, id := range ids { // ids sorted ascending, so the last write per month wins
		if len(id) < 7 {
			continue
		}
		out[id[:7]] = byID[id]
	}
	return out
}
