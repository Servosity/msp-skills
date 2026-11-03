// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Hand-written (handfixes.json spec-names-pinned): the credential-name rule
// pinned against Better Stack's own spec in both directions, and the names a
// monitor carries for the site it probes, which the spec does not name.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestStandardAuthNamesAreCredentialNames(t *testing.T) {
	for _, name := range []string{"Authorization", "authorization", "Proxy-Authorization", "WWW-Authenticate", "Cookie", "Set-Cookie",
		"X-Auth-Token", "x-auth", "auth", "sessionId", "session_key", "session", "bearer", "jwt", "id_token", "signature", "sig",
		"client_assertion", "SAMLResponse", "otp", "x-api-key", "api_key", "private_key", "clientSecret",
		"auth_username", "authUsername", "AUTHUSER", "authuser", "auth_user", "auth_password", "request_headers", "incident_token"} {
		if !SecretFieldName(name) {
			t.Errorf("%q is not read as a credential name", name)
		}
	}
	for _, name := range []string{"name", "id", "companyId", "productId", "status", "authorName", "price", "description", "website",
		"on_call_users", "user_id", "last_login_at", "acknowledged_by", "url", "pronounceable_name", "headers_count"} {
		if SecretFieldName(name) {
			t.Errorf("%q is read as a credential name", name)
		}
	}
}

// TestNoBetterstackSpecNameReadsAsACredential keeps the credential-name rule honest
// against Better Stack's own spec in both directions: of every parameter, body,
// type-field and header name in spec.yaml (the internal press format), exactly
// these read as credentials: the auth carrier (the Authorization header) and an
// escalation policy's incident_token (the token that opens incidents through the
// policy's integrations). The names are pinned in testdata/betterstack-spec-names.json
// (the published module ships no spec); when the library's spec.yaml sits beside
// the module, the pinned list must still match it, so a spec change cannot slip
// past the guard (press-traps: a guard that reads spec.json on a spec.yaml
// connector silently skips).
func TestNoBetterstackSpecNameReadsAsACredential(t *testing.T) {
	var names []string
	data, err := os.ReadFile(filepath.Join("testdata", "betterstack-spec-names.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &names); err != nil || len(names) < 80 {
		t.Fatalf("pinned spec names: %d read, err %v", len(names), err)
	}
	if spec, err := os.ReadFile(filepath.Join("..", "..", "spec.yaml")); err == nil {
		live := map[string]bool{}
		for _, m := range specNameLine.FindAllStringSubmatch(string(spec), -1) {
			if m[1] != "betterstack" {
				live[m[1]] = true
			}
		}
		pinned := map[string]bool{}
		for _, n := range names {
			pinned[n] = true
			if !live[n] {
				t.Errorf("pinned name %q is no longer in spec.yaml; re-pin the list", n)
			}
		}
		for n := range live {
			if !pinned[n] {
				t.Errorf("spec.yaml name %q is not in testdata/betterstack-spec-names.json; re-pin the list", n)
			}
		}
	}
	want := map[string]bool{"Authorization": true, "incident_token": true}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
		if SecretFieldName(n) != want[n] {
			t.Errorf("Betterstack spec name %q: SecretFieldName = %v, want %v", n, SecretFieldName(n), want[n])
		}
	}
	for n := range want {
		if !seen[n] {
			t.Errorf("expected spec name %q not found in the pinned names", n)
		}
	}
}

// specNameLine is every `name:`, `flag_name:` and `header:` value of the
// internal spec (parameters, body fields, type fields, the auth header).
var specNameLine = regexp.MustCompile(`(?m)^\s*(?:- )?(?:name|flag_name|header): "?([^"\s#]+)"?\s*$`)
