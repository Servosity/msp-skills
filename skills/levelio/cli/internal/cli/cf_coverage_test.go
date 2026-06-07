// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputeCfCoverage(t *testing.T) {
	fields := lvlFxCustomFields()
	values := lvlFxCustomFieldValues()
	devices := lvlFxDevices()

	t.Run("coverage and ordering", func(t *testing.T) {
		res := lvlComputeCfCoverage(fields, values, devices, "", false, false)
		if res.TotalDevices != 4 || len(res.Fields) != 2 {
			t.Fatalf("totals = %d devices, %d fields", res.TotalDevices, len(res.Fields))
		}
		// Warranty has 0 device coverage (only a group value) -> sorts first.
		if res.Fields[0].FieldName != "Warranty" || res.Fields[0].CoveragePct != 0 || res.Fields[0].OtherAssignees != 1 {
			t.Errorf("worst field = %+v, want Warranty 0%% other1", res.Fields[0])
		}
		// Asset Tag: dev1+dev2 valued (dev3 empty), 2/4 = 50%.
		var asset *cfCoverageField
		for i := range res.Fields {
			if res.Fields[i].FieldName == "Asset Tag" {
				asset = &res.Fields[i]
			}
		}
		if asset == nil || asset.DevicesWithValue != 2 || asset.CoveragePct != 50 {
			t.Errorf("Asset Tag = %+v, want withValue2 coverage50", asset)
		}
	})

	t.Run("missing list", func(t *testing.T) {
		res := lvlComputeCfCoverage(fields, values, devices, "asset", true, false)
		if len(res.Fields) != 1 {
			t.Fatalf("field filter returned %d fields, want 1", len(res.Fields))
		}
		// dev3 (empty value) and dev4 (no value) are missing -> charlie, delta.
		want := map[string]bool{"charlie": true, "delta": true}
		if len(res.Fields[0].Missing) != 2 {
			t.Fatalf("missing = %v, want 2", res.Fields[0].Missing)
		}
		for _, m := range res.Fields[0].Missing {
			if !want[m] {
				t.Errorf("unexpected missing host %q", m)
			}
		}
	})
}
