package store

import "testing"

// Regression tests for issue #137: the four queries-custom-fields* report
// variants were deferred from the #136 fix because their row shape was not
// confirmable from the connector source. The source OpenAPI schema (spec.yaml)
// confirms it:
//
//   - custom-fields / custom-fields-detailed (NodeAttributes /
//     NodeAttributesDetailed): one row per device, all values packed into a
//     `fields` map/array -> key on deviceId alone.
//   - scoped-custom-fields / scoped-custom-fields-detailed (ScopedAttributes /
//     ScopedAttributesDetailed): one row per (scope, entityId). entityId is
//     unique only WITHIN a scope, so the key is entityId + a `scope`
//     discriminator.
//
// Like the #136 group these rows carry no top-level id/name, so before this fix
// ExtractResourceID returned "" and zero rows were stored. These tests drive the
// REAL production decode path (DecodeJSONObject) so numeric formatting and the
// composite-key join are exercised exactly as sync does.

// Non-scoped custom-fields rows resolve to their deviceId and store one row per
// device (the `fields` map/array means a single row carries all of a device's
// fields, so deviceId alone is the unique key - no discriminator).
func TestCustomFieldsKeyOnDeviceID(t *testing.T) {
	cases := map[string]string{
		// NodeAttributes: deviceId + fields map
		"queries-custom-fields": `{"deviceId":1001,"fields":{"AssetTag":"A-1","Owner":"jdoe"}}`,
		// NodeAttributesDetailed: deviceId + entityType + fields array
		"queries-custom-fields-detailed": `{"deviceId":1001,"entityType":"NODE","fields":[{"name":"AssetTag","value":"A-1"}]}`,
	}
	for resource, raw := range cases {
		if id := ExtractResourceID(resource, decodeOrFatal(t, raw)); id != "1001" {
			t.Errorf("%s: want primary id 1001, got %q", resource, id)
		}
		if got := storageKey(t, resource, raw); got != "1001" {
			t.Errorf("%s: want bare deviceId key 1001 (one row per device), got %q", resource, got)
		}
	}
}

// Two devices' custom-fields rows must not collapse onto one key.
func TestCustomFieldsNoCrossDeviceCollapse(t *testing.T) {
	for _, resource := range []string{"queries-custom-fields", "queries-custom-fields-detailed"} {
		k1 := storageKey(t, resource, `{"deviceId":1001,"fields":{"Owner":"a"}}`)
		k2 := storageKey(t, resource, `{"deviceId":2002,"fields":{"Owner":"b"}}`)
		if k1 == k2 {
			t.Errorf("%s: device 1001 and 2002 collapsed to one key %q", resource, k1)
		}
	}
}

// Scoped custom-fields rows key on entityId. The `scope` discriminator is what
// makes the key correct: entityId is unique only WITHIN a scope, so the SAME
// entityId under two scopes (e.g. a device #5 and an organization #5) must NOT
// collapse onto one stored row. This is the core #137 correctness property.
func TestScopedCustomFieldsKeyDisambiguatesScope(t *testing.T) {
	for _, resource := range []string{"queries-scoped-custom-fields", "queries-scoped-custom-fields-detailed"} {
		// entityId resolves as the primary id.
		if id := ExtractResourceID(resource, decodeOrFatal(t, `{"scope":"NODE","entityId":5,"fields":{}}`)); id != "5" {
			t.Errorf("%s: want primary id 5 (entityId), got %q", resource, id)
		}
		// Same entityId, different scope -> distinct storage keys.
		node := storageKey(t, resource, `{"scope":"NODE","entityId":5,"fields":{"k":"v"}}`)
		org := storageKey(t, resource, `{"scope":"ORGANIZATION","entityId":5,"fields":{"k":"v"}}`)
		if node == org {
			t.Errorf("%s: entityId 5 under NODE and ORGANIZATION collapsed to one key %q (cross-scope data loss)", resource, node)
		}
		// Different entities in the same scope are also distinct.
		a := storageKey(t, resource, `{"scope":"NODE","entityId":5,"fields":{}}`)
		b := storageKey(t, resource, `{"scope":"NODE","entityId":6,"fields":{}}`)
		if a == b {
			t.Errorf("%s: entityId 5 and 6 in NODE scope collided on key %q", resource, a)
		}
	}
}

// Safe-by-construction guard: the spec marks NodeAttributesDetailed.deviceId
// writeOnly (request-only), so the live response MAY omit it. When it does,
// ExtractResourceID falls through to "" and the row is skipped - exactly the
// zero-rows behavior the resource already has today. The override can therefore
// only add rows or stay neutral; it can never collapse or corrupt existing data.
func TestCustomFieldsDetailedWriteOnlyDeviceIDFallsThroughCleanly(t *testing.T) {
	// deviceId absent (writeOnly omitted from the response), no other id/name.
	raw := `{"entityType":"NODE","fields":[{"name":"AssetTag","value":"A-1"}]}`
	if id := ExtractResourceID("queries-custom-fields-detailed", decodeOrFatal(t, raw)); id != "" {
		t.Errorf("expected clean fallthrough to \"\" when writeOnly deviceId is absent, got %q", id)
	}
}
