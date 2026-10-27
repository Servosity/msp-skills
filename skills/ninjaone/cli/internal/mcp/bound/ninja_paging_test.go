// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package bound

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func ninjaDevices(from, n int) json.RawMessage {
	var rows []string
	for id := from; id < from+n; id++ {
		rows = append(rows, fmt.Sprintf(`{"id":%d,"systemName":"PC-%d"}`, id, id))
	}
	return json.RawMessage("[" + strings.Join(rows, ",") + "]")
}

func decodePage(t *testing.T, text string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("page is not JSON: %v\n%s", err, text)
	}
	return out
}

func TestKeysetPageOffersAfterIDUntilAShortPage(t *testing.T) {
	opts := PageOptions{CursorParam: "after", NextCursorPath: "after", PageSize: 3}
	full := decodePage(t, EndpointPageResponse("GET", ninjaDevices(1, 3), opts))
	next, _ := full["next_cursor"].(string)
	if next == "" {
		t.Fatalf("a full keyset page must offer a next_cursor: %v", full)
	}
	if up, err := UpstreamCursor(next); err != nil || up != "3" {
		t.Fatalf("next cursor resumes after %q (%v), want after the last id 3", up, err)
	}
	if full["count"] != float64(-1) {
		t.Fatalf("count = %v on a page with more to come, want -1 (unknown total)", full["count"])
	}
	short := decodePage(t, EndpointPageResponse("GET", ninjaDevices(4, 2), opts))
	if _, ok := short["next_cursor"]; ok {
		t.Fatalf("a short keyset page is the end: %v", short)
	}
	unknown := decodePage(t, EndpointPageResponse("GET", ninjaDevices(1, 2), PageOptions{CursorParam: "after", NextCursorPath: "after"}))
	if _, ok := unknown["next_cursor"]; !ok {
		t.Fatalf("without a known page size a keyset page must not claim to be the end: %v", unknown)
	}
}

func ninjaReport(offset, n int) json.RawMessage {
	var rows []string
	for i := 0; i < n; i++ {
		rows = append(rows, fmt.Sprintf(`{"deviceId":%d,"name":"C:"}`, offset+i+1))
	}
	return json.RawMessage(fmt.Sprintf(`{"cursor":{"name":"SESSION_TOKEN","offset":%d,"count":"%d"},"results":[%s]}`, offset, n, strings.Join(rows, ",")))
}

func TestObjectCursorPageKeepsTheRepeatedTokenUntilAShortPage(t *testing.T) {
	opts := PageOptions{CursorParam: "cursor", NextCursorPath: "cursor", PageSize: 3}
	first := decodePage(t, EndpointPageResponse("GET", ninjaReport(0, 3), opts))
	next, _ := first["next_cursor"].(string)
	if up, _ := UpstreamCursor(next); up != "SESSION_TOKEN" {
		t.Fatalf("first report page next = %q, want the cursor token", up)
	}
	opts.Cursor, opts.RequestCursor = next, "SESSION_TOKEN"
	second := decodePage(t, EndpointPageResponse("GET", ninjaReport(3, 3), opts))
	if _, ok := second["next_cursor"]; !ok {
		t.Fatalf("a full page under the same token is not the end: %v", second)
	}
	last := decodePage(t, EndpointPageResponse("GET", ninjaReport(6, 1), opts))
	if _, ok := last["next_cursor"]; ok {
		t.Fatalf("a short report page is the end: %v", last)
	}
}

// Class mcp-continuation-never-replays-a-stateful-cursor: a split page's
// continuation names the retained page; a page that is gone or was fetched for
// another request is an error, never a silent re-request.
func TestSplitPageContinuationIsServedFromTheRetainedPage(t *testing.T) {
	opts := PageOptions{CursorParam: "cursor", NextCursorPath: "cursor", PageSize: 100, RetainScope: "GET /v2/queries/volumes\x00pageSize=100"}
	first := decodePage(t, EndpointPageResponse("GET", ninjaReport(0, 100), opts))
	next, _ := first["next_cursor"].(string)
	if next == "" {
		t.Fatalf("a split page must offer a continuation: %v", first)
	}
	state, err := decodeEndpointCursor(next)
	if err != nil || state.Retained == "" || state.Offset != MaxItems {
		t.Fatalf("continuation %+v (%v) does not name the retained page at offset %d", state, err, MaxItems)
	}
	data, held, err := RetainedPage(next, opts.RetainScope)
	if !held || err != nil || string(data) != string(ninjaReport(0, 100)) {
		t.Fatalf("RetainedPage = held %v err %v", held, err)
	}
	if _, held, err := RetainedPage(next, "GET /v2/queries/volumes\x00pageSize=99"); !held || err != ErrRetainedPageScope {
		t.Fatalf("a continuation resent with other params: held %v err %v, want ErrRetainedPageScope", held, err)
	}
	gone := encodeEndpointCursor(endpointCursor{Version: 1, Offset: 50, UpstreamCursor: "SESSION_TOKEN", Retained: "0123456789abcdef0123456789abcdef"})
	if _, held, err := RetainedPage(gone, opts.RetainScope); !held || err != ErrRetainedPageGone {
		t.Fatalf("a continuation of a page no longer held: held %v err %v, want ErrRetainedPageGone", held, err)
	}
	if _, held, err := RetainedPage(encodeEndpointCursor(endpointCursor{Version: 1, UpstreamCursor: "SESSION_TOKEN"}), opts.RetainScope); held || err != nil {
		t.Fatalf("an upstream continuation is not a retained page: held %v err %v", held, err)
	}
	// Without a bound request (no RetainScope) the generated behavior is unchanged.
	plain := decodePage(t, EndpointPageResponse("GET", ninjaReport(0, 100), PageOptions{CursorParam: "cursor", NextCursorPath: "cursor", PageSize: 100}))
	if s, err := decodeEndpointCursor(plain["next_cursor"].(string)); err != nil || s.Retained != "" {
		t.Fatalf("unbound page retained: %+v %v", s, err)
	}
}
