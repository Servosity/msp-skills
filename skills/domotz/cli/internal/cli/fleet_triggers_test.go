// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Tests for the fleet triggers (down-sensor sweep) command: TCP eye decoding
// across the API's id shapes (numbers and strings), DOWN filtering, and the
// live-only data-source contract.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestTCPEyeItemDecodeAndFilter(t *testing.T) {
	payload := `[
		{"id":1,"device_id":42,"port":443,"status":"DOWN","last_update":"2026-06-06T10:00:00Z"},
		{"id":"2","device_id":"43","port":80,"status":"UP","last_update":"2026-06-06T10:00:00Z"},
		{"id":3,"device_id":44,"port":22,"status":"DOWN","last_update":"2026-06-06T09:00:00Z"}
	]`
	var eyes []tcpEyeItem
	if err := json.Unmarshal([]byte(payload), &eyes); err != nil {
		t.Fatalf("decode TCP eyes: %v", err)
	}
	if len(eyes) != 3 {
		t.Fatalf("decoded %d eyes, want 3", len(eyes))
	}

	down := make([]fleetTriggerRow, 0)
	for _, eye := range eyes {
		if eye.Status != "DOWN" {
			continue
		}
		down = append(down, fleetTriggerRow{
			DeviceID:   trimJSONString(eye.DeviceID),
			EyeID:      trimJSONString(eye.ID),
			Port:       eye.Port,
			Status:     eye.Status,
			LastUpdate: eye.LastUpdate,
		})
	}
	if len(down) != 2 {
		t.Fatalf("filtered %d DOWN sensors, want 2\nrows: %+v", len(down), down)
	}
	for _, r := range down {
		if r.Status != "DOWN" {
			t.Fatalf("non-DOWN row survived the filter: %+v", r)
		}
		if r.Port == 80 {
			t.Fatal("UP sensor on port 80 must not appear in the down-sensor sweep")
		}
	}
	// Numeric JSON ids normalize to bare strings.
	if down[0].DeviceID != "42" || down[0].EyeID != "1" {
		t.Fatalf("numeric ids normalized wrong: device=%q eye=%q, want 42/1", down[0].DeviceID, down[0].EyeID)
	}
}

func TestFleetTriggersRejectsLocalDataSource(t *testing.T) {
	flags := &rootFlags{asJSON: true, dataSource: "local"}
	cmd := newNovelFleetTriggersCmd(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("fleet triggers with --data-source local must error (live-only command)")
	}
}
