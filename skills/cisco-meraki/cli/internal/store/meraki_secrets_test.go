// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-mirror-redacts-secrets".

package store

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const merakiSentinel = "SENTINEL-SECRET-9f3c"

// TestMirrorNeverStoresMerakiSecrets drives every write funnel (batch, generic
// and typed upserts) with a payload carrying credentials and greps the whole
// database file for the sentinel. Non-secret fields must survive.
func TestMirrorNeverStoresMerakiSecrets(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ssid := json.RawMessage(`{"number":0,"networkId":"L_1","name":"Front Desk","psk":"` + merakiSentinel + `","radiusServers":[{"host":"10.0.0.5","secret":"` + merakiSentinel + `"}],"ldap":{"credentials":{"distinguishedName":"cn=ap","password":"` + merakiSentinel + `"}},"gre":{"key":5},"id":"L_1:0"}`)
	if _, _, err := db.UpsertBatch("networks_wireless_ssids", []json.RawMessage{ssid}); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert("webhooks", "w1", json.RawMessage(`{"id":"w1","url":"https://x","sharedSecret":"`+merakiSentinel+`"}`)); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertDevices(json.RawMessage(`{"serial":"Q2XX-1","name":"mx","snmp":{"v3AuthPass":"` + merakiSentinel + `","communityString":"` + merakiSentinel + `"},"umbrella":{"key":"` + merakiSentinel + `"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("licenses", []json.RawMessage{json.RawMessage(`{"key":"Z2LICENSEKEEP","mode":"addDevices","counts":[]}`)}); err != nil {
		t.Fatal(err)
	}
	// A generated API key has no other identity; it must never become the row id.
	_, _, _ = db.UpsertBatch("administered", []json.RawMessage{json.RawMessage(`{"key":"` + merakiSentinel + `"}`)})
	rows, err := db.DB().Query(`SELECT id || resource_type || data FROM resources`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var all strings.Builder
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		all.WriteString(d)
	}
	var typed, ids string
	_ = db.DB().QueryRow(`SELECT data FROM devices`).Scan(&typed)
	all.WriteString(typed)
	_ = db.DB().QueryRow(`SELECT group_concat(id) FROM resources_fts`).Scan(&ids)
	all.WriteString(ids)
	got := all.String()
	if strings.Contains(got, merakiSentinel) {
		t.Fatalf("mirror stored a credential: %s", got)
	}
	for _, keep := range []string{"Front Desk", "10.0.0.5", "cn=ap", `"key":5`, "Z2LICENSEKEEP", RedactedSecret} {
		if !strings.Contains(got, keep) {
			t.Fatalf("redaction dropped non-secret %q: %s", keep, got)
		}
	}
}

// TestChangeLogValuesNeverCarrySecrets: an API change logs its request body
// as a JSON string, and a UI change can carry a secret-labelled value; both
// are redacted in the mirror and in the fleet changes / blame output twin.
func TestChangeLogValuesNeverCarrySecrets(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := []json.RawMessage{
		json.RawMessage(`{"ts":"2026-10-01T00:00:00Z","networkId":"N_1","page":"via API","label":"PUT /api/v1/networks/N_1/wireless/ssids/0","oldValue":"{\"name\":\"Front Desk\",\"psk\":\"` + merakiSentinel + `\"}","newValue":"{\"name\":\"Front Desk\"}"}`),
		json.RawMessage(`{"ts":"2026-10-01T00:00:01Z","networkId":"N_1","page":"Access control","label":"Pre-shared key","oldValue":"` + merakiSentinel + `","newValue":"` + merakiSentinel + `"}`),
	}
	if _, _, err := db.UpsertBatch("configuration_changes", rows); err != nil {
		t.Fatal(err)
	}
	var all string
	_ = db.DB().QueryRow(`SELECT group_concat(id || data) FROM resources`).Scan(&all)
	if strings.Contains(all, merakiSentinel) || !strings.Contains(all, "Front Desk") {
		t.Fatalf("change-log secret stored or non-secret lost: %s", all)
	}
	if got := RedactMerakiChangeValue("PUT /api/v1/networks/N_1/snmp", `{"communityString":"`+merakiSentinel+`"}`); strings.Contains(got, merakiSentinel) {
		t.Fatalf("output twin leaked: %s", got)
	}
	if got := RedactMerakiChangeValue("RADIUS secret", merakiSentinel); got != RedactedSecret {
		t.Fatalf("labelled secret not redacted: %s", got)
	}
	if got := RedactMerakiChangeValue("Name", "HQ"); got != "HQ" {
		t.Fatalf("plain value changed: %s", got)
	}
	if got := RedactMerakiChangeValue("Password expiration", "90 days"); got != "90 days" {
		t.Fatalf("non-secret label blanked: %s", got)
	}
	if got := RedactMerakiChangeValue("Access policy: RADIUS password", merakiSentinel); got != RedactedSecret {
		t.Fatalf("password under a policy-named page not redacted: %s", got)
	}
	for _, body := range []string{
		`{"name":"Lobby","psk":"` + merakiSentinel + `","psk":""}`,
		`{"name":"Lobby","psk":"` + merakiSentinel + `","psk":"[redacted]"}`,
		`{"name":"Lobby","psk":"` + merakiSentinel + `","psk":5}`,
		`{"name":"Lobby","\u0070sk":"` + merakiSentinel,
		`{"radiusServers":[{"secret":"` + merakiSentinel + `"}],"radiusServers":[]}`,
		`{"ldap":{"credentials":{"password":"` + merakiSentinel + `"}},"ldap":null}`,
		`[{"a":{"psk":"` + merakiSentinel + `"},"a":{}}]`,
		`{"name":"Lobby"}{"psk":"` + merakiSentinel + `"}`,
		`{"md5AuthenticationKey":{"id":1234,"passphrase":"` + merakiSentinel + `"}}`,
		`{"parsedToken":{"orgName":"Acme","hashedToken":"` + merakiSentinel + `"}}`,
		`{"n":1e400,"radiusServers":[{"secret":"` + merakiSentinel + `"}],"radiusServers":[]}`,
		`{"name":"ServiceNow","headers":[{"name":"Authorization","template":"Basic ` + merakiSentinel + `"}]}`,
		`{"psk":["` + merakiSentinel + `"]}`,
		`{"token":{"value":{"deeper":"` + merakiSentinel + `"}}}`,
		`{"label":"RADIUS secret","oldValue":["` + merakiSentinel + `"]}`,
		`{"headers":[{"name":"Authorization","template":"Basic ` + merakiSentinel + `"}],"body":"{\"text\":\"aaaa`,
		`{"headers":[{"template":"Basic ` + merakiSentinel + `","name":"Authorization"},`,
		`{"name":"Pushover","body":"{\"token\":\"` + merakiSentinel + `\",\"user\":\"u1\",\"message\":\"{{alertType}}`,
		`{"headers":[{"template":"` + merakiSentinel + `","name":"X-Api-Ke`,
		`{"name":"PagerDuty","body":"{\"routing_key\":\"` + merakiSentinel + `\",\"event_action\":\"trigger\"}"}`,
		`{"api_key":"` + merakiSentinel + `","access_token":"` + merakiSentinel + `","accessToken":"` + merakiSentinel + `","privateKey":"` + merakiSentinel + `"}`,
		`{"community_string":"` + merakiSentinel + `","v3_auth_pass":"` + merakiSentinel + `"}`,
		`{"list":["{\"token\":\"` + merakiSentinel + `\"}"]}`,
		`{"headers":[{"name":"x-functions-key","template":"` + merakiSentinel + `"},{"name":"Ocp-Apim-Subscription-Key","value":"` + merakiSentinel + `"},{"name":"X-Auth-Key","template":"` + merakiSentinel + `"}]}`,
	} {
		if got := RedactMerakiChangeValue("PUT /api/v1/networks/N_1/wireless/ssids/0", body); strings.Contains(got, merakiSentinel) {
			t.Fatalf("change-log body leaked a secret: %s -> %s", body, got)
		}
		if got := string(RedactMerakiSecrets(json.RawMessage(`{"oldValue":` + strconv.Quote(body) + `}`))); strings.Contains(got, merakiSentinel) {
			t.Fatalf("mirror kept a secret: %s -> %s", body, got)
		}
		if got := string(RedactMerakiSecrets(json.RawMessage(body))); strings.Contains(got, merakiSentinel) {
			t.Fatalf("mirror row kept a secret: %s -> %s", body, got)
		}
	}
	if got := RedactMerakiChangeValue("Group policy: PSK", merakiSentinel); got != RedactedSecret {
		t.Fatalf("policy-worded PSK label not redacted: %s", got)
	}
	if got := RedactMerakiChangeValue("Pre-shared key (PSK)", merakiSentinel); got != RedactedSecret {
		t.Fatalf("qualified secret label not redacted: %s", got)
	}
	if got := RedactMerakiChangeValue("PUT /api/v1/networks/N_1/wireless/ssids/0", `{"psk":"`+merakiSentinel+`\`); strings.Contains(got, merakiSentinel) {
		t.Fatalf("truncated body ending in a backslash leaked: %s", got)
	}
	if got := RedactMerakiChangeValue("PUT /api/v1/networks/N_1/wireless/ssids/0", `{"name":"Lobby","psk":"`+merakiSentinel); got != RedactedCutOff {
		t.Fatalf("cut-off JSON body not withheld whole: %s", got)
	}
	for _, name := range []string{"[Default] Guest", `[Cedar] "Clinic"`, "{{alertType}}", "[1, 2"} {
		if got := RedactMerakiChangeValue("SSID name", name); got != name {
			t.Fatalf("non-JSON value treated as JSON: %s -> %s", name, got)
		}
	}
	// Webhook bodies are free-form templates: withheld whole by label.
	if got := RedactMerakiChangeValue("PUT /api/v1/networks/N_1/webhooks/payloadTemplates/wpt_1", `{"name":"Push","body":"token=`+merakiSentinel+`&user=u1"}`); got != RedactedSecret {
		t.Fatalf("webhook change value not withheld: %s", got)
	}
	// headersFile/bodyFile are base64; the secret never appears in clear.
	headers := base64.StdEncoding.EncodeToString([]byte(`{"Authorization":"Bearer ` + merakiSentinel + `"}`))
	if got := RedactMerakiChangeValue("PUT /api/v1/networks/N_1/webhooks/payloadTemplates/wpt_1", `{"name":"Ticketing","headersFile":"`+headers+`","bodyFile":"`+headers+`"}`); strings.Contains(got, headers) {
		t.Fatalf("base64 webhook headers kept: %s", got)
	}
}

// TestRedactKeepsCleanJSONByteForByte: a key reused in sibling or nested
// objects is not a duplicate, so secret-free rows keep their original bytes.
// The {"key":{...}} fixture is already in sorted key order on purpose: a
// secret-named key forces a sorted re-encode, which must not change data.
func TestRedactKeepsCleanJSONByteForByte(t *testing.T) {
	for _, body := range []string{
		`{"z":1,"a":{"a":[{"a":2},{"a":3}]},"b":[]}`,
		`[{"name":"x"},{"name":"y"}]`,
		`{"id":12345678901234567890,"tags":["a","a"]}`,
		`{"headers":[{"name":"Content-Type","template":"application/json"}],"details":[{"name":"Model","value":"MX68"}]}`,
		`{"key":{"created":true},"n":1e400}`,
		`{"licenseKey":"Z2KEEP","name":"[Cedar] \"Clinic\""}`,
	} {
		if got := string(RedactMerakiSecrets(json.RawMessage(body))); got != body {
			t.Fatalf("clean JSON rewritten: %s -> %s", body, got)
		}
	}
	if !merakiHasDuplicateKey([]byte(`[{"a":{"b":1,"b":2}}]`)) {
		t.Fatal("nested duplicate key not detected")
	}
}
