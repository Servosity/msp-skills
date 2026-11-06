// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Class credential-field-not-recognized: a credential name matches in every
// spelling (camelCase, snake_case, kebab-case flags and headers, any case),
// and the public half of a credential pair (an access key ID) counts as a
// credential; a resource identifier does not (apiKeyId keeps matching
// "apikey", a signal the rule already had). A Maxio client ID is half of the
// client-credentials pair, so it is a credential name.
func TestCredentialNamesMatchInEverySpelling(t *testing.T) {
	for _, name := range []string{
		"clientSecret", "client_secret", "client-secret", "password", "psk", "secretAccessKey", "encryption_key",
		"encryptionKey", "privacy-key", "signing_key",
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
		"password_complexity", "passwordComplexity", "keyName", "deviceKey", "psk-id-prefix", "originId",
		// maxio handfix maxio-credential-names-exact: a bare key is an
		// inventory field's name (DeviceInventoryField.key), not a credential.
		"key", "Key",
	} {
		if SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = true, want false", name)
		}
	}
}

// TestNoMaxioSpecNameReadsAsACredential keeps the credential-name rule honest
// against Maxio's own spec in both directions: of every parameter, property
// and auth header name in spec.json (OpenAPI 3.1), exactly the names in want
// read as credentials. The names are pinned in testdata/maxio-spec-names.json
// (the published module ships no spec); when the library's spec.json sits
// beside the module, the pinned list must still match it, so a spec change
// cannot slip past the guard (press-traps: a guard that reads the wrong spec
// format silently skips).
func TestNoMaxioSpecNameReadsAsACredential(t *testing.T) {
	var names []string
	data, err := os.ReadFile(filepath.Join("testdata", "maxio-spec-names.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &names); err != nil || len(names) < 250 {
		t.Fatalf("pinned spec names: %d read, err %v", len(names), err)
	}
	if spec, err := os.ReadFile(filepath.Join("..", "..", "spec.json")); err == nil {
		var doc any
		if err := json.Unmarshal(spec, &doc); err != nil {
			t.Fatalf("spec.json: %v", err)
		}
		live := map[string]bool{}
		specNames(doc, live)
		pinned := map[string]bool{}
		for _, n := range names {
			pinned[n] = true
			if !live[n] {
				t.Errorf("pinned name %q is no longer in spec.json; re-pin the list", n)
			}
		}
		for n := range live {
			if !pinned[n] {
				t.Errorf("spec.json name %q is not in testdata/maxio-spec-names.json; re-pin the list", n)
			}
		}
	}
	want := map[string]bool{}
	for _, n := range maxioCredentialSpecNames {
		want[n] = true
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
		if SecretFieldName(n) != want[n] {
			t.Errorf("Maxio spec name %q: SecretFieldName = %v, want %v", n, SecretFieldName(n), want[n])
		}
	}
	for n := range want {
		if !seen[n] {
			t.Errorf("expected spec name %q not found in the pinned names", n)
		}
	}
}

// specNames collects every property name, every parameter (and security
// scheme) name: an object with "in" and a string "name".
func specNames(o any, out map[string]bool) {
	switch v := o.(type) {
	case map[string]any:
		if props, ok := v["properties"].(map[string]any); ok {
			for k := range props {
				out[k] = true
			}
		}
		if _, ok := v["in"]; ok {
			if n, ok := v["name"].(string); ok {
				out[n] = true
			}
		}
		for _, c := range v {
			specNames(c, out)
		}
	case []any:
		for _, c := range v {
			specNames(c, out)
		}
	}
}

// maxioCredentialSpecNames are the Maxio spec names that carry a credential:
// the Authorization header (HTTP Basic, the API key with the password x); the
// card data a payment-profile or signup request takes (full_number, cvv, its
// dotted form payment_profile.full_number) and the bank account number and
// IBAN; the single-use tokens that stand in for card data (chargify_token from
// Maxio.js, a Braintree payment_method_nonce); a gateway's vault references
// (vault_token, customer_vault_token); the self-service page token (a URL
// credential for the customer's billing portal); and a webhook's signatures.
// Everything else is public: an event's key (its type), an ACH authorizer's
// name, a dedupe uniqueness_token, the PSD2 stored_credential_transaction_id,
// a policy URL and a boolean setting, whose values would otherwise be redacted.
var maxioCredentialSpecNames = []string{"Authorization", "bank_account_number", "bank_iban", "chargify_token",
	"customer_vault_token", "cvv", "full_number", "payment_method_nonce", "payment_profile.chargify_token",
	"payment_profile.full_number", "self_service_page_token", "signature", "signature_hmac_sha_256", "vault_token"}

// credentialDescription matches a spec description that says its value is a
// credential (a community, a password, a secret, an API key, a token).
var credentialDescription = regexp.MustCompile(`(?i)(communit|passw|passphrase|secret|credential|api[ _-]?key|token|private key)`)

// describedPublicSpecNames are the spec names whose description mentions a
// credential but whose value is not one, each reviewed against spec.json: an
// ACH authorizer's name, include and card metadata (last four digits, expiry),
// PSD2 and dedupe references, a boolean setting, and the
// bank_account_attributes container (its JSON value is read member by member:
// bank_account_number and chargify_token inside it are held by name).
var describedPublicSpecNames = map[string]bool{
	"authorizer_first_name": true, "authorizer_last_name": true, "include": true, "last_four": true,
	"expiration_month": true, "expiration_year": true, "stored_credential_transaction_id": true,
	"uniqueness_token": true, "requires_security_token": true, "bank_account_attributes": true,
}

// TestMaxioSpecNameDescribedAsACredentialIsHeld is the class guard behind
// the pinned list above: a property or parameter the vendor DESCRIBES as a
// credential reads as a credential name, whatever it is called, unless it is
// a reviewed public name. A reprint over a spec that adds such a name fails
// here instead of printing its value.
func TestMaxioSpecNameDescribedAsACredentialIsHeld(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "spec.json"))
	if err != nil {
		t.Skipf("no spec.json beside the module: %v", err)
	}
	var doc any
	if err := json.Unmarshal(spec, &doc); err != nil {
		t.Fatalf("spec.json: %v", err)
	}
	described := map[string]string{}
	specDescriptions(doc, described)
	if len(described) < 5 {
		t.Fatalf("only %d credential-described spec names; the walk is broken", len(described))
	}
	for n, d := range described {
		if !describedPublicSpecNames[n] && !SecretFieldName(n) {
			t.Errorf("spec name %q is described as a credential (%.80q) and does not read as one", n, d)
		}
		if describedPublicSpecNames[n] && SecretFieldName(n) {
			t.Errorf("reviewed public spec name %q reads as a credential", n)
		}
	}
}

