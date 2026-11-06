// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// Hand-written (handfixes.json domotz-credential-names-exact and
// spec-credential-under-a-generic-name): the credential-name rule pinned
// against Domotz's own spec in both directions, plus the class guard behind
// it, a spec name the vendor describes as a credential. The credential kit's
// declarations are pinned against the same list (internal/credwire
// TestDeclarationsMatchTheSpec).

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
		"read", "write", "snmp-read-community", "authentication-key", "encryption-key", "credentials-username", "credentials-password", "leader-password"} {
		if !SecretFieldName(name) {
			t.Errorf("%q is not read as a credential name", name)
		}
	}
	for _, name := range []string{"name", "id", "companyId", "productId", "status", "authorName", "price", "description", "website",
		"key", "user", "vpn_session_id", "authentication_status", "authentication_protocol", "requires_credentials", "can_write"} {
		if SecretFieldName(name) {
			t.Errorf("%q is read as a credential name", name)
		}
	}
}

// TestNoDomotzSpecNameReadsAsACredential keeps the credential-name rule honest
// against Domotz's own spec in both directions: of every parameter, property
// and auth header name in spec.json (OpenAPI 3.1), exactly the names in want
// read as credentials. The names are pinned in testdata/domotz-spec-names.json
// (the published module ships no spec); when the library's spec.json sits
// beside the module, the pinned list must still match it, so a spec change
// cannot slip past the guard (press-traps: a guard that reads the wrong spec
// format silently skips).
func TestNoDomotzSpecNameReadsAsACredential(t *testing.T) {
	var names []string
	data, err := os.ReadFile(filepath.Join("testdata", "domotz-spec-names.json"))
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
				t.Errorf("spec.json name %q is not in testdata/domotz-spec-names.json; re-pin the list", n)
			}
		}
	}
	want := map[string]bool{}
	for _, n := range domotzCredentialSpecNames {
		want[n] = true
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
		if SecretFieldName(n) != want[n] {
			t.Errorf("Domotz spec name %q: SecretFieldName = %v, want %v", n, SecretFieldName(n), want[n])
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

// domotzCredentialSpecNames are the Domotz spec names that carry a credential:
// the X-Api-Key auth header; the SNMP v1/v2 community strings and the SNMP v3
// user, authentication_key and encryption_key (SNMPDomotzAuthentication); the
// device and custom-driver credentials object and its username and password
// (DeviceCredentials, CustomDriverAssociationCreation); a team leader's
// password (TeamCreation). Everything else is public: an inventory field's
// key (its name), an activity log's user, the VPN session id, and the
// authentication and credential metadata (authentication_status,
// authentication_type, required_authentication_type, authentication_protocol,
// requires_credentials), whose values (REQUIRED, MD5, true) would otherwise
// be withheld wherever they appear.
//
// Black-box class spec-credential-under-a-generic-name (domotz 4.34.2
// reprint): DeviceSnmpCommunity carries the SNMP v1/v2 read and write
// communities under the bare names read and write (PUT
// /agent/{agent_id}/device/{device_id}/snmp-community), which matched no rule,
// so `agent device set-snmp-community --read <community> --dry-run` and the
// domotz_execute preview printed the community.
var domotzCredentialSpecNames = []string{"X-Api-Key", "authentication_key", "credentials", "encryption_key", "password",
	"read", "snmp_read_community", "snmp_write_community", "username", "write"}

// credentialDescription matches a spec description that says its value is a
// credential (a community, a password, a secret, an API key, a token).
var credentialDescription = regexp.MustCompile(`(?i)(communit|passw|passphrase|secret|credential|api[ _-]?key|token|private key)`)

// describedPublicSpecNames are the spec names whose description mentions a
// credential but whose value is not one, each reviewed against spec.json: a
// credential's metadata (authentication_status, requires_credentials,
// snmp_status, a credentials scope enum, include_unrecoverable), and an API
// key's id and mnemonic name.
var describedPublicSpecNames = map[string]bool{
	"authentication_status": true, "id": true, "include_unrecoverable": true, "name": true,
	"requires_credentials": true, "scope": true, "snmp_status": true,
}

// TestDomotzSpecNameDescribedAsACredentialIsHeld is the class guard behind
// the pinned list above: a property or parameter the vendor DESCRIBES as a
// credential reads as a credential name, whatever it is called, unless it is
// a reviewed public name. A reprint over a spec that adds such a name fails
// here instead of printing its value.
func TestDomotzSpecNameDescribedAsACredentialIsHeld(t *testing.T) {
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
