// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"cisco-umbrella-pp-cli/internal/credkit"
)

// Class credential-field-not-recognized: a credential name matches in every
// spelling (camelCase, snake_case, kebab-case flags and headers, any case),
// and the public half of a credential pair (an API key's clientId, an access
// key ID, the S3 bucket key's IDs) counts as a credential; a resource identifier
// (an API key's own id) does not.
func TestCredentialNamesMatchInEverySpelling(t *testing.T) {
	for _, name := range []string{
		"clientSecret", "client_secret", "client-secret", "password", "psk", "key", "Key", "secretAccessKey", "currentKeyId", "oldKeyId",
		"clientId", "client_id", "client-id", "ClientID", "CLIENT_ID",
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
		"password_complexity", "passwordComplexity", "keyName", "deviceKey", "psk-id-prefix", "originId",
		"apiKeyId", "apiKeys", "token_type",
	} {
		if SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = true, want false", name)
		}
	}
}

// heldBy records what fn holds, and holds nothing in the kit: its held set is
// process-wide with no reset.
func heldBy(t *testing.T, fn func()) []string {
	t.Helper()
	var got []string
	prior := Hold
	Hold = func(_ credkit.Source, values ...string) { got = append(got, values...) }
	defer func() { Hold = prior }()
	fn()
	return got
}

// Class credential-field-not-recognized, by value: an AWS access key ID is
// the public half of the S3 bucket key pair whatever field carries it, and the S3
// key's IDs are credentials by name whatever shape they have.
func TestAccessKeyIDHeldByShapeAndName(t *testing.T) {
	const id = "AKIA" + "UNITTEST5Q2X9K4M"
	held := heldBy(t, func() {
		HoldCredentialValues(map[string]any{"rotation": map[string]any{"anyName": id, "currentKeyId": "BUCKETKEY" + "NOTAWSSHAPE01", "oldKeyId": "BUCKETKEY" + "NOTAWSSHAPE02", "note": "AKIA is not a key"}})
	})
	for _, v := range []string{id, "BUCKETKEY" + "NOTAWSSHAPE01", "BUCKETKEY" + "NOTAWSSHAPE02"} {
		if !slices.Contains(held, v) {
			t.Errorf("%s not held: %q", v, held)
		}
	}
	if slices.Contains(held, "AKIA is not a key") {
		t.Errorf("ordinary text held: %q", held)
	}
}

// Class credential-misread-as-mask: a word is never a mask. A password that is
// literally "hidden" is held; only asterisks, empty values and this program's own
// redaction marker are skipped, by shape.
func TestAWordIsNeverAMask(t *testing.T) {
	held := heldBy(t, func() {
		HoldCredentialValues(map[string]any{"password": "hidden", "secret": "masked", "psk": "withheld", "token": "redacted", "clientSecret": "****", "clientId": "[redacted]", "secretKey": ""})
	})
	for _, v := range []string{"hidden", "masked", "withheld", "redacted"} {
		if !slices.Contains(held, v) {
			t.Errorf("the credential %q was taken for a mask (held %q)", v, held)
		}
	}
	for _, v := range []string{"****", "[redacted]", ""} {
		if slices.Contains(held, v) {
			t.Errorf("the mask %q was held", v)
		}
	}
}

// The spec's only "key" wraps a whole API key record, so an object under it is
// walked by name: the record's name and id stay readable and unheld, its
// credentials are held and replaced. A string-valued key is a credential.
func TestKeyWrapperIsARecordAndAStringKeyIsACredential(t *testing.T) {
	body := []byte(`{"key":{"id":"wrapperId-1","name":"Reports RO","clientId":"wrapperClient-2","clientSecret":"wrapperSecret-3"}}`)
	held := heldBy(t, func() { HoldCredentialFields(body) })
	if !slices.Contains(held, "wrapperClient-2") || !slices.Contains(held, "wrapperSecret-3") {
		t.Errorf("the record's credentials were not held: %q", held)
	}
	if slices.Contains(held, "Reports RO") || slices.Contains(held, "wrapperId-1") {
		t.Errorf("the record's own name or id was held: %q", held)
	}
	out := string(RedactBody(body))
	for _, v := range []string{"wrapperClient-2", "wrapperSecret-3"} {
		if strings.Contains(out, v) {
			t.Errorf("%s not replaced: %s", v, out)
		}
	}
	if !strings.Contains(out, "Reports RO") || !strings.Contains(out, "wrapperId-1") {
		t.Errorf("the record lost its name or id: %s", out)
	}
	str := []byte(`{"id":"k1","key":"stringKeyValue-4"}`)
	if held := heldBy(t, func() { HoldCredentialFields(str) }); !slices.Contains(held, "stringKeyValue-4") {
		t.Errorf("a string-valued key was not held: %q", held)
	}
	if out := string(RedactBody(str)); strings.Contains(out, "stringKeyValue-4") {
		t.Errorf("a string-valued key was not replaced: %s", out)
	}
}

