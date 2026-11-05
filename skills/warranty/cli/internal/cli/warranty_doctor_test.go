// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-doctor): doctor's credentials row
// tracks what the vendor answered, issue #282.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

func TestDoctorVerdictTracksDell(t *testing.T) {
	cases := []struct {
		name, wantCreds, wantAPI string
		dell                     func(r *http.Request) (int, string)
		token                    func() (int, string)
	}{
		{name: "working", wantCreds: "valid"},
		{name: "rejected", wantCreds: "ERROR rejected",
			dell: func(*http.Request) (int, string) { return 401, `{}` }},
		{name: "wrong base", wantCreds: "ERROR not verified (HTTP 404", wantAPI: "ERROR",
			dell: func(*http.Request) (int, string) { return 404, `{}` }},
		{name: "server error", wantCreds: "WARN not verified (HTTP 500",
			dell: func(*http.Request) (int, string) { return 500, `{}` }},
		{name: "mint refused", wantCreds: "ERROR rejected", wantAPI: "reachable",
			token: func() (int, string) { return 401, `{"error":"invalid_client"}` }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := defaultFakeVendor()
			if tc.dell != nil {
				f.dell = tc.dell
			}
			if tc.token != nil {
				f.dellToken = tc.token
			}
			warrantyTestEnv(t, f)
			out, errOut, _ := execWarranty(t, "", "doctor", "--json")
			var rep map[string]any
			decodeJSON(t, out, &rep)
			creds, _ := rep["credentials"].(string)
			if !strings.HasPrefix(creds, tc.wantCreds) {
				t.Fatalf("credentials = %q, want prefix %q\nstderr: %s", creds, tc.wantCreds, errOut)
			}
			if api, _ := rep["api"].(string); tc.wantAPI != "" && !strings.HasPrefix(api, tc.wantAPI) {
				t.Fatalf("api = %q, want prefix %q", api, tc.wantAPI)
			}
		})
	}
}

// The probe is one read of the entitlement endpoint with a tag no device has,
// so it needs no device of the operator's and cannot change anything.
func TestDoctorProbeIsOneDellRead(t *testing.T) {
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	if _, errOut, err := execWarranty(t, "", "doctor", "--json"); err != nil {
		t.Fatalf("doctor: %v\n%s", err, errOut)
	}
	probes := 0
	for _, r := range f.requests() {
		if strings.Contains(r.path, "/asset-entitlements") {
			probes++
			if r.method != http.MethodGet || !strings.Contains(r.query, "servicetags=0000000") {
				t.Errorf("probe sent %s %s?%s", r.method, r.path, r.query)
			}
		}
	}
	if probes != 1 {
		t.Fatalf("want 1 entitlement probe, got %d", probes)
	}
}

// Lenovo or Apple alone: nothing was probed, so the row must not read as a
// clean bill of health.
func TestDoctorNoDellCredentialIsNotAVerdict(t *testing.T) {
	warrantyTestEnv(t, defaultFakeVendor())
	t.Setenv("WARRANTY_DELL_CLIENT_ID", "")
	t.Setenv("WARRANTY_DELL_CLIENT_SECRET", "")
	out, errOut, _ := execWarranty(t, "", "doctor", "--json")
	var rep map[string]any
	decodeJSON(t, out, &rep)
	creds, _ := rep["credentials"].(string)
	if !strings.HasPrefix(creds, "INFO not verified") {
		t.Fatalf("credentials = %q\nstderr: %s", creds, errOut)
	}
}
