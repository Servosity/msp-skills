// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"testing"
)

// Class credential-field-not-recognized: a credential name matches in every
// spelling (camelCase, snake_case, kebab-case flags and headers, any case),
// and the public half of a credential pair (an access key ID) counts as a
// credential; a resource identifier does not (apiKeyId keeps matching
// "apikey", a signal the rule already had). A Graph clientId is an object or
// app id, printed by 'apps consent', so it is not a credential name.
func TestCredentialNamesMatchInEverySpelling(t *testing.T) {
	for _, name := range []string{
		"clientSecret", "client_secret", "client-secret", "password", "psk", "key", "Key", "secretAccessKey",
		"apiKey", "api_key", "api-key", "x-api-key", "X-API-KEY",
		"accessKeyId", "access_key_id", "access-key-id",
		"private-key", "license-key", "pre-shared-key", "pass-phrase", "passphrase",
	} {
		if !SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"id", "name", "keyId", "creatorKeyId", "client", "clientProfile", "client-device-type",
		"clientId", "client_id", "client-id", "ClientID", "CLIENT_ID",
		"password_complexity", "passwordComplexity", "keyName", "deviceKey", "psk-id-prefix", "originId",
	} {
		if SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = true, want false", name)
		}
	}
}
