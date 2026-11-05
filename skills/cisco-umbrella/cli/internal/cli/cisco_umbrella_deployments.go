// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// One evaluator per deployment record type, shared by hygiene (synced rows) and
// fleet health (live rows). Each check reads its own field: a missing, null, or
// mistyped field leaves only that check unchecked (listed by finding kind), and a
// record that is not a JSON object is unreadable. Nothing unchecked reads as clean.

import (
	"encoding/json"
	"strings"
	"time"
)

// Documented roaming computer status values (deployments roamingcomputers status/swgStatus patterns).
var (
	roamingStatuses = map[string]bool{"Off": true, "Open": true, "Transparent": true, "Encrypted": true, "VA": true, "Network": true,
		"Disabled": true, "Uninstalled": true, "TrustedCustomerNetwork": true, "CiscoTrustedNetwork": true, "DisabledDueToACVpnFullTunnel": true}
	swgStatuses = map[string]bool{"NA": true, "Protected": true, "Unprotected": true, "Disabled": true, "DisabledDueToVPN": true,
		"DisabledDueToTrustedNetwork": true, "ConfigError": true, "CloudServiceUnavailable": true}
)

// fieldString reads one string field; ok=false when absent, null, or not a string.
func fieldString(fields map[string]json.RawMessage, key string) (string, bool) {
	var s *string
	if raw, present := fields[key]; !present || json.Unmarshal(raw, &s) != nil || s == nil {
		return "", false
	}
	return *s, true
}

// fieldBool reads one boolean field; ok=false when absent, null, or not a boolean.
func fieldBool(fields map[string]json.RawMessage, key string) (bool, bool) {
	var b *bool
	if raw, present := fields[key]; !present || json.Unmarshal(raw, &b) != nil || b == nil {
		return false, false
	}
	return *b, true
}

func recordFields(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if decodeObject(raw, &fields) != nil || fields == nil {
		return nil, false
	}
	return fields, true
}

type roamingEval struct {
	Name           string
	Status         string
	Off            bool
	Uninstalled    bool
	SWGUnprotected bool
	Stale          bool
	LastSync       string   // "" when the API recorded no sync
	Unchecked      []string // finding kinds whose field was unusable
}

func evalRoaming(raw json.RawMessage, staleCutoff time.Time) (roamingEval, bool) {
	fields, ok := recordFields(raw)
	if !ok {
		return roamingEval{}, false
	}
	ev := roamingEval{}
	ev.Name, _ = fieldString(fields, "name")
	if status, ok := fieldString(fields, "status"); ok && roamingStatuses[status] {
		ev.Status = status
		switch status {
		case "Off", "Disabled", "DisabledDueToACVpnFullTunnel":
			ev.Off = true
		case "Uninstalled":
			ev.Uninstalled = true
		}
	} else {
		ev.Unchecked = append(ev.Unchecked, "roaming-off", "roaming-uninstalled")
	}
	if swg, ok := fieldString(fields, "swgStatus"); ok && swgStatuses[swg] {
		ev.SWGUnprotected = swg == "Unprotected"
	} else {
		ev.Unchecked = append(ev.Unchecked, "swg-unprotected")
	}
	switch raw, present := fields["lastSync"]; {
	case !present || strings.TrimSpace(string(raw)) == "null":
		ev.Stale = true // never synced counts as stale
	default:
		last, ok := fieldString(fields, "lastSync")
		if !ok {
			ev.Unchecked = append(ev.Unchecked, "roaming-stale")
			break
		}
		if strings.TrimSpace(last) == "" {
			ev.Stale = true
			break
		}
		t, err := time.Parse(time.RFC3339, last)
		if err != nil {
			ev.Unchecked = append(ev.Unchecked, "roaming-stale")
			break
		}
		ev.LastSync = t.UTC().Format("2006-01-02")
		ev.Stale = t.Before(staleCutoff)
	}
	return ev, true
}

type vaEval struct {
	ID         string
	Name       string
	Health     string
	NotHealthy bool
	Upgradable bool
	Unchecked  []string
}

func evalVA(raw json.RawMessage) (vaEval, bool) {
	fields, ok := recordFields(raw)
	if !ok {
		return vaEval{}, false
	}
	ev := vaEval{ID: rawID(fields["originId"])}
	ev.Name, _ = fieldString(fields, "name")
	if health, ok := fieldString(fields, "health"); ok && strings.TrimSpace(health) != "" {
		ev.Health = health
		ev.NotHealthy = !strings.EqualFold(health, "healthy")
	} else {
		ev.Unchecked = append(ev.Unchecked, "va-not-healthy")
	}
	if up, ok := fieldBool(fields, "isUpgradable"); ok {
		ev.Upgradable = up
	} else {
		ev.Unchecked = append(ev.Unchecked, "va-upgrade-available")
	}
	return ev, true
}

type tunnelEval struct {
	ID        string
	Down      bool
	Detail    string
	Unchecked []string
}

func evalTunnel(raw json.RawMessage) (tunnelEval, bool) {
	fields, ok := recordFields(raw)
	if !ok {
		return tunnelEval{}, false
	}
	ev := tunnelEval{ID: rawID(fields["tunnelId"])}
	status, ok := fieldString(fields, "status")
	if !ok || strings.TrimSpace(status) == "" {
		ev.Unchecked = append(ev.Unchecked, "tunnel-down")
		return ev, true
	}
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "UP":
	case "DOWN", "FAILED":
		ev.Down = true
	default:
		// UNKNOWN is "pending updated information" (spec.yaml): not a verdict.
		ev.Unchecked = append(ev.Unchecked, "tunnel-down")
		return ev, true
	}
	ike, _ := fieldString(fields, "ikeState")
	ipsec, _ := fieldString(fields, "ipsecState")
	dc, _ := fieldString(fields, "dcName")
	ev.Detail = "status " + status + ", IKE " + ike + ", IPsec " + ipsec + ", DC " + dc
	return ev, true
}

type networkEval struct {
	Name       string
	IPAddress  string
	Unverified bool
	Unchecked  []string
}

func evalNetwork(raw json.RawMessage) (networkEval, bool) {
	fields, ok := recordFields(raw)
	if !ok {
		return networkEval{}, false
	}
	ev := networkEval{}
	ev.Name, _ = fieldString(fields, "name")
	ev.IPAddress, _ = fieldString(fields, "ipAddress")
	if verified, ok := fieldBool(fields, "isVerified"); ok {
		ev.Unverified = !verified
	} else {
		ev.Unchecked = append(ev.Unchecked, "network-unverified")
	}
	return ev, true
}
