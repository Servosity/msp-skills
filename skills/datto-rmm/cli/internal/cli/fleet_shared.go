// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"datto-rmm-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// Resource-type keys as written by the sync layer.
const (
	fleetDevicesResource        = "account-devices"
	fleetAlertsOpenResource     = "account-alerts-open"
	fleetAlertsResolvedResource = "account-alerts-resolved"
	fleetSitesResource          = "account"
	fleetSoftwareResource       = "device-software"
)

type fleetDeviceType struct {
	Category string `json:"category"`
	Type     string `json:"type"`
}

type fleetAntivirus struct {
	AntivirusProduct string `json:"antivirusProduct"`
	AntivirusStatus  string `json:"antivirusStatus"`
}

type fleetPatch struct {
	PatchStatus            string `json:"patchStatus"`
	PatchesApprovedPending int    `json:"patchesApprovedPending"`
	PatchesNotApproved     int    `json:"patchesNotApproved"`
	PatchesInstalled       int    `json:"patchesInstalled"`
}

type fleetDevice struct {
	UID             string          `json:"uid"`
	Hostname        string          `json:"hostname"`
	Description     string          `json:"description"`
	SiteName        string          `json:"siteName"`
	SiteUID         string          `json:"siteUid"`
	OperatingSystem string          `json:"operatingSystem"`
	DeviceType      fleetDeviceType `json:"deviceType"`
	Online          bool            `json:"online"`
	Suspended       bool            `json:"suspended"`
	Deleted         bool            `json:"deleted"`
	LastSeen        json.RawMessage `json:"lastSeen"`
	CagVersion      string          `json:"cagVersion"`
	WarrantyDate    string          `json:"warrantyDate"`
	Antivirus       fleetAntivirus  `json:"antivirus"`
	PatchManagement fleetPatch      `json:"patchManagement"`
}

type fleetAlertSource struct {
	DeviceUID  string `json:"deviceUid"`
	DeviceName string `json:"deviceName"`
	SiteUID    string `json:"siteUid"`
	SiteName   string `json:"siteName"`
}

type fleetAlert struct {
	AlertUID        string           `json:"alertUid"`
	Priority        string           `json:"priority"`
	Resolved        bool             `json:"resolved"`
	Muted           bool             `json:"muted"`
	Timestamp       json.RawMessage  `json:"timestamp"`
	AlertSourceInfo fleetAlertSource `json:"alertSourceInfo"`
	AlertContext    json.RawMessage  `json:"alertContext"`
}

type fleetDevicesStatus struct {
	NumberOfDevices        int `json:"numberOfDevices"`
	NumberOfOnlineDevices  int `json:"numberOfOnlineDevices"`
	NumberOfOfflineDevices int `json:"numberOfOfflineDevices"`
}

