// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"strings"
	"testing"
)

// TestReportQuietPrintsRowIdentity: --quiet on the fleet and abb reports
// printed the first sorted scalar of each row ("true" for fleet status, a
// result id for abb failures). It prints the row's identity: the NAS for the
// fleet reports, the task or host for the abb reports (the NAS for a row
// about a whole NAS).
func TestReportQuietPrintsRowIdentity(t *testing.T) {
	sweepIsolate(t)
	srv := fakeABBNAS(t)
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	if _, errText, code := runCLIErr(t, "fleet", "add", "acme", "--url", srv.URL, "--json"); code != 0 {
		t.Fatalf("fleet add: %d %s", code, errText)
	}
	if _, errText, code := runCLIErr(t, "fleet", "add", "never", "--url", "https://never.invalid:5001", "--json"); code != 0 {
		t.Fatalf("fleet add never: %d %s", code, errText)
	}
	if out, _, code := runCLIErr(t, "fleet", "sync", "--nas", "acme", "--quiet"); code != 0 || out != "acme\n" {
		t.Fatalf("fleet sync --quiet: exit %d %q", code, out)
	}
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"fleet", "status"}, []string{"acme", "never"}},
		{[]string{"fleet", "list"}, []string{"acme", "never"}},
		{[]string{"fleet", "health"}, []string{"acme", "never"}},
		{[]string{"abb", "failures"}, []string{"File Server", "never"}},
		{[]string{"abb", "stale", "--older-than", "1h"}, []string{"FS01", "BOB-LAPTOP", "never"}},
		{[]string{"abb", "trend"}, []string{"File Server", "Office PCs", "never"}},
		{[]string{"abb", "device", "FS01"}, []string{"FS01", "never"}},
		{[]string{"abb", "triage", "8", "--nas", "acme"}, []string{"File Server"}},
	} {
		args := append(c.args, "--quiet")
		if c.args[0] != "abb" || c.args[1] != "triage" {
			if c.args[1] != "list" {
				args = append(args, "--no-fail")
			}
		}
		out, errText, _ := runCLIErr(t, args...)
		got := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			got[line] = true
		}
		for _, w := range c.want {
			if !got[w] {
				t.Fatalf("%v --quiet = %q (err %s), want identities %q", c.args, out, errText, c.want)
			}
		}
		for line := range got {
			if line == "true" || line == "false" || strings.TrimLeft(line, "0123456789") == "" {
				t.Fatalf("%v --quiet printed a non-identity value %q: %q", c.args, line, out)
			}
		}
	}
}
