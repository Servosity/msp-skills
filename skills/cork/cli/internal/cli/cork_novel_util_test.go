// Copyright 2026 geekbrownbear and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored regression tests for Cork novel-command decoding.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

// Cork returns `integration` as a nested OBJECT on both associated_tenants and
// associated_endpoints. Typing it as a string made json.Unmarshal fail for the
// entire record, so every client (and every device) was silently skipped and
// commands returned a confident empty result. These tests pin the real shape.

func TestCorkClientDecodesNestedIntegrationObject(t *testing.T) {
	const payload = `{
	  "uuid": "11111111-2222-3333-4444-555555555555",
	  "name": "Example Client",
	  "hidden": false,
	  "warranty_status": "active",
	  "score_history": [
	    {"score": 500, "created_at": "2026-08-14T09:05:04.709349Z"},
	    {"score": 540, "created_at": "2026-08-07T09:05:04.709349Z"}
	  ],
	  "associated_tenants": [
	    {
	      "uuid": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
	      "name": "Tenant One",
	      "integration": {
	        "uuid": "99999999-8888-7777-6666-555555555555",
	        "display_name": "Example EDR",
	        "connection_status": "ok",
	        "last_synced_at": null,
	        "vendor": {"key": "example-edr", "name": "Example EDR"}
	      }
	    }
	  ]
	}`
	var c corkClient
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		t.Fatalf("client with nested integration object must decode, got: %v", err)
	}
	if c.UUID == "" {
		t.Fatal("uuid lost")
	}
	if len(c.ScoreHistory) != 2 {
		t.Fatalf("score_history = %d, want 2", len(c.ScoreHistory))
	}
	if len(c.Tenants) != 1 {
		t.Fatalf("tenants = %d, want 1", len(c.Tenants))
	}
	if got := c.Tenants[0].Integration.UUID; got != "99999999-8888-7777-6666-555555555555" {
		t.Fatalf("tenant integration uuid = %q", got)
	}
	if got := c.Tenants[0].Integration.Vendor.Key; got != "example-edr" {
		t.Fatalf("tenant vendor key = %q", got)
	}
}

func TestCorkClientDeviceDecodesNestedIntegrationObject(t *testing.T) {
	const payload = `{
	  "uuid": "dddddddd-1111-2222-3333-444444444444",
	  "name": "RECEPTION-02",
	  "device_type": "workstation",
	  "can_install_software": true,
	  "os": {"name": "Windows", "version": "11"},
	  "associated_endpoints": [
	    {
	      "name": "RECEPTION-02",
	      "last_seen": "2026-08-14T09:05:04Z",
	      "ip_addresses": {"v4": ["10.0.0.5"]},
	      "properties": [],
	      "integration_identifier": "EDR-ABC-123",
	      "integration": {
	        "uuid": "99999999-8888-7777-6666-555555555555",
	        "vendor": {"key": "example-edr", "name": "Example EDR"}
	      }
	    }
	  ]
	}`
	var d corkClientDevice
	if err := json.Unmarshal([]byte(payload), &d); err != nil {
		t.Fatalf("device with nested integration object must decode, got: %v", err)
	}
	if len(d.AssociatedEndpoints) != 1 {
		t.Fatalf("associated_endpoints = %d, want 1", len(d.AssociatedEndpoints))
	}
	if got := d.AssociatedEndpoints[0].IntegrationIdentifier; got != "EDR-ABC-123" {
		t.Fatalf("integration_identifier = %q; this is the coverage-gap join key", got)
	}
	if got := d.AssociatedEndpoints[0].Integration.UUID; got == "" {
		t.Fatal("nested integration uuid lost")
	}
}

func TestCorkClientMinimalFallbackKeepsIdentity(t *testing.T) {
	// A future upstream shape change in a nested field must not zero out the
	// client roster; the minimal shape has to keep identity and score history.
	const payload = `{
	  "uuid": "11111111-2222-3333-4444-555555555555",
	  "name": "Example Client",
	  "warranty_status": "expired",
	  "score_history": [{"score": 480, "created_at": "2026-08-14T09:05:04Z"}],
	  "associated_tenants": "this-was-an-object-yesterday"
	}`
	var full corkClient
	if err := json.Unmarshal([]byte(payload), &full); err == nil {
		t.Fatal("expected the full shape to reject a string associated_tenants")
	}
	var min corkClientMinimal
	if err := json.Unmarshal([]byte(payload), &min); err != nil {
		t.Fatalf("minimal fallback must still decode, got: %v", err)
	}
	if min.UUID == "" || min.Name == "" || len(min.ScoreHistory) != 1 {
		t.Fatalf("fallback lost identity or history: %+v", min)
	}
}

func TestCorkParseTimeAcceptsCorkTimestamps(t *testing.T) {
	for _, in := range []string{
		"2026-08-14T09:05:04.709349Z",
		"2026-08-05T09:23:55.24193Z",
		"2026-08-14T09:05:04Z",
	} {
		if _, ok := corkParseTime(in); !ok {
			t.Errorf("corkParseTime(%q) failed", in)
		}
	}
	if _, ok := corkParseTime(""); ok {
		t.Error("empty timestamp must not parse")
	}
}

