// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/cisco-meraki/handfixes.json
// under "meraki-mirror-redacts-secrets".

package store

import (
	"encoding/json"
	"path/filepath"
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
	rows, err := db.DB().Query(`SELECT data FROM resources`)
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
	var typed string
	_ = db.DB().QueryRow(`SELECT data FROM devices`).Scan(&typed)
	all.WriteString(typed)
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
