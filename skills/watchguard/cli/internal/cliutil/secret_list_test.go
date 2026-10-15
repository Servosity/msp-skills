// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/json"
	"strings"
	"testing"
)

// hand-fix (handfixes.json secret-field-list-unified): every credential field
// a scan of every 2xx response and request-body schema in the WatchGuard spec
// found (pass/secret/token/key/psk/pfx/p12/pkcs/private/cred/otp/seed/cert/
// signature/auth by name and by description) is secret-named; the lookalikes
// the same scan found are not.
var specCredentialFields = []string{
	"psk", "psk_encrypted", "pvt_key", "pfx", "password", "passphrase", "otp", "auth_token",
	"activationKey", "initialServiceLicenseKey", "parentLicenseKey", "activationCode", "verificationCode",
	// generic spellings kept so an undocumented field is caught by name
	"client_secret", "shared_secret", "secret", "token", "access_token", "refreshToken", "apiKey", "api_key", "private_key", "p12", "pkcs12",
}

var specNonCredentialFields = []string{
	"authentication", "auth_method", "auth_mode", "authenticationResult", "cert_fingerprint", "pubkey_fingerprint",
	"cert_type", "certificate", "certificate_id", "certs", "signature_id", "signature_algo", "key", "key_length",
	"key_usage", "keyword", "keywords", "serialOrLicense", "author", "authorizedActivities", "total_tokens", "pem",
}

func TestSecretFieldListCoversSpecCredentials(t *testing.T) {
	for _, k := range specCredentialFields {
		if !IsSecretFieldKey(k) {
			t.Errorf("credential field %q is not secret-named", k)
		}
	}
	for _, k := range specNonCredentialFields {
		if IsSecretFieldKey(k) {
			t.Errorf("non-credential field %q treated as secret", k)
		}
	}
	// A certificate import carries the PKCS#12 bundle as pfx.
	body := `{"name":"vpn","pem":"v-pem","pvt_key":"S1","pfx":"S2","password":"S3","isInForgotToken":true,"psk_set":true}`
	out, removed := RedactSecretFields(json.RawMessage(body))
	if !removed || strings.Contains(string(out), `"S`) {
		t.Fatalf("certificate secrets survived: %s", out)
	}
	if !strings.Contains(string(out), `"isInForgotToken":true`) || !strings.Contains(string(out), `"pem":"v-pem"`) {
		t.Fatalf("booleans or non-secret fields were dropped: %s", out)
	}
}

// hand-fix guard (handfixes.json error-body-redacts-secrets): a secret the
// server echoes back base64-encoded, alone or glued into a longer run, is
// still seen.
func TestEchoDetectedInBase64(t *testing.T) {
	secret := "SENTINEL_PRIVATE_KEY"
	for _, body := range []string{
		`{"message":"U0VOVElORUxfUFJJVkFURV9LRVk="}`,
		`{"message":"keyU0VOVElORUxfUFJJVkFURV9LRVk"}`,
		`{"echo":"eyJwdnRfa2V5IjoiU0VOVElORUxfUFJJVkFURV9LRVkifQ"}`,
	} {
		if !EchoesSecret(body, []string{secret}) {
			t.Errorf("base64 echo not detected: %s", body)
		}
	}
	if EchoesSecret(`{"message":"invalid request body"}`, []string{secret}) {
		t.Error("plain message flagged as an echo")
	}
}
