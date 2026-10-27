// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/acronis/handfixes.json under
// the "credential-fields-never-persisted" entry. See docs/reprint-survival.md.

package cliutil

import (
	"encoding/json"
	"testing"
)

func TestIsSecretFieldNameMatchesEveryCasing(t *testing.T) {
	for _, name := range []string{
		"password", "lan_password", "LanPassword", "EncryptionPassword", "RESTIC_PASSWORD",
		"AWS_SECRET_ACCESS_KEY", "postmark_server_api_token", "agent_provision_token_id",
		"product_key", "key", "code", "codes", "mfa_codes", "secret", "token", "client-secret",
		"client_secret", "clientSecret", "access_token", "refresh_token", "id_token", "api_key", "otp",
	} {
		if !IsSecretFieldName(name) {
			t.Errorf("%q is a secret field name", name)
		}
	}
}

// TestIsSecretFieldNameKeepsAcronisIdentifiers pins that the fields Acronis
// rows are keyed and filtered by are not credentials, so the store keeps them.
func TestIsSecretFieldNameKeepsAcronisIdentifiers(t *testing.T) {
	for _, name := range []string{
		"client_id", "clientId", "tenant_id", "tenantId", "id", "type", "status", "usage", "value",
		"name", "token_type", "expires_in", "scope", "result_code", "public_key", "login", "kind",
		"parent_id", "application_id", "offering_items", "keyword", "token_expiry", "zip_code",
	} {
		if IsSecretFieldName(name) {
			t.Errorf("%q is not a secret field name", name)
		}
	}
}

func TestIsSecretFieldKeepsStatusCodeWords(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  bool
	}{
		{"ok", false}, {"error", false}, {"warning", false},
		{"cancelled", false}, {"abandoned", false}, {"timedout", false},
		{"123456", true}, {"A1B2", true}, {float64(400), true}, {nil, true},
		// Only the documented result words: a letter-only code is withheld.
		{"TestSecretLetterCode", true}, {"status-ok", true}, {"OK", true},
	} {
		if got := IsSecretField("code", tc.value); got != tc.want {
			t.Errorf("IsSecretField(code, %v) = %v, want %v", tc.value, got, tc.want)
		}
	}
	if !IsSecretField("client_secret", "") || IsSecretField("client_id", "c-1") {
		t.Error("the value only refines a field named exactly code")
	}
}

func TestMayCarrySecretFieldSeesSeparatedAndEscapedKeys(t *testing.T) {
	bs := string(rune(92))
	for _, body := range []string{
		`{"to_ken":"x"}`, `{"PASS-word":"x"}`, `{"a":"b","to` + bs + `u006ben":"x"}`, `{"client_secret":"x"}`,
	} {
		if !MayCarrySecretField([]byte(body)) {
			t.Errorf("%s must reach the parser", body)
		}
	}
	if MayCarrySecretField([]byte(`{"id":"t-1","name":"Acme","status":"enabled"}`)) {
		t.Error("an ordinary row was sent to the parser")
	}
	if !IsSecretFieldName("to_ken") {
		t.Error("the prefilter and the predicate must agree on a separated key")
	}
}

func TestJSONCarriesSecret(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"client_id":"new-client","client_secret":"test-secret-aaaaaaaaaaaa"}`, true},
		{`{"items":[{"id":1,"settings":{"password":"p"}}]}`, true},
		{`{"client_id":"new-client","client_secret":""}`, false},
		{`{"items":[{"id":1,"result":{"code":"ok"}}]}`, false},
		{`{"id":1,"name":"Acme"}`, false},
		{`not json`, true},
		{``, false},
	} {
		if got := JSONCarriesSecret([]byte(tc.body)); got != tc.want {
			t.Errorf("JSONCarriesSecret(%s) = %v, want %v", tc.body, got, tc.want)
		}
	}
}

func TestIsSecretFieldNameMatchesPluralNames(t *testing.T) {
	for _, name := range []string{
		"password", "passwords", "secret", "secrets", "token", "tokens", "api_key", "api_keys", "apikeys",
		"credential", "credentials", "private_key", "private_keys", "Secrets", "PrivateKeys", "client_secrets",
	} {
		if !IsSecretFieldName(name) {
			t.Errorf("%q is a secret name", name)
		}
	}
	for _, name := range []string{"client_id", "tenant_id", "token_type", "credential_id", "credentials_count", "id", "value", "status"} {
		if IsSecretFieldName(name) {
			t.Errorf("%q is not a secret name", name)
		}
	}
	if !MayCarrySecretField([]byte(`{"credentials":{"user":"u"}}`)) {
		t.Error("the prefilter must send a credentials key to the parser")
	}
}

func TestIsSecretFieldKeepsPluralCounts(t *testing.T) {
	for _, c := range []struct {
		name  string
		value any
		want  bool
	}{
		{"total_tokens", json.Number("10"), false},
		{"input_tokens", float64(7), false},
		{"tokens", []any{"test-secret-plural-llll"}, true},
		{"access_tokens", map[string]any{"a": "x"}, true},
		{"api_keys", "test-secret-plural-mmmm", true},
		{"token", json.Number("123456"), true},
		// A number is a count only under a count name.
		{"max_api_keys", json.Number("5"), false},
		{"api_keys", json.Number("98765432109876543210"), true},
		{"access_tokens", float64(5), true},
		{"tokens", json.Number("3"), true},
		{"user_total_tokens", json.Number("3"), true},
	} {
		if got := IsSecretField(c.name, c.value); got != c.want {
			t.Errorf("IsSecretField(%q, %v) = %v, want %v", c.name, c.value, got, c.want)
		}
	}
	if !JSONCarriesSecret([]byte(`{"items":[{"client_id":"c1","data":{"api_keys":98765432109876543210}}]}`)) {
		t.Error("a number under api_keys must keep the body out of the response cache")
	}
}
