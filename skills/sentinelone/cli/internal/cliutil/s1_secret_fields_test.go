// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): both directions of the shared credential
// rules in s1_secret_fields.go.

package cliutil

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSecretFieldNameRule(t *testing.T) {
	for _, k := range []string{
		"passphrase", "Passphrase", "password", "uninstall_password", "smtpPassword", "passwd",
		"passcode", "clientSecret", "shared-secret", "token", "registrationToken",
		"registration_token", "REGISTRATION-TOKEN", "apiToken", "api token", "apiKey", "api_key",
		"API-KEY", "privateKey", "private_key", "privkey", "recoveryCodes", "recovery_code",
		"qrCode", "sessionToken", "downloadToken",
	} {
		if !IsSecretFieldName(k) {
			t.Errorf("%q should be a secret field name", k)
		}
	}
	for _, k := range []string{
		"id", "name", "computerName", "nextCursor", "next_cursor", "nextPageToken",
		"next_page_token", "pageToken", "totalItems", "siteId", "accountName", "key",
		"keyword", "registrationDate", "agentVersion", "passive", "credType", "",
	} {
		if IsSecretFieldName(k) {
			t.Errorf("%q should not be a secret field name", k)
		}
	}
}

func TestSecretBearingPathRule(t *testing.T) {
	for _, p := range []string{
		"/agents/passphrases",
		"/agents/passphrases/",
		"/agents/passphrases?siteIds=1&limit=10",
		"/web/api/v2.1/agents/passphrases",
		"https://usea1.sentinelone.net/web/api/v2.1/agents/passphrases?x=1",
		"/accounts/{account_id}/uninstall-password/view",
		"/accounts/123/uninstall-password/view",
		"/ACCOUNTS/123/Uninstall-Password/VIEW",
		"/users/login/by-token",
		"/users/login/by-token?token=x",
		"/ranger/cred-groups/details",
		"/ranger/cred-groups/details?credGroupIds=1",
		"/ranger/cred-groups",
		"/ranger/cred-groups?siteIds=1",
	} {
		if !IsSecretBearingPath(p) {
			t.Errorf("%q should be a secret-bearing path", p)
		}
	}
	for _, p := range []string{
		"", "/", "/agents", "/agents/passphrase", "/sites", "/sites/1", "/groups",
		"/accounts/123/uninstall-password/metadata",
		"/accounts/123/uninstall-password/generate",
		"/accounts//uninstall-password/view",
		"/users/login", "/users/1/api-token-details",
		"/ranger/cred-groups/details/123", "/ranger/table-view",
	} {
		if IsSecretBearingPath(p) {
			t.Errorf("%q should not be a secret-bearing path", p)
		}
	}
	if !IsNeverStoredResource("ranger-cred-groups") {
		t.Errorf("ranger-cred-groups (GET /ranger/cred-groups, the Ranger credential groups) must never be stored")
	}
	for name, path := range NeverStoredResources() {
		if !IsSecretBearingPath(path) {
			t.Errorf("never-stored resource %s maps to %s, which the path rule does not cover", name, path)
		}
		if !IsNeverStoredResource(name) || !IsNeverStoredResource(strings.ReplaceAll(strings.ToUpper(name), "-", "_")) {
			t.Errorf("IsNeverStoredResource(%s) = false", name)
		}
	}
	for _, r := range []string{"agents", "sites", "groups", "ranger-table-view", "ranger-gateways", ""} {
		if IsNeverStoredResource(r) {
			t.Errorf("%q should not be a never-stored resource", r)
		}
	}
}

