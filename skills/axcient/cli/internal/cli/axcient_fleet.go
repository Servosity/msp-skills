// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored fleet helpers shared by the axcient novel commands
// (health, rpo, compliance, billing, appliance-map, client-rollup).

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"axcient-pp-cli/internal/store"
)

// fleetHealthStatus mirrors the spec's health_status_obj.
type fleetHealthStatus struct {
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Timestamp string `json:"timestamp"`
}

// fleetDevice is the subset of org_level_device the fleet commands read.
// IDs decode as json.Number so large int64 identifiers never round-trip
// through float64 scientific notation.
type fleetDevice struct {
	ID           json.Number        `json:"id"`
	IDAlt        json.Number        `json:"id_"`
	Name         string             `json:"name"`
	ClientID     json.Number        `json:"client_id"`
	Type         string             `json:"type"`
	D2C          bool               `json:"d2c"`
	ServiceID    string             `json:"service_id"`
	AgentVersion string             `json:"agent_version"`
	Current      *fleetHealthStatus `json:"current_health_status"`
	LatestLocal  string             `json:"latest_local_rp"`
	LatestCloud  string             `json:"latest_cloud_rp"`
	LatestVault  string             `json:"latest_vault_rp"`
	LocalUsage   int64              `json:"local_usage"`
	CloudUsage   int64              `json:"cloud_usage"`
	VaultUsage   int64              `json:"vault_usage"`
}

// deviceID coalesces the spec's id field with the live API's Python-style
// id_ spelling (the vendor backend leaks trailing-underscore attribute names;
// confirmed against the public mock and community module output).
func (d *fleetDevice) deviceID() string {
	if d.ID.String() != "" {
		return d.ID.String()
	}
	return d.IDAlt.String()
}

// resolveClientID returns the device's client id, falling back to the
// client_device mapping synced from /client/{client_id}/device when the
// org-level device row omits client_id (community-documented live quirk).
func (d *fleetDevice) resolveClientID(fallback map[string]string) string {
	if d.ClientID.String() != "" {
		return d.ClientID.String()
	}
	return fallback[d.deviceID()]
}

// loadDeviceClientMap maps device id -> client id from the synced
// client_device dependent rows. Best-effort: an empty map degrades grouping
// to "(unattributed)" rather than failing the command.
func loadDeviceClientMap(db *store.Store) map[string]string {
	m := map[string]string{}
	rows, err := db.DB().Query(`SELECT client_id, data FROM "client_device"`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var clientID, data sql.NullString
		if err := rows.Scan(&clientID, &data); err != nil {
			continue
		}
		var d struct {
			ID    json.Number `json:"id"`
			IDAlt json.Number `json:"id_"`
		}
		if err := json.Unmarshal([]byte(data.String), &d); err != nil {
			continue
		}
		key := d.ID.String()
		if key == "" {
			key = d.IDAlt.String()
		}
		if key != "" && clientID.String != "" {
			m[key] = clientID.String
		}
	}
	return m
}

