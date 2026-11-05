// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeper

import (
	"strings"
	"testing"
)

// injections are inputs that would change a Commander command if they
// reached it: a second command, a quote, an escape, an expansion, a flag, a
// separator, a newline, a host path, a Unicode space.
var injections = []string{
	"1 ; msp-remove 1", "1 msp-remove", "1\nmsp-remove 1", "1\r\n", "1\x00", `1"`, `1'`, `1\`, "$(id)", "${HOME}", "`id`",
	"--output=/tmp/x", "-f", "--", "1 --save", "1 --attempt-fix", "1|tee", "1&&x", "1>x", "1<x", "1#x", "1 1", "1 1",
	"--record-details", " 1", "1 ", "", "-1", "0", "1e3", "１", "a b", "a\tb",
}

func noArgEscapes(t *testing.T, what string, c Command, err error, payload string) {
	t.Helper()
	if err != nil {
		return
	}
	// Anything accepted must still be exactly one safe token.
	for _, tok := range c.args {
		if strings.ContainsAny(tok, " \t\r\n\x00\"'\\$`;|&<>()#") || tok == "--" {
			t.Fatalf("%s accepted %q and emitted unsafe token %q", what, payload, tok)
		}
	}
	if err := c.validate(); err != nil {
		t.Fatalf("%s accepted %q but the command fails its own check: %v", what, payload, err)
	}
	if strings.Contains(c.String(), payload) && strings.ContainsAny(payload, " \t\r\n;|&$`\"'\\-") {
		t.Fatalf("%s carried %q into %q", what, payload, c.String())
	}
}

func TestEveryArgumentRefusesInjection(t *testing.T) {
	for _, p := range injections {
		c, err := Company(p)
		noArgEscapes(t, "Company", c, err, p)
		if err == nil && p != "1" {
			t.Errorf("Company accepted %q", p)
		}
		c, err = SwitchToMC(p)
		if err == nil {
			t.Errorf("SwitchToMC accepted %q", p)
		}
		c, err = Billing(p, false, false)
		if err == nil && p != "" {
			t.Errorf("Billing month accepted %q", p)
		}
		noArgEscapes(t, "Billing", c, err, p)
		c, err = AuditEvents(p, 10, "", "")
		if err == nil {
			t.Errorf("AuditEvents --created accepted %q", p)
		}
		c, err = AuditEvents("today", 10, p, "")
		if err == nil && p != "" {
			t.Errorf("AuditEvents --username accepted %q", p)
		}
		noArgEscapes(t, "AuditEvents username", c, err, p)
		c, err = AuditEvents("today", 10, "", p)
		if err == nil && p != "" {
			t.Errorf("AuditEvents --event-type accepted %q", p)
		}
		noArgEscapes(t, "AuditEvents event type", c, err, p)
		c, err = AuditSummary(p, "today")
		if err == nil {
			t.Errorf("AuditSummary --period accepted %q", p)
		}
		c, err = AuditSummary("day", p)
		if err == nil {
			t.Errorf("AuditSummary --created accepted %q", p)
		}
	}
	for _, email := range []string{"a@b.c ; msp-remove 1", "-x@b.co", "a@b.co --save", "a@-b.co", "a@b", "a b@c.co", "a@b.co\n", "${X}@b.co"} {
		if _, err := AuditEvents("today", 10, email, ""); err == nil {
			t.Errorf("AuditEvents --username accepted %q", email)
		}
	}
	for _, n := range []int{0, -1, 3651, 1 << 40} {
		if _, err := LastLogin(n); err == nil {
			t.Errorf("LastLogin accepted %d days", n)
		}
	}
	for _, n := range []int{0, -1, 1001} {
		if _, err := AuditEvents("today", n, "", ""); err == nil {
			t.Errorf("AuditEvents accepted limit %d", n)
		}
	}
}

func TestAcceptedValuesStayOneToken(t *testing.T) {
	c, err := Company("4421")
	if err != nil || c.String() != "msp-info --format=json --verbose --managed-company 4421" {
		t.Fatalf("Company: %q %v", c.String(), err)
	}
	c, err = AuditEvents("last_7_days", 50, "jane.o'x@example.com", "")
	if err == nil {
		t.Fatalf("a quote in an email was accepted: %q", c.String())
	}
	c, err = AuditEvents("last_7_days", 50, "jane+it@ex-ample.co.uk", "record_add")
	if err != nil {
		t.Fatal(err)
	}
	want := "audit-report --format=json --report-type raw --report-format message --created last_7_days --order desc --limit 50 --username jane+it@ex-ample.co.uk --event-type record_add"
	if c.String() != want {
		t.Fatalf("got %q", c.String())
	}
	c, err = Billing("2026-09", true, true)
	if err != nil || c.String() != "msp-billing-report --format=json --month 2026-09 --show-company --show-date" {
		t.Fatalf("Billing: %q %v", c.String(), err)
	}
}

// The allowlist is the whole surface: every constructor emits a known name,
// never a write or host-file flag, and a hand-made Command is refused.
func TestOnlyAllowlistedReadOnlyCommands(t *testing.T) {
	ll, _ := LastLogin(90)
	ae, _ := AuditEvents("today", 1, "", "")
	as, _ := AuditSummary("span", "today")
	co, _ := Company("1")
	bi, _ := Billing("", false, false)
	sw, _ := SwitchToMC("1")
	all := []Command{Companies(), co, Products(), Permits(), bi, Users(), Nodes(), Teams(), Roles(), RolePosture(), ll,
		SecurityAudit(false), SecurityAudit(true), ae, as, sw, SwitchToMSP()}
	banned := []string{"--output", "--save", "--attempt-fix", "--record-details", "--apply-action", "--force", "-f", "--debug", "FILEDATA"}
	for _, c := range all {
		if err := c.validate(); err != nil {
			t.Errorf("%s: %v", c.String(), err)
		}
		for _, b := range banned {
			for _, a := range c.args {
				if a == b || strings.HasPrefix(a, b+"=") {
					t.Errorf("%s carries %s", c.String(), b)
				}
			}
		}
		if c.kind == kindRows && (len(c.args) == 0 || c.args[0] != "--format=json") {
			t.Errorf("%s: --format=json must follow the command name", c.String())
		}
	}
	for _, bad := range []Command{{}, {name: "get", args: []string{"x"}, kind: kindRows}, {name: "msp-remove", kind: kindRows},
		{name: "msp-info", args: []string{"a b"}, kind: kindRows}, {name: "msp-info", args: []string{"--"}, kind: kindRows},
		{name: "msp-info", kind: 0}} {
		if err := bad.validate(); err == nil {
			t.Errorf("hand-made %q was not refused", bad.String())
		}
	}
}
