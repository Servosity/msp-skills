// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

// TestSyncUnwrapsMerakiEnvelopeArray guards the hand-fix
// meraki-sync-envelope-array: an ARRAY of {items, meta} envelopes yields the
// records inside it, while an ordinary array of records is left alone.
func TestSyncUnwrapsMerakiEnvelopeArray(t *testing.T) {
	page := json.RawMessage(`[{"items":[{"serial":"Q234-ABCD-0001"},{"serial":"Q234-ABCD-0002"}],"meta":{"counts":{"items":{"total":2}}}}]`)
	items, _, _ := extractPageItemsWithPagination(page, "", "")
	if len(items) != 2 {
		t.Fatalf("got %d items from an envelope array, want the 2 records inside it", len(items))
	}
	plain := json.RawMessage(`[{"serial":"Q234-ABCD-0001","items":["x"]},{"serial":"Q234-ABCD-0002"}]`)
	if items, _, _ := extractPageItemsWithPagination(plain, "", ""); len(items) != 2 {
		t.Fatalf("an ordinary record array was altered: %d items", len(items))
	}
}
