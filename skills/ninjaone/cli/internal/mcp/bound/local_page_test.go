// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package bound

// Hand-fix tests (handfixes.json unpaged-list-mcp-continuation, issue #704).

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// sessions builds n ~1 KB items, so 131 of them (~134 KB) are over both
// MaxItems and MaxBytes, the shape of the reported host-pool answer.
func sessions(n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := range out {
		out[i] = map[string]any{"id": fmt.Sprintf("s-%03d", i), "userPrincipalName": fmt.Sprintf("user%03d@contoso.example", i), "pad": strings.Repeat("x", 900)}
	}
	return out
}

// walk reads every page the way an agent does: pass the cursor back until
// there is none. It returns the ids read in order (an item shown only as a
// preview counts as "too-large:<index>") and the first page.
func walk(t *testing.T, raw []byte, field string) ([]string, map[string]json.RawMessage) {
	t.Helper()
	var ids []string
	var first map[string]json.RawMessage
	scope := LocalScope("host-pools.sessions", "/rest-api/v1/x", map[string]string{"a": "1"})
	cursor, offset := "", 0
	for page := 0; page < 200; page++ {
		text := EndpointLocalPageResponse("GET", raw, offset, scope)
		if len(text) > MaxBytes {
			t.Fatalf("page %d is %d bytes, over MaxBytes", page, len(text))
		}
		var out map[string]json.RawMessage
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if first == nil {
			first = out
		}
		var items []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(out[field], &items); err != nil {
			t.Fatalf("page %d %s: %v", page, field, err)
		}
		for _, it := range items {
			ids = append(ids, it.ID)
		}
		for _, key := range []string{"item_too_large", "_pp_item_too_large"} {
			if raw, ok := out[key]; ok {
				var big struct {
					Index int `json:"index"`
				}
				_ = json.Unmarshal(raw, &big)
				ids = append(ids, fmt.Sprintf("too-large:%d", big.Index))
			}
		}
		// Inside a vendor object the cursor is _pp_next_cursor; a vendor's own
		// next_cursor member is data, not this server's cursor.
		key := "next_cursor"
		if field != "items" || out["_pp_total_count"] != nil {
			key = "_pp_next_cursor"
		}
		var next string
		_ = json.Unmarshal(out[key], &next)
		if next == "" {
			return ids, first
		}
		if next == cursor {
			t.Fatalf("page %d returned the cursor it was given", page)
		}
		got, err := LocalPageCursor(next, scope)
		if err != nil {
			t.Fatalf("issued cursor rejected: %v", err)
		}
		cursor, offset = next, got
	}
	t.Fatal("cursor never ended")
	return nil, nil
}

func wantAll(t *testing.T, ids []string, n int) {
	t.Helper()
	if len(ids) != n {
		t.Fatalf("read %d items, want %d", len(ids), n)
	}
	for i, id := range ids {
		if id != fmt.Sprintf("s-%03d", i) {
			t.Fatalf("item %d is %s: skipped or repeated", i, id)
		}
	}
}

func TestLocalPageBareListReadsEveryItem(t *testing.T) {
	raw, _ := json.Marshal(sessions(131))
	ids, first := walk(t, raw, "items")
	wantAll(t, ids, 131)
	var note string
	_ = json.Unmarshal(first["note"], &note)
	if !strings.Contains(note, "_pp_cursor") || strings.Contains(note, "limit, offset") {
		t.Fatalf("note does not fit an unpaged endpoint: %q", note)
	}
	if string(first["count"]) != "131" {
		t.Fatalf("count = %s, want 131", first["count"])
	}
}

func TestLocalPageWrappedListReadsEveryItemAndKeepsVendorMembers(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"totalCount": 1357, "count": 999, "note": "vendor warning", "next_cursor": "vendor-owned", "items": sessions(1357)})
	ids, first := walk(t, raw, "items")
	wantAll(t, ids, 1357)
	for key, want := range map[string]string{"totalCount": "1357", "count": "999", "note": `"vendor warning"`, "next_cursor": `"vendor-owned"`} {
		if string(first[key]) != want {
			t.Fatalf("vendor member %s = %s, want %s", key, first[key], want)
		}
	}
}

