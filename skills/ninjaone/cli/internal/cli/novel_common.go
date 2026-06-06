// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// Shared helpers for the hand-built transcendence commands (patch-compliance,
// backup-coverage, av-sweep, fleet-health, stale-devices, software-audit,
// drift, os-eol). These commands read the local SQLite store the `sync`
// command populates and compute fleet-wide answers no single NinjaOne API
// call returns. They are intentionally defensive about JSON field names so a
// schema variation in the upstream report shape degrades to a missing value
// rather than a crash, and they treat an empty store as "no data yet" (exit 0)
// rather than an error.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ninjaone-pp-cli/internal/store"
)

// Resource-type keys as written into the generic `resources` table by `sync`.
const (
	rtDevices         = "devices-detailed"
	rtOrgs            = "organizations-detailed"
	rtOsPatches       = "queries-os-patches"
	rtSoftwarePatches = "queries-software-patches"
	rtBackupUsage     = "queries-backup-usage"
	rtAVThreats       = "queries-antivirus-threats"
	rtAVStatus        = "queries-antivirus-status"
	rtSoftware        = "queries-software"
	rtOperatingSys    = "queries-operating-systems"
	rtDeviceHealth    = "queries-device-health"
)

// openNovelStore opens the local store read/write (drift needs writes) with a
// friendly error pointing at `sync`.
func openNovelStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("ninjaone-cli")
	}
	db, err := store.OpenWithContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'ninjaone-cli sync' first.", err)
	}
	return db, nil
}

// loadRows returns every synced row of a resource type, decoded to maps. A
// missing table or zero rows returns an empty slice and no error.
func loadRows(db *store.Store, resourceType string) ([]map[string]any, error) {
	rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		// A brand-new store without the resources table reads as "no rows";
		// any other query failure (locked/corrupt DB) must surface rather
		// than masquerade as an empty store.
		if novelMissingTable(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(data, &obj); err != nil {
			continue
		}
		out = append(out, obj)
	}
	return out, rows.Err()
}

// novelMissingTable reports whether err is SQLite's missing-table error,
// the one query failure that legitimately means "nothing synced yet".
// Deliberately NOT reusing the generated syncHintMissingTable twin: hand-built
// novel code must not couple to a generated file a reprint overwrites.
func novelMissingTable(err error) bool {
	for err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

// nvStr returns the first present key as a trimmed string ("" if none).
func nvStr(obj map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := obj[k]; ok && v != nil {
			return strings.TrimSpace(nvScalarString(v))
		}
	}
	return ""
}

// nvScalarString stringifies a scalar JSON value, rendering whole-number
// float64 IDs without a trailing ".0" so device/org IDs join cleanly.
func nvScalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%t", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// nvFloat returns the first present key parsed as a float.
func nvFloat(obj map[string]any, keys ...string) (float64, bool) {
	for _, k := range keys {
		if v, ok := obj[k]; ok && v != nil {
			switch t := v.(type) {
			case float64:
				return t, true
			case string:
				var f float64
				if _, err := fmt.Sscanf(strings.TrimSpace(t), "%g", &f); err == nil {
					return f, true
				}
			}
		}
	}
	return 0, false
}

// nvBool returns the first present key as a bool.
func nvBool(obj map[string]any, keys ...string) (bool, bool) {
	for _, k := range keys {
		if v, ok := obj[k]; ok && v != nil {
			switch t := v.(type) {
			case bool:
				return t, true
			case string:
				return strings.EqualFold(t, "true"), true
			}
		}
	}
	return false, false
}

// nvEpoch returns the first present key as a time. Handles Unix epoch seconds
// (NinjaOne's lastContact is fractional epoch seconds) and RFC3339 strings.
func nvEpoch(obj map[string]any, keys ...string) (time.Time, bool) {
	for _, k := range keys {
		if v, ok := obj[k]; ok && v != nil {
			switch t := v.(type) {
			case float64:
				if t <= 0 {
					return time.Time{}, false
				}
				sec := int64(t)
				nsec := int64((t - float64(sec)) * 1e9)
				return time.Unix(sec, nsec).UTC(), true
			case string:
				if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(t)); err == nil {
					return ts.UTC(), true
				}
			}
		}
	}
	return time.Time{}, false
}

// nvNested descends one level into an object value (e.g. os.name).
func nvNested(obj map[string]any, parent string, childKeys ...string) string {
	if v, ok := obj[parent]; ok {
		if child, ok := v.(map[string]any); ok {
			return nvStr(child, childKeys...)
		}
	}
	return ""
}

