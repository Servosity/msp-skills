// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"net/http/httptest"
	"strings"
	"testing"

	"synology-pp-cli/internal/dsm"
)

// plainHTTPWarnings counts the lines of text that warn about plain http://.
func plainHTTPWarnings(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "warning") && strings.Contains(line, "http://") {
			n++
		}
	}
	return n
}

// TestPlainHTTPLoginWarnsOnEveryPath: plain http:// on the single-NAS path
// (auth login, every live command, doctor) sent the password in clear text
// with no warning; only fleet add warned. Every login about to send
// credentials over http:// now prints exactly one warning on stderr (the
// generated session manager and the fleet client both log in through
// dsm.Login), doctor reports a WARN transport row, and an https:// NAS gets
// no warning.
func TestPlainHTTPLoginWarnsOnEveryPath(t *testing.T) {
	srv := fakeABBNAS(t) // http://
	for _, args := range [][]string{
		{"auth", "login"},
		{"system", "dsm", "--json"},
		{"doctor", "--json"},
		{"fleet", "sync", "--json"},
		{"abb", "triage", "8", "--json"},
	} {
		sweepIsolate(t)
		dsm.ForgetPlainHTTPWarnings() // one CLI run is one process
		t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
		t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
		t.Setenv("SYNOLOGY_PASSWORD", "s3cret")
		out, errOut, _ := sweepExecute(t, args...)
		if n := plainHTTPWarnings(errOut); n != 1 {
			t.Errorf("%v: %d plain-http warning(s) on stderr, want exactly 1: %s", args, n, errOut)
		}
		if plainHTTPWarnings(out) != 0 && args[0] != "doctor" {
			t.Errorf("%v: the warning went to stdout: %s", args, out)
		}
		if args[0] == "doctor" && !strings.Contains(out, `"transport": "WARN `) {
			t.Errorf("doctor does not report plain http:// as a WARN: %s", out)
		}
	}

	// fleet add of an http:// NAS with --check: the registration and the
	// login share one warning.
	sweepIsolate(t)
	dsm.ForgetPlainHTTPWarnings()
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	if _, errOut, code := sweepExecute(t, "fleet", "add", "acme", "--url", srv.URL, "--check", "--json"); code != 0 || plainHTTPWarnings(errOut) != 1 {
		t.Errorf("fleet add --check: exit %d, %d warning(s), want 1: %s", code, plainHTTPWarnings(errOut), errOut)
	}

	// https:// is not warned about.
	tlsSrv := httptest.NewTLSServer(srv.Config.Handler)
	defer tlsSrv.Close()
	sweepIsolate(t)
	dsm.ForgetPlainHTTPWarnings()
	t.Setenv("SYNOLOGY_BASE_URL", tlsSrv.URL)
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "s3cret")
	t.Setenv("SYNOLOGY_SKIP_TLS_VERIFY", "1")
	out, errOut, code := sweepExecute(t, "doctor", "--json")
	if code != 0 || plainHTTPWarnings(errOut) != 0 || strings.Contains(out, `"transport": "WARN`) || !strings.Contains(out, `"transport": "https`) {
		t.Errorf("https doctor: exit %d out=%s err=%s", code, out, errOut)
	}
}
