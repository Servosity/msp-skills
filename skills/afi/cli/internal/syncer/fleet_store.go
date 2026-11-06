// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package syncer

import (
	"afi-pp-cli/internal/store"
)

// StoreSink is the local store as a fleet-sync Sink. The fleet sync writes only
// the generic resources table, so pruning reads and deletes only there (the
// typed tables the generator emitted stay empty; see the package comment).
type StoreSink struct{ *store.Store }

// Prune implements Sink over the store's reconcile helpers.
func (s StoreSink) Prune(resourceType, scopeField, scopeValue string, keep []string) error {
	if scopeField == "" {
		_, err := s.ReconcileAll(resourceType, keep, "", nil)
		return err
	}
	_, err := s.ReconcilePartition(resourceType, "$."+scopeField, scopeValue, keep, "", nil)
	return err
}

// ScopeValues implements Sink.
func (s StoreSink) ScopeValues(resourceType, scopeField string) ([]string, error) {
	rows, err := s.DB().Query(`SELECT DISTINCT json_extract(data, ?) FROM resources
		WHERE resource_type = ? AND json_valid(data) AND json_type(data, ?) = 'text'`,
		"$."+scopeField, resourceType, "$."+scopeField)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
