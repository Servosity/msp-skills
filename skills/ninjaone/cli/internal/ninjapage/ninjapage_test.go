// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package ninjapage

import (
	"encoding/json"
	"testing"
)

func TestContinuationRule(t *testing.T) {
	for _, c := range []struct{ req, def, want int }{{50, 100, 50}, {1000, 100, 100}, {0, 100, 100}, {50, 0, 50}, {0, 0, 0}} {
		if got := Limit(c.req, c.def); got != c.want {
			t.Fatalf("Limit(%d, %d) = %d, want %d", c.req, c.def, got, c.want)
		}
	}
	if Full(0, 0) || !Full(1, 0) || Full(99, 100) || !Full(100, 100) {
		t.Fatal("Full: an empty page or one short of the limit ends a walk; any other page may continue")
	}
}

func TestKeysetLastID(t *testing.T) {
	items := []json.RawMessage{json.RawMessage(`{"id":1}`), json.RawMessage(`{"id":42,"naturalId":"u-9"}`)}
	k, ok := KeysetForParam(AfterParam)
	if id, got := k.LastID(items); !ok || !got || id != "42" {
		t.Fatalf("after keyset last id = %q %v, want 42", id, got)
	}
	k, ok = KeysetForPath("/v2/ticketing/app-user-contact/?pageSize=10")
	if id, got := k.LastID(items); !ok || !got || id != "u-9" {
		t.Fatalf("anchorNaturalId keyset last id = %q %v, want u-9", id, got)
	}
	if _, got := (Keyset{}).LastID([]json.RawMessage{json.RawMessage(`{"id":0}`)}); got {
		t.Fatal("a row with no usable key must stop the walk")
	}
	if !IsBareArray(json.RawMessage(` [{"id":1}]`)) || IsBareArray(json.RawMessage(`{"results":[]}`)) {
		t.Fatal("IsBareArray")
	}
}

func TestReportCursorProgressIncludesOffset(t *testing.T) {
	tok, p1, ok := ReportCursor(json.RawMessage(`{"name":"C","offset":0,"count":2}`))
	_, p2, _ := ReportCursor(json.RawMessage(`{"name":"C","offset":2,"count":2}`))
	if !ok || tok != "C" || p1 == p2 {
		t.Fatalf("token %q progress %q -> %q: a stable name with an advancing offset is progress", tok, p1, p2)
	}
	if _, _, ok := ReportCursor(json.RawMessage(`{"offset":2}`)); ok {
		t.Fatal("a cursor with no token must not continue")
	}
}