// nvDevice is the normalized device record used to join report rows to their
// owning organization, location, name, OS, and last-contact time.
type nvDevice struct {
	ID          string
	Name        string
	OrgID       string
	LocationID  string
	OSName      string
	LastContact time.Time
	HasContact  bool
	Offline     bool
}

// buildDeviceIndex loads devices-detailed into an id-keyed map.
func buildDeviceIndex(db *store.Store) (map[string]nvDevice, error) {
	rows, err := loadRows(db, rtDevices)
	if err != nil {
		return nil, err
	}
	idx := make(map[string]nvDevice, len(rows))
	for _, r := range rows {
		id := nvStr(r, "id", "deviceId", "uid")
		if id == "" {
			continue
		}
		d := nvDevice{
			ID:         id,
			Name:       nvStr(r, "systemName", "dnsName", "displayName", "name", "hostname"),
			OrgID:      nvStr(r, "organizationId", "organisationId", "orgId"),
			LocationID: nvStr(r, "locationId"),
			OSName:     nvNested(r, "os", "name", "displayName"),
		}
		if d.Name == "" {
			d.Name = id
		}
		if d.OSName == "" {
			d.OSName = nvStr(r, "osName")
		}
		if t, ok := nvEpoch(r, "lastContact", "lastContactTime", "lastSeen"); ok {
			d.LastContact = t
			d.HasContact = true
		}
		if off, ok := nvBool(r, "offline"); ok {
			d.Offline = off
		}
		idx[id] = d
	}
	return idx, nil
}

// buildOrgNames maps organization id -> display name.
func buildOrgNames(db *store.Store) (map[string]string, error) {
	rows, err := loadRows(db, rtOrgs)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(rows))
	for _, r := range rows {
		id := nvStr(r, "id", "organizationId", "orgId")
		if id == "" {
			continue
		}
		names[id] = nvStr(r, "name", "displayName")
	}
	return names, nil
}

// orgLabel returns "name (id)" when a name is known, else the bare id.
func orgLabel(orgNames map[string]string, orgID string) string {
	if orgID == "" {
		return "(unassigned)"
	}
	if n := orgNames[orgID]; n != "" {
		return n
	}
	return orgID
}

// rowDeviceID extracts the device reference from a query/report row.
func rowDeviceID(r map[string]any) string {
	return nvStr(r, "deviceId", "id", "nodeId", "device_id")
}

// --- Shared per-org metrics (reused by patch-compliance, backup-coverage,
// stale-devices, fleet-health, and drift) ----------------------------------

// orgPatchStat holds compliance numbers for one organization.
type orgPatchStat struct {
	OrgID            string
	Devices          int
	NonCompliant     int // devices with >=1 pending/failed patch row
	FailedPatches    int
	PendingPatches   int
	WorstDeviceID    string
	WorstDeviceName  string
	WorstDeviceCount int
}

// compliancePct returns the percent of devices with no pending/failed patches.
func (s orgPatchStat) compliancePct() float64 {
	if s.Devices == 0 {
		return 100
	}
	return roundPct(float64(s.Devices-s.NonCompliant) / float64(s.Devices) * 100)
}

// computePatchStats joins os-patches + software-patches rows (each row is an
// outstanding patch) against the device->org index. Rows whose status contains
// FAIL count as failed; every other outstanding row counts as pending.
func computePatchStats(db *store.Store, devices map[string]nvDevice) (map[string]*orgPatchStat, error) {
	stats := map[string]*orgPatchStat{}
	get := func(orgID string) *orgPatchStat {
		s := stats[orgID]
		if s == nil {
			s = &orgPatchStat{OrgID: orgID}
			stats[orgID] = s
		}
		return s
	}
	// Seed every org that has devices so a fully-compliant org still appears.
	devicePending := map[string]int{}
	for _, d := range devices {
		get(d.OrgID).Devices++
	}
	for _, rt := range []string{rtOsPatches, rtSoftwarePatches} {
		rows, err := loadRows(db, rt)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			did := rowDeviceID(r)
			d, ok := devices[did]
			orgID := ""
			if ok {
				orgID = d.OrgID
			}
			s := get(orgID)
			devicePending[did]++
			status := strings.ToUpper(nvStr(r, "status", "installStatus", "state"))
			if strings.Contains(status, "FAIL") {
				s.FailedPatches++
			} else {
				s.PendingPatches++
			}
		}
	}
	// Count non-compliant devices and find the worst per org.
	worst := map[string]struct {
		id    string
		name  string
		count int
	}{}
	for did, cnt := range devicePending {
		d, ok := devices[did]
		orgID := ""
		name := did
		if ok {
			orgID = d.OrgID
			name = d.Name
		}
		s := get(orgID)
		s.NonCompliant++
		w := worst[orgID]
		if cnt > w.count {
			worst[orgID] = struct {
				id    string
				name  string
				count int
			}{did, name, cnt}
		}
	}
	for orgID, w := range worst {
		s := get(orgID)
		s.WorstDeviceID = w.id
		s.WorstDeviceName = w.name
		s.WorstDeviceCount = w.count
	}
	return stats, nil
}

