// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored (NOT generated). Concise-projection for the MCP `search` tool.
// Issue #101 residual item #2: the v0.2.4 fix gave the CLI `search` command a
// concise id/name/type/match projection, but the MCP `search` tool (a separate
// hand-wired handler) kept dumping whole raw records — a token sink for the very
// agents the projection exists to protect. This mirrors the CLI projection on the
// MCP surface; `full=true` restores raw records (parity with the CLI's --full).
//
// ponytail: this is a small deliberate mirror of internal/cli/search_projection.go
// (projectSearchHit/matchIndicator). The two live in different packages and the CLI
// copy is pinned by its own unit tests; the durable single-source home is the press
// (see handfixes.json: search-concise-projection spec_encode_followup + the
// /printing-press-retro filed for issue #101). Consolidate there, not here.

package mcp

import (
	"encoding/json"
	"sort"
	"strings"

	"axcient-pp-cli/internal/store"
)

type searchRow struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Match string `json:"match"`
}

// searchIDFields mirrors the live x360Recover key shapes (Python-style id_
// included); searchNameFields lists common human-label fields, best first.
// Kept in sync with internal/cli/search_projection.go.
var searchIDFields = []string{"id_", "id", "uid", "uuid", "guid", "gid", "sid", "key", "code"}
var searchNameFields = []string{"name", "title", "identifier", "alias", "hostname", "label", "display_name", "client_name", "device_name", "email", "subject"}

// projectSearchHit reduces a raw record to {id, name, type, match}.
func projectSearchHit(hit store.SearchHit, query string) searchRow {
	row := searchRow{Type: hit.ResourceType}
	var obj map[string]any
	if err := json.Unmarshal(hit.Data, &obj); err != nil {
		return row
	}
	for _, f := range searchIDFields {
		if v := store.LookupFieldValue(obj, f); v != nil {
			if s := store.ResourceIDString(v); s != "" && s != "<nil>" {
				row.ID = s
				break
			}
		}
	}
	for _, f := range searchNameFields {
		if v := store.LookupFieldValue(obj, f); v != nil {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				row.Name = strings.TrimSpace(s)
				break
			}
		}
	}
	row.Match = searchMatchIndicator(obj, query)
	return row
}

// searchMatchIndicator finds the first top-level string field (deterministic
// order) whose value contains the query term case-insensitively, as "field~term".
func searchMatchIndicator(obj map[string]any, query string) string {
	term := strings.TrimSpace(query)
	if term == "" {
		return ""
	}
	needle := strings.ToLower(term)
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if s, ok := obj[k].(string); ok && strings.Contains(strings.ToLower(s), needle) {
			return k + "~" + term
		}
	}
	return ""
}
