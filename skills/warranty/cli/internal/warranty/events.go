// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-novels): coverage events.

package warranty

import "strings"

// Event types, the vendor_events.type values.
const (
	EventExpiring = "coverage.expiring"
	EventExpired  = "coverage.expired"
	EventChanged  = "coverage.changed"
)

// EventTypes lists every event type.
var EventTypes = []string{EventExpiring, EventExpired, EventChanged}

// Event is one coverage event ready for vendor_events. EventID is stable, so
// the same condition seen by a later lookup never repeats.
type Event struct {
	EventID string
	Type    string
	Serial  string
	Data    map[string]any
}

// Prior is the device state the mirror held before this lookup.
type Prior struct {
	Fingerprint string
	End         string
	Open        bool
	// Checked: the vendor answered for this device before (coverage or a
	// definitive negative).
	Checked bool
	// CheckedAt is when; it is part of a change's identity, so a state that
	// returns later is a new change while a retried lookup is not.
	CheckedAt string
}

// Events compares a fresh summary with the prior state. asOf is today
// (YYYY-MM-DD); windowDays is how far ahead counts as expiring.
func Events(serial, vendor string, prior Prior, now Summary, asOf string, windowDays int) []Event {
	var out []Event
	if prior.Checked && prior.Fingerprint != now.Fingerprint {
		out = append(out, Event{
			EventID: strings.Join([]string{EventChanged, serial, prior.Fingerprint, prior.CheckedAt, now.Fingerprint}, ":"),
			Type:    EventChanged,
			Serial:  serial,
			Data: map[string]any{"vendor": vendor, "before_coverage_end": prior.End, "after_coverage_end": now.End,
				"before_coverage_open": prior.Open, "after_coverage_open": now.Open},
		})
	}
	if now.Open || now.End == "" {
		return out
	}
	status, days := Status(now, asOf, windowDays)
	switch status {
	case "expiring":
		out = append(out, Event{
			EventID: strings.Join([]string{EventExpiring, serial, now.End}, ":"),
			Type:    EventExpiring,
			Serial:  serial,
			Data:    map[string]any{"vendor": vendor, "coverage_end": now.End, "days_left": *days},
		})
	case "expired":
		out = append(out, Event{
			EventID: strings.Join([]string{EventExpired, serial, now.End}, ":"),
			Type:    EventExpired,
			Serial:  serial,
			Data:    map[string]any{"vendor": vendor, "coverage_end": now.End},
		})
	}
	return out
}
