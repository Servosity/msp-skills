// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// The credential name rule (kbms_secrets.go: store-never-keeps-secrets,
// credential-names-match-bms-data), kept from the hand layer the shared
// credential kit replaced: the store's redaction and the credential-endpoint
// rule still read names through it.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStandardAuthNamesAreCredentialNames(t *testing.T) {
	for _, name := range []string{"Authorization", "authorization", "Proxy-Authorization", "WWW-Authenticate", "Cookie", "Set-Cookie",
		"X-Auth-Token", "x-auth", "auth", "sessionId", "session_key", "session", "bearer", "jwt", "id_token", "signature", "sig",
		"client_assertion", "SAMLResponse", "otp", "x-api-key", "api_key", "private_key", "clientSecret"} {
		if !SecretFieldName(name) {
			t.Errorf("%q is not read as a credential name", name)
		}
	}
	for _, name := range []string{"name", "id", "companyId", "productId", "status", "authorName", "price", "description", "website"} {
		if SecretFieldName(name) {
			t.Errorf("%q is read as a credential name", name)
		}
	}
}

// bmsCredentialNames are Kaseya BMS's real credential fields and bmsDataNames
// the data names the credential pattern matches (the vendor spec, read by
// TestNoBMSSpecNameReadsAsACredential wherever the spec is present).
var bmsCredentialNames = map[string]bool{
	"Password": true, "MFACode": true, "ClientSecret": true, "SamlKey": true, "SamlResponse": true, "ItcToken": true,
	"AccessToken": true, "RefreshToken": true, "PasswordOrToken": true, "Secret": true, "ClientId": true,
	"Token": true, "AuthorizationLink": true, "FullCalendarLicenseKey": true,
}

var bmsDataNames = []string{"AccessTokenExpireOn", "RefreshTokenExpireOn", "HasSecret", "UseAccessToken", "Is_Console_Session",
	"SuggestedPasswords", "passwordId", "Sessions", "ExternalClientId", "AppClientIdentifier", "skipToken"}

// TestBMSCredentialNamesAreClassified runs everywhere (the published skill
// tree included): every BMS credential field reads as a credential and every
// BMS data name the pattern matches is exempt.
func TestBMSCredentialNamesAreClassified(t *testing.T) {
	for n := range bmsCredentialNames {
		if !SecretFieldName(n) {
			t.Errorf("Kaseya BMS credential field %q does not read as a credential", n)
		}
	}
	for _, n := range bmsDataNames {
		if !publicSecretLikeFields[normalizedFieldName(n)] || SecretFieldName(n) {
			t.Errorf("Kaseya BMS data field %q is not exempted by publicSecretLikeFields", n)
		}
	}
}

