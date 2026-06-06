// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Package insights holds the pure, store-fed analytics that power the
// microsoft-graph-cli transcendence commands (licenses waste/orphans,
// admins audit, security triage, managed-devices drift, tenant snapshot).
//
// Every function takes already-fetched JSON rows (the `data` column of the
// local SQLite domain tables) and returns typed results. Keeping the logic
// here — separate from Cobra wiring — makes it unit-testable against fixtures
// without a live tenant, which is the whole point of a credential-less build.
package insights

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// ParseGraphTime parses a Microsoft Graph ISO-8601 timestamp (e.g.
// "2026-05-05T08:30:00.0000000Z"). Graph emits up to 7 fractional-second
// digits; Go's RFC3339 parser accepts a variable-length fraction, so a single
// layout covers both fractional and non-fractional forms. Returns ok=false on
// empty or unparseable input so callers can treat unknown times as "very old"
// or skip them, never as the zero time silently.
func ParseGraphTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// severityRank orders Graph alert severities for stable high-first sorting.
func severityRank(sev string) int {
	switch strings.ToLower(sev) {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	case "informational":
		return 3
	default:
		return 4
	}
}

// ---- licenses waste --------------------------------------------------------

// SkuWaste is one tenant SKU with its unused (paid-but-unconsumed) seats.
type SkuWaste struct {
	SkuPartNumber    string `json:"skuPartNumber"`
	SkuID            string `json:"skuId"`
	Enabled          int    `json:"enabledUnits"`
	Consumed         int    `json:"consumedUnits"`
	Unused           int    `json:"unusedUnits"`
	Suspended        int    `json:"suspendedUnits"`
	Warning          int    `json:"warningUnits"`
	CapabilityStatus string `json:"capabilityStatus,omitempty"`
}

type prepaidUnits struct {
	Enabled   int `json:"enabled"`
	Suspended int `json:"suspended"`
	Warning   int `json:"warning"`
}

type subscribedSku struct {
	SkuPartNumber    string       `json:"skuPartNumber"`
	SkuID            string       `json:"skuId"`
	ConsumedUnits    int          `json:"consumedUnits"`
	CapabilityStatus string       `json:"capabilityStatus"`
	PrepaidUnits     prepaidUnits `json:"prepaidUnits"`
}

