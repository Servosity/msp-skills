// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFleetVulnTriage(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetVulnTriageCmd, db)

	var rows []vulnTriageRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 CVEs, got %d", len(rows))
	}
	// KEV CVE ranks first regardless of raw priority.
	if rows[0].CVEID != "CVE-2025-0001" || !rows[0].CISAKEV {
		t.Fatalf("KEV CVE should rank first, got %+v", rows[0])
	}
	if rows[0].CVSS != 9.8 {
		t.Fatalf("cvss not parsed from string: %+v", rows[0])
	}
	// Priority = cvss * (count+1) * 2 (KEV) = 9.8 * 6 * 2 = 117.6
	if rows[0].Priority < 117 || rows[0].Priority > 118 {
		t.Fatalf("priority miscomputed: %v", rows[0].Priority)
	}
	if rows[0].Software != "Acme Reader" {
		t.Fatalf("software name not extracted: %q", rows[0].Software)
	}
}

func TestFleetVulnTriageKevOnly(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetVulnTriageCmd, db, "--kev-only")
	var rows []vulnTriageRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 || rows[0].CVEID != "CVE-2025-0001" {
		t.Fatalf("--kev-only should leave only the KEV CVE: %+v", rows)
	}
}

func TestFleetVulnTriageMinCVSS(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetVulnTriageCmd, db, "--min-cvss", "8.0")
	var rows []vulnTriageRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("--min-cvss 8.0 should drop the 7.1 CVE: %+v", rows)
	}
}
