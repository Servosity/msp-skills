// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"salesbuildr-pp-cli/internal/store"
)

// newSQLCmd is the hand-built read-only SQL escape hatch over the local store.
// pp:data-source local
func newSQLCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "sql <query>",
		Short: "Run a read-only SQL query against the local store",
		Long: "Execute a SELECT (or WITH ... SELECT) against the synced SQLite store and emit rows as JSON\n" +
			"objects. Mutating statements are rejected. Typed tables: company, contact, product,\n" +
			"opportunity, quote, pricing_book, category, quote_template, quote_widget_template,\n" +
			"quote_discount_group, sync_state — each with extracted columns plus the raw `data` JSON.\n" +
			"The generic `resources` table (resource_type, id, data) and `resources_fts` are also queryable.",
		Example: "  salesbuildr-cli sql \"SELECT name, markup FROM product WHERE markup < 20 ORDER BY markup\"\n" +
			"  salesbuildr-cli sql \"SELECT status, count(*) n FROM quote GROUP BY status\"",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a SELECT query is required"))
			}
			query := strings.TrimSpace(strings.Join(args, " "))
			if !isReadOnlyQuery(query) {
				return usageErr(fmt.Errorf("only read-only SELECT/WITH queries are allowed"))
			}
			if dbPath == "" {
				dbPath = defaultDBPath("salesbuildr-cli")
			}
			// Read-only open (SQLite mode=ro): the engine itself rejects every
			// write, so the read-only guarantee does not rest on the keyword
			// filter alone. A missing file means nothing was ever synced.
			db, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return notFoundErr(fmt.Errorf("local store at %s is not readable (run `salesbuildr-cli sync` first or pass --db): %w", dbPath, err))
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "") {
				hintIfStale(cmd, db, "", flags.maxAge)
			}

			// A store with zero synced rows and no sync history is almost
			// always a wrong --db path or a sync that never ran. Empty
			// result sets from a phantom store are indistinguishable from
			// "no data exists" — fail with an actionable hint instead.
			var resourceRows int
			_ = db.DB().QueryRowContext(cmd.Context(), `SELECT count(*) FROM resources`).Scan(&resourceRows)
			if resourceRows == 0 {
				if state, stateErr := readSyncHintState(db, ""); stateErr == nil && !state.hasState {
					return notFoundErr(fmt.Errorf("local store at %s has never been synced and holds no data; run `salesbuildr-cli sync` first (or pass --db)", db.Path()))
				}
			}

			rows, err := db.DB().QueryContext(cmd.Context(), query)
			if err != nil {
				return usageErr(fmt.Errorf("query: %w", err))
			}
			defer rows.Close()
			cols, err := rows.Columns()
			if err != nil {
				return apiErr(err)
			}
			out := make([]map[string]any, 0)
			for rows.Next() {
				cells := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range cells {
					ptrs[i] = &cells[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return apiErr(err)
				}
				row := map[string]any{}
				for i, col := range cols {
					row[col] = normalizeCell(cells[i])
				}
				out = append(out, row)
			}
			if err := rows.Err(); err != nil {
				return apiErr(err)
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (defaults to standard location)")
	return cmd
}

func isReadOnlyQuery(q string) bool {
	lower := strings.ToLower(strings.TrimSpace(q))
	if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") {
		return false
	}
	for _, banned := range []string{"insert", "update", "delete", "drop", "alter", "create", "attach", "pragma", "vacuum", "analyze", "reindex"} {
		if containsWord(lower, banned) {
			return false
		}
	}
	return true
}

func containsWord(s, word string) bool {
	idx := 0
	for {
		i := strings.Index(s[idx:], word)
		if i < 0 {
			return false
		}
		i += idx
		before := i == 0 || !isWordChar(s[i-1])
		afterPos := i + len(word)
		after := afterPos >= len(s) || !isWordChar(s[afterPos])
		if before && after {
			return true
		}
		idx = i + len(word)
	}
}

func isWordChar(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// normalizeCell converts []byte (SQLite TEXT) to string and leaves numbers
// as-is so JSON output is clean. Embedded JSON (the raw data columns) is
// preserved as structured output.
func normalizeCell(v any) any {
	switch t := v.(type) {
	case []byte:
		s := string(t)
		if len(s) > 0 && (s[0] == '{' || s[0] == '[') {
			var js json.RawMessage
			if json.Unmarshal([]byte(s), &js) == nil {
				return js
			}
		}
		return s
	}
	return v
}
