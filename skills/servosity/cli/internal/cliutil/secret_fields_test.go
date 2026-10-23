// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"strings"
	"testing"
)

func TestIsSecretFieldNameMatchesEveryCasing(t *testing.T) {
	for _, name := range []string{
		"password", "lan_password", "LanPassword", "EncryptionPassword", "RESTIC_PASSWORD",
		"AWS_SECRET_ACCESS_KEY", "postmark_server_api_token", "agent_provision_token_id",
		"product_key", "key", "code", "codes", "mfa_codes", "secret", "token", "client-secret",
	} {
		if !IsSecretFieldName(name) {
			t.Errorf("%q is a secret field name", name)
		}
	}
	for _, name := range []string{
		"name", "encryption_key", "backup_encryption_key_option", "enforce_mfa", "support_code",
		"zip_code", "keyword", "token_expiry", "id", "hostname", "status",
	} {
		if IsSecretFieldName(name) {
			t.Errorf("%q is not a secret field name", name)
		}
	}
}

func TestWithholdSecretFieldsText(t *testing.T) {
	doc := `{"id":7,"name":"Partner","postmark_server_api_token":"pm-value","ApplicationSettings":{"EncryptionPassword":"enc-value","Password":""},"params_schema":{"properties":{"token":{"type":"string"}}},"codes":["111111","222222"]}`
	out := WithholdSecretFieldsText(doc)
	for _, gone := range []string{"pm-value", "enc-value", "111111"} {
		if strings.Contains(out, gone) {
			t.Fatalf("%s survived: %s", gone, out)
		}
	}
	for _, kept := range []string{`"name":"Partner"`, `"token":{"type":"string"}`, `"Password":""`} {
		if !strings.Contains(out, kept) {
			t.Fatalf("%s was changed: %s", kept, out)
		}
	}
	ndjson := "{\"event\":\"data\",\"data\":{\"username\":\"admin\",\"password\":\"cred-value\"}}\n{\"event\":\"end\"}\n"
	if out := WithholdSecretFieldsText(ndjson); strings.Contains(out, "cred-value") || !strings.Contains(out, `"username":"admin"`) || !strings.HasSuffix(out, "\n") {
		t.Fatalf("NDJSON line not withheld cleanly: %q", out)
	}
	if plain := "Synced 3 companies."; WithholdSecretFieldsText(plain) != plain {
		t.Fatal("plain text changed")
	}
}

func TestWithholdSecretFieldsTextCatchesEscapedAndNestedKeys(t *testing.T) {
	bs := string(rune(92)) // a backslash: the key below is spelled with a JSON \u escape
	for name, text := range map[string]string{
		"escaped key": `{"id":7,"postmark_server_api_to` + bs + `u006ben":"pm-value"}`,
		"nested body": `{"detail":"echo","body":"{\"password\":\"pm-value\"}"}`,
		"cut doc":     `{"id":7,"name":"x","password":"pm-value","more":[1,2`,
	} {
		if out := WithholdSecretFieldsText(text); strings.Contains(out, "pm-value") {
			t.Fatalf("%s: %s", name, out)
		}
	}
	if !MayCarrySecretField([]byte(`{"postmark_server_api_to` + bs + `u006ben":"x"}`)) {
		t.Fatal("an escaped key must reach the parser")
	}
}

func TestWithholdSecretFieldsTextCutValueAndErrorCodes(t *testing.T) {
	if out := WithholdSecretFieldsText(`{"id":7,"name":"x","agent_provision_token_id":"cut-value-XXXX`); strings.Contains(out, "cut-value") || !strings.Contains(out, `"name":"x"`) {
		t.Fatalf("a value cut before its closing quote survived: %s", out)
	}
	if out := WithholdSecretFieldsText(`{"detail":"Given token not valid","code":"token_not_valid"}`); !strings.Contains(out, "token_not_valid") {
		t.Fatalf("an API error code word was withheld: %s", out)
	}
	if out := WithholdSecretFieldsText(`{"prefix":"ab","code":"123456"}`); strings.Contains(out, "123456") {
		t.Fatalf("an MFA code was shown: %s", out)
	}
	if out := WithholdSecretFieldsText(`Error: GET /x returned HTTP 401: {"code": "token_not_valid", "detail": "x"`); !strings.Contains(out, "token_not_valid") {
		t.Fatalf("the error-code word was withheld in unparseable text: %s", out)
	}
}