// An answer that is not cut must render exactly as before the fix.
func TestLocalPageUncutAnswersUnchanged(t *testing.T) {
	three, _ := json.Marshal(sessions(3))
	nearBudget := []byte(`[{"payload":"` + strings.Repeat("x", 59960) + `"}]`)
	for name, raw := range map[string][]byte{
		"empty":         []byte(`[]`),
		"three":         three,
		"near budget":   nearBudget,
		"small object":  []byte(`{"name":"pool","hosts":[{"id":"h1"}]}`),
		"two arrays":    []byte(`{"a":[1],"b":[2]}`),
		"scalar object": []byte(`{"id":1}`),
	} {
		if got, want := EndpointLocalPageResponse("GET", raw, 0, "scope"), EndpointResponse("GET", raw); got != want {
			t.Fatalf("%s changed:\n got %.200s\nwant %.200s", name, got, want)
		}
	}
}

// An item larger than the whole budget is reported, never dropped silently,
// and the walk continues after it.
func TestLocalPageOversizedItemIsReportedAndSkippedPast(t *testing.T) {
	list := sessions(3)
	list[1]["pad"] = strings.Repeat("y", 70000)
	raw, _ := json.Marshal(list)
	ids, _ := walk(t, raw, "items")
	if strings.Join(ids, ",") != "s-000,too-large:1,s-002" {
		t.Fatalf("walk = %v", ids)
	}
}

func TestLocalPageCursorRefusesForeignCursors(t *testing.T) {
	scope := LocalScope("alerts.get", "/v2/alerts", map[string]string{"sourceType": "A"})
	other := LocalScope("alerts.get", "/v2/alerts", map[string]string{"sourceType": "B"})
	if scope == other {
		t.Fatal("different queries share a scope")
	}
	for _, c := range []string{"", "not-a-cursor", encodeEndpointCursor(endpointCursor{Version: 1, Offset: 5}), encodeLocalCursor(50, other), encodeLocalCursor(-1, scope)} {
		if _, err := LocalPageCursor(c, scope); err == nil {
			t.Fatalf("accepted %q", c)
		}
	}
	if offset, err := LocalPageCursor(encodeLocalCursor(50, scope), scope); err != nil || offset != 50 {
		t.Fatalf("refused its own cursor: %d %v", offset, err)
	}
	// An offset past the end reads nothing and offers no cursor.
	raw, _ := json.Marshal(sessions(3))
	var past map[string]json.RawMessage
	_ = json.Unmarshal([]byte(EndpointLocalPageResponse("GET", raw, 1<<30, scope)), &past)
	if _, ok := past["next_cursor"]; ok || string(past["items"]) != "[]" {
		t.Fatalf("offset past end: %v", past)
	}
}

// A vendor member too large to share a page with an item is left out (and
// named) rather than reducing every item to a preview.
func TestLocalPageLargeVendorMemberDoesNotForcePreviews(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"blob": strings.Repeat("z", 59500), "items": sessions(20)})
	ids, first := walk(t, raw, "items")
	wantAll(t, ids, 20)
	if string(first["_pp_omitted_members"]) != `["blob"]` {
		t.Fatalf("omitted members = %s", first["_pp_omitted_members"])
	}
}

// A page that leaves out a vendor member says it is not the whole answer,
// even when every item fits.
func TestLocalPageOmittedMemberIsFlaggedTruncated(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"blob": strings.Repeat("z", 70000), "items": sessions(5)})
	var out map[string]json.RawMessage
	_ = json.Unmarshal([]byte(EndpointLocalPageResponse("GET", raw, 0, "scope")), &out)
	if string(out["_pp_truncated"]) != "true" || string(out["_pp_omitted_members"]) != `["blob"]` || out["_pp_note"] == nil {
		t.Fatalf("omitted member not flagged: truncated=%s omitted=%s", out["_pp_truncated"], out["_pp_omitted_members"])
	}
}