// TestNoBMSSpecNameReadsAsACredential keeps the credential-name rule in step
// with the Kaseya BMS API document (spec.json beside the module on the library
// and reprint trees; there a missing or unreadable document fails): every
// schema property, parameter and form field name the credential pattern
// matches is either one of BMS's real credential fields (held and redacted)
// or a data name publicSecretLikeFields exempts (kaseya-bms handfix
// credential-names-match-bms-data). The real credentials are the login form's
// Password, MFACode, ClientSecret, SamlKey, SamlResponse and ItcToken;
// TokenOutputDto's AccessToken and RefreshToken (POST /v2/security/
// authenticate and /v2/security/refreshtoken); the IT Glue and VSA access
// info AccessToken; IntegrationSettingDto.PasswordOrToken; the webhook
// Secret; RemoteControl's ClientId and ClientSecret; the Pulseway session
// Token; RemoteSessionsOutputDto.AuthorizationLink; FullCalendarLicenseKey.
// The exempt data names are timestamps, booleans, a count, record ids and a
// paging cursor.
func TestNoBMSSpecNameReadsAsACredential(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "spec.json"))
	if os.IsNotExist(err) {
		// A published msp-skills skill ships no vendor spec (skills/<slug>/cli
		// holds the module only); the walk runs on the library and reprint
		// trees, where every spec change happens. The classification itself
		// is checked everywhere by TestBMSCredentialNamesAreClassified.
		if _, herr := os.Stat(filepath.Join("..", "..", "..", "handfixes.json")); herr == nil {
			t.Skip("published skill tree: no vendor spec.json is shipped; the spec walk runs on the library tree")
		}
	}
	if err != nil {
		t.Fatalf("the connector tree carries spec.json beside the module; it cannot be read: %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("spec.json does not parse: %v", err)
	}
	names := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if props, ok := x["properties"].(map[string]any); ok {
				for k := range props {
					names[k] = true
				}
			}
			if enc, ok := x["encoding"].(map[string]any); ok { // multipart/form-data field names
				for k := range enc {
					names[k] = true
				}
			}
			if params, ok := x["parameters"].([]any); ok {
				for _, p := range params {
					if m, ok := p.(map[string]any); ok {
						if n, ok := m["name"].(string); ok {
							names[n] = true
						}
					}
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(doc)
	if len(names) < 500 {
		t.Fatalf("read only %d names from spec.json; the walk is not reading the document", len(names))
	}
	credentials, exempt := bmsCredentialNames, bmsDataNames
	for n := range credentials {
		if !names[n] {
			t.Errorf("spec.json no longer names the credential field %q; the list is stale", n)
		}
	}
	for _, n := range exempt {
		if !names[n] {
			t.Errorf("spec.json no longer names %q; its exemption check is vacuous", n)
		}
	}
	matched := 0
	for n := range names {
		if !secretFieldPattern.MatchString(normalizedFieldName(n)) {
			continue
		}
		matched++
		if credentials[n] {
			continue
		}
		if !publicSecretLikeFields[normalizedFieldName(n)] {
			t.Errorf("Kaseya BMS field %q reads as a credential but is neither a known BMS credential nor exempted in publicSecretLikeFields (classify it)", n)
		}
	}
	if matched < len(credentials)+len(exempt) {
		t.Fatalf("only %d spec names matched the credential pattern; want at least %d", matched, len(credentials)+len(exempt))
	}
}

// Class credential-field-not-recognized: a credential name matches in every
// spelling (camelCase, snake_case, kebab-case flags and headers, any case),
// and the public half of a credential pair (an access key ID) counts as a
// credential; a resource identifier does not (apiKeyId keeps matching
// "apikey", a signal the rule already had). A client ID (BMS RemoteControl's
// ClientId) is half of a client-credentials pair, so it is a credential name;
// BMS's QuickBooks and Kaseya One record ids (ExternalClientId,
// AppClientIdentifier) and its expiry timestamps, booleans and paging cursor
// are data.
func TestCredentialNamesMatchInEverySpelling(t *testing.T) {
	for _, name := range []string{
		"clientSecret", "client_secret", "client-secret", "password", "psk", "key", "Key", "secretAccessKey",
		"apiKey", "api_key", "api-key", "x-api-key", "X-API-KEY",
		"accessKeyId", "access_key_id", "access-key-id",
		"clientId", "client_id", "client-id", "ClientID", "CLIENT_ID",
		"private-key", "license-key", "pre-shared-key", "pass-phrase", "passphrase",
	} {
		if !SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"id", "name", "keyId", "creatorKeyId", "client", "clientProfile", "client-device-type",
		"keyName", "deviceKey", "psk-id-prefix", "originId",
		// Kaseya BMS data names the pattern matches, in every spelling.
		"AccessTokenExpireOn", "access_token_expire_on", "refresh-token-expire-on", "HasSecret", "has_secret",
		"UseAccessToken", "Is_Console_Session", "isConsoleSession", "SuggestedPasswords", "passwordId", "password-id",
		"Sessions", "ExternalClientId", "external_client_id", "AppClientIdentifier", "skipToken", "skip_token",
	} {
		if SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = true, want false", name)
		}
	}
}
