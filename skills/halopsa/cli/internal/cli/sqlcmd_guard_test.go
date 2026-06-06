// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestValidateSelectOnly(t *testing.T) {
	ok := []string{
		"SELECT * FROM tickets",
		"select id from clients;",
		"WITH x AS (SELECT 1) SELECT * FROM x",
		"EXPLAIN QUERY PLAN SELECT 1",
		"SELECT 'we updated the deleted items' AS note", // suffixed forms don't hit the word-boundary regex
	}
	reject := []string{
		"DELETE FROM tickets",
		"SELECT 1; DELETE\tFROM tickets",
		"SELECT 1; DROP\nTABLE tickets",
		"SELECT 1; PRAGMA writable_schema=1",
		"select 1; vacuum",
		"INSERT INTO tickets VALUES (1)",
		"ATTACH DATABASE 'x' AS y",
		"SELECT 'please update this row' AS note", // bare keyword inside a literal: conservative rejection
	}
	for _, q := range ok {
		if err := validateSelectOnly(q); err != nil {
			t.Errorf("expected OK, got error for %q: %v", q, err)
		}
	}
	for _, q := range reject {
		if err := validateSelectOnly(q); err == nil {
			t.Errorf("expected rejection for %q", q)
		}
	}
}
