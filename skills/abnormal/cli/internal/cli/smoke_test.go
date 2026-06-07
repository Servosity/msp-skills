// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

func TestSmokeEndpointsStable(t *testing.T) {
	eps := smokeEndpoints()
	if len(eps) < 3 {
		t.Fatalf("expected at least 3 smoke endpoints, got %d", len(eps))
	}
	seen := map[string]bool{}
	for _, ep := range eps {
		if ep == "" || ep[0] != '/' {
			t.Errorf("endpoint must be a rooted path: %q", ep)
		}
		if seen[ep] {
			t.Errorf("duplicate smoke endpoint %q", ep)
		}
		seen[ep] = true
	}
}

func TestSmokeViewMarshal(t *testing.T) {
	view := smokeView{
		BaseURL: "https://api.abnormalplatform.com/v1",
		Probes: []smokeProbe{
			{Endpoint: "/threats", OK: true, ElapsedMS: 120},
			{Endpoint: "/cases", OK: false, Error: "401 unauthorized", ElapsedMS: 80},
		},
		Passed: 1,
		Failed: 1,
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back smokeView
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	if back.Passed != 1 || back.Failed != 1 || len(back.Probes) != 2 {
		t.Errorf("roundtrip lost data: %+v", back)
	}
	if back.Probes[0].Error != "" {
		t.Errorf("ok probe should omit error, got %q", back.Probes[0].Error)
	}
}
