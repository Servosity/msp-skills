package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"proxmox-pp-cli/internal/client"
)

// A PBS-only setup whose datastore-usage read fails keeps that failure as the
// cause: the absent PVE host is not the reason nothing could be measured.
func TestPBSOnlyCapacityKeepsPBSFailureCause(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_BASE_URL", "")
	t.Setenv("PROXMOX_HOST", "")
	broken := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status/datastore-usage") {
			http.Error(w, `{"data":null}`, http.StatusForbidden)
			return
		}
		f.pbs.Config.Handler.ServeHTTP(w, r)
	}))
	defer broken.Close()
	t.Setenv("PROXMOX_PBS_HOST", broken.URL)
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", client.CertFingerprint(broken.Certificate()))
	out, _, err := runCLI(t, "capacity", "--json")
	if out == "" {
		t.Fatalf("capacity printed nothing: %v", err)
	}
	fails := failureByScope(decodeJSON[capacityReport](t, out).FetchFailures)
	if fails["pbs datastore-usage"] == "" {
		t.Errorf("the PBS datastore-usage failure was dropped: %v", fails)
	}
	for scope, msg := range fails {
		if strings.Contains(msg, "no Proxmox VE host configured") {
			t.Errorf("%s: the absent PVE host reported as the cause: %q", scope, msg)
		}
	}
}

// --only that matches no guest says so; "no guests found" is reserved for an
// empty inventory.
func TestPostureOnlyFilteredToNothingIsNotNoGuests(t *testing.T) {
	f := newFixture(t)
	guests, _ := f.pveRoutes["/cluster/resources"].([]map[string]any)
	var fresh []map[string]any
	for _, g := range guests {
		if g["vmid"] == 100 { // the fixture's one fresh, verified guest
			fresh = append(fresh, g)
		}
	}
	f.pveRoutes["/cluster/resources"] = fresh
	out, _, err := runCLI(t, "posture", "--only", "problems", "--human-friendly")
	if err != nil {
		t.Fatalf("posture: %v\n%s", err, out)
	}
	if strings.Contains(out, "No guests found") {
		t.Errorf("a filter that matched nothing was reported as an empty inventory:\n%s", out)
	}
	if !strings.Contains(out, "none matched --only problems") || !strings.Contains(out, "1 guest(s)") {
		t.Errorf("want the seen-but-filtered count, got:\n%s", out)
	}
	out, _, err = runCLI(t, "posture", "--only", "green", "--human-friendly")
	if err != nil || !strings.Contains(out, "fresh") {
		t.Errorf("--only green must still list the fresh guest: %v\n%s", err, out)
	}
}