func TestCorkPriorityRankOrdersSSVCTiers(t *testing.T) {
	if !(corkPriorityRank("critical") < corkPriorityRank("accelerated") &&
		corkPriorityRank("accelerated") < corkPriorityRank("routine") &&
		corkPriorityRank("routine") < corkPriorityRank("")) {
		t.Fatal("priority ordering must be critical < accelerated < routine < unknown")
	}
	if corkPriorityRank("CRITICAL") != corkPriorityRank("critical") {
		t.Fatal("priority rank must be case-insensitive")
	}
}

func TestCorkSinceAcceptsLooseDurations(t *testing.T) {
	got, err := corkSince("7d", time.Hour)
	if err != nil {
		t.Fatalf("7d must parse: %v", err)
	}
	if got != 7*24*time.Hour {
		t.Fatalf("7d = %v", got)
	}
	if d, err := corkSince("", 42*time.Hour); err != nil || d != 42*time.Hour {
		t.Fatalf("empty must fall back to the default, got %v %v", d, err)
	}
	if _, err := corkSince("banana", time.Hour); err == nil {
		t.Fatal("invalid duration must error")
	}
}

func TestOverdueBucketBoundaries(t *testing.T) {
	cases := map[float64]string{1: "<1d", 23.9: "<1d", 24: "1-3d", 71: "1-3d", 72: "3-7d", 167: "3-7d", 168: "7-30d", 719: "7-30d", 720: ">30d"}
	for h, want := range cases {
		if got := overdueBucket(h); got != want {
			t.Errorf("overdueBucket(%v) = %q, want %q", h, got, want)
		}
	}
}

// --- Regressions added after the Phase 4.95 review ---

// A non-KEV CVE must never outrank a KEV one however high its EPSS, and the
// answer must not depend on the order CVEs arrive in.
func TestTriageTopCVEPrefersKEVRegardlessOfOrder(t *testing.T) {
	kev := corkCVE{CVEID: "CVE-2026-0001", EPSS: 0.10, CVSS: 7.0, IsKEV: true, Priority: "critical"}
	loud := corkCVE{CVEID: "CVE-2026-9999", EPSS: 0.95, CVSS: 9.8, IsKEV: false, Priority: "routine"}

	for _, order := range [][]corkCVE{{kev, loud}, {loud, kev}} {
		top, isKEV := pickTopCVEForTest(order)
		if top != kev.CVEID {
			t.Fatalf("order %v: top_cve = %q, want the KEV CVE %q", []string{order[0].CVEID, order[1].CVEID}, top, kev.CVEID)
		}
		if !isKEV {
			t.Fatal("top CVE should be flagged KEV")
		}
	}
}

// Within the same KEV class, higher EPSS wins, then higher CVSS.
func TestTriageTopCVEWithinSameKEVClass(t *testing.T) {
	a := corkCVE{CVEID: "CVE-A", EPSS: 0.20, CVSS: 9.0, IsKEV: false}
	b := corkCVE{CVEID: "CVE-B", EPSS: 0.80, CVSS: 4.0, IsKEV: false}
	if top, _ := pickTopCVEForTest([]corkCVE{a, b}); top != "CVE-B" {
		t.Fatalf("higher EPSS should win, got %q", top)
	}
	c := corkCVE{CVEID: "CVE-C", EPSS: 0.80, CVSS: 9.9, IsKEV: false}
	if top, _ := pickTopCVEForTest([]corkCVE{b, c}); top != "CVE-C" {
		t.Fatalf("equal EPSS should fall through to CVSS, got %q", top)
	}
}

// The live /clients decode path must degrade like the mirror path, not drop
// records, and must report how many were wholly unreadable.
func TestCorkDecodeClientsDegradesInsteadOfDropping(t *testing.T) {
	good := json.RawMessage(`{"uuid":"u1","name":"Good","associated_tenants":[{"uuid":"t1","integration":{"uuid":"i1"}}]}`)
	drifted := json.RawMessage(`{"uuid":"u2","name":"Drifted","associated_tenants":"was-an-object"}`)
	junk := json.RawMessage(`{"name":"no uuid"}`)

	clients, undecodable := corkDecodeClients([]json.RawMessage{good, drifted, junk})
	if len(clients) != 2 {
		t.Fatalf("decoded %d clients, want 2 (the drifted one must survive in reduced form)", len(clients))
	}
	if undecodable != 1 {
		t.Fatalf("undecodable = %d, want 1", undecodable)
	}
	if clients[1].UUID != "u2" || clients[1].Name != "Drifted" {
		t.Fatalf("drifted client lost identity: %+v", clients[1])
	}
}

func TestCorkPathSegEscapesInjection(t *testing.T) {
	if got := corkPathSeg("abc?partner_uuid=other"); got == "abc?partner_uuid=other" {
		t.Fatal("query injection must be escaped out of a path segment")
	}
	if got := corkPathSeg("../../me"); got == "../../me" {
		t.Fatal("dot segments must be escaped")
	}
}
