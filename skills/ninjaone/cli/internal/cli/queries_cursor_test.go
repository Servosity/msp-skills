// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

// Regression tests for issue #136 (pagination half): NinjaOne /v2/queries/*
// returns an OBJECT-valued cursor, "cursor":{"name":"<token>",...}. The old
// findCursorInMap only unmarshalled a string cursor, so the next-page token was
// never found and sync stopped after the first 100 rows (pagination_cursor_missing).
// extractPageItems must now surface cursor.name as the next cursor, while plain
// string cursors keep working unchanged.

func TestExtractPageItemsFollowsObjectCursor(t *testing.T) {
	data := json.RawMessage(`{
		"cursor": {"name":"PAGE_TOKEN_2","offset":0,"count":"250","expires":"2026-01-01T00:00:00Z"},
		"results": [
			{"deviceId":1001,"productName":"SentinelOne"},
			{"deviceId":1002,"productName":"Microsoft Defender Antivirus"}
		]
	}`)
	items, nextCursor, hasMore := extractPageItems(data, "cursor")
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if nextCursor != "PAGE_TOKEN_2" {
		t.Errorf("want nextCursor PAGE_TOKEN_2 (cursor.name), got %q - pagination would truncate at 100 rows", nextCursor)
	}
	if !hasMore {
		t.Errorf("want hasMore=true when a next cursor is present")
	}
}

// A terminal page whose cursor object carries no usable token must not advance.
func TestExtractPageItemsObjectCursorEmptyTokenStops(t *testing.T) {
	data := json.RawMessage(`{"cursor":{"offset":250,"count":"250"},"results":[{"deviceId":1003}]}`)
	_, nextCursor, _ := extractPageItems(data, "cursor")
	if nextCursor != "" {
		t.Errorf("want empty nextCursor when cursor object has no token, got %q", nextCursor)
	}
}

// Regression guard: a plain STRING cursor must still resolve (object-cursor
// support runs only after the string attempt fails).
func TestExtractPageItemsStringCursorUnaffected(t *testing.T) {
	data := json.RawMessage(`{"next_cursor":"STRING_TOKEN","results":[{"id":1}]}`)
	_, nextCursor, hasMore := extractPageItems(data, "cursor")
	if nextCursor != "STRING_TOKEN" {
		t.Errorf("string cursor regressed: want STRING_TOKEN, got %q", nextCursor)
	}
	if !hasMore {
		t.Errorf("want hasMore=true for a present string cursor")
	}
}

func TestCursorTokenFromObject(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"ninjaone name field", `{"name":"abc","offset":0}`, "abc"},
		{"next_cursor field", `{"next_cursor":"xyz"}`, "xyz"},
		{"empty name", `{"name":"","offset":0}`, ""},
		{"no token fields", `{"offset":0,"count":"5"}`, ""},
		{"not an object", `"a-plain-string"`, ""},
		{"numeric token ignored", `{"name":12345}`, ""},
	}
	for _, c := range cases {
		if got := cursorTokenFromObject(json.RawMessage(c.raw)); got != c.want {
			t.Errorf("%s: cursorTokenFromObject(%s) = %q, want %q", c.name, c.raw, got, c.want)
		}
	}
}
