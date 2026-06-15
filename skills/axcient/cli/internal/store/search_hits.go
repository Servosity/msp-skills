// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored (NOT generated). SearchHits returns each full-text match paired
// with its source resource_type, so the CLI's `search` command can render a
// concise id/name/type/match projection instead of dumping whole raw records.
// Kept in a hand-authored file so a cli-printing-press reprint of store.go does
// not clobber it. See handfixes.json: search-concise-projection.

package store

import (
	"database/sql"
	"encoding/json"
	"strings"
)

// SearchHit pairs a matched record's raw JSON with its source resource_type.
type SearchHit struct {
	ResourceType string
	Data         json.RawMessage
}

// SearchHits runs the same ranked FTS5 MATCH as Search but also returns each
// row's resource_type, letting callers label results without re-deriving the
// type from the data blob.
func (s *Store) SearchHits(query string, limit int, resourceTypes ...string) ([]SearchHit, error) {
	if limit <= 0 {
		limit = 50
	}
	matchQuery := ftsMatchQuery(query)
	if matchQuery == "" {
		return nil, nil
	}
	resourceType := ""
	if len(resourceTypes) > 0 {
		resourceType = strings.TrimSpace(resourceTypes[0])
	}

	var rows *sql.Rows
	var err error
	if resourceType != "" {
		rows, err = s.db.Query(
			`SELECT r.resource_type, r.data FROM resources r
			 JOIN resources_fts f ON r.id = f.id AND r.resource_type = f.resource_type
			 WHERE resources_fts MATCH ?
			 AND r.resource_type = ?
			 ORDER BY rank
			 LIMIT ?`,
			matchQuery, resourceType, limit,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT r.resource_type, r.data FROM resources r
			 JOIN resources_fts f ON r.id = f.id AND r.resource_type = f.resource_type
			 WHERE resources_fts MATCH ?
			 ORDER BY rank
			 LIMIT ?`,
			matchQuery, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var rt, data string
		if err := rows.Scan(&rt, &data); err != nil {
			return nil, err
		}
		hits = append(hits, SearchHit{ResourceType: rt, Data: json.RawMessage(data)})
	}
	return hits, rows.Err()
}
