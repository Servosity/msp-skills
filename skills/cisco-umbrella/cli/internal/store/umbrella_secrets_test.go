// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// store-never-keeps-secrets: every merge redacts credential members by name, at
// any depth, whether or not anything held them. A record that cannot be decoded
// is not kept. An API key's wrapper object stays a record.
func TestStoreRedactsCredentialMembersByName(t *testing.T) {
	const psk, client, secret = "storePskUnitWq28", "storeClientUnitKe71", "storeSecretUnitZu55"
	rec := `{"id":77,"name":"Branch-Tunnel","Count":12345678901234567890,"client":{"authentication":{"parameters":{"psk":"` + psk + `"}}},"keys":[{"clientId":"` + client + `"}],"key":{"name":"Reports","clientSecret":"` + secret + `"}}`
	got := string(withoutStoredSecrets(json.RawMessage(rec)))
	for _, v := range []string{psk, client, secret} {
		if strings.Contains(got, v) {
			t.Errorf("kept %s: %s", v, got)
		}
	}
	for _, v := range []string{`"Branch-Tunnel"`, `"Reports"`, "12345678901234567890"} {
		if !strings.Contains(got, v) {
			t.Errorf("lost %s: %s", v, got)
		}
	}
	if got := string(withoutStoredSecrets(json.RawMessage(`{not json`))); got != "{}" {
		t.Errorf("undecodable record stored as %s", got)
	}
}