// specDescriptions collects every property and parameter name whose
// description reads as a credential (credentialDescription).
func specDescriptions(o any, out map[string]string) {
	switch v := o.(type) {
	case map[string]any:
		if props, ok := v["properties"].(map[string]any); ok {
			for k, p := range props {
				if pm, ok := p.(map[string]any); ok {
					if d, _ := pm["description"].(string); credentialDescription.MatchString(d) {
						out[k] = d
					}
				}
			}
		}
		if _, ok := v["in"]; ok {
			if n, ok := v["name"].(string); ok {
				if d, _ := v["description"].(string); credentialDescription.MatchString(d) {
					out[n] = d
				}
			}
		}
		for _, c := range v {
			specDescriptions(c, out)
		}
	case []any:
		for _, c := range v {
			specDescriptions(c, out)
		}
	}
}

// Hand-fix response-credentials-redacted: a stored-card reference is taken out of
// a response, the rest stays, and a body with nothing to redact comes back byte
// for byte (its key order and number spellings kept).
func TestRedactResponseCredentials(t *testing.T) {
	in := []byte(`{"subscription":{"id":7,"credit_card":{"vault_token":"1","masked_card_number":"XXXX-1111","customer_vault_token":null},"self_service_page_token":"abcdefgh"},"tokens":3}`)
	out := string(RedactResponseCredentials(in))
	for _, want := range []string{`"vault_token":"[redacted]"`, `"self_service_page_token":"[redacted]"`, `"masked_card_number":"XXXX-1111"`, `"customer_vault_token":null`, `"tokens":3`, `"id":7`} {
		if !strings.Contains(out, want) {
			t.Errorf("redacted body %s lacks %s", out, want)
		}
	}
	for _, clean := range []string{`{"b":1,"a":1.50}`, `not json`, ``, `[{"id":1}]`, `<html>vault_token</html>`} {
		if got := string(RedactResponseCredentials([]byte(clean))); got != clean {
			t.Errorf("clean body %q came back %q", clean, got)
		}
	}
}

// Hand-fix basic-constant-password-public: the constant x is never a held part.
func TestAuthHeaderHeldPartsLeaveTheConstantPassword(t *testing.T) {
	// "key123:x"
	got := AuthHeaderHeldParts("Basic a2V5MTIzOng=")
	for _, p := range got {
		if p == "x" || strings.HasPrefix(p, "Basic") {
			t.Errorf("held part %q", p)
		}
	}
	if strings.Join(got, ",") != "a2V5MTIzOng=,key123:x,key123" {
		t.Errorf("parts %v", got)
	}
	if got := AuthHeaderHeldParts("Bearer abc"); len(got) != 1 || got[0] != "Bearer abc" {
		t.Errorf("a non-Basic header: %v", got)
	}
	// "key123:other"
	if got := AuthHeaderHeldParts("Basic a2V5MTIzOm90aGVy"); len(got) != 1 {
		t.Errorf("a Basic header with another password stays whole so the kit holds that password too: %v", got)
	}
}
