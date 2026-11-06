// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/base64"
	"strings"
	"testing"
)

// The masking test's echo input (zoom-phone-mint-error-masking), now against
// the allowlist that replaced masking: neither the secret nor the Basic pair
// nor any other body text survives.
func TestZoomTokenEndpointErrorShowsOnlyStatusAndCode(t *testing.T) {
	basic := base64.StdEncoding.EncodeToString([]byte("cid:s3cret"))
	got := ZoomTokenEndpointError(400, []byte(`{"echo":"client_secret=s3cret","auth":"Basic `+basic+`","error":"invalid_client"}`))
	if strings.Contains(got, "s3cret") || strings.Contains(got, basic) || strings.Contains(got, "echo") {
		t.Fatalf("token error repeats the body: %s", got)
	}
	if got != "HTTP 400 (invalid_client: the client ID or client secret was rejected)" {
		t.Fatalf("token error = %q", got)
	}
	for _, body := range []string{`{"error":"s3cret"}`, `{"error":"invalid_client","error":"s3cret"}`, `s3cret`, `{"error":["invalid_client"]}`, ``} {
		if got := ZoomTokenEndpointError(401, []byte(body)); got != "HTTP 401 (response body withheld)" {
			t.Fatalf("body %q: %q", body, got)
		}
	}
}

func TestZoomRequireSecureURL(t *testing.T) {
	for _, ok := range []string{"", "https://api.zoom.us/v2", "http://127.0.0.1:8080/v2", "http://localhost/oauth/token", "http://[::1]:9/v2", "mock-token-for-testing"} {
		if err := ZoomRequireSecureURL("base URL", ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"http://api.zoom.us/v2", "http://10.0.0.5/oauth/token", "HTTP://evil.example/oauth/token"} {
		if err := ZoomRequireSecureURL("token URL", bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
