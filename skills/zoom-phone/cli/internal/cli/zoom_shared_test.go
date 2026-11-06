// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

func TestZoomUserInactiveReason(t *testing.T) {
	cases := []struct{ status, activation, want string }{
		{"activate", "activated", ""},
		{"activate", "ready to activate", ""},
		{"", "", ""},
		{"deactivate", "activated", "deactivated"},
		{"Pending", "", "pending"},
		{"activate", "not ready", "not_ready"},
	}
	for _, c := range cases {
		if got := zoomUserInactiveReason(c.status, c.activation); got != c.want {
			t.Errorf("zoomUserInactiveReason(%q,%q) = %q, want %q", c.status, c.activation, got, c.want)
		}
	}
}

func TestZoomPhoneKey(t *testing.T) {
	for _, in := range []string{"+1 415 555 0123", "(415) 555-0123", "4155550123", "+14155550123"} {
		if got := zoomPhoneKey(in); got != "+14155550123" {
			t.Errorf("zoomPhoneKey(%q) = %q", in, got)
		}
	}
	if zoomPhoneKey("+442071234567") != "+442071234567" {
		t.Error("non-NANP number must keep its country code")
	}
}

// Codex review r1 (class phone-identity-collision). Folding a leading 1 off
// every 11-digit number made +43 1 5550123 (Vienna) equal +1 431 555 0123
// (Manitoba). A number written with + keeps the country code it was written
// with; only a bare national number is read as North American.
func TestZoomPhoneKeyKeepsExplicitCountryCode(t *testing.T) {
	distinct := [][2]string{
		{"+4315550123", "+14315550123"},
		{"+4315550123", "4315550123"},
		{"+4315550123", "14315550123"},
		{"+442071234567", "+1442071234567"},
		{"+33123456789", "+133123456789"},
		{"0412345678", "+10412345678"},
		{"0412345678", "+61412345678"},
	}
	for _, p := range distinct {
		a, b := zoomPhoneKey(p[0]), zoomPhoneKey(p[1])
		if a == "" || b == "" || a == b {
			t.Errorf("zoomPhoneKey(%q)=%q and zoomPhoneKey(%q)=%q must differ", p[0], a, p[1], b)
		}
	}
	same := [][2]string{
		{"+1 415 555 0123", "(415) 555-0123"},
		{"+14155550123", "1-415-555-0123"},
		{"+4315550123", "+43 1 5550123"},
		{"0412345678", "0412 345 678"},
	}
	for _, p := range same {
		if a, b := zoomPhoneKey(p[0]), zoomPhoneKey(p[1]); a == "" || a != b {
			t.Errorf("zoomPhoneKey(%q)=%q and zoomPhoneKey(%q)=%q must match", p[0], a, p[1], b)
		}
	}
	if zoomPhoneKey("") != "" || zoomPhoneKey("+") != "" {
		t.Error("no digits must give no key")
	}
}