// LicenseWaste returns the SKUs with at least one unused paid seat
// (prepaidUnits.enabled − consumedUnits > 0), ranked by unused seats
// descending. SKUs that are fully consumed are omitted — the command answers
// "where am I wasting spend", not "list every SKU".
func LicenseWaste(skus []json.RawMessage) []SkuWaste {
	out := []SkuWaste{}
	for _, raw := range skus {
		var s subscribedSku
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		unused := s.PrepaidUnits.Enabled - s.ConsumedUnits
		if unused <= 0 {
			continue
		}
		out = append(out, SkuWaste{
			SkuPartNumber:    s.SkuPartNumber,
			SkuID:            s.SkuID,
			Enabled:          s.PrepaidUnits.Enabled,
			Consumed:         s.ConsumedUnits,
			Unused:           unused,
			Suspended:        s.PrepaidUnits.Suspended,
			Warning:          s.PrepaidUnits.Warning,
			CapabilityStatus: s.CapabilityStatus,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Unused != out[j].Unused {
			return out[i].Unused > out[j].Unused
		}
		return out[i].SkuPartNumber < out[j].SkuPartNumber
	})
	return out
}

// SkuNameMap builds a skuId -> skuPartNumber lookup from subscribedSku rows so
// per-user license references (which carry only skuId) can be shown with their
// human SKU name.
func SkuNameMap(skus []json.RawMessage) map[string]string {
	m := map[string]string{}
	for _, raw := range skus {
		var s subscribedSku
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		if s.SkuID != "" && s.SkuPartNumber != "" {
			m[s.SkuID] = s.SkuPartNumber
		}
	}
	return m
}

// ---- licenses orphans ------------------------------------------------------

// OrphanAccount is a disabled or guest user that still holds paid licenses.
type OrphanAccount struct {
	UserPrincipalName string   `json:"userPrincipalName"`
	DisplayName       string   `json:"displayName"`
	AccountEnabled    bool     `json:"accountEnabled"`
	UserType          string   `json:"userType,omitempty"`
	Reason            string   `json:"reason"`
	LicenseCount      int      `json:"licenseCount"`
	Skus              []string `json:"skus,omitempty"`
}

type assignedLicense struct {
	SkuID string `json:"skuId"`
}

type userLite struct {
	UserPrincipalName string            `json:"userPrincipalName"`
	DisplayName       string            `json:"displayName"`
	AccountEnabled    *bool             `json:"accountEnabled"`
	UserType          string            `json:"userType"`
	AssignedLicenses  []assignedLicense `json:"assignedLicenses"`
}

// LicensedOrphans returns users that still hold at least one assigned license
// but are either disabled (accountEnabled=false) or guests (userType=Guest) —
// licenses an MSP is paying for that no active member is using. skuNames maps
// skuId -> skuPartNumber for friendlier output; pass nil to fall back to raw
// skuIds. Rows without assignedLicenses, or whose account state is unknown and
// not a guest, are skipped.
func LicensedOrphans(users []json.RawMessage, skuNames map[string]string) []OrphanAccount {
	out := []OrphanAccount{}
	for _, raw := range users {
		var u userLite
		if err := json.Unmarshal(raw, &u); err != nil {
			continue
		}
		if len(u.AssignedLicenses) == 0 {
			continue
		}
		disabled := u.AccountEnabled != nil && !*u.AccountEnabled
		guest := strings.EqualFold(u.UserType, "Guest")
		if !disabled && !guest {
			continue
		}
		reason := "guest"
		if disabled {
			reason = "disabled"
			if guest {
				reason = "disabled-guest"
			}
		}
		skus := make([]string, 0, len(u.AssignedLicenses))
		for _, l := range u.AssignedLicenses {
			if name, ok := skuNames[l.SkuID]; ok && name != "" {
				skus = append(skus, name)
			} else if l.SkuID != "" {
				skus = append(skus, l.SkuID)
			}
		}
		out = append(out, OrphanAccount{
			UserPrincipalName: u.UserPrincipalName,
			DisplayName:       u.DisplayName,
			AccountEnabled:    u.AccountEnabled != nil && *u.AccountEnabled,
			UserType:          u.UserType,
			Reason:            reason,
			LicenseCount:      len(u.AssignedLicenses),
			Skus:              skus,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LicenseCount != out[j].LicenseCount {
			return out[i].LicenseCount > out[j].LicenseCount
		}
		return out[i].UserPrincipalName < out[j].UserPrincipalName
	})
	return out
}

// ---- admins audit ----------------------------------------------------------

// AdminAssignment is one (role, member) pair from the privileged-access audit.
type AdminAssignment struct {
	Role              string `json:"role"`
	RoleID            string `json:"roleId,omitempty"`
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
	UserType          string `json:"userType,omitempty"`
	AccountEnabled    *bool  `json:"accountEnabled,omitempty"`
	Risk              string `json:"risk,omitempty"`
}

type roleMember struct {
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
	UserType          string `json:"userType"`
	AccountEnabled    *bool  `json:"accountEnabled"`
}

type directoryRole struct {
	ID          string       `json:"id"`
	DisplayName string       `json:"displayName"`
	Members     []roleMember `json:"members"`
}

// AdminsAudit flattens directory roles and their embedded members into one
// (role, member) table. Each role row is expected to carry a `members` array
// (the pull command embeds /directoryRoles/{id}/members into the role object).
// Risk is flagged for guest or disabled accounts that hold an admin role.
func AdminsAudit(roles []json.RawMessage) []AdminAssignment {
	out := []AdminAssignment{}
	for _, raw := range roles {
		var r directoryRole
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		for _, m := range r.Members {
			guest := strings.EqualFold(m.UserType, "Guest")
			disabled := m.AccountEnabled != nil && !*m.AccountEnabled
			risk := ""
			switch {
			case disabled && guest:
				risk = "disabled-guest-admin"
			case disabled:
				risk = "disabled-admin"
			case guest:
				risk = "guest-admin"
			}
			out = append(out, AdminAssignment{
				Role:              r.DisplayName,
				RoleID:            r.ID,
				DisplayName:       m.DisplayName,
				UserPrincipalName: m.UserPrincipalName,
				UserType:          m.UserType,
				AccountEnabled:    m.AccountEnabled,
				Risk:              risk,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].UserPrincipalName < out[j].UserPrincipalName
	})
	return out
}

// ---- security triage -------------------------------------------------------

// TriageBucket is a counted group (a severity or a service source).
type TriageBucket struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// TriageAlert is a single open alert in the triage window.
type TriageAlert struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Severity        string `json:"severity"`
	Status          string `json:"status"`
	ServiceSource   string `json:"serviceSource"`
	CreatedDateTime string `json:"createdDateTime"`
}

// TriageResult is the grouped triage view returned by SecurityTriage.
type TriageResult struct {
	Since           string         `json:"since"`
	TotalOpen       int            `json:"totalOpen"`
	BySeverity      []TriageBucket `json:"bySeverity"`
	ByServiceSource []TriageBucket `json:"byServiceSource"`
	Alerts          []TriageAlert  `json:"alerts"`
}

type alertLite struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Severity        string `json:"severity"`
	Status          string `json:"status"`
	ServiceSource   string `json:"serviceSource"`
	CreatedDateTime string `json:"createdDateTime"`
}

// openAlertStatus reports whether a status counts as an open (still-actionable)
// alert. new and inProgress are open; resolved/dismissed are not. Unknown
// statuses are treated as open so a new Graph status value never silently
// drops alerts from triage.
func openAlertStatus(status string) bool {
	switch strings.ToLower(status) {
	case "resolved", "dismissed", "closed":
		return false
	default:
		return true
	}
}

// SecurityTriage returns open alerts created at or after `since`, grouped and
// counted by severity and by service source. Alerts are sorted high-severity
// first, then most-recent first. An alert with an unparseable createdDateTime
// is included only when since is the zero time (no window), so a bad timestamp
// never silently hides a fresh alert under a real window.
func SecurityTriage(alerts []json.RawMessage, since time.Time) TriageResult {
	res := TriageResult{
		BySeverity:      []TriageBucket{},
		ByServiceSource: []TriageBucket{},
		Alerts:          []TriageAlert{},
	}
	if !since.IsZero() {
		res.Since = since.UTC().Format(time.RFC3339)
	}
	sevCounts := map[string]int{}
	srcCounts := map[string]int{}
	for _, raw := range alerts {
		var a alertLite
		if err := json.Unmarshal(raw, &a); err != nil {
			continue
		}
		if !openAlertStatus(a.Status) {
			continue
		}
		if !since.IsZero() {
			t, ok := ParseGraphTime(a.CreatedDateTime)
			if !ok || t.Before(since) {
				continue
			}
		}
		sev := a.Severity
		if sev == "" {
			sev = "unknown"
		}
		src := a.ServiceSource
		if src == "" {
			src = "unknown"
		}
		sevCounts[sev]++
		srcCounts[src]++
		res.Alerts = append(res.Alerts, TriageAlert(a))
	}
	res.TotalOpen = len(res.Alerts)
	res.BySeverity = bucketsBySeverity(sevCounts)
	res.ByServiceSource = bucketsByCount(srcCounts)
	sort.SliceStable(res.Alerts, func(i, j int) bool {
		ri, rj := severityRank(res.Alerts[i].Severity), severityRank(res.Alerts[j].Severity)
		if ri != rj {
			return ri < rj
		}
		return res.Alerts[i].CreatedDateTime > res.Alerts[j].CreatedDateTime
	})
	return res
}

func bucketsBySeverity(counts map[string]int) []TriageBucket {
	out := make([]TriageBucket, 0, len(counts))
	for k, v := range counts {
		out = append(out, TriageBucket{Key: k, Count: v})
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Key), severityRank(out[j].Key)
		if ri != rj {
			return ri < rj
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func bucketsByCount(counts map[string]int) []TriageBucket {
	out := make([]TriageBucket, 0, len(counts))
	for k, v := range counts {
		out = append(out, TriageBucket{Key: k, Count: v})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// ---- managed-devices drift -------------------------------------------------

// DriftDevice is one Intune device flagged for a compliance-drift reason.
type DriftDevice struct {
	DeviceName        string   `json:"deviceName"`
	UserPrincipalName string   `json:"userPrincipalName"`
	OperatingSystem   string   `json:"operatingSystem"`
	OsVersion         string   `json:"osVersion"`
	ComplianceState   string   `json:"complianceState"`
	LastSyncDateTime  string   `json:"lastSyncDateTime"`
	IsEncrypted       bool     `json:"isEncrypted"`
	Reasons           []string `json:"reasons"`
}

type managedDeviceLite struct {
	DeviceName        string `json:"deviceName"`
	UserPrincipalName string `json:"userPrincipalName"`
	OperatingSystem   string `json:"operatingSystem"`
	OsVersion         string `json:"osVersion"`
	ComplianceState   string `json:"complianceState"`
	LastSyncDateTime  string `json:"lastSyncDateTime"`
	IsEncrypted       *bool  `json:"isEncrypted"`
}

// noncompliantState reports whether a complianceState should be flagged. Only
// "compliant", "notApplicable", and "" (unknown/unsynced metadata) are treated
// as clean; everything else (noncompliant, error, conflict, inGracePeriod) is
// drift.
func noncompliantState(state string) bool {
	switch strings.ToLower(state) {
	case "compliant", "notapplicable", "":
		return false
	default:
		return true
	}
}

// DeviceDrift returns managed devices that are non-compliant, unencrypted, or
// have not checked in since `staleBefore`. Each device lists every reason it
// was flagged. Devices with no drift reason are omitted. A device with an
// unparseable lastSyncDateTime is treated as stale only when staleBefore is
// non-zero (a real window) — an explicit "we asked for staleness and can't
// prove freshness" flag, never a silent pass.
func DeviceDrift(devices []json.RawMessage, staleBefore time.Time) []DriftDevice {
	out := []DriftDevice{}
	for _, raw := range devices {
		var d managedDeviceLite
		if err := json.Unmarshal(raw, &d); err != nil {
			continue
		}
		var reasons []string
		if noncompliantState(d.ComplianceState) {
			reasons = append(reasons, "noncompliant")
		}
		if !staleBefore.IsZero() {
			t, ok := ParseGraphTime(d.LastSyncDateTime)
			if !ok || t.Before(staleBefore) {
				reasons = append(reasons, "stale")
			}
		}
		if d.IsEncrypted != nil && !*d.IsEncrypted {
			reasons = append(reasons, "unencrypted")
		}
		if len(reasons) == 0 {
			continue
		}
		out = append(out, DriftDevice{
			DeviceName:        d.DeviceName,
			UserPrincipalName: d.UserPrincipalName,
			OperatingSystem:   d.OperatingSystem,
			OsVersion:         d.OsVersion,
			ComplianceState:   d.ComplianceState,
			LastSyncDateTime:  d.LastSyncDateTime,
			IsEncrypted:       d.IsEncrypted != nil && *d.IsEncrypted,
			Reasons:           reasons,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Reasons) != len(out[j].Reasons) {
			return len(out[i].Reasons) > len(out[j].Reasons)
		}
		return out[i].DeviceName < out[j].DeviceName
	})
	return out
}

// ---- tenant snapshot -------------------------------------------------------

// Snapshot is the one-shot tenant posture rollup.
type Snapshot struct {
	Users                  int `json:"users"`
	EnabledUsers           int `json:"enabledUsers"`
	GuestUsers             int `json:"guestUsers"`
	Groups                 int `json:"groups"`
	PrivilegedAssignments  int `json:"privilegedRoleAssignments"`
	RiskyAdminAssignments  int `json:"riskyAdminAssignments"`
	LicenseSkus            int `json:"licenseSkus"`
	UnusedLicenseSeats     int `json:"unusedLicenseSeats"`
	OpenAlerts             int `json:"openAlerts"`
	HighSeverityOpenAlerts int `json:"highSeverityOpenAlerts"`
	ManagedDevices         int `json:"managedDevices"`
	NonCompliantDevices    int `json:"nonCompliantDevices"`
}

// TenantSnapshot aggregates every synced surface into a single posture summary.
// All inputs are the `data` JSON rows of the corresponding domain tables; any
// may be empty (a partial sync produces a partial-but-honest snapshot).
func TenantSnapshot(users, groups, licenses, roles, alerts, devices []json.RawMessage) Snapshot {
	var snap Snapshot

	snap.Users = len(users)
	for _, raw := range users {
		var u userLite
		if err := json.Unmarshal(raw, &u); err != nil {
			continue
		}
		if u.AccountEnabled != nil && *u.AccountEnabled {
			snap.EnabledUsers++
		}
		if strings.EqualFold(u.UserType, "Guest") {
			snap.GuestUsers++
		}
	}

	snap.Groups = len(groups)

	for _, w := range LicenseWaste(licenses) {
		snap.UnusedLicenseSeats += w.Unused
	}
	snap.LicenseSkus = len(licenses)

	for _, a := range AdminsAudit(roles) {
		snap.PrivilegedAssignments++
		if a.Risk != "" {
			snap.RiskyAdminAssignments++
		}
	}

	open := SecurityTriage(alerts, time.Time{})
	snap.OpenAlerts = open.TotalOpen
	for _, b := range open.BySeverity {
		if strings.EqualFold(b.Key, "high") {
			snap.HighSeverityOpenAlerts += b.Count
		}
	}

	snap.ManagedDevices = len(devices)
	for _, raw := range devices {
		var d managedDeviceLite
		if err := json.Unmarshal(raw, &d); err != nil {
			continue
		}
		if noncompliantState(d.ComplianceState) {
			snap.NonCompliantDevices++
		}
	}

	return snap
}
