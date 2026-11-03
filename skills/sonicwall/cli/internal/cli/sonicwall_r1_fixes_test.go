// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sonicwall-pp-cli/internal/cliutil/testenv"
)

// Codex review 2026-11-02 round 1, one mock per class.
func newR1Mock(t *testing.T, routes map[string]string, sonicos map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range routes {
		body := body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		})
	}
	mux.HandleFunc("/firewall/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, ok := sonicos[r.URL.Path]
		if !ok {
			body = `{"status":{"info":[{"code":"E_FAIL","message":"no config"}],"success":false}}`
		}
		_, _ = io.WriteString(w, body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// An Internet-facing zone is found by its security type, not its name; a zone
// whose type cannot be read is unknown, never "not reachable".
func TestExposureGradesZonesBySecurityType(t *testing.T) {
	testenv.Isolate(t)
	routes := map[string]string{
		"/devices/tenant":     `[{"serialNumber":"` + serialA + `","friendlyName":"HQ","firmwareVersion":"7.0.1-5145","liveStatus":true,"tenantId":"t1"}]`,
		"/devices/vulnerable": `[{"serialNumber":"` + serialA + `","friendlyName":"HQ","vulnList":[{"advisoryID":"SNWLID-2024-0015","title":"x","haveWorkaroud":true}]}]`,
	}
	access := `{"ssl_vpn":{"server":{"access":[{"zone":"Internet","enable":true}]}}}`
	for _, tc := range []struct {
		name, zones, want string
	}{
		{"untrusted zone named Internet", `{"zones":[{"name":"Internet","security_type":"untrusted"}]}`, "yes"},
		{"trusted zone", `{"zones":[{"name":"Internet","security_type":"trusted"}]}`, "no"},
		{"zone type not reported", `{"zones":[{"name":"Internet"}]}`, "unknown"},
		{"zones unreadable", "", "unknown"},
	} {
		sonicos := map[string]string{"/firewall/ssl-vpn/server/accesses": access}
		if tc.zones != "" {
			sonicos["/firewall/zones"] = tc.zones
		}
		srv := newR1Mock(t, routes, sonicos)
		out, _ := runCLI(t, srv, "exposure", "SNWLID-2024-0015")
		var hits []exposureRow
		decodeResults(t, out, &hits)
		if len(hits) != 1 || hits[0].WANReachable != tc.want {
			t.Errorf("%s: hits = %+v, want sslvpn_wan_reachable %s", tc.name, hits, tc.want)
		}
	}
}

// An empty MySonicWall registry compares nothing and must not read as an all-clear.
func TestBlindSpotsEmptyRegistryIsNotAllClear(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SONICWALL_MSW_API_KEY", "test-key")
	srv := newR1Mock(t, map[string]string{
		"/api/users/productlist-customer": `{"operationStatus":"Success","content":{"totalProducts":"0","filterResultCount":"0","prodList":[]}}`,
		"/devices/tenant":                 `[]`,
	}, nil)
	out, err := runCLI(t, srv, "blind-spots")
	if err == nil || ExitCode(err) != 3 || !strings.Contains(out+err.Error(), "nothing was compared") {
		t.Fatalf("empty registry: want exit 3 'nothing was compared', got err=%v out=%s", err, out)
	}
}

// A firewall that does not report its tenant cannot be filtered out by --tenant.
func TestTenantFilterRefusesUntenantedFirewalls(t *testing.T) {
	testenv.Isolate(t)
	srv := newR1Mock(t, map[string]string{
		"/devices/tenant": `[{"serialNumber":"` + serialA + `","firmwareVersion":"7.1.1-7058","tenantId":"t1"},{"serialNumber":"` + serialB + `","firmwareVersion":"7.1.1-7058"}]`,
	}, nil)
	for _, args := range [][]string{{"posture", "--tenant", "t1"}, {"firmware", "drift", "--tenant", "t1"}, {"mfa-audit", "--tenant", "t1"}} {
		if _, err := runCLI(t, srv, args...); err == nil || !strings.Contains(err.Error(), "report no tenantId") {
			t.Errorf("%v: want the untenanted firewall to block --tenant, got %v", args, err)
		}
	}
}
