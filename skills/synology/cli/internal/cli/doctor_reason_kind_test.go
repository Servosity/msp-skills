// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDoctorCredentialsRowNamesTheRealReason: the Credentials row said
// "SYNOLOGY_BASE_URL does not answer as a DSM WebAPI" for every probe
// failure, including a NAS that is down and a DSM whose certificate failed
// verification or did not match the pin. It now names the same reason kind
// as the API row.
func TestDoctorCredentialsRowNamesTheRealReason(t *testing.T) {
	dsmNAS := httptest.NewTLSServer(fakeABBNAS(t).Config.Handler)
	defer dsmNAS.Close()
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!doctype html><title>Router</title>"))
	}))
	defer html.Close()
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()

	for _, tc := range []struct {
		name, url, pin  string
		apiWant, credOK string
	}{
		{"unreachable", downURL, "", "ERROR unreachable: cannot reach the NAS", "the NAS did not answer (cannot reach the NAS"},
		{"certificate", dsmNAS.URL, "", "ERROR certificate: the NAS certificate failed verification", "the NAS certificate failed verification"},
		{"pin-mismatch", dsmNAS.URL, strings.Repeat("ab", 32), "ERROR certificate pin mismatch:", "does not match the pinned SHA-256 fingerprint"},
		{"not-dsm", html.URL, "", "is not a DSM WebAPI", "does not answer as a DSM WebAPI"},
	} {
		sweepIsolate(t)
		t.Setenv("SYNOLOGY_BASE_URL", tc.url)
		t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
		t.Setenv("SYNOLOGY_PASSWORD", "s3cret")
		t.Setenv("SYNOLOGY_TLS_FINGERPRINT", tc.pin)
		out, _, _ := runCLIErr(t, "doctor", "--json")
		var report map[string]any
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatalf("%s: %v %s", tc.name, err, out)
		}
		api, _ := report["api"].(string)
		cred, _ := report["credentials"].(string)
		if !strings.Contains(api, tc.apiWant) {
			t.Errorf("%s: api = %q, want %q", tc.name, api, tc.apiWant)
		}
		if !strings.HasPrefix(cred, "ERROR not verified: ") || !strings.Contains(cred, tc.credOK) || !strings.HasSuffix(cred, "so the password was not sent") {
			t.Errorf("%s: credentials = %q, want the %s reason", tc.name, cred, tc.name)
		}
		if tc.name != "not-dsm" && strings.Contains(cred, "does not answer as a DSM WebAPI") {
			t.Errorf("%s: credentials blames SYNOLOGY_BASE_URL: %q", tc.name, cred)
		}
	}
}
