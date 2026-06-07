// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestFleetAutomationHealth(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetAutomationHealthCmd, db)

	var res automationHealthResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if res.TotalInstances != 3 {
		t.Fatalf("want 3 instances, got %d", res.TotalInstances)
	}
	if res.Succeeded != 1 || res.Failed != 1 || res.Running != 1 {
		t.Fatalf("want 1 each succeeded/failed/running, got %+v", res)
	}
	// success_rate = succeeded / (succeeded+failed) = 1/2 = 50%
	if res.SuccessRate != 50 {
		t.Fatalf("success rate should be 50%%, got %v", res.SuccessRate)
	}
	if res.InstancesByStatus["Completed"] != 1 {
		t.Fatalf("by-status map should count Completed: %+v", res.InstancesByStatus)
	}
}

func TestFleetAutomationHealthOrgFilter(t *testing.T) {
	db := seedFleetStore(t)
	out := runFleet(t, newNovelFleetAutomationHealthCmd, db, "--org", "org-b")
	var res automationHealthResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.TotalInstances != 1 || res.Running != 1 {
		t.Fatalf("org-b has 1 running instance: %+v", res)
	}
}

func TestClassifyAutomationStatus(t *testing.T) {
	cases := map[string]string{
		"Completed": "succeeded", "Success": "succeeded", "Failed": "failed",
		"Error": "failed", "Running": "running", "In_Progress": "running",
		"Scheduled": "running", "Weird": "other",
	}
	for in, want := range cases {
		if got := classifyAutomationStatus(in); got != want {
			t.Errorf("classify(%q) = %q, want %q", in, got, want)
		}
	}
}
