// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gapsStubInts is gapsStub with the connected integrations supplied by the test,
// each with a tenant on the client (documented Client.associated_tenants[].integration).
func gapsStubInts(t *testing.T, ints []map[string]any, devices []any, integrationDevices map[string][]any) *httptest.Server {
	t.Helper()
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	tenants := make([]any, 0, len(ints))
	items := make([]any, 0, len(ints))
	for _, in := range ints {
		tenants = append(tenants, map[string]any{"uuid": "t-" + in["uuid"].(string), "integration": in})
		items = append(items, in)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/clients", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": []any{map[string]any{"uuid": gapsClient, "name": "Acme", "associated_tenants": tenants}}})
	})
	mux.HandleFunc("/clients/"+gapsClient+"/devices", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": devices})
	})
	mux.HandleFunc("/integrations/connected", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"items": items})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/integrations/"), "/devices")
		out := integrationDevices[id]
		if out == nil {
			out = []any{}
		}
		send(w, map[string]any{"items": out, "pagination": map[string]any{"total_pages": 1}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func vendorEndpoint(vendorKey, ident string) map[string]any {
	return map[string]any{"integration_identifier": ident, "integration": map[string]any{"vendor": map[string]any{"key": vendorKey}}}
}

func connected(uuid, vendorKey, vendorName string) map[string]any {
	return map[string]any{"uuid": uuid, "display_name": uuid, "connection_status": "ok", "vendor": map[string]any{"key": vendorKey, "name": vendorName}}
}

func hasGap(v coverageGapsView, device, missingFrom string) bool {
	for _, g := range v.Gaps {
		if g.Device != device {
			continue
		}
		for _, m := range g.MissingFrom {
			if m == missingFrom {
				return true
			}
		}
	}
	return false
}

// Codex round 2, incomplete coverage comparison: an endpoint record Cork still
// keeps for the EDR is not evidence the EDR reports the device. Only the EDR's
// own, freshly read device list is.
func TestCoverageGapsEndpointRecordIsNotEvidenceTheConnectorReportsIt(t *testing.T) {
	v := gapsFor(t, gapsStub(t,
		[]any{map[string]any{"uuid": "d1", "name": "acme-pc", "associated_endpoints": []any{endpoint("int-rmm", "r1"), endpoint("int-edr", "e1")}}},
		map[string][]any{"int-rmm": {map[string]any{"uuid": "x1", "name": "acme-pc", "integration_identifier": "r1", "tenant_uuid": "t-rmm"}}}))
	if !hasGap(v, "acme-pc", "SentinelOne") {
		t.Fatalf("the EDR returned no devices, so acme-pc must be a gap missing from SentinelOne; got %+v (note %q)", v.Gaps, v.Note)
	}
}

// Codex round 2, incorrect coverage join: an endpoint that names only a vendor
// key cannot say which of two same-vendor connectors owns it, so it must not
// clear B's unrelated device "1" and must not count as B reporting acme-pc.
func TestCoverageGapsVendorFallbackNeedsAUniqueConnector(t *testing.T) {
	ints := []map[string]any{connected("int-a", "NINJA_RMM", "NinjaOne"), connected("int-b", "NINJA_RMM", "NinjaOne")}
	v := gapsFor(t, gapsStubInts(t, ints,
		[]any{map[string]any{"uuid": "d1", "name": "acme-pc", "associated_endpoints": []any{vendorEndpoint("NINJA_RMM", "1")}}},
		map[string][]any{
			"int-a": {map[string]any{"uuid": "a1", "name": "acme-pc", "integration_identifier": "1", "tenant_uuid": "t-int-a"}},
			"int-b": {map[string]any{"uuid": "b1", "name": "other-box", "integration_identifier": "1", "tenant_uuid": "t-int-b"}},
		}))
	if len(v.Gaps) == 0 {
		t.Fatalf("two same-vendor connectors and an endpoint without integration.uuid must not read as no coverage gaps; note %q", v.Note)
	}
	found := false
	for _, g := range v.Gaps {
		if g.Device == "other-box" {
			found = true
		}
	}
	if !found {
		t.Fatalf("B's unattributed device other-box must be flagged, got %+v", v.Gaps)
	}
}

// The vendor fallback still works when exactly one connector carries that
// vendor key: no false gap for a correctly matched device.
func TestCoverageGapsVendorFallbackWithOneConnectorStillMatches(t *testing.T) {
	ints := []map[string]any{connected("int-a", "NINJA_RMM", "NinjaOne")}
	v := gapsFor(t, gapsStubInts(t, ints,
		[]any{map[string]any{"uuid": "d1", "name": "acme-pc", "associated_endpoints": []any{vendorEndpoint("NINJA_RMM", "1")}}},
		map[string][]any{"int-a": {map[string]any{"uuid": "a1", "name": "acme-pc", "integration_identifier": "1", "tenant_uuid": "t-int-a"}}}))
	if len(v.Gaps) != 0 {
		t.Fatalf("one NinjaOne connector reporting acme-pc as 1 is a clean match, got %+v", v.Gaps)
	}
}

// An endpoint with no integration identity at all is owner-unknown: RMM "1"
// and EDR "1" are different machines, so it clears neither connector's device.
func TestCoverageGapsOwnerUnknownEndpointClearsNothing(t *testing.T) {
	v := gapsFor(t, gapsStub(t,
		[]any{map[string]any{"uuid": "d1", "name": "acme-pc", "associated_endpoints": []any{map[string]any{"integration_identifier": "1"}}}},
		map[string][]any{
			"int-rmm": {map[string]any{"uuid": "r1", "name": "acme-pc", "integration_identifier": "1", "tenant_uuid": "t-rmm"}},
			"int-edr": {map[string]any{"uuid": "x1", "name": "rogue-box", "integration_identifier": "1", "tenant_uuid": "t-edr"}},
		}))
	found := false
	for _, g := range v.Gaps {
		if g.Device == "rogue-box" {
			found = true
		}
	}
	if !found {
		t.Fatalf("an endpoint that names no connector must not clear the EDR's rogue-box, got %+v (note %q)", v.Gaps, v.Note)
	}
}
