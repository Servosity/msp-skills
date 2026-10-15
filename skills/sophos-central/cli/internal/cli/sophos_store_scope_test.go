package cli

import (
	"strings"
	"testing"
)

// The documented first-run flow: refresh, then read, on the default store.
func TestDefaultStoreStableFromFirstRun(t *testing.T) {
	drop := false
	srv := fleetFake(t, &drop, false)
	fleetEnv(t, srv)
	if _, err := runRoot(t, "fleet", "refresh", "--resources", "tenants,endpoints,alerts", "--json"); err != nil {
		t.Fatal(err)
	}
	out, err := runRoot(t, "fleet", "triage", "--severity", "high,medium,low", "--json")
	if err != nil || !containsAll(out, `"synced_at": "`) {
		t.Fatalf("triage after the first refresh must read the same store: %v\n%s", err, out)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, x := range subs {
		if !strings.Contains(s, x) {
			return false
		}
	}
	return true
}

// A health-only refresh must not make triage look fresh or the tenants look
// fully read: endpoints and alerts were never fetched.
func TestPartialRefreshNotFreshForTriage(t *testing.T) {
	drop := false
	srv := fleetFake(t, &drop, false)
	fleetEnv(t, srv)
	if _, err := runRoot(t, "fleet", "refresh", "--resources", "tenants,health", "--json"); err != nil {
		t.Fatal(err)
	}
	out, err := runRoot(t, "fleet", "triage", "--json")
	if err != nil || !containsAll(out, `"synced_at": null`, `not fetched endpoints, alerts`, `"status": "not_refreshed"`) {
		t.Fatalf("triage after a health-only refresh must say endpoints and alerts were not fetched: %v\n%s", err, out)
	}
	out, err = runRoot(t, "fleet", "health", "--json")
	if err != nil || !containsAll(out, `"synced_at": "`) {
		t.Fatalf("health after a health refresh must be fresh: %v\n%s", err, out)
	}
}
