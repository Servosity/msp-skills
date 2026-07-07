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

func TestPaginationAdvanceKeyUsesObjectCursorOffset(t *testing.T) {
	page1 := json.RawMessage(`{
		"cursor": {"name":"SESSION_TOKEN","offset":100,"count":"100"},
		"results": [{"deviceId":1001}]
	}`)
	page2 := json.RawMessage(`{
		"cursor": {"name":"SESSION_TOKEN","offset":200,"count":"100"},
		"results": [{"deviceId":1002}]
	}`)

	_, next1, hasMore1 := extractPageItems(page1, "cursor")
	_, next2, hasMore2 := extractPageItems(page2, "cursor")
	if next1 != "SESSION_TOKEN" || next2 != "SESSION_TOKEN" {
		t.Fatalf("next cursors = (%q, %q), want same valid API cursor token", next1, next2)
	}
	if !hasMore1 || !hasMore2 {
		t.Fatalf("hasMore = (%v, %v), want both true", hasMore1, hasMore2)
	}

	key1 := paginationAdvanceKey(page1, "cursor", next1)
	key2 := paginationAdvanceKey(page2, "cursor", next2)
	if key1 != "SESSION_TOKEN|offset=100" {
		t.Fatalf("page1 advance key = %q, want offset-qualified key", key1)
	}
	if key2 != "SESSION_TOKEN|offset=200" {
		t.Fatalf("page2 advance key = %q, want offset-qualified key", key2)
	}
	if key1 == key2 {
		t.Fatalf("same cursor token with advancing offsets looked sticky: %q", key1)
	}
}

func TestPaginationAdvanceKeyStableObjectOffsetRemainsSticky(t *testing.T) {
	page := json.RawMessage(`{
		"cursor": {"name":"SESSION_TOKEN","offset":100,"count":"100"},
		"results": [{"deviceId":1001}]
	}`)
	_, nextCursor, _ := extractPageItems(page, "cursor")

	first := paginationAdvance(page, "cursor", nextCursor)
	second := paginationAdvance(page, "cursor", nextCursor)
	if first.key == "" || first.key != second.key {
		t.Fatalf("stable object cursor keys = (%q, %q), want same non-empty key", first.key, second.key)
	}
	if !second.stuckAfter(first) {
		t.Fatalf("stable object cursor offset was not treated as stuck")
	}
}

func TestPaginationProgressRequiresMonotonicObjectOffset(t *testing.T) {
	page100 := json.RawMessage(`{
		"cursor": {"name":"SESSION_TOKEN","offset":100,"count":"100"},
		"results": [{"deviceId":1001}]
	}`)
	page200 := json.RawMessage(`{
		"cursor": {"name":"SESSION_TOKEN","offset":200,"count":"100"},
		"results": [{"deviceId":1002}]
	}`)
	page50 := json.RawMessage(`{
		"cursor": {"name":"SESSION_TOKEN","offset":50,"count":"100"},
		"results": [{"deviceId":1003}]
	}`)

	_, nextCursor, _ := extractPageItems(page100, "cursor")
	first := paginationAdvance(page100, "cursor", nextCursor)
	second := paginationAdvance(page200, "cursor", nextCursor)
	regressed := paginationAdvance(page50, "cursor", nextCursor)

	if second.stuckAfter(first) {
		t.Fatalf("advancing object cursor offset was treated as stuck: %q after %q", second.key, first.key)
	}
	if !regressed.stuckAfter(second) {
		t.Fatalf("regressed object cursor offset was not treated as stuck: %q after %q", regressed.key, second.key)
	}
}

func TestPaginationAdvanceKeyStringCursorFallsBackToToken(t *testing.T) {
	data := json.RawMessage(`{"next_cursor":"STRING_TOKEN","results":[{"id":1}]}`)
	key := paginationAdvanceKey(data, "cursor", "STRING_TOKEN")
	if key != "STRING_TOKEN" {
		t.Fatalf("string cursor advance key = %q, want token fallback", key)
	}
}
