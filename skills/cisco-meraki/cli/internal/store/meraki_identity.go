// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import "strings"

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
					return s
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
