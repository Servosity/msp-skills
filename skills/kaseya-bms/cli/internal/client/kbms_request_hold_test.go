// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// kaseya-bms handfix credential-endpoint-request-held (Codex review r1 class
// holder incomplete): a request to an endpoint whose path names a credential
// has every value held but the allowlisted identifiers.

import (
	"testing"

	"kaseya-bms-pp-cli/internal/cliutil"
)

func TestCredentialEndpointRequestHoldsEveryValueButTheIdentifiers(t *testing.T) {
	const code, user, tenant = "authCodeX9b2K7m4Q8p3N6r5", "login.user.r1@example.com", "tenantR1Visible"
	holdCredentialEndpointRequest("/v2/security/authenticate", map[string]string{"state": "stateQ7r1Held"}, multipartRequestBody{Fields: map[string]string{
		"Code": code, "UserName": user, "Tenant": tenant, "GrantType": "authorization_code", "RedirectUri": "https://redirect.example/cb"}})
	if !cliutil.Held(code) || !cliutil.Held("stateQ7r1Held") {
		t.Fatal("the authorization code and the query value of a login request are not held")
	}
	for _, v := range []string{user, tenant, "authorization_code", "https://redirect.example/cb"} {
		if cliutil.Held(v) {
			t.Errorf("the allowlisted identifier %q was held", v)
		}
	}
	holdCredentialEndpointRequest("/v2/security/refreshtoken", nil, map[string]any{"Extra": "refreshExtraR1-4b2c"})
	if !cliutil.Held("refreshExtraR1-4b2c") {
		t.Fatal("a JSON body value of a credential request is not held")
	}
	holdCredentialEndpointRequest("/v2/servicedesk/tickets", nil, multipartRequestBody{Fields: map[string]string{"Code": "ticketCodeR1-7d1e"}})
	if cliutil.Held("ticketCodeR1-7d1e") {
		t.Fatal("an ordinary request's Code was held: the credential-path rule over-matches")
	}
}
