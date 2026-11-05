// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Helpers shared by the hand-written local (footprint, offboard, audits) and
// live (routing, callbacks) commands so both apply one rule.

// zoomUserInactiveReason returns why a phone user cannot take calls, or "".
// GET /phone/users documents status activate|deactivate and activation_status
// activated|ready to activate|not ready ("ready to activate" users can still
// make and receive calls). Any other non-empty status is treated as inactive.
func zoomUserInactiveReason(status, activationStatus string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch {
	case s == "deactivate":
		return "deactivated"
	case s != "" && s != "activate":
		return s
	case strings.EqualFold(strings.TrimSpace(activationStatus), "not ready"):
		return "not_ready"
	}
	return ""
}

// zoomDigits keeps only ASCII digits.
func zoomDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// zoomPhoneE164 is the one phone-number parser. A number written with a
// leading + keeps exactly the country code it was written with (Zoom documents
// its DIDs as E.164). A bare number has a known country only as a North
// American national number: 10 digits starting 2-9, or 11 digits starting 1
// then 2-9. Any other bare number is ambiguous and ok is false.
func zoomPhoneE164(s string) (e164 string, ok bool) {
	t := strings.TrimSpace(s)
	d := zoomDigits(t)
	switch {
	case d == "":
		return "", false
	case strings.HasPrefix(t, "+"):
		return "+" + d, true
	case len(d) == 10 && d[0] >= '2':
		return "+1" + d, true
	case len(d) == 11 && d[0] == '1' && d[1] >= '2':
		return "+" + d, true
	}
	return "", false
}

// zoomPhoneKey normalizes a phone number for comparison: its E.164 form, so
// "+1 415 555 0123", "(415) 555-0123" and "4155550123" compare equal while
// +43 1 5550123 and +1 431 555 0123 never do. A bare number with no known
// country (see zoomPhoneE164) keys on its digits in a separate namespace: it
// equals only the same digits written without a country code, never an E.164
// number. No digits gives "".
func zoomPhoneKey(s string) string {
	if e164, ok := zoomPhoneE164(s); ok {
		return e164
	}
	if d := zoomDigits(s); d != "" {
		return "national:" + d
	}
	return ""
}

// zoomFlexString decodes a JSON string, number or bool into a string so an
// integer field (extension_number, status, level) compares the same whether the
// API sent 1001 or "1001". Objects, arrays and null decode to "".
type zoomFlexString string

func (f *zoomFlexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case len(b) == 0 || string(b) == "null" || b[0] == '{' || b[0] == '[':
		*f = ""
	case b[0] == '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = zoomFlexString(s)
	default:
		*f = zoomFlexString(string(b))
	}
	return nil
}

func (f zoomFlexString) String() string { return strings.TrimSpace(string(f)) }
