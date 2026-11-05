// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"os"
	"testing"
)

// Binding reads argv with no parser, so it acts only where it cannot disagree with one:
// a line that starts with fleet add. Everything else keeps the environment's address.
func TestBindOnlyOnFleetAdd(t *testing.T) {
	nas := []registered{{Name: "acme", URL: "https://acme.example.net:5001"}, {Name: "bravo", URL: "https://bravo.example.net:5001"}}
	const other = "https://other.example.net:5001"
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"abb", "triage", "8", "--nas", "bravo"}, ""},
		{[]string{"fleet", "sync", "--nas", "bravo"}, ""},
		{[]string{"auth", "login", "--select", "--nas", "bravo"}, ""},
		{[]string{"auth", "login", "--json", "--", "fleet", "add", "--url", other}, ""},
		{[]string{"--select", "x", "fleet", "add", "--url", other}, ""},
		{[]string{"fleet", "add", "new", "--url", other, "--check"}, other},
		{[]string{"--home", "/h", "fleet", "add", "new", "--url", other}, other},
		{[]string{"--config=/c", "fleet", "add", "new", "--url", other}, other},
		{[]string{"fleet", "add", "bravo", "--check"}, nas[1].URL},
		{[]string{"fleet", "add", "new", "--check"}, ""},
		{[]string{"--home"}, ""},
	} {
		t.Setenv("SYNOLOGY_BASE_URL", "")
		bindProcessToNAS(c.args, nas)
		if got := os.Getenv("SYNOLOGY_BASE_URL"); got != c.want {
			t.Errorf("%q bound to %q, want %q", c.args, got, c.want)
		}
	}
}
