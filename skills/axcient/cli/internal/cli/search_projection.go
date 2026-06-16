// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-authored (NOT generated). Concise-projection renderer for `search`.
// By default `search` shows id/name/type/match rows instead of dumping whole
// raw records (a token sink for agents); `--full` restores the raw records by
// delegating to the generated outputSearchResults. Kept hand-authored so a
// cli-printing-press reprint of search.go does not clobber it.
// See handfixes.json: search-concise-projection.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"axcient-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// searchRow is the concise projection of a single search hit.
type searchRow struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Match string `json:"match"`
}

// searchIDFields mirrors the live x360Recover key shapes (Python-style id_
// included); searchNameFields lists common human-label fields, best first.
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
	row.Match = matchIndicator(obj, query)
	return row
}

// matchIndicator finds the first top-level string field (in deterministic order)
// whose value contains the query term case-insensitively, rendered as
// "field~term". Empty when the match came only from FTS tokenization.
func matchIndicator(obj map[string]any, query string) string {
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

func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// renderSearchResults is the search command's output path. With full=true it
// delegates to the generated raw renderer (whole records). Otherwise it emits
// the concise id/name/type/match projection — a table for TTY users, a JSON
// array (wrapped with provenance) for --json / piped output.
func renderSearchResults(cmd *cobra.Command, flags *rootFlags, hits []store.SearchHit, limit int, prov DataProvenance, full bool, query string) error {
	// Drop records with no usable identifier (same gate as the raw path).
	filtered := make([]store.SearchHit, 0, len(hits))
	for _, h := range hits {
		if !isNilOrEmpty(h.Data) {
			filtered = append(filtered, h)
		}
	}
	hits = filtered
	if len(hits) > limit {
		hits = hits[:limit]
	}

	if full {
		raw := make([]json.RawMessage, 0, len(hits))
		for _, h := range hits {
			raw = append(raw, h.Data)
		}
		return outputSearchResults(cmd, flags, raw, limit, prov)
	}

	rows := make([]searchRow, 0, len(hits))
	for _, h := range hits {
		rows = append(rows, projectSearchHit(h, query))
	}

	jsonMode := flags.asJSON || !isTerminal(cmd.OutOrStdout())
	if jsonMode {
		data, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		wrapped, err := wrapWithProvenance(data, prov)
		if err != nil {
			return err
		}
		return printOutput(cmd.OutOrStdout(), wrapped, true)
	}

	if len(rows) == 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "No results (source: %s)\n", prov.Source)
		return nil
	}

	printProvenance(cmd, len(rows), prov)
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tTYPE\tMATCH")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", dashIfEmpty(r.ID), dashIfEmpty(r.Name), dashIfEmpty(r.Type), dashIfEmpty(r.Match))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "\nConcise view; pass --full for whole records.")
	return nil
}
