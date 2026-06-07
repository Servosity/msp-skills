// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestLvlComputeSince(t *testing.T) {
	alerts := lvlFxAlerts()
	updates := lvlFxUpdates()
	devices := lvlFxDevices()
	now := lvlFxNow()

	t.Run("24h window", func(t *testing.T) {
		res := lvlComputeSince(alerts, updates, devices, 24*time.Hour, now)
		s := res.Summary
		if s.NewAlerts != 1 { // al1 (2h ago); al2 30h, al3 50h, al4 200h excluded
			t.Errorf("new alerts = %d, want 1", s.NewAlerts)
		}
		if s.ResolvedAlerts != 1 { // al3 resolved 1h ago
			t.Errorf("resolved = %d, want 1", s.ResolvedAlerts)
		}
		if s.PublishedUpdates != 2 { // u1 (5h), u4 (2h); u2 100h, u3 200h excluded
			t.Errorf("published = %d, want 2", s.PublishedUpdates)
		}
		if s.InstalledUpdates != 1 { // u3 installed 3h ago
			t.Errorf("installed = %d, want 1", s.InstalledUpdates)
		}
		if s.ActiveDevices != 2 { // alpha (1h), delta (2h)
			t.Errorf("active devices = %d, want 2", s.ActiveDevices)
		}
	})

	t.Run("wide window catches more", func(t *testing.T) {
		res := lvlComputeSince(alerts, updates, devices, 14*24*time.Hour, now)
		if res.Summary.NewAlerts != 4 { // all alerts within 14 days (max 200h ~ 8.3d)
			t.Errorf("14d new alerts = %d, want 4", res.Summary.NewAlerts)
		}
	})
}
