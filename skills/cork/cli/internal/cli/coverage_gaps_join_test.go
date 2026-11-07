// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const gapsClient = "11111111-1111-1111-1111-111111111111"

// gapsStub serves one client with a tenant per integration (documented Client.associated_tenants[].integration),
// its attributed devices (documented Device.associated_endpoints) and each integration's devices (IntegrationDevice).
func gapsStub(t *testing.T, devices []any, integrationDevices map[string][]any) *httptest.Server {
	t.Helper()
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	ints := []any{
		map[string]any{"uuid": "int-rmm", "display_name": "RMM", "connection_status": "ok", "vendor": map[string]any{"key": "NINJA_RMM", "name": "NinjaOne"}},
		map[string]any{"uuid": "int-edr", "display_name": "EDR", "connection_status": "ok", "vendor": map[string]any{"key": "SENTINELONE", "name": "SentinelOne"}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/clients", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": []any{map[string]any{"uuid": gapsClient, "name": "Acme", "associated_tenants": []any{
			map[string]any{"uuid": "t-rmm", "integration": ints[0]},
			map[string]any{"uuid": "t-edr", "integration": ints[1]},
		}}}})
	})
	mux.HandleFunc("/clients/"+gapsClient+"/devices", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": devices})
	})
	mux.HandleFunc("/integrations/connected", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": ints})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/integrations/"), "/devices")
		items := integrationDevices[id]
		if items == nil {
			items = []any{}
		}
		send(w, map[string]any{"items": items})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func gapsFor(t *testing.T, srv *httptest.Server) coverageGapsView {
	t.Helper()
	out := runCoverageGaps(t, srv, "coverage", "gaps", "--client", gapsClient, "--db", t.TempDir()+"/absent.db", "--json")
	var v coverageGapsView
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &v); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	return v
}

func endpoint(integrationUUID, ident string) map[string]any {
	return map[string]any{"integration_identifier": ident, "integration": map[string]any{"uuid": integrationUUID}}
}

// Identifiers are per connector: RMM's "1" and EDR's "1" are different machines.
func TestCoverageGapsMatchesIdentifiersWithinTheirIntegration(t *testing.T) {
	v := gapsFor(t, gapsStub(t,
		[]any{map[string]any{"uuid": "d1", "name": "acme-pc", "associated_endpoints": []any{endpoint("int-rmm", "1"), endpoint("int-edr", "e-9")}}},
		map[string][]any{
			"int-rmm": {map[string]any{"uuid": "r1", "name": "acme-pc", "integration_identifier": "1", "tenant_uuid": "t-rmm"}},
			"int-edr": {
				map[string]any{"uuid": "x9", "name": "acme-pc", "integration_identifier": "e-9", "tenant_uuid": "t-edr"},
				map[string]any{"uuid": "x1", "name": "rogue-box", "integration_identifier": "1", "tenant_uuid": "t-edr"},
			},
		}))
	if len(v.Gaps) != 1 || v.Gaps[0].Device != "rogue-box" {
		t.Fatalf("EDR's unattributed device with RMM's identifier must be a gap, got %+v", v.Gaps)
	}
}

// The other direction: a device the client owns that one of its connectors does not report.
func TestCoverageGapsReportsAttributedDeviceAConnectorMisses(t *testing.T) {
	v := gapsFor(t, gapsStub(t,
		[]any{map[string]any{"uuid": "d1", "name": "acme-pc", "associated_endpoints": []any{endpoint("int-rmm", "r-1")}}},
		map[string][]any{"int-rmm": {map[string]any{"uuid": "r1", "name": "acme-pc", "integration_identifier": "r-1", "tenant_uuid": "t-rmm"}}}))
	if len(v.Gaps) != 1 || v.Gaps[0].Device != "acme-pc" || len(v.Gaps[0].MissingFrom) != 1 || v.Gaps[0].MissingFrom[0] != "SentinelOne" {
		t.Fatalf("acme-pc is reported by the RMM but not the EDR on the same client; want one gap missing from SentinelOne, got %+v", v.Gaps)
	}
}
