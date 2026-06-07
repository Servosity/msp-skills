// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence support for levelio-cli: shared entity types,
// store loaders, and pure-logic helpers used by the offline fleet-analytics
// commands (at-risk, fleet, stale, patch-posture, alert-triage, security-posture,
// group-tree, cf-coverage, since). These read the local SQLite store populated by
// `sync` and never call the live API.

package cli

import (
	"encoding/json"
	"strings"
	"time"

	"levelio-pp-cli/internal/store"
)

// ---- Entity types (only the fields the analytics commands use) ----

type lvlOS struct {
	FullOperatingSystem string `json:"full_operating_system"`
	MajorVersion        string `json:"major_version"`
	Architecture        string `json:"architecture"`
}

type lvlDevice struct {
	ID               string   `json:"id"`
	Hostname         string   `json:"hostname"`
	Nickname         string   `json:"nickname"`
	Role             string   `json:"role"`
	GroupID          string   `json:"group_id"`
	Tags             []string `json:"tags"`
	Online           bool     `json:"online"`
	MaintenanceMode  bool     `json:"maintenance_mode"`
	Notes            string   `json:"notes"`
	Manufacturer     string   `json:"manufacturer"`
	Model            string   `json:"model"`
	SerialNumber     string   `json:"serial_number"`
	TotalMemory      int64    `json:"total_memory"`
	CPUCores         int      `json:"cpu_cores"`
	LastLoggedInUser string   `json:"last_logged_in_user"`
	LastRebootTime   string   `json:"last_reboot_time"`
	LastSeenAt       string   `json:"last_seen_at"`
	City             string   `json:"city"`
	Country          string   `json:"country"`
	SecurityScore    *int     `json:"security_score"`
	Platform         string   `json:"platform"`
	OperatingSystem  *lvlOS   `json:"operating_system"`
}

type lvlGroup struct {
	ID                    string   `json:"id"`
	ParentID              string   `json:"parent_id"`
	ChildIDs              []string `json:"child_ids"`
	Name                  string   `json:"name"`
	DeviceCount           int      `json:"device_count"`
	DescendentDeviceCount int      `json:"descendent_device_count"`
}

type lvlAlert struct {
	ID             string `json:"id"`
	DeviceID       string `json:"device_id"`
	DeviceHostname string `json:"device_hostname"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Severity       string `json:"severity"`
	IsResolved     bool   `json:"is_resolved"`
	StartedAt      string `json:"started_at"`
	ResolvedAt     string `json:"resolved_at"`
}

type lvlUpdate struct {
	ID             string   `json:"id"`
	DeviceID       string   `json:"device_id"`
	DeviceHostname string   `json:"device_hostname"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Version        string   `json:"version"`
	Size           int64    `json:"size"`
	KBIDs          []string `json:"kb_ids"`
	IsAvailable    bool     `json:"is_available"`
	Error          string   `json:"error"`
	PublishedOn    string   `json:"published_on"`
	InstalledOn    string   `json:"installed_on"`
}

type lvlCustomField struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Reference string `json:"reference"`
	AdminOnly bool   `json:"admin_only"`
}

type lvlCustomFieldValue struct {
	CustomFieldID   string `json:"custom_field_id"`
	CustomFieldName string `json:"custom_field_name"`
	AssignedToID    string `json:"assigned_to_id"`
	Value           string `json:"value"`
}

// ---- Store loaders ----

// lvlLoad reads every row of a resource_type out of the local store and
// unmarshals it into T. Rows that fail to parse are skipped, not fatal.
func lvlLoad[T any](db *store.Store, resourceType string) ([]T, error) {
	rows, err := db.Query(`SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func lvlDevices(db *store.Store) ([]lvlDevice, error) {
	return lvlLoad[lvlDevice](db, "devices")
}
func lvlGroups(db *store.Store) ([]lvlGroup, error) {
	return lvlLoad[lvlGroup](db, "groups")
}
func lvlAlerts(db *store.Store) ([]lvlAlert, error) {
	return lvlLoad[lvlAlert](db, "alerts")
}
func lvlUpdates(db *store.Store) ([]lvlUpdate, error) {
	return lvlLoad[lvlUpdate](db, "updates")
}
func lvlCustomFields(db *store.Store) ([]lvlCustomField, error) {
	return lvlLoad[lvlCustomField](db, "custom-fields")
}
func lvlCustomFieldValues(db *store.Store) ([]lvlCustomFieldValue, error) {
	return lvlLoad[lvlCustomFieldValue](db, "custom-field-values")
}

// ---- Pure helpers ----

// lvlParseTime parses Level's ISO-8601 timestamps (e.g. "2024-01-01T01:00:00.000Z").
func lvlParseTime(s string) (time.Time, bool) {
	if strings.TrimSpace(s) == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// lvlSeverityWeight ranks alert severities for risk scoring and ordering.
func lvlSeverityWeight(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "emergency":
		return 4
	case "critical":
		return 3
	case "warning":
		return 2
	case "information", "info":
		return 1
	default:
		return 1
	}
}

// lvlOSLabel returns a human OS label for a device, falling back to platform.
func lvlOSLabel(d lvlDevice) string {
	if d.OperatingSystem != nil && strings.TrimSpace(d.OperatingSystem.FullOperatingSystem) != "" {
		return d.OperatingSystem.FullOperatingSystem
	}
	if strings.TrimSpace(d.Platform) != "" {
		return d.Platform
	}
	return "unknown"
}

// lvlDeviceLabel returns the best display name for a device.
func lvlDeviceLabel(d lvlDevice) string {
	if strings.TrimSpace(d.Hostname) != "" {
		return d.Hostname
	}
	if strings.TrimSpace(d.Nickname) != "" {
		return d.Nickname
	}
	return d.ID
}

// lvlDaysDark returns whole days since a device was last seen, relative to now.
// Returns (days, true) when last_seen_at parsed, else (0, false).
func lvlDaysDark(d lvlDevice, now time.Time) (float64, bool) {
	t, ok := lvlParseTime(d.LastSeenAt)
	if !ok {
		return 0, false
	}
	return now.Sub(t).Hours() / 24.0, true
}

// lvlGroupIndex builds id->group and parent->children maps plus a descendants
// resolver for rolling metrics up the group tree.
type lvlGroupIndex struct {
	byID     map[string]lvlGroup
	children map[string][]string
}

func lvlBuildGroupIndex(groups []lvlGroup) lvlGroupIndex {
	idx := lvlGroupIndex{
		byID:     make(map[string]lvlGroup, len(groups)),
		children: make(map[string][]string),
	}
	for _, g := range groups {
		idx.byID[g.ID] = g
	}
	for _, g := range groups {
		if g.ParentID != "" {
			idx.children[g.ParentID] = append(idx.children[g.ParentID], g.ID)
		}
	}
	return idx
}

// name returns a group's display name, or the id if the group is unknown.
func (idx lvlGroupIndex) name(id string) string {
	if id == "" {
		return "(no group)"
	}
	if g, ok := idx.byID[id]; ok && strings.TrimSpace(g.Name) != "" {
		return g.Name
	}
	return id
}

// descendants returns the set of group ids that are the given root or any group
// beneath it in the hierarchy (inclusive). Cycle-safe via a visited set.
func (idx lvlGroupIndex) descendants(root string) map[string]bool {
	seen := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		for _, c := range idx.children[id] {
			walk(c)
		}
	}
	walk(root)
	return seen
}

type lvlTag struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DeviceCount int    `json:"device_count"`
}

func lvlTags(db *store.Store) ([]lvlTag, error) {
	return lvlLoad[lvlTag](db, "tags")
}
