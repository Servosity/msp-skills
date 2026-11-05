// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression test (handfixes.json: redirect-never-forwards-credentials).

package cli

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// sameHostAuthRedirect sends /v1/current_user to another path on the same host,
// which answers 401 to a request without a key.
type sameHostAuthRedirect struct{}

func (sameHostAuthRedirect) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"data":{"id":"1"}}`)), Request: req}
	switch {
	case req.URL.Path == "/v1/current_user":
		resp.StatusCode = http.StatusFound
		resp.Header.Set("Location", "/v1/users/current")
	case req.Header.Get("Authorization") == "":
		resp.StatusCode = http.StatusUnauthorized
		resp.Body = io.NopCloser(strings.NewReader(`{"error":"Not Authorized"}`))
	}
	return resp, nil
}

// A redirect hop carries no key, so a 401 behind it neither marks the key
// invalid nor the API unreachable: doctor says the probe was redirected.
func TestDoctorRedirectedAuthFailureIsNotInvalid(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("DNSFILTER_API_KEY", "raw-key-redirect")
	t.Setenv("DNSFILTER_BASE_URL", "https://api.dnsfilter.test")
	useR17Transport(t, sameHostAuthRedirect{})
	report, err := runR17JSON(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	api, _ := report["api"].(string)
	creds, _ := report["credentials"].(string)
	if !strings.HasPrefix(api, "WARN") || !strings.Contains(api, "redirected to") {
		t.Errorf("api verdict = %q, want a WARN naming the redirect", api)
	}
	if !strings.HasPrefix(creds, "WARN") || !strings.Contains(creds, "redirected to") {
		t.Errorf("credentials verdict = %q, want a WARN naming the redirect, not invalid or an error", creds)
	}
}
