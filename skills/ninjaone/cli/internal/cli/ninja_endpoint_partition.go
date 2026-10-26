// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"
	"sync"

	"ninjaone-pp-cli/internal/store"
)

// Class endpoint-reads-use-the-sync-partition (hand-fix; reapply/endpoint_partitions.py wires it
// into the generated data_source.go).
//
// Endpoint commands are generated per command group: all 24 /v2/queries/* report commands name
// the partition "queries", and every /v2/organization/... command names "organization". Each sync
// partition holds exactly one endpoint's rows (syncResourcePath), and several were pinned to the
// endpoint the shipped release used ("queries" = /v2/queries/disks, "organization" =
// /v2/organization/documents). So a local read, an auto-mode write-through or a mutation
// write-through resolves its partition from the endpoint path:
//
//   - a path that is a sync partition's list path uses that partition, whatever the group;
//   - a path no sync partition lists, under a group that IS a sync partition, has no local
//     partition: a local read refuses and nothing is written, because that partition holds a
//     different endpoint's rows (/v2/organization/{id} is an organization, not a document);
//   - a group that is not a sync partition keeps the generated behavior.

var (
	ninjaPartitionByPathOnce sync.Once
	ninjaPartitionByPath     map[string]string
)

func ninjaSyncPartitionForPath(path string) (string, bool) {
	ninjaPartitionByPathOnce.Do(func() {
		ninjaPartitionByPath = make(map[string]string, len(syncOwnedPartitions))
		for name := range syncOwnedPartitions {
			if p, err := syncResourcePath(name); err == nil && p != "" {
				ninjaPartitionByPath[ninjaCleanEndpointPath(p)] = name
			}
		}
	})
	name, ok := ninjaPartitionByPath[ninjaCleanEndpointPath(path)]
	return name, ok
}

func ninjaCleanEndpointPath(path string) string {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}

// ninjaEndpointPartition returns the partition an endpoint's rows live in, and false when no
// local partition holds them.
func ninjaEndpointPartition(resourceType, path string) (string, bool) {
	if owner, ok := ninjaSyncPartitionForPath(path); ok {
		return owner, true
	}
	if syncOwnsPartition(resourceType) {
		return "", false
	}
	return resourceType, true
}

// ninjaLocalPartition is resolveLocal's partition, or an error naming why the endpoint is not
// in the local store.
func ninjaLocalPartition(resourceType, path string) (string, error) {
	if owner, ok := ninjaEndpointPartition(resourceType, path); ok {
		return owner, nil
	}
	return "", fmt.Errorf("%s is not mirrored in the local store: the %q partition holds a different endpoint's rows; use --data-source live", ninjaCleanEndpointPath(path), resourceType)
}

// ninjaMutationMayWriteThrough reports whether a mutation response may be written into a
// partition. A mutation response carries no list path, so it cannot prove it belongs to a sync
// partition's endpoint; sync stays the only writer of sync partitions.
func ninjaMutationMayWriteThrough(resourceType string) bool {
	return !syncOwnsPartition(resourceType)
}

// ninjaWriteThroughMayWrite reports whether an endpoint read may write its rows through into a
// partition. A report keyed by row ordinal (class report-row-identity) is written only by sync:
// a row's ordinal is its position in a walk from page one, and an endpoint read cannot know which
// page, cursor or filter its rows came from, so it would store them over other rows' positions.
func ninjaWriteThroughMayWrite(partition string) bool {
	return !store.ReportRowsAreOrdinal(partition)
}
