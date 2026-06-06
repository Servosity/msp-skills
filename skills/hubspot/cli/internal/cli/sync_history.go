// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
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
		PropertiesWithHistory map[string][]propertyHistoryRaw `json:"propertiesWithHistory"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode item: %w", err)
	}
	if envelope.ID == "" || len(envelope.PropertiesWithHistory) == 0 {
		return nil
	}
	var entries []store.PropertyHistoryEntry
	for prop, hist := range envelope.PropertiesWithHistory {
		for _, h := range hist {
			ts := h.parseTimestamp()
			if ts.IsZero() {
				continue
			}
			entries = append(entries, store.PropertyHistoryEntry{
				Property:  prop,
				Value:     h.Value,
				Timestamp: ts,
				Source:    h.Source,
				SourceID:  h.SourceID,
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
	Value     string          `json:"value"`
	Timestamp json.RawMessage `json:"timestamp"`
	Source    string          `json:"source"`
	SourceID  string          `json:"sourceId"`
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
