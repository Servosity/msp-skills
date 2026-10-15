// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/json"
	"strings"
	"testing"
)

// hand-fix guard (handfixes.json secret-fields-never-persisted): the cache
// refuses any body for which ContainsSecretField or IsSecretBearingPath is true.
func TestSecretFieldDetection(t *testing.T) {
	for _, body := range []string{
		`{"objects":[{"object":"t1","psk":"x"}]}`,
		`{"objects":[{"pvt_key":"x"}]}`,
		`{"interfaces":[{"interface":{"wireless_settings":{"Passphrase":"x"}}}]}`,
		`{"results":[{"firstLineItem":{"activationKey":"x"}}]}`,
		`{"dhcp_client":{"auth_token":"x"}}`,
	} {
		if !ContainsSecretField(json.RawMessage(body)) {
			t.Errorf("secret not detected in %s", body)
		}
		out, _ := RedactSecretFields(json.RawMessage(body))
		if strings.Contains(string(out), `"x"`) {
			t.Errorf("secret survived redaction: %s", out)
		}
	}
	if ContainsSecretField(json.RawMessage(`{"objects":[{"object":"t1","name":"vpn","psk_set":true}]}`)) {
		t.Error("non-secret body flagged")
	}
	if !IsSecretBearingPath("/rest/endpoint-security/management/api/v1/accounts/ACC-1/installers/") {
		t.Error("installers path not treated as secret-bearing")
	}
	if IsSecretBearingPath("/rest/endpoint-security/management/api/v1/accounts/ACC-1/devices") {
		t.Error("devices path treated as secret-bearing")
	}
}
