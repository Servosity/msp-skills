// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import "testing"

// TestMerakiCompositeTimestampIDIsUsable guards the hand-fix
// meraki-store-iso-date-class: the generated isoDatePattern wrote its
// character class as [0-9:.+-Zz], and "+-Z" is a RANGE (0x2B-0x5A) that
// covers every uppercase letter, so a composite key such as
// "<ts>+<serial>" (Meraki serials are uppercase) was rejected as "a date".
// Every device availability change-history row failed ID extraction and
// `sync` exited 1 on any org with a recent status change.
func TestMerakiCompositeTimestampIDIsUsable(t *testing.T) {
	row := map[string]any{
		"ts":     "2026-10-12T01:40:29.090210Z",
		"device": map[string]any{"serial": "Q2AA-0001-0001"},
	}
	if got := ExtractResourceID("organizations_devices_availabilities_change_history", row); got == "" {
		t.Fatal("change-history row lost its composite ts+serial ID")
	}
	for _, date := range []string{"2026-10-12", "2026-10-12T01:40:29Z", "2026-10-12T01:40:29.090210Z", "2026-10-12T01:40:29+00:00", "2026-10-12 01:40:29-05:00"} {
		if !unusableResourceID(date) {
			t.Fatalf("bare timestamp %q must still be refused as an ID", date)
		}
	}
	for _, id := range []string{"2026-10-12T01:40:29Z+Q2AA-0001-0001", "2026-10-12T01:40:29Z+646829496481105433+VPN+NAME"} {
		if unusableResourceID(id) {
			t.Fatalf("composite ID %q was refused as a date", id)
		}
	}
}
