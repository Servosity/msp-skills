// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written tests for the incident novel feature.

package cli

import (
	"encoding/json"
	"testing"
)

func TestDecodeThreatSummary(t *testing.T) {
	data := json.RawMessage(`{
		"id": "threat-1",
		"name": "Invoice phish wave",
		"type": "url",
		"category": "phish",
		"status": "active",
		"severity": 750,
		"attackSpread": 42,
		"identifiedAt": "2026-06-01T00:00:00Z",
		"notable": true,
		"actors": [{"id": "a1", "name": "TA000"}],
		"families": [{"id": "f1", "name": "credential-phish"}],
		"malware": [],
		"techniques": [{"id": "t1", "name": "attachment-lure"}],
		"brands": [{"id": "b1", "name": "ExampleBank"}]
	}`)
	summary, err := decodeThreatSummary(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.ID != "threat-1" || summary.Category != "phish" || summary.Status != "active" {
		t.Fatalf("scalar fields not projected: %+v", summary)
	}
	if summary.Severity == nil || *summary.Severity != 750 {
		t.Fatalf("severity not projected: %+v", summary.Severity)
	}
	if summary.AttackSpread == nil || *summary.AttackSpread != 42 {
		t.Fatalf("attackSpread not projected: %+v", summary.AttackSpread)
	}
	if len(summary.Actors) != 1 || summary.Actors[0] != "TA000" {
		t.Fatalf("actor names not flattened: %+v", summary.Actors)
	}
	if len(summary.Malware) != 0 {
		t.Fatalf("empty malware must stay empty: %+v", summary.Malware)
	}
	if len(summary.Brands) != 1 || summary.Brands[0] != "ExampleBank" {
		t.Fatalf("brand names not flattened: %+v", summary.Brands)
	}
}

func TestDecodeThreatSummaryMalformed(t *testing.T) {
	if _, err := decodeThreatSummary(json.RawMessage(`[1,2,3]`)); err == nil {
		t.Fatal("array payload must error")
	}
}

func TestDecodeThreatSummaryMissingOptionalFields(t *testing.T) {
	summary, err := decodeThreatSummary(json.RawMessage(`{"id":"t2","name":"minimal"}`))
	if err != nil {
		t.Fatalf("minimal summary must decode: %v", err)
	}
	if summary.Severity != nil {
		t.Fatalf("absent severity must stay nil, got %v", *summary.Severity)
	}
	if len(summary.Actors) != 0 {
		t.Fatalf("absent actors must stay empty: %+v", summary.Actors)
	}
}
