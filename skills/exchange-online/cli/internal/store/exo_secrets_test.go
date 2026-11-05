// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// store-never-keeps-secrets: every merge redacts credential members by name, at
// any depth, whether or not anything held them. A record that cannot be decoded
// is not kept.
func TestStoreRedactsCredentialMembersByName(t *testing.T) {
	const password, token, secret = "storePskUnitWq28", "storeClientUnitKe71", "storeSecretUnitZu55"
	rec := `{"Identity":"Reports","Name":"Rosa Diaz","Count":12345678901234567890,"Password":"` + password + `","Connector":{"auth":{"token":"` + token + `"}},"Apps":[{"client_secret":"` + secret + `"}]}`
	got := string(withoutStoredSecrets(json.RawMessage(rec)))
	for _, v := range []string{password, token, secret} {
		if strings.Contains(got, v) {
			t.Errorf("kept %s: %s", v, got)
		}
	}
	for _, v := range []string{`"Rosa Diaz"`, `"Reports"`, "12345678901234567890"} {
		if !strings.Contains(got, v) {
			t.Errorf("lost %s: %s", v, got)
		}
	}
	if got := string(withoutStoredSecrets(json.RawMessage(`{not json`))); got != "{}" {
		t.Errorf("undecodable record stored as %s", got)
	}
}
