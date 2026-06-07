// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the defender riskiest novel command.

package cli

import "testing"

func TestRankRiskyDevices(t *testing.T) {
	items := rawItems(
		`{"hostname": "LOW", "deviceId": "d1", "detections": {"malicious": 0, "suspicious": 5, "informational": 100}}`,
		`{"hostname": "HIGH", "deviceId": "d2", "detections": {"malicious": 3, "suspicious": 1, "informational": 0}}`,
		`{"hostname": "MID", "deviceId": "d3", "detections": {"malicious": 1, "suspicious": 2, "informational": 7}}`,
	)
	devices := rankRiskyDevices(items, 10)
	if len(devices) != 3 {
		t.Fatalf("devices = %d, want 3", len(devices))
	}
	if devices[0].Hostname != "HIGH" || devices[0].RiskScore != 31 {
		t.Errorf("top device = %s score %d, want HIGH score 31 (3x10+1)", devices[0].Hostname, devices[0].RiskScore)
	}
	if devices[1].Hostname != "MID" || devices[1].RiskScore != 12 {
		t.Errorf("second device = %s score %d, want MID score 12", devices[1].Hostname, devices[1].RiskScore)
	}
	if devices[2].RiskScore != 5 {
		t.Errorf("informational must not affect risk score: got %d, want 5", devices[2].RiskScore)
	}
}

func TestRankRiskyDevicesTopCap(t *testing.T) {
	items := rawItems(
		`{"hostname": "A", "detections": {"malicious": 1, "suspicious": 0}}`,
		`{"hostname": "B", "detections": {"malicious": 2, "suspicious": 0}}`,
		`{"hostname": "C", "detections": {"malicious": 3, "suspicious": 0}}`,
	)
	devices := rankRiskyDevices(items, 2)
	if len(devices) != 2 {
		t.Fatalf("devices = %d, want 2 (top cap)", len(devices))
	}
	if devices[0].Hostname != "C" {
		t.Errorf("top device = %s, want C", devices[0].Hostname)
	}
}

func TestRankRiskyDevicesStringEncodedCounts(t *testing.T) {
	// Defensive: feeds with string-encoded numbers must not silently zero out.
	items := rawItems(`{"hostname": "S", "detections": {"malicious": "2", "suspicious": "180", "informational": "0"}}`)
	devices := rankRiskyDevices(items, 10)
	if len(devices) != 1 || devices[0].RiskScore != 200 {
		t.Errorf("string-encoded detections not extracted: %+v", devices)
	}
}