// Every member of a duplicate key is held: encoding/json keeps only the last.
func TestDuplicateMembersAreAllHeld(t *testing.T) {
	held := heldBy(t, func() {
		HoldCredentialFields([]byte(`{"CLIENT-SECRET":"dupFirst-Zq41","CLIENT-SECRET":"dupLast-Wx93"}`))
	})
	for _, v := range []string{"dupFirst-Zq41", "dupLast-Wx93"} {
		if !slices.Contains(held, v) {
			t.Errorf("%s not held: %q", v, held)
		}
	}
}

// RedactBody leaves a body with nothing to redact byte for byte, keeps a number's
// spelling, and leaves text that is not JSON alone.
func TestRedactBodyChangesOnlyWhatItMust(t *testing.T) {
	plain := []byte("{ \"name\" : \"x\",\n \"n\": 12345678901234567890 }")
	if got := RedactBody(plain); string(got) != string(plain) {
		t.Errorf("a body with no credential changed: %s", got)
	}
	if got := string(RedactBody([]byte(`{"n":12345678901234567890,"password":"pw-unit-1"}`))); !strings.Contains(got, "12345678901234567890") || strings.Contains(got, "pw-unit-1") {
		t.Errorf("redacted body: %s", got)
	}
	if got := RedactBody([]byte("not json")); string(got) != "not json" {
		t.Errorf("text changed: %s", got)
	}
}

// specNames is every field and parameter name the spec gives this connector: the
// JSON tags of internal/types (generated from it) and the tool parameters of the
// press manifest. The spec file itself stays in the library: its vendor examples
// trip secret scanners.
func specNames(t *testing.T) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	src, err := os.ReadFile(filepath.Join("..", "types", "types.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile("json:\"([^\",]+)").FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "press", "tools-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Tools []struct {
			Params []struct {
				Name string `json:"name"`
			} `json:"params"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, tool := range manifest.Tools {
		for _, p := range tool.Params {
			seen[p.Name] = true
		}
	}
	if len(seen) < 300 {
		t.Fatalf("read only %d names from the spec; the scan is broken", len(seen))
	}
	return seen
}

// credentialVocabulary is wider than the rule: anything that could carry a
// credential gets a decision, here or in the rule.
var credentialVocabulary = regexp.MustCompile(`(?i)(pass|secret|cred|token|key|psk|auth|bearer|cert|sign|hash|session|cookie|private)`)

// publicSpecNames are spec names with credential vocabulary that are ordinary
// data, each with the reason.
var publicSpecNames = map[string]string{
	"apiKeyId":                            "an API key's own id, in a request path",
	"apiKeys":                             "a query filter naming API key ids",
	"authentication":                      "an object: its members are judged by name",
	"authentication-parameters-id-prefix": "the public prefix of a tunnel's IKE id",
	"authentication-type":                 "an enum: PSK or similar",
	"client-authentication-parameters":    "a JSON object flag: its members are judged by name",
	"creatorKeyId":                        "the id of the key that created a record",
	"deprecateCurrentKeys":                "a boolean on S3 key rotation",
	"deviceKey":                           "a device's public identifier, used in paths",
	"hash":                                "a file's SHA in security activity",
	"imageKey":                            "a storage name for a logo image",
	"keyCreationDate":                     "a timestamp",
	"keys":                                "an array of records: their members are judged by name",
	"privateapplicationgroup":             "a resource name",
	"psk-id-prefix":                       "the public prefix of a tunnel's IKE id",
	"sessionid":                           "a reporting session number, not an auth session",
	"signature":                           "an IPS signature record",
	"signaturelist":                       "an IPS signature list record",
	"signaturelistids":                    "IPS signature list ids",
	"signatures":                          "IPS signature records",
	"startKey":                            "a pagination cursor",
}

// Class credential-field-not-recognized, closed for the whole vendor schema: every
// name in the spec that looks like it could carry a credential is either
// recognized by the one rule or reviewed as public. A name a reprint's spec adds
// fails here until it is judged.
func TestEverySpecNameThatCouldCarryACredentialIsJudged(t *testing.T) {
	for n := range specNames(t) {
		if !credentialVocabulary.MatchString(n) || SecretFieldName(n) {
			continue
		}
		if _, ok := publicSpecNames[n]; !ok {
			t.Errorf("spec name %q may carry a credential: add it to the rule (umbrella_secrets.go) or review it into publicSpecNames", n)
		}
	}
	for n := range publicSpecNames {
		if SecretFieldName(n) {
			t.Errorf("publicSpecNames lists %q, but the rule matches it", n)
		}
	}
}
