// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"encoding/json"
	"net/url"
	"testing"

	"acronis-pp-cli/internal/cliutil"
)

func TestHoldRequestCredentialsHoldsWhatTheRequestCarries(t *testing.T) {
	form := url.Values{"client_id": {"idp-client-id-0001"}, "client_secret": {"idp-form-secret-0001"}, "grant_type": {"client_credentials"}}
	holdRequestCredentials("/api/2/idp/token", nil, formRequestBody{Fields: form})
	holdRequestCredentials("/api/2/clients", map[string]string{"access_token": "query-token-0000001"},
		map[string]any{"name": "Northwind IT Services", "nested": map[string]any{"clientSecret": "json-nested-secret-01"}, "list": []any{map[string]any{"password": "json-list-password-01"}}})
	for _, held := range []string{"idp-client-id-0001", "idp-form-secret-0001", "query-token-0000001", "json-nested-secret-01", "json-list-password-01"} {
		if !cliutil.FunnelWithheld(cliutil.FunnelHeld("server said " + held)) {
			t.Errorf("%q was not held", held)
		}
	}
	// Outside the credential tier an ordinary field is not a credential.
	if got := cliutil.FunnelHeld("tenant Northwind IT Services already exists"); cliutil.FunnelWithheld(got) {
		t.Errorf("an ordinary body value was held: %q", got)
	}
}

// TestRequestKeyIsCredentialKeepsEverySignal pins that moving the request
// detector onto the shared predicate (cliutil.IsSecretFieldName, hand-fix
// handfixes.json credential-fields-never-persisted) kept every name it held
// before and gained the shared ones, while an identifier stays unheld.
func TestRequestKeyIsCredentialKeepsEverySignal(t *testing.T) {
	for _, key := range []string{
		// Signals the detector had before.
		"code", "authorization_code", "client_secret", "secretValue", "password", "new_password",
		"passwd", "passphrase", "token", "refresh_token", "tokenHint", "client_assertion",
		"credential", "credentials_json", "apiKey", "x-api-key", "private_key", "signing_key",
		// Signals the shared predicate adds.
		"key", "otp", "totp", "codes", "mfa_codes", "product_key", "passcode",
	} {
		if !requestKeyIsCredential(key) {
			t.Errorf("%q must be held as a credential", key)
		}
	}
	for _, key := range []string{"client_id", "grant_type", "tenant_id", "name", "scope", "type"} {
		if requestKeyIsCredential(key) {
			t.Errorf("%q is not a credential", key)
		}
	}
}

// TestHoldRequestCredentialsHoldsNumbers: a credential-named field holds a
// number as well as a string, in the digits the request sent (no float
// rounding), from a raw body and from a Go value.
func TestHoldRequestCredentialsHoldsNumbers(t *testing.T) {
	holdRequestCredentials("/api/2/tenants/T/clients", nil, json.RawMessage(`{"type":"api_client","password":123456789012,"nested":{"api_key":98765432109876543210}}`))
	holdRequestCredentials("/api/2/clients", nil, map[string]any{"token": int64(555666777888)})
	for _, held := range []string{"123456789012", "98765432109876543210", "555666777888"} {
		if !cliutil.FunnelWithheld(cliutil.FunnelHeld("invalid value " + held)) {
			t.Errorf("numeric credential %s was not held", held)
		}
	}
	// A number under an ordinary name is not a credential.
	holdRequestCredentials("/api/2/clients", nil, json.RawMessage(`{"quota":424242424242}`))
	if cliutil.FunnelWithheld(cliutil.FunnelHeld("quota 424242424242 exceeded")) {
		t.Error("an ordinary number was held")
	}
}
