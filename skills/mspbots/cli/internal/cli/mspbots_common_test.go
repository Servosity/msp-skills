// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"mspbots-pp-cli/internal/store"
)

func TestCompileWhere(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		col   string
		value string
	}{
		{"equals", "Status = Open", "Status", "Open"},
		{"contains", "Name contains Tod", "Name", "Tod"},
		{"gte date", "Update Date >= 2026-05-01", "Update Date", "2026-05-01,"},
		{"lte number", "Price <= 56.3", "Price", ",56.3"},
		{"between", "Price between 12.6,56.3", "Price", "12.6,56.3"},
		{"between intervals", "Price between 12.6, 56.3, 103, 210", "Price", "12.6,56.3,103,210"},
		{"is empty", "Status is empty", "Status", ""},
		{"spaced column equals", "Real Name = n", "Real Name", "n"},
		{"case-insensitive op", "Name CONTAINS tod", "Name", "tod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := compileWhere([]string{tt.in})
			if err != nil {
				t.Fatalf("compileWhere(%q): %v", tt.in, err)
			}
			got, ok := params[tt.col]
			if !ok {
				t.Fatalf("compileWhere(%q): column %q missing from %v", tt.in, tt.col, params)
			}
			if got != tt.value {
				t.Fatalf("compileWhere(%q): got %q want %q", tt.in, got, tt.value)
			}
		})
	}
}

func TestCompileWhereErrors(t *testing.T) {
	bad := []string{
		"",
		"NoOperatorHere",
		"Price between 12.6",        // odd bound count
		"Price between 12.6,56.3,9", // odd bound count
		"Status is empty trailing",
		"Status =",
	}
	for _, in := range bad {
		if _, err := compileWhere([]string{in}); err == nil {
			t.Fatalf("compileWhere(%q): expected error, got none", in)
		}
	}
	if _, err := compileWhere([]string{"A = 1", "A = 2"}); err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("duplicate column should error, got %v", err)
	}
}

func TestExtractRows(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
		ok   bool
	}{
		{"bare array", `[{"a":1},{"a":2}]`, 2, true},
		{"records envelope", `{"code":200,"records":[{"a":1}],"total":1}`, 1, true},
		{"rows envelope", `{"rows":[{"a":1},{"b":2},{"c":3}]}`, 3, true},
		{"data array", `{"data":[{"a":1}]}`, 1, true},
		{"nested data.records", `{"data":{"records":[{"a":1},{"a":2}],"current":1}}`, 2, true},
		{"non-tabular object", `{"message":"ok"}`, 0, false},
		{"empty", ``, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, ok := extractRows(json.RawMessage(tt.raw))
			if ok != tt.ok {
				t.Fatalf("extractRows(%s): ok=%v want %v", tt.raw, ok, tt.ok)
			}
			if len(rows) != tt.want {
				t.Fatalf("extractRows(%s): %d rows, want %d", tt.raw, len(rows), tt.want)
			}
		})
	}
}

func TestRowIdentityKey(t *testing.T) {
	withID := json.RawMessage(`{"id":"abc-123","value":9}`)
	if got := rowIdentityKey(withID); got != "id:abc-123" {
		t.Fatalf("id row key = %q", got)
	}
	numericID := json.RawMessage(`{"Id":42,"value":9}`)
	if got := rowIdentityKey(numericID); got != "id:42" {
		t.Fatalf("numeric Id row key = %q", got)
	}
	noID := json.RawMessage(`{"value":9,"name":"x"}`)
	key1 := rowIdentityKey(noID)
	// Same content, different field order → same hash key.
	key2 := rowIdentityKey(json.RawMessage(`{"name":"x","value":9}`))
	if !strings.HasPrefix(key1, "hash:") || key1 != key2 {
		t.Fatalf("hash keys differ for equal content: %q vs %q", key1, key2)
	}
	if rowIdentityKey(json.RawMessage(`{"value":10,"name":"x"}`)) == key1 {
		t.Fatal("different content must hash differently")
	}
}

