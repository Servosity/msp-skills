// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Shared deterministic fixtures for the hand-built fleet-analytics command tests.

package cli

import "time"

func lvlIntp(v int) *int { return &v }

// lvlFxNow is the fixed reference time all analytics tests evaluate against.
func lvlFxNow() time.Time { return time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC) }

func lvlFxAgo(h float64) string {
	return lvlFxNow().Add(-time.Duration(h * float64(time.Hour))).Format(time.RFC3339)
}

func lvlFxDevices() []lvlDevice {
	return []lvlDevice{
		{ID: "dev1", Hostname: "alpha", GroupID: "g1", Online: true, SecurityScore: lvlIntp(40),
			LastSeenAt: lvlFxAgo(1), Platform: "windows", Tags: []string{"prod"},
			OperatingSystem: &lvlOS{FullOperatingSystem: "Windows 10"}},
		{ID: "dev2", Hostname: "bravo", GroupID: "g2", Online: false, SecurityScore: lvlIntp(90),
			LastSeenAt: lvlFxAgo(240), Platform: "darwin", Tags: []string{"prod", "db"},
			OperatingSystem: &lvlOS{FullOperatingSystem: "macOS 14"}},
		{ID: "dev3", Hostname: "charlie", GroupID: "g1", Online: false, MaintenanceMode: true,
			LastSeenAt: "", Platform: "linux"},
		{ID: "dev4", Hostname: "delta", GroupID: "", Online: true, SecurityScore: lvlIntp(65),
			LastSeenAt: lvlFxAgo(2), Platform: "windows"},
	}
}

func lvlFxGroups() []lvlGroup {
	return []lvlGroup{
		{ID: "g1", Name: "Root", ParentID: "", ChildIDs: []string{"g2"}, DeviceCount: 2, DescendentDeviceCount: 3},
		{ID: "g2", Name: "Child", ParentID: "g1", DeviceCount: 1, DescendentDeviceCount: 1},
	}
}

func lvlFxAlerts() []lvlAlert {
	return []lvlAlert{
		{ID: "al1", DeviceID: "dev1", DeviceHostname: "alpha", Severity: "critical", IsResolved: false, StartedAt: lvlFxAgo(2), Name: "High CPU"},
		{ID: "al2", DeviceID: "dev1", DeviceHostname: "alpha", Severity: "warning", IsResolved: false, StartedAt: lvlFxAgo(30), Name: "Low disk"},
		{ID: "al3", DeviceID: "dev2", DeviceHostname: "bravo", Severity: "emergency", IsResolved: true, StartedAt: lvlFxAgo(50), ResolvedAt: lvlFxAgo(1), Name: "Down"},
		{ID: "al4", DeviceID: "dev4", DeviceHostname: "delta", Severity: "information", IsResolved: false, StartedAt: lvlFxAgo(200), Name: "Info"},
	}
}

func lvlFxUpdates() []lvlUpdate {
	return []lvlUpdate{
		{ID: "u1", DeviceID: "dev1", Category: "Security Updates", IsAvailable: true, PublishedOn: lvlFxAgo(5)},
		{ID: "u2", DeviceID: "dev1", Category: "Security Updates", IsAvailable: true, PublishedOn: lvlFxAgo(100)},
		{ID: "u3", DeviceID: "dev2", Category: "Feature", IsAvailable: false, InstalledOn: lvlFxAgo(3), PublishedOn: lvlFxAgo(200)},
		{ID: "u4", DeviceID: "dev3", Category: "Security Updates", IsAvailable: true, Error: "Install failed", PublishedOn: lvlFxAgo(2)},
	}
}

func lvlFxCustomFields() []lvlCustomField {
	return []lvlCustomField{
		{ID: "cf1", Name: "Asset Tag", Reference: "asset_tag"},
		{ID: "cf2", Name: "Warranty", Reference: "warranty"},
	}
}

func lvlFxCustomFieldValues() []lvlCustomFieldValue {
	return []lvlCustomFieldValue{
		{CustomFieldID: "cf1", AssignedToID: "dev1", Value: "A-100"},
		{CustomFieldID: "cf1", AssignedToID: "dev2", Value: "A-200"},
		{CustomFieldID: "cf2", AssignedToID: "g1", Value: "2026"},
		{CustomFieldID: "cf1", AssignedToID: "dev3", Value: ""},
	}
}
