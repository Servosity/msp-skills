// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestClassifyJobVerdict(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"Success", "pass"},
		{"Warning", "warning"},
		{"Failure", "fail"},
		{"Expired", "fail"},
		{"Pending", "running"},
		{"Running", "running"},
		{"", "no-result"},
		{"SomethingNew", "fail"},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			if got := classifyJobVerdict(tc.status); got != tc.want {
				t.Fatalf("classifyJobVerdict(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

func TestBuildJobRollup(t *testing.T) {
	results := []jobDeviceResult{
		{DeviceUID: "d1", Hostname: "alpha", Verdict: "pass", Status: "Success"},
		{DeviceUID: "d2", Hostname: "bravo", Verdict: "fail", Status: "Failure"},
		{DeviceUID: "d3", Hostname: "charlie", Verdict: "warning", Status: "Warning"},
		{DeviceUID: "d4", Hostname: "delta", Verdict: "no-result", Status: ""},
	}
	failures := []jobResultsFailure{{DeviceUID: "d5", Error: "timeout"}}

	view := buildJobRollup("job-1", results, failures, 5, false)
	if view.Total != 4 || view.Passed != 1 || view.Failed != 1 || view.Warnings != 1 || view.NoResult != 1 {
		t.Fatalf("rollup counts wrong: %+v", view)
	}
	if view.ScannedDevices != 5 {
		t.Fatalf("scanned = %d, want 5", view.ScannedDevices)
	}
	if len(view.FetchFailures) != 1 {
		t.Fatalf("fetch_failures = %d, want 1", len(view.FetchFailures))
	}
	// Failures sort first, passes last.
	if view.Results[0].Verdict != "fail" || view.Results[len(view.Results)-1].Verdict != "pass" {
		t.Fatalf("sort order wrong: %+v", view.Results)
	}
}

func TestBuildJobRollupFailedOnly(t *testing.T) {
	results := []jobDeviceResult{
		{DeviceUID: "d1", Hostname: "alpha", Verdict: "pass"},
		{DeviceUID: "d2", Hostname: "bravo", Verdict: "fail"},
	}
	view := buildJobRollup("job-1", results, nil, 2, true)
	// Counts still cover everything; the listing drops passes.
	if view.Total != 2 || view.Passed != 1 || view.Failed != 1 {
		t.Fatalf("counts wrong: %+v", view)
	}
	if len(view.Results) != 1 || view.Results[0].Verdict != "fail" {
		t.Fatalf("failed-only listing wrong: %+v", view.Results)
	}
}
