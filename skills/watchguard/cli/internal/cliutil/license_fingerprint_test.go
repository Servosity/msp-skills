// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json license-keys-fingerprinted.

package cliutil

import (
	"encoding/json"
	"strings"
	"testing"
)

// A software license key in serialOrLicense is replaced by a one-way
// fingerprint before it can be persisted; a Firebox or access point serial
// number (the row's own productName says which) is kept as-is.
func TestLicenseKeysFingerprinted(t *testing.T) {
	const key = "SENT-LICENSE-KEY-0001"
	soft, changed := RedactSecretFields(json.RawMessage(`{"productName":"endPoint","allocationType":"linked_to_license","serialOrLicense":"` + key + `","quantityTotal":5}`))
	if !changed || strings.Contains(string(soft), key) || !strings.Contains(string(soft), `"serialOrLicense":"`+LicenseFingerprintPrefix) {
		t.Fatalf("software license key persisted: changed=%v %s", changed, soft)
	}
	again, _ := RedactSecretFields(soft)
	if string(again) != string(soft) {
		t.Fatalf("fingerprint is not stable on re-redaction: %s vs %s", again, soft)
	}
	unknown, _ := RedactSecretFields(json.RawMessage(`{"serialOrLicense":"` + key + `"}`))
	if strings.Contains(string(unknown), key) {
		t.Fatalf("row without a productName kept the key: %s", unknown)
	}
	for _, product := range []string{"firebox", "fireboxRetention", "accessPoint"} {
		hw := json.RawMessage(`{"productName":"` + product + `","serialOrLicense":"80B50000SERIAL","modelName":"T45"}`)
		got, changed := RedactSecretFields(hw)
		if changed || string(got) != string(hw) {
			t.Fatalf("%s serial was altered: %s", product, got)
		}
	}
	snake := map[string]any{"product_name": "authPoint", "serial_or_license": key}
	if !FingerprintLicenseKeys(snake) || snake["serial_or_license"] == key {
		t.Fatalf("snake_case row kept the key: %v", snake)
	}
}