// parseAxTime parses the timestamp shapes the x360Recover API emits:
// RFC3339 ("2023-01-20T04:56:07Z") and zone-less ISO8601
// ("2021-07-01T10:08:00", treated as UTC).
func parseAxTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t.UTC(), true
	}
	// Lenient layouts: vendor fixtures emit non-padded date components
	// ("2024-01-3T11:33:00"); Go's single-digit reference accepts both.
	if t, err := time.Parse("2006-1-2T15:4:5Z07:00", s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse("2006-1-2T15:4:5", s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// newestRestorePoint returns the most recent of the device's local, cloud,
// and private-vault restore-point timestamps, the target it came from, and
// whether any timestamp parsed at all.
func (d *fleetDevice) newestRestorePoint() (time.Time, string, bool) {
	var best time.Time
	target := ""
	for _, cand := range []struct {
		raw    string
		source string
	}{
		{d.LatestLocal, "local"},
		{d.LatestCloud, "cloud"},
		{d.LatestVault, "vault"},
	} {
		if t, ok := parseAxTime(cand.raw); ok && t.After(best) {
			best = t
			target = cand.source
		}
	}
	return best, target, !best.IsZero()
}

// restorePointFor returns the restore-point time for one target
// (local|cloud|vault|any). "any" means the newest across all three.
func (d *fleetDevice) restorePointFor(target string) (time.Time, string, bool) {
	switch target {
	case "", "any":
		return d.newestRestorePoint()
	case "local":
		t, ok := parseAxTime(d.LatestLocal)
		return t, "local", ok
	case "cloud":
		t, ok := parseAxTime(d.LatestCloud)
		return t, "cloud", ok
	case "vault":
		t, ok := parseAxTime(d.LatestVault)
		return t, "vault", ok
	}
	return time.Time{}, "", false
}

// isFailing reports whether the device's current health status is set and
// not NORMAL.
func (d *fleetDevice) isFailing() bool {
	return d.Current != nil && d.Current.Status != "" && !strings.EqualFold(d.Current.Status, "NORMAL")
}

// loadFleetDevices reads every synced device row from the local store,
// optionally filtered to one client id (0 = all clients).
func loadFleetDevices(db *store.Store, clientFilter int64) ([]fleetDevice, error) {
	rows, err := db.DB().Query(`SELECT data FROM resources WHERE resource_type = 'device'`)
	if err != nil {
		return nil, fmt.Errorf("querying devices: %w", err)
	}
	defer rows.Close()
	devices := make([]fleetDevice, 0)
	for rows.Next() {
		var data sql.NullString
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var d fleetDevice
		if err := json.Unmarshal([]byte(data.String), &d); err != nil {
			continue
		}
		if clientFilter > 0 {
			cid, _ := d.ClientID.Int64()
			if cid != clientFilter {
				continue
			}
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}

// loadClientNames maps client id -> client name from the synced clients table.
// Missing or unsynced clients yield an empty map (callers fall back to the id).
func loadClientNames(db *store.Store) map[string]string {
	names := map[string]string{}
	rows, err := db.DB().Query(`SELECT data FROM resources WHERE resource_type = 'clients'`)
	if err != nil {
		return names
	}
	defer rows.Close()
	for rows.Next() {
		var data sql.NullString
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var c struct {
			ID    json.Number `json:"id"`
			IDAlt json.Number `json:"id_"`
			Name  string      `json:"name"`
		}
		if err := json.Unmarshal([]byte(data.String), &c); err != nil {
			continue
		}
		key := c.ID.String()
		if key == "" {
			key = c.IDAlt.String()
		}
		if key != "" {
			names[key] = c.Name
		}
	}
	return names
}

// fleetClientName resolves a display name for a client id, falling back to
// "client <id>" when the clients resource has not been synced.
func fleetClientName(names map[string]string, id json.Number) string {
	if n, ok := names[id.String()]; ok && n != "" {
		return n
	}
	if id.String() == "" {
		return "(unattributed)"
	}
	return "client " + id.String()
}

// fleetAutoverify summarizes the most recent AutoVerify run for a device.
type fleetAutoverify struct {
	Status        string `json:"status"`
	IsHealthy     *bool  `json:"is_healthy"`
	Timestamp     string `json:"timestamp"`
	RestorePoint  string `json:"rp"`
	ScreenshotURL string `json:"screenshot_url"`
}

type fleetAVDetail struct {
	ID             string `json:"id"`
	Timestamp      string `json:"timestamp"`
	StartTimestamp string `json:"start_timestamp"`
	EndTimestamp   string `json:"end_timestamp"`
	RP             string `json:"rp"`
	Status         string `json:"status"`
	IsHealthy      *bool  `json:"is_healthy"`
	ScreenshotURL  string `json:"screenshot_url"`
}

// loadLatestAutoverify maps device id -> the latest AutoVerify detail across
// every synced autoverify row (a device may have one row per vault/appliance).
// Errors are returned (not swallowed): the map drives load-bearing pass/fail
// verdicts in compliance and client-rollup, so a partial read must surface.
func loadLatestAutoverify(db *store.Store) (map[string]fleetAutoverify, error) {
	latest := map[string]fleetAutoverify{}
	latestAt := map[string]time.Time{}
	rows, err := db.DB().Query(`SELECT device_id, data FROM "autoverify"`)
	if err != nil {
		return nil, fmt.Errorf("querying autoverify: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var deviceID, data sql.NullString
		if err := rows.Scan(&deviceID, &data); err != nil {
			continue
		}
		var rec struct {
			Details []fleetAVDetail `json:"autoverify_details"`
		}
		if err := json.Unmarshal([]byte(data.String), &rec); err != nil {
			continue
		}
		for _, det := range rec.Details {
			at, ok := parseAxTime(det.EndTimestamp)
			if !ok {
				at, ok = parseAxTime(det.Timestamp)
			}
			if !ok {
				at = time.Time{}
			}
			if prev, seen := latestAt[deviceID.String]; !seen || at.After(prev) {
				latestAt[deviceID.String] = at
				latest[deviceID.String] = fleetAutoverify{
					Status:        det.Status,
					IsHealthy:     det.IsHealthy,
					Timestamp:     det.Timestamp,
					RestorePoint:  det.RP,
					ScreenshotURL: det.ScreenshotURL,
				}
			}
		}
	}
	return latest, rows.Err()
}

// avPassed reports whether an AutoVerify summary represents a passing,
// healthy boot verification.
func avPassed(av fleetAutoverify, found bool) bool {
	if !found {
		return false
	}
	if !strings.EqualFold(av.Status, "success") {
		return false
	}
	return av.IsHealthy == nil || *av.IsHealthy
}

// fleetAppliance is the subset of org_level_appliance appliance-map reads.
type fleetAppliance struct {
	ID        json.Number `json:"id"`
	IDAlt     json.Number `json:"id_"`
	ServiceID string      `json:"service_id"`
	ClientID  json.Number `json:"client_id"`
	Alias     string      `json:"alias"`
	IPAddress string      `json:"ip_address"`
	Active    bool        `json:"active"`
}

// applianceID coalesces id with the live API's id_ spelling.
func (a *fleetAppliance) applianceID() string {
	if a.ID.String() != "" {
		return a.ID.String()
	}
	return a.IDAlt.String()
}

// loadFleetAppliances reads every synced appliance row from the local store.
func loadFleetAppliances(db *store.Store) ([]fleetAppliance, error) {
	rows, err := db.DB().Query(`SELECT data FROM resources WHERE resource_type = 'appliance'`)
	if err != nil {
		return nil, fmt.Errorf("querying appliances: %w", err)
	}
	defer rows.Close()
	appliances := make([]fleetAppliance, 0)
	for rows.Next() {
		var data sql.NullString
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var a fleetAppliance
		if err := json.Unmarshal([]byte(data.String), &a); err != nil {
			continue
		}
		appliances = append(appliances, a)
	}
	return appliances, rows.Err()
}

// hoursSince renders the age of t against now, rounded to one decimal.
func hoursSince(now, t time.Time) float64 {
	if t.IsZero() {
		return -1
	}
	return float64(int(now.Sub(t).Hours()*10)) / 10
}

// sortedClientIDs returns map keys ordered numerically when possible so
// grouped output is deterministic.
func sortedClientIDs[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) < len(keys[j])
		}
		return keys[i] < keys[j]
	})
	return keys
}

// openFleetStore opens the local store read-only for fleet queries and emits
// the standard unsynced/stale hints for the named resource.
func openFleetStore(dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("axcient-cli")
	}
	return store.OpenReadOnly(dbPath)
}