func TestRedactSecretFieldsBothWays(t *testing.T) {
	in := `{"id":"a1","n":12345678901234567890,"passwordExpired":true,"token":null,` +
		`"passphrase":"P1","nested":{"deep":[{"apiToken":"P2","keep":"<k&>"}]},` +
		`"cfg":[{"name":"clientSecret","value":"P3"},{"name":"region","value":"us"}]}`
	out, removed := RedactSecretFields(json.RawMessage(in))
	if !removed {
		t.Fatal("nothing was removed")
	}
	for _, s := range []string{"P1", "P2", "P3"} {
		if strings.Contains(string(out), `"`+s+`"`) {
			t.Errorf("%s survived: %s", s, out)
		}
	}
	for _, keep := range []string{`"n":12345678901234567890`, `"passwordExpired":true`, `"token":null`, `"keep":"<k&>"`, `"value":"us"`, `"name":"clientSecret"`} {
		if !strings.Contains(string(out), keep) {
			t.Errorf("%s lost: %s", keep, out)
		}
	}
	clean := json.RawMessage(`{"id":"a1",  "name":"no secrets here","pagination":{"nextCursor":"c"}}`)
	if got, removed := RedactSecretFields(clean); removed || string(got) != string(clean) {
		t.Errorf("a clean document was rewritten: %s", got)
	}
	// A JSON escape cannot hide a key from the rule.
	escaped := json.RawMessage(`{"registration\u0054oken":"P4"}`)
	if got, removed := RedactSecretFields(escaped); !removed || strings.Contains(string(got), "P4") {
		t.Errorf("an escaped key slipped through: %s", got)
	}
	if !ContainsSecretField(json.RawMessage(`[{"a":{"b":{"privateKey":"x"}}}]`)) {
		t.Error("ContainsSecretField missed a deep key")
	}
	if ContainsSecretField(json.RawMessage(`{"password_set":false,"secret":null}`)) {
		t.Error("ContainsSecretField flagged a boolean/null")
	}
	// A number under a name that is secret only through "token" is a count;
	// under any other secret name (or as a string) it is still a secret.
	if ContainsSecretField(json.RawMessage(`{"total_tokens":10,"tokenCount":3}`)) {
		t.Error("a token count was treated as a credential")
	}
	for _, in := range []string{`{"passcode":123456}`, `{"token":"12345"}`, `{"apiTokenPassword":42}`} {
		if !ContainsSecretField(json.RawMessage(in)) {
			t.Errorf("%s should be a secret", in)
		}
	}
}

func TestResponseMayCarrySecretBothWays(t *testing.T) {
	for _, in := range []string{
		`{"data":{"registrationToken":"x"}}`,
		"id,computerName,Passphrase\n1,WS1,ZEBRA\n",
		"{\"apiKey\":\"a\"}\n{\"apiKey\":\"b\"}\n",
		`not json token=abc`,
	} {
		if !ResponseMayCarrySecret([]byte(in)) {
			t.Errorf("%q should not be cacheable", in)
		}
	}
	for _, in := range []string{
		`{"data":[{"id":"1","name":"a"}],"pagination":{"nextCursor":"c"}}`,
		`{"data":{"passwordExpired":true,"total_tokens":4}}`,
		"id,computerName\n1,WS1\n",
		``,
	} {
		if ResponseMayCarrySecret([]byte(in)) {
			t.Errorf("%q should be cacheable", in)
		}
	}
}

func TestWithholdSecretFieldsTextBothWays(t *testing.T) {
	for name, tc := range map[string]struct{ in, gone, kept string }{
		"json":     {`{"registrationToken":"W1","name":"n"}`, "W1", `"name":"n"`},
		"escaped":  {`{"registration\u0054oken":"W2"}`, "W2", WithheldMarker},
		"in-str":   {`{"rows":[{"data":"{\"passphrase\":\"W3\",\"id\":\"9\"}"}]}`, "W3", `\"id\":\"9\"`},
		"cut":      {`{"preview":"{\"data\":[{\"token\":\"W4partial"}`, "W4partial", "preview"},
		"ndjson":   {"{\"apiKey\":\"W5\"}\n{\"apiKey\":\"W5\"}\n", "W5", "apiKey"},
		"plain":    {"registrationToken: W6 name: ok", "W6", "name: ok"},
		"pair":     {`[{"key":"smtp_password","value":"W7"}]`, "W7", "smtp_password"},
		"urlquery": {"see https://x.example/dl?token=W8&x=1", "W8", "x=1"},
		"esc-cut":  {`{"preview":"{\"api\\u004bey\":\"W9cut"}`, "W9cut", "preview"},
	} {
		got := WithholdSecretFieldsText(tc.in)
		if strings.Contains(got, tc.gone) {
			t.Errorf("%s: %q survived: %s", name, tc.gone, got)
		}
		if !strings.Contains(got, tc.kept) {
			t.Errorf("%s: %q lost: %s", name, tc.kept, got)
		}
	}
	for _, clean := range []string{
		`{"id":"1","summary":"Get the uninstall password metadata","path":"/agents/passphrases"}`,
		`{"passwordExpired":true,"token":null,"nextPageToken":"p","total_tokens":10}`,
		`{"preview":"{\"total_tokens\":10,\"name\":\"x\""}`,
		"plain text with the word token in it",
		"",
	} {
		if got := WithholdSecretFieldsText(clean); got != clean {
			t.Errorf("clean text altered: %q -> %q", clean, got)
		}
	}
}
