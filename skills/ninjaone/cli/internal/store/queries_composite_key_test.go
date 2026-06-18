package store

import "testing"

// Regression tests for issue #136: the /v2/queries/* report endpoints return
// objects with no top-level id, so ExtractResourceID returned "" and every row
// was skipped (all_items_failed_id_extraction -> zero rows stored). The fix
// keys these device-scoped resources on deviceId, plus a secondary
// discriminator for the resources that return many rows per device. These tests
// drive the REAL production decode path (DecodeJSONObject, UseNumber) so the
// numeric-id formatting and composite-key joins are exercised exactly as sync
// does, not via hand-built maps.

func decodeOrFatal(t *testing.T, raw string) map[string]any {
	t.Helper()
	obj, err := DecodeJSONObject([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return obj
}

func storageKey(t *testing.T, resource, raw string) string {
	t.Helper()
	obj := decodeOrFatal(t, raw)
	id := ExtractResourceID(resource, obj)
	if id == "" {
		t.Fatalf("%s: ExtractResourceID returned \"\" (bug #136 not fixed)", resource)
	}
	return resourceStorageID(resource, id, obj)
}

// Every affected queries-* resource must now yield a non-empty primary id.
func TestQueriesPrimaryIDResolves(t *testing.T) {
	cases := map[string]string{
		"queries-antivirus-status":   `{"productName":"SentinelOne","productState":"ON","deviceId":1001}`,
		"queries-device-health":      `{"healthStatus":"UNHEALTHY","criticalVulnerabilityCount":6,"deviceId":1001}`,
		"queries-network-interfaces": `{"deviceId":1001,"interfaceIndex":"4","interfaceName":"NDIS"}`,
		"queries-logged-on-users":    `{"deviceId":1001,"userName":"acme\\jdoe","logonTime":1700000000}`,
		"queries-policy-overrides":   `{"deviceId":1001,"overrides":{"foo":"bar"}}`,
		"queries-raid-controllers":   `{"deviceId":1001,"controllerIndex":0,"model":"PERC"}`,
		"queries-raid-drives":        `{"deviceId":1001,"driveId":"naa.500","controllerIndex":0}`,
	}
	for resource, raw := range cases {
		if id := ExtractResourceID(resource, decodeOrFatal(t, raw)); id != "1001" {
			t.Errorf("%s: want primary id 1001, got %q", resource, id)
		}
	}
}

// Multi-row-per-device resources must produce DISTINCT storage keys so rows do
// not collapse (clobber) onto a single deviceId.
func TestQueriesCompositeKeysDoNotCollide(t *testing.T) {
	type pair struct{ resource, a, b string }
	pairs := []pair{
		{"queries-antivirus-status",
			`{"deviceId":1001,"productName":"SentinelOne"}`,
			`{"deviceId":1001,"productName":"Microsoft Defender Antivirus"}`},
		{"queries-network-interfaces",
			`{"deviceId":1001,"interfaceIndex":"4"}`,
			`{"deviceId":1001,"interfaceIndex":"7"}`},
		{"queries-logged-on-users",
			`{"deviceId":1001,"userName":"acme\\jdoe"}`,
			`{"deviceId":1001,"userName":"acme\\asmith"}`},
		{"queries-raid-controllers",
			`{"deviceId":1001,"controllerIndex":0}`,
			`{"deviceId":1001,"controllerIndex":1}`},
		{"queries-raid-drives",
			`{"deviceId":1001,"driveId":"naa.500"}`,
			`{"deviceId":1001,"driveId":"naa.501"}`},
	}
	for _, p := range pairs {
		ka := storageKey(t, p.resource, p.a)
		kb := storageKey(t, p.resource, p.b)
		if ka == kb {
			t.Errorf("%s: two rows on one device collided on key %q (rows would clobber)", p.resource, ka)
		}
		// The deviceId must be the leading key segment.
		if want := "1001"; len(ka) < len(want) || ka[:len(want)] != want {
			t.Errorf("%s: composite key %q does not lead with deviceId %q", p.resource, ka, want)
		}
	}
}

// Single-row-per-device resources key on deviceId alone (no discriminator).
func TestQueriesSingleRowResourcesKeyOnDeviceID(t *testing.T) {
	for _, resource := range []string{
		"queries-device-health", "queries-policy-overrides",
		"queries-operating-systems", "queries-computer-systems",
	} {
		raw := `{"deviceId":1001,"name":"Windows 11","healthStatus":"OK","overrides":{}}`
		if got := storageKey(t, resource, raw); got != "1001" {
			t.Errorf("%s: want bare deviceId key 1001, got %q", resource, got)
		}
	}
}

// The core #136-adversarial-review regression: report rows that share an id or
// a name ACROSS devices must NOT collapse to one stored row. Before the fix,
// queries-os-patches keyed on the patch `id` (same KB on every device -> one
// row) and queries-operating-systems keyed on `name` ("Windows 11" everywhere
// -> one row). deviceId must now lead every key so two devices never collide.
func TestQueriesNoCrossDeviceCollapse(t *testing.T) {
	type c struct{ resource, dev1, dev2 string }
	cases := []c{
		// same patch id on two devices
		{"queries-os-patches", `{"deviceId":1001,"id":"KB5034123"}`, `{"deviceId":2002,"id":"KB5034123"}`},
		{"queries-software-patches", `{"deviceId":1001,"id":"PCH-9"}`, `{"deviceId":2002,"id":"PCH-9"}`},
		// same OS / CPU / volume / service name on two devices
		{"queries-operating-systems", `{"deviceId":1001,"name":"Windows 11"}`, `{"deviceId":2002,"name":"Windows 11"}`},
		{"queries-processors", `{"deviceId":1001,"name":"Intel i7"}`, `{"deviceId":2002,"name":"Intel i7"}`},
		{"queries-volumes", `{"deviceId":1001,"name":"C:"}`, `{"deviceId":2002,"name":"C:"}`},
		{"queries-windows-services", `{"deviceId":1001,"name":"Spooler"}`, `{"deviceId":2002,"name":"Spooler"}`},
		{"queries-software", `{"deviceId":1001,"name":"Chrome"}`, `{"deviceId":2002,"name":"Chrome"}`},
		{"queries-antivirus-threats", `{"deviceId":1001,"threatId":7}`, `{"deviceId":2002,"threatId":7}`},
		{"queries-computer-systems", `{"deviceId":1001,"name":"HOST"}`, `{"deviceId":2002,"name":"HOST"}`},
	}
	for _, tc := range cases {
		k1 := storageKey(t, tc.resource, tc.dev1)
		k2 := storageKey(t, tc.resource, tc.dev2)
		if k1 == k2 {
			t.Errorf("%s: rows on devices 1001 and 2002 collapsed to one key %q (cross-device data loss)", tc.resource, k1)
		}
		if want := "1001"; len(k1) < len(want) || k1[:len(want)] != want {
			t.Errorf("%s: key %q does not lead with deviceId 1001", tc.resource, k1)
		}
		if want := "2002"; len(k2) < len(want) || k2[:len(want)] != want {
			t.Errorf("%s: key %q does not lead with deviceId 2002", tc.resource, k2)
		}
	}
}

// Large integer deviceIds (JSON numbers) must format as plain integers, never
// scientific notation or a trailing .0 - they are the storage key.
func TestQueriesNumericDeviceIDFormatting(t *testing.T) {
	raw := `{"deviceId":123456789,"productName":"SentinelOne"}`
	key := storageKey(t, "queries-antivirus-status", raw)
	const wantPrefix = "123456789"
	if len(key) < len(wantPrefix) || key[:len(wantPrefix)] != wantPrefix {
		t.Errorf("numeric deviceId mis-formatted: key=%q want prefix %q", key, wantPrefix)
	}
}