type fleetSite struct {
	UID           string             `json:"uid"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	OnDemand      bool               `json:"onDemand"`
	DevicesStatus fleetDevicesStatus `json:"devicesStatus"`
}

type fleetSoftware struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// openFleetStore opens the local store, defaulting the DB path when empty.
func openFleetStore(cmd *cobra.Command, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("datto-rmm-cli")
	}
	return store.OpenWithContext(cmd.Context(), dbPath)
}

// queryRaw runs an unbounded SELECT data query for a resource type.
func queryRaw(ctx context.Context, db *store.Store, resourceType string) ([][]byte, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out [][]byte
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

func loadFleetDevices(ctx context.Context, db *store.Store) ([]fleetDevice, error) {
	raws, err := queryRaw(ctx, db, fleetDevicesResource)
	if err != nil {
		return nil, err
	}
	out := make([]fleetDevice, 0, len(raws))
	for _, raw := range raws {
		var d fleetDevice
		if err := json.Unmarshal(raw, &d); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func loadFleetAlerts(ctx context.Context, db *store.Store) ([]fleetAlert, error) {
	out := []fleetAlert{}
	for _, rt := range []string{fleetAlertsOpenResource, fleetAlertsResolvedResource} {
		raws, err := queryRaw(ctx, db, rt)
		if err != nil {
			return nil, err
		}
		for _, raw := range raws {
			var a fleetAlert
			if err := json.Unmarshal(raw, &a); err != nil {
				continue
			}
			out = append(out, a)
		}
	}
	return out, nil
}

func loadFleetSites(ctx context.Context, db *store.Store) ([]fleetSite, error) {
	raws, err := queryRaw(ctx, db, fleetSitesResource)
	if err != nil {
		return nil, err
	}
	out := make([]fleetSite, 0, len(raws))
	for _, raw := range raws {
		var s fleetSite
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		// The generated write-through cache stores the /v2/account single
		// object under the same resource type as sites; it has no uid/name
		// and would pollute site rollups. Skip it defensively.
		if s.UID == "" && s.Name == "" {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// loadFleetSoftware returns a map keyed by device uid (the resources.id) to the
// list of installed software. The stored payload may be either a raw array or a
// {"software":[...]} envelope; both are handled.
func loadFleetSoftware(ctx context.Context, db *store.Store) (map[string][]fleetSoftware, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT id, data FROM resources WHERE resource_type = ?`, fleetSoftwareResource)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]fleetSoftware{}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		out[id] = unmarshalSoftware(raw)
	}
	return out, rows.Err()
}

// unmarshalSoftware accepts either a bare array or an envelope object.
func unmarshalSoftware(raw []byte) []fleetSoftware {
	var arr []fleetSoftware
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	var env struct {
		Software []fleetSoftware `json:"software"`
	}
	if err := json.Unmarshal(raw, &env); err == nil {
		return env.Software
	}
	return nil
}

// parseDattoTime parses a Datto timestamp that may be a JSON number (epoch
// seconds or milliseconds) or a JSON string (RFC3339, datetime, date, or a
// numeric epoch as text). Returns ok=false on empty/null/unparseable input.
func parseDattoTime(raw json.RawMessage) (time.Time, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return time.Time{}, false
	}

	// JSON string: strip the surrounding quotes.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "" {
			return time.Time{}, false
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, inner); err == nil {
				return t.UTC(), true
			}
		}
		// Numeric epoch held as a string.
		if n, err := strconv.ParseInt(inner, 10, 64); err == nil {
			return epochToTime(n), true
		}
		return time.Time{}, false
	}

	// JSON number epoch.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return epochToTime(n), true
	}
	// Tolerate a float epoch.
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return epochToTime(int64(f)), true
	}
	return time.Time{}, false
}

// epochToTime treats values greater than 1e12 as milliseconds, else seconds.
func epochToTime(n int64) time.Time {
	if n > 1_000_000_000_000 {
		return time.UnixMilli(n).UTC()
	}
	return time.Unix(n, 0).UTC()
}

// parseWarranty parses a warranty date string. Returns ok=false if empty.
func parseWarranty(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func daysSince(t time.Time, now time.Time) int {
	return int(now.Sub(t).Hours() / 24)
}

// shouldPrintJSON reports whether output should be JSON given the flags and the
// terminal state. Mirrors the standard output gate.
func shouldPrintJSON(cmd *cobra.Command, flags *rootFlags) bool {
	return flags.asJSON || flags.agent ||
		(!isTerminal(cmd.OutOrStdout()) && !flags.csv && !flags.quiet && !flags.plain)
}

// avIsHealthy reports whether an antivirusStatus value represents a healthy,
// running AV agent. Datto reports healthy agents as "Running" or
// "RunningAndUpToDate" (separator variants included); everything else —
// empty, NotRunning, Disabled, NotInstalled, OutOfDate — is a gap. This is
// the single AV-OK predicate shared by av-gaps, scorecard, and diff so the
// same device never gets three different verdicts.
func avIsHealthy(status string) bool {
	switch normalizeAvStatus(status) {
	case "running", "runninganduptodate", "running and up to date":
		return true
	}
	return false
}
