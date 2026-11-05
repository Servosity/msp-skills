// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored helpers behind `sync --with-history`: HubSpot propertiesWithHistory
// capture into the hubspot_property_history table. Lives beside the generated
// sync.go so regen-merge preserves it whole; sync.go carries only the small
// inline hooks (flag, params, capture loop) that call into these.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hubspot-pp-cli/internal/store"
)

// supportsPropertyHistory reports whether `propertiesWithHistory` is meaningful
// for the named resource. HubSpot only supports it on CRM objects; we whitelist
// the four the --with-history flag advertises.
func supportsPropertyHistory(resource string) bool {
	switch resource {
	case "hubspot-meetings-crm", "hubspot-deals-crm", "hubspot-contacts-crm", "hubspot-companies-crm":
		return true
	}
	return false
}

// historyObjectType maps a sync resource name to the object_type string the
// hubspot_property_history table stores; mirrors the prior CLI's table-naming
// convention (meetings/deals/contacts/companies).
func historyObjectType(resource string) string {
	switch resource {
	case "hubspot-meetings-crm":
		return "meetings"
	case "hubspot-deals-crm":
		return "deals"
	case "hubspot-contacts-crm":
		return "contacts"
	case "hubspot-companies-crm":
		return "companies"
	}
	return ""
}

// captureHistoryFromItem parses one HubSpot object's `propertiesWithHistory`
// blob and persists each entry. The blob shape is
// {"id":"<obj id>","propertiesWithHistory":{"<prop>":[{"value":"...","timestamp":"...","source":"...","sourceId":"..."}]}}.
// We accept a missing block silently (the API only returns it when the request
// asked for it AND the object actually has history).
func captureHistoryFromItem(ctx context.Context, db *store.Store, objectType string, raw json.RawMessage) error {
	var envelope struct {
		ID                    string                          `json:"id"`
		UpdatedAt             string                          `json:"updatedAt"`
		Properties            map[string]any                  `json:"properties"`
		PropertiesWithHistory map[string][]propertyHistoryRaw `json:"propertiesWithHistory"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode item: %w", err)
	}
	if envelope.ID == "" || len(envelope.PropertiesWithHistory) == 0 {
		return nil
	}
	// The snapshot time HubSpot reported with this history (hand-wired,
	// handfixes.json hubspot-velocity-history-must-describe-current-stage).
	snapshotAt, _ := envelope.Properties["hs_lastmodifieddate"].(string)
	if snapshotAt == "" {
		snapshotAt = envelope.UpdatedAt
	}
	var entries []store.PropertyHistoryEntry
	for prop, hist := range envelope.PropertiesWithHistory {
		for _, h := range hist {
			ts := h.parseTimestamp()
			if ts.IsZero() {
				continue
			}
			entries = append(entries, store.PropertyHistoryEntry{
				Property:   prop,
				Value:      h.Value,
				Timestamp:  ts,
				Source:     firstNonEmptyString(h.SourceType, h.Source),
				SourceID:   h.SourceID,
				SnapshotAt: snapshotAt,
			})
		}
	}
	return db.UpsertPropertyHistoryBatch(ctx, objectType, envelope.ID, entries)
}

// propertyHistoryRaw mirrors the unmarshalled shape of one history entry from
// HubSpot's `propertiesWithHistory` blob. Timestamps arrive either as ISO 8601
// strings (REST) or as epoch-millisecond numbers — handle both rather than
// failing on the second shape.
type propertyHistoryRaw struct {
	Value      string          `json:"value"`
	Timestamp  json.RawMessage `json:"timestamp"`
	Source     string          `json:"source"`
	SourceType string          `json:"sourceType"` // HubSpot's documented ValueWithTimestamp field
	SourceID   string          `json:"sourceId"`
}

func (h propertyHistoryRaw) parseTimestamp() time.Time {
	if len(h.Timestamp) == 0 {
		return time.Time{}
	}
	// Try string first.
	var s string
	if err := json.Unmarshal(h.Timestamp, &s); err == nil && s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	// Fall back to epoch ms (number).
	var n int64
	if err := json.Unmarshal(h.Timestamp, &n); err == nil && n > 0 {
		return time.UnixMilli(n).UTC()
	}
	return time.Time{}
}

// fillPropertiesFromHistory copies each history property's newest value into
// the item's regular properties block when HubSpot did not return it there.
// --with-history never narrows properties= (that would strip the snapshot's
// default properties), so the current value of a non-default property such as
// hs_meeting_outcome arrives only inside propertiesWithHistory. A value HubSpot
// did return in properties always wins. Undecodable items pass through
// unchanged; the capture step reports them.
func fillPropertiesFromHistory(raw json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return raw
	}
	var hist map[string][]propertyHistoryRaw
	if json.Unmarshal(obj["propertiesWithHistory"], &hist) != nil || len(hist) == 0 {
		return raw
	}
	props := map[string]json.RawMessage{}
	if p, ok := obj["properties"]; ok && json.Unmarshal(p, &props) != nil {
		return raw
	}
	if props == nil { // "properties": null
		props = map[string]json.RawMessage{}
	}
	changed := false
	for name, entries := range hist {
		if _, ok := props[name]; ok {
			continue
		}
		var newest *propertyHistoryRaw
		var newestAt time.Time
		for i := range entries {
			if ts := entries[i].parseTimestamp(); !ts.IsZero() && (newest == nil || ts.After(newestAt)) {
				newest, newestAt = &entries[i], ts
			}
		}
		if newest == nil {
			continue
		}
		v, _ := json.Marshal(newest.Value)
		props[name] = v
		changed = true
	}
	if !changed {
		return raw
	}
	p, err := json.Marshal(props)
	if err != nil {
		return raw
	}
	obj["properties"] = p
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}