// backupCoverageRow describes one device with no/zero backup usage.
type backupCoverageRow struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	OrgID      string `json:"orgId"`
	Org        string `json:"org"`
	Reason     string `json:"reason"`
}

// computeBackupCoverage returns the set of device IDs that DO have backup usage
// and the list of devices that do not.
func computeBackupCoverage(db *store.Store, devices map[string]nvDevice, orgNames map[string]string) ([]backupCoverageRow, int, error) {
	rows, err := loadRows(db, rtBackupUsage)
	if err != nil {
		return nil, 0, err
	}
	protected := map[string]bool{}
	seen := map[string]bool{}
	for _, r := range rows {
		did := rowDeviceID(r)
		if did == "" {
			continue
		}
		seen[did] = true
		// Treat any non-zero usage as protected; fall back to "row exists".
		used, ok := nvFloat(r, "usedBytes", "used", "totalSize", "size", "revisionCount", "count")
		if !ok || used > 0 {
			protected[did] = true
		}
	}
	var gaps []backupCoverageRow
	for _, d := range devices {
		if protected[d.ID] {
			continue
		}
		reason := "no backup-usage record"
		if seen[d.ID] {
			reason = "backup usage is zero"
		}
		gaps = append(gaps, backupCoverageRow{
			DeviceID:   d.ID,
			DeviceName: d.Name,
			OrgID:      d.OrgID,
			Org:        orgLabel(orgNames, d.OrgID),
			Reason:     reason,
		})
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].Org != gaps[j].Org {
			return gaps[i].Org < gaps[j].Org
		}
		return gaps[i].DeviceName < gaps[j].DeviceName
	})
	return gaps, len(devices), nil
}

// staleRow describes a device past its last-contact threshold.
type staleRow struct {
	DeviceID    string `json:"deviceId"`
	DeviceName  string `json:"deviceName"`
	OrgID       string `json:"orgId"`
	Org         string `json:"org"`
	DaysSince   int    `json:"daysSince"`
	LastContact string `json:"lastContact,omitempty"`
	Offline     bool   `json:"offline"`
}

// computeStaleDevices returns devices whose last contact is older than `days`.
func computeStaleDevices(devices map[string]nvDevice, orgNames map[string]string, days int, now time.Time) []staleRow {
	var out []staleRow
	cutoff := now.AddDate(0, 0, -days)
	for _, d := range devices {
		if !d.HasContact {
			// No contact timestamp at all -> treat as stale (never seen).
			out = append(out, staleRow{
				DeviceID: d.ID, DeviceName: d.Name, OrgID: d.OrgID,
				Org: orgLabel(orgNames, d.OrgID), DaysSince: -1, Offline: d.Offline,
			})
			continue
		}
		if d.LastContact.Before(cutoff) {
			out = append(out, staleRow{
				DeviceID:    d.ID,
				DeviceName:  d.Name,
				OrgID:       d.OrgID,
				Org:         orgLabel(orgNames, d.OrgID),
				DaysSince:   int(now.Sub(d.LastContact).Hours() / 24),
				LastContact: d.LastContact.Format(time.RFC3339),
				Offline:     d.Offline,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DaysSince > out[j].DaysSince })
	return out
}

// roundPct rounds to one decimal place, half away from zero. The int64
// truncation form rounded negatives toward +inf (roundPct(-4) == -3.9),
// which biased negative drift deltas toward zero.
func roundPct(f float64) float64 {
	return math.Round(f*10) / 10
}

// wantsStructured reports whether the user asked for machine output. Any of
// --json/--agent, --csv, --select, or --quiet routes through the shared
// printer (which honors --select/--compact/--csv) instead of the human table.
func wantsStructured(flags *rootFlags) bool {
	return flags.asJSON || flags.csv || flags.quiet || flags.selectFields != ""
}

// emptyStoreNote prints a one-line hint (to stderr) when no devices are synced.
func emptyStoreNote(devices map[string]nvDevice) string {
	if len(devices) == 0 {
		return "no devices in local store — run 'ninjaone-cli sync' first"
	}
	return ""
}
