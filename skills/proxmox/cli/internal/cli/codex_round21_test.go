// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// Codex round 21: a cluster's identity folds only the case-insensitive parts
// of its PVE base. Two clusters behind one gateway at /A and /a stay two, so
// neither hides as the other's sharer on a common PBS namespace.
func TestPVEIDKeepsPathCase(t *testing.T) {
	if pveID("https://gateway/A/api2/json") == pveID("https://gateway/a/api2/json") {
		t.Error("bases differing only in path case collapsed into one cluster identity")
	}
	same := []string{
		"https://gateway/A/api2/json",
		"https://GATEWAY/A/api2/json/",
		"HTTPS://gateway:443/A/api2/json",
		" https://gateway./A/api2/json ",
	}
	for _, s := range same[1:] {
		if pveID(s) != pveID(same[0]) {
			t.Errorf("%q and %q are one cluster but got %q and %q", s, same[0], pveID(s), pveID(same[0]))
		}
	}
	if pveID("https://[fd00:0::10]:8006/api2/json") != pveID("https://[FD00::10]:8006/api2/json") {
		t.Error("one IPv6 address spelled two ways is two identities")
	}
	if pveID("https://pve1:8006/api2/json") == pveID("https://pve1:8007/api2/json") {
		t.Error("two ports on one host collapsed")
	}
	if pveID("") != "" {
		t.Error("an empty base must stay empty (no PVE identity)")
	}
}
