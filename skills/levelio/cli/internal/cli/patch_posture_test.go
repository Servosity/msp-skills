// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestLvlComputePatchPosture(t *testing.T) {
	updates := lvlFxUpdates()

	t.Run("all categories", func(t *testing.T) {
		res := lvlComputePatchPosture(updates, "", false)
		s := res.Summary
		if s.TotalUpdates != 4 || s.Pending != 2 || s.Installed != 1 || s.Errored != 1 {
			t.Fatalf("summary = %+v, want total4 pending2 installed1 errored1", s)
		}
		if s.DevicesWithPending != 1 { // both pending updates are on dev1
			t.Errorf("devices_with_pending = %d, want 1", s.DevicesWithPending)
		}
		if res.ByCategory[0].Category != "Security Updates" || res.ByCategory[0].Pending != 2 {
			t.Errorf("top category = %+v, want Security Updates pending2", res.ByCategory[0])
		}
	})

	t.Run("category filter", func(t *testing.T) {
		res := lvlComputePatchPosture(updates, "security", false)
		if res.Summary.TotalUpdates != 3 || res.Summary.Pending != 2 || res.Summary.Errored != 1 {
			t.Errorf("filtered summary = %+v, want total3 pending2 errored1", res.Summary)
		}
	})
}

func TestPatchState(t *testing.T) {
	cases := []struct {
		u    lvlUpdate
		want string
	}{
		{lvlUpdate{IsAvailable: true}, "pending"},
		{lvlUpdate{InstalledOn: "2024-01-01T00:00:00Z"}, "installed"},
		{lvlUpdate{Error: "boom", IsAvailable: true}, "errored"},
		{lvlUpdate{}, "other"},
	}
	for _, c := range cases {
		if got := patchState(c.u); got != c.want {
			t.Errorf("patchState(%+v) = %q, want %q", c.u, got, c.want)
		}
	}
}
