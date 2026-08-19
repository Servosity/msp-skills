package cli

import "testing"

// extractID and store.ExtractResourceID are separate implementations reading
// separate override maps. Divergence between them is itself a silent-drop bug:
// sync would count an alert as consumed while the store dropped it. #258 fixed
// both maps; this test fails if a future edit touches only one.
func TestExtractID_AlertUidOverride(t *testing.T) {
	obj := map[string]any{
		"alertUid": "16c466e9-9dfd-4ad1-a9cf-c02ea18eebcd",
		"priority": "High",
		"resolved": false,
	}
	const want = "16c466e9-9dfd-4ad1-a9cf-c02ea18eebcd"
	for _, resource := range []string{"account-alerts-open", "account-alerts-resolved"} {
		if got := extractID(resource, obj); got != want {
			t.Errorf("extractID(%q) = %q, want %q", resource, got, want)
		}
	}
}

// Both alert resources are in the default sync set, so the override map keys
// must match the resource names sync actually dispatches — a typo here would
// silently restore the 0-stored behaviour.
func TestAlertOverridesMatchSyncResourceNames(t *testing.T) {
	known := map[string]bool{}
	for _, name := range knownSyncResourceNames() {
		known[name] = true
	}
	for resource, field := range resourceIDFieldOverrides {
		if !known[resource] {
			t.Errorf("override for %q is not a known sync resource", resource)
		}
		if field == "" {
			t.Errorf("override for %q is empty", resource)
		}
	}
	for _, resource := range []string{"account-alerts-open", "account-alerts-resolved"} {
		if resourceIDFieldOverrides[resource] != "alert_uid" {
			t.Errorf("missing alert_uid override for %q", resource)
		}
	}
}

func TestExtractID_NonAlertResourcesUnchanged(t *testing.T) {
	if got := extractID("account-devices", map[string]any{"uid": "dev", "id": "7"}); got != "7" {
		t.Errorf("account-devices = %q, want 7", got)
	}
	if got := extractID("account-users", map[string]any{"alertUid": "leaked"}); got != "" {
		t.Errorf("alertUid must not leak into the generic list, got %q", got)
	}
}
