// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by skills/sophos-central/handfixes.json under
// "fleet-rows-replace" and "fleet-copies-own-type".

package store

import (
	"encoding/json"
	"sync"
)

var replacing sync.Map // *Store -> struct{} while a ReplaceBatch runs

// ReplaceBatch stores items like UpsertBatch, in the same single transaction
// (generic row, typed row and search index together), but each item replaces
// the stored row instead of going through the keep-richer merge. It is for
// callers whose items are whole objects (a fleet refresh fetches view=full)
// or snapshots, where a field the API cleared must not survive.
// ponytail: per-Store flag; an UpsertBatch on the same *Store from another goroutine during a ReplaceBatch also skips the merge. Fleet refresh owns its Store.
func (s *Store) ReplaceBatch(resourceType string, items []json.RawMessage) (int, int, error) {
	replacing.Store(s, struct{}{})
	defer replacing.Delete(s)
	return s.UpsertBatch(resourceType, items)
}

func replacingWrites(s *Store) bool {
	_, ok := replacing.Load(s)
	return ok
}

// fleetCopyNotIndexed keeps fleet refresh's own copies of endpoints and alerts
// out of the search index: the mirror row is already indexed, and search
// would otherwise return every endpoint twice.
func fleetCopyNotIndexed(resourceType string) bool {
	return resourceType == "fleet_endpoints" || resourceType == "fleet_alerts"
}
