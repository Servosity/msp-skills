// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// store-never-keeps-secrets: a credential-named member is redacted at any
// depth, whether or not anything held it; every other member, a
// secrets_schema definition included, is stored as written; a record that
// does not decode is stored as {}.
func TestStoreRedactsCredentialMembersByName(t *testing.T) {
	in := `{"id":"PWHS001","delivery_method":{"type":"http_delivery_method","url":"https://hooks.example.com/pd",` +
		`"secret":"whsecUnitStoreA1b2","custom_headers":[{"name":"X-Relay","value":"relayUnitStoreC3d4"}]},` +
		`"integrations":[{"routing_key":"rkUnitStoreE5f6","integration_key":"ikUnitStoreG7h8"}],` +
		`"secrets_schema":{"properties":{"Authentication Headers":{"type":"text","format":"TEXTAREA"}}},` +
		`"summary":"Checkout webhook"}`
	out := string(withoutStoredSecrets(json.RawMessage(in)))
	for _, secret := range []string{"whsecUnitStoreA1b2", "relayUnitStoreC3d4", "rkUnitStoreE5f6", "ikUnitStoreG7h8"} {
		if strings.Contains(out, secret) {
			t.Errorf("stored %q: %s", secret, out)
		}
	}
	for _, kept := range []string{"Checkout webhook", "TEXTAREA", "Authentication Headers", "https://hooks.example.com/pd", storedSecretMarker} {
		if !strings.Contains(out, kept) {
			t.Errorf("dropped %q: %s", kept, out)
		}
	}
	if got := string(withoutStoredSecrets(json.RawMessage(`{"secret":`))); got != "{}" {
		t.Errorf("an undecodable record was stored as %s, want {}", got)
	}
}