func TestResolveResource(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWithContext(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.EnsureMspbotsSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.RegistryAdd(ctx, store.RegistryEntry{Alias: "open-tickets", ResourceID: "1534956341424005122", ResourceType: "dataset"}); err != nil {
		t.Fatal(err)
	}

	got, err := resolveResource(ctx, db, "open-tickets", "")
	if err != nil {
		t.Fatalf("alias resolve: %v", err)
	}
	if got.ResourceID != "1534956341424005122" || got.ResourceType != "dataset" || got.Alias != "open-tickets" {
		t.Fatalf("alias resolve = %+v", got)
	}

	raw, err := resolveResource(ctx, db, "9999999999999999999", "widget")
	if err != nil {
		t.Fatalf("raw id resolve: %v", err)
	}
	if raw.ResourceID != "9999999999999999999" || raw.ResourceType != "widget" || raw.Alias != "" {
		t.Fatalf("raw id resolve = %+v", raw)
	}

	if _, err := resolveResource(ctx, db, "unknown-alias", ""); err == nil {
		t.Fatal("unknown alias should error")
	} else if !strings.Contains(err.Error(), "open-tickets") {
		t.Fatalf("unknown-alias error should list known aliases, got: %v", err)
	}
}

func TestResourceEndpointPath(t *testing.T) {
	r := resolvedResource{ResourceID: "123", ResourceType: "widget"}
	if got := resourceEndpointPath(r); got != "/api/widget/123" {
		t.Fatalf("path = %q", got)
	}
}

func TestWhereFlagPreservesBetweenCommas(t *testing.T) {
	// Locks the StringArrayVar choice: pflag's slice mode comma-splits a
	// single --where value, shredding "between A,B" into broken predicates.
	flags := &rootFlags{}
	cmd := newNovelPullCmd(flags)
	if err := cmd.Flags().Parse([]string{"--where", "Price between 12.6,56.3", "--where", "Status = Open"}); err != nil {
		t.Fatal(err)
	}
	got, err := cmd.Flags().GetStringArray("where")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "Price between 12.6,56.3" {
		t.Fatalf("--where values = %q; commas inside a predicate must survive flag parsing", got)
	}
	params, err := compileWhere(got)
	if err != nil {
		t.Fatal(err)
	}
	if params["Price"] != "12.6,56.3" || params["Status"] != "Open" {
		t.Fatalf("compiled params = %v", params)
	}
}

func TestCanonicalRowJSONPreservesLargeIntegers(t *testing.T) {
	// 19-digit snowflake IDs exceed float64's 2^53 integer range; identity
	// hashing must not round them into collisions.
	a := json.RawMessage(`{"v":1534956341424005122}`)
	b := json.RawMessage(`{"v":1534956341424005123}`)
	if string(canonicalRowJSON(a)) == string(canonicalRowJSON(b)) {
		t.Fatal("distinct 19-digit values canonicalized identically (float64 rounding)")
	}
	if rowIdentityKey(a) == rowIdentityKey(b) {
		t.Fatal("distinct 19-digit values produced colliding identity keys")
	}
	if got := string(canonicalRowJSON(a)); got != `{"v":1534956341424005122}` {
		t.Fatalf("canonical form mangled the integer: %s", got)
	}
}

func TestResolveResourceRejectsMalformedStoredID(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWithContext(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.EnsureMspbotsSchema(ctx); err != nil {
		t.Fatal(err)
	}
	// Bypass registry add's validation, simulating a hand-edited DB.
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO mspbots_registry (alias, resource_id, resource_type, notes, created_at) VALUES ('evil', '../../admin', 'dataset', '', '2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveResource(ctx, db, "evil", ""); err == nil {
		t.Fatal("malformed stored resource ID must be rejected before reaching the URL path")
	}
}

func TestValidateResourceType(t *testing.T) {
	tests := []struct {
		in         string
		allowEmpty bool
		wantErr    bool
	}{
		{"dataset", true, false},
		{"widget", true, false},
		{"", true, false},
		{"", false, true},
		{"report", true, true},
		{"Dataset", false, true},
	}
	for _, tt := range tests {
		err := validateResourceType(tt.in, tt.allowEmpty)
		if (err != nil) != tt.wantErr {
			t.Fatalf("validateResourceType(%q, %v) err=%v, wantErr=%v", tt.in, tt.allowEmpty, err, tt.wantErr)
		}
	}
}
