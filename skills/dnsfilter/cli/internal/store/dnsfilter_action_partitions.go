// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: action-response-stored-on-parent). A regen
// preserves this file by construction; the one hook into generated store.go
// is the splitActionResponseItems call at the top of UpsertBatchDetailed.

package store

import (
	"encoding/json"
	"strings"
)

// actionPartition names the parent a child partition belongs to.
type actionPartition struct {
	parent     string // the parent's own partition
	parentType string // the parent's JSON:API type, folded (see foldJSONAPIType)
}

// actionResponsePartitions lists child partitions that the write-through
// fills from an action on a parent record whose response IS that parent:
//
//	POST/PATCH /v1/networks/{id}/secret_key -> {"data": NetworkSpec}
//	POST /v1/api_keys/{id}/revoke           -> {"data": ApiKeySpec}
//
// The generated write-through stored those responses under secret_key and
// revoke, whose typed tables require the parent key (networks_id,
// api_keys_id) that a parent record never carries, so every key generation,
// rotation and revocation printed "typed-table upsert failed" after a
// successful call. Such a row belongs with its parent: the scrubbed network
// merges into networks and the revoked key (status revoked) into api-keys.
var actionResponsePartitions = map[string]actionPartition{
	"secret_key": {parent: "networks", parentType: "network"},
	"revoke":     {parent: "api-keys", parentType: "apikey"},
}

// splitActionResponseItems separates, for a listed child partition, the
// items whose JSON:API type declares them to be the parent record (what the
// action returned) from every other item (a real child row as a dependent
// sync would write it, or a row with no type). moved is stored on parent;
// kept stays on resourceType. For any other partition moved is empty.
func splitActionResponseItems(resourceType string, items []json.RawMessage) (parent string, moved, kept []json.RawMessage) {
	ap, ok := actionResponsePartitions[resourceType]
	if !ok {
		return "", nil, items
	}
	for _, item := range items {
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(item, &head) == nil && head.Type != "" && foldJSONAPIType(head.Type) == ap.parentType {
			moved = append(moved, item)
			continue
		}
		kept = append(kept, item)
	}
	return ap.parent, moved, kept
}

// foldJSONAPIType lower-cases a JSON:API type, keeps letters and digits and
// drops one trailing s, so networks, network, api_keys, api-keys and ApiKey
// compare equal to their singular folded form.
func foldJSONAPIType(t string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(t) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return strings.TrimSuffix(b.String(), "s")
}

// upsertActionResponse stores moved on the parent partition and any kept
// child rows on resourceType, summing the counts. It runs before
// UpsertBatchDetailed takes the write lock; neither recursive call can split
// again (parent is not a listed child, and no kept row is typed as the parent).
// storedIDs (may be nil) receives only the ids stored under resourceType: a
// moved row lives in the parent's partition, not this one.
func (s *Store) upsertActionResponse(resourceType, parent string, moved, kept []json.RawMessage, storedIDs *[]string) (int, int, int, error) {
	stored, extractFailures, typedFailures, err := s.upsertBatchDetailed(parent, moved, nil)
	if err != nil || len(kept) == 0 {
		return stored, extractFailures, typedFailures, err
	}
	keptStored, keptExtract, keptTyped, err := s.upsertBatchDetailed(resourceType, kept, storedIDs)
	return stored + keptStored, extractFailures + keptExtract, typedFailures + keptTyped, err
}
