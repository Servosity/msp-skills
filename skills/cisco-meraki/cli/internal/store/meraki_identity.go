// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// merakiIdentityGroups is hand-wired (handfixes.json: meraki-sync-identity-fallback).
// Many Meraki org-level reads (VPN statuses, uplink statuses, packet loss by
// device, usage by network and SSID, wireless-controller views, sensor
// readings) return rows with no id field at all: the identity is the network,
// device, SSID or client the row describes. The generated extractor only knows
// id-like fields, so every such row failed extraction and sync exited 1. When
// nothing else resolves, the row's identity is composed from these fields, in
// this order: scope first, then the entity, then a timestamp for history rows.
// The first part is never a timestamp, so a composite never reads as a date.
var merakiIdentityGroups = [][]string{
	{"network.id", "networkId"},
	{"cluster.id"},
	{"policy.id"},
	{"profile.id"},
	{"ssid.number", "ssid.name"},
	merakiDeviceIdentity,
	{"gateway.serial", "controller.serial"},
	{"client.id", "client.mac"},
	{"port.interface.name", "port.interface"}, // interface is an object on MX port rows
	{"interfaceId", "ruleGroupId", "policyId"},
	{"adminId"},
	{"method"},
	{"path"},
	{"metric"},
	{"alertType"},
	{"url"},
}

var merakiDeviceIdentity = []string{"serial", "deviceSerial", "device.serial", "sensor.serial"}

var merakiIdentityTimeFields = []string{"startTs", "ts", "sentAt", "createdAt", "loggedAt"}

// merakiIdentityFallback returns "" when the row carries no identity field, so
// a row with nothing but a timestamp still fails extraction honestly. With
// requireDevice, it answers only for rows that name a device serial: those run
// ahead of the generic display-name fallback, because two devices can share a
// name but never a serial.
func merakiIdentityFallback(obj map[string]any, requireDevice bool) string {
	parts := make([]string, 0, 4)
	pick := func(keys []string) string {
		for _, key := range keys {
			if v, ok := lookupRawFieldValue(obj, key); ok {
				// Unlike the composite override path, 0 is a real value here
				// (SSID number 0 is the first SSID of every network).
				if s := strings.TrimSpace(ResourceIDString(v)); s != "" && s != "<nil>" {
					// Scrubbed per part, so a receiver hash never swallows
					// the parts (the timestamp) joined after it.
					return ScrubMerakiURLSecrets(s)
				}
			}
		}
		return ""
	}
	if requireDevice && pick(merakiDeviceIdentity) == "" {
		return ""
	}
	for _, group := range merakiIdentityGroups {
		if s := pick(group); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if s := pick(merakiIdentityTimeFields); s != "" {
		parts = append(parts, s)
	}
	return CanonicalResourceID(joinCompositeResourceID(parts))
}

// merakiNullableCompositeKeys is hand-wired (handfixes.json:
// meraki-nullable-composite-key). The generated composite key needs every
// part, but Meraki leaves some null: a change made by Meraki support or an
// automated process has no adminId, an org-level change has no networkId, and
// policy is not required on intrusion rules. The composite then failed and a
// generic fallback picked a colliding key (a timestamp without page or label;
// the Snort gid shared by nearly every rule). These keys keep a missing part as
// an empty slot, so the key stays positional. The first field is required.
var merakiNullableCompositeKeys = map[string][]string{
	"configuration_changes":                            {"ts", "networkId", "ssidNumber", "adminId", "page", "label", "client.id"},
	"organizations_appliance_security_intrusion_rules": {"ruleId", "policy.id"},
	// An API request log row is an immutable record: every request field is
	// its identity (two requests in the same second differ by query string).
	"api_requests": {"ts", "adminId", "method", "host", "path", "queryString", "sourceIp", "userAgent", "responseCode", "client.id", "operationId"},
}

func merakiNullableCompositeID(resourceType string, obj map[string]any) string {
	fields, ok := merakiNullableCompositeKeys[resourceType]
	if !ok {
		return ""
	}
	values := make([]string, len(fields))
	for i, field := range fields {
		if v, ok := lookupRawFieldValue(obj, field); ok {
			if s := strings.TrimSpace(ResourceIDString(v)); s != "<nil>" {
				values[i] = ScrubMerakiURLSecrets(s)
			}
		}
	}
	if values[0] == "" {
		return ""
	}
	if resourceType == "configuration_changes" {
		values = append(values, merakiChangeValueFingerprint(obj))
	}
	return CanonicalResourceID(joinCompositeResourceID(values))
}

// merakiChangeValueFingerprint keeps two changes to one setting in the same
// microsecond apart ({"policy":"deny"} then {"policy":"allow"}). It hashes the
// REDACTED old and new values, so no credential (a PSK a dictionary attack
// could test against a hash) enters the identity; two changes that differ only
// in a secret still share a row, and that row shows nothing either would.
func merakiChangeValueFingerprint(obj map[string]any) string {
	label, _ := obj["label"].(string)
	str := func(k string) string {
		v, ok := obj[k]
		if !ok || v == nil {
			return ""
		}
		if s, ok := v.(string); ok {
			return s
		}
		return ResourceIDString(v)
	}
	sum := sha256.Sum256([]byte(RedactMerakiChangeValue(label, str("oldValue")) + "\x00" + RedactMerakiChangeValue(label, str("newValue"))))
	return "v:" + hex.EncodeToString(sum[:8])
}
