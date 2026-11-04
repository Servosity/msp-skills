// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"nerdio-pp-cli/internal/cliutil"
)

type nerdioCannedTripper struct {
	status int
	ctype  string
	body   string
}

func (c nerdioCannedTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: c.status, Header: http.Header{"Content-Type": {c.ctype}},
		Body: io.NopCloser(bytes.NewReader([]byte(c.body))), Request: req}, nil
}

// Codex r2 open P1 (class tenant secret written to disk), fixed after the stop
// rule: a response on a tenant-secret path is registered whatever its status.
// The r1 guard registered only 2xx answers and held only credential-named
// members of the rest, so a secure-variables HTTP 400 echoing the value was
// neither held nor registered: shown to the MCP caller, then accepted back as
// `teach --notes <value>` and written to disk. No status, path shape or body
// shape on these paths may leave a secret member both unheld and unregistered.
func TestR2TenantPathErrorResponseIsNeverUnheld(t *testing.T) {
	for _, c := range []struct {
		name, path string
		status     int
		ctype      string
		body       string
		secret     string
	}{
		{"secure-variables 400 JSON value", "/rest-api/v1/secure-variables", 400, "application/json", `{"error":"denied","value":"ErrTenantSecret-29Oct-a"}`, "ErrTenantSecret-29Oct-a"},
		{"account secure-variables 500 JSON value", "/rest-api/v1/accounts/101/secure-variables", 500, "application/json", `{"message":"failed","value":"ErrTenantSecret-29Oct-b"}`, "ErrTenantSecret-29Oct-b"},
		{"LAPS 403 JSON password", "/rest-api/v1-beta/accounts/101/devices/3f2b/local-admin-password", 403, "application/json", `{"error":"forbidden","password":"` + "ErrTenantSecret-29Oct-c" + `"}`, "ErrTenantSecret-29Oct-c"},
		{"secure-variables 404 markup value", "/rest-api/v1/secure-variables", 404, "application/xml", `<error><value>ErrTenantSecret-29Oct-d</value></error>`, "ErrTenantSecret-29Oct-d"},
	} {
		rt := tenantSecretTripper{next: nerdioCannedTripper{c.status, c.ctype, c.body}}
		req, _ := http.NewRequest(http.MethodGet, "https://nmm.contoso.com"+c.path, nil)
		resp, err := rt.RoundTrip(req)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		_ = resp.Body.Close()
		if !cliutil.ArgvRepeatsSecret("teach\n--notes=" + c.secret) {
			t.Errorf("%s: the secret member is neither held nor registered (a child argv repeating it is not refused)", c.name)
		}
	}
	// An error answer's credential-named member is registered too. The
	// credential kit's transport, underneath in a real client, also holds an
	// access_token member (Declarations.ResponseFields).
	rt := tenantSecretTripper{next: nerdioCannedTripper{401, "application/json", `{"error":"invalid","access_token":"ErrHeldToken-29Oct-e"}`}}
	req, _ := http.NewRequest(http.MethodGet, "https://nmm.contoso.com/rest-api/v1/secure-variables", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !cliutil.TenantSecretEchoes("ErrHeldToken-29Oct-e") {
		t.Error("a credential-named member of an error answer on a tenant-secret path is not registered")
	}
}
