// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: guards the command `doctor` tells operators to run.
// See skills/threatlocker/handfixes.json (doctor-verify-command-runnable).

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDoctorVerifyCandidatesAreRunnable is the regression guard for #217.
//
// The bug it prevents: `doctor` suggested `applications get`, which requires
// --application-id and, on a bare invocation, prints help and returns nil. The
// operator runs the command doctor told them to run, sees exit 0, and reads it
// as "my token works". A credential check that silently passes is worse than no
// check at all.
//
// Asserting the command merely EXISTS would not catch that, because the broken
// suggestion existed too. So this drives each candidate against a fixture and
// requires that it actually issued an HTTP request.
func TestDoctorVerifyCandidatesAreRunnable(t *testing.T) {
	for _, candidate := range doctorVerifyCandidates {
		t.Run(candidate, func(t *testing.T) {
			var hits int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"action":"post","data":[],"success":true}`))
			}))
			defer srv.Close()

			t.Setenv("THREATLOCKER_API_KEY", strings.Repeat("a", 64))
			t.Setenv("THREATLOCKER_BASE_URL", srv.URL)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			t.Setenv("XDG_CACHE_HOME", t.TempDir())

			var flags rootFlags
			root := newRootCmd(&flags)
			args := append(splitCommandPath(candidate), "--data-source", "live", "--no-cache")
			root.SetArgs(args)
			root.SetOut(&strings.Builder{})
			root.SetErr(&strings.Builder{})

			if err := root.Execute(); err != nil {
				t.Fatalf("%q returned %v; doctor must not suggest a command that errors", candidate, err)
			}
			if hits == 0 {
				t.Fatalf("%q made no API request. It printed help or short-circuited, so it cannot verify a token "+
					"and doctor must not suggest it (this is exactly bug #217).", candidate)
			}
		})
	}
}

// TestDoctorRejectsRequiredInputCommand pins the specific command that caused
// #217: a bare `applications get` must never be what doctor suggests.
func TestDoctorRejectsRequiredInputCommand(t *testing.T) {
	var flags rootFlags
	root := newRootCmd(&flags)

	got := threatlockerVerifyCommand(root)
	if got == "" {
		t.Fatal("no verify command resolved; doctor would fall back to the generic message")
	}
	if strings.Contains(got, "applications get") {
		t.Errorf("doctor suggests %q, the required-input command from #217", got)
	}
	if !commandPathExists(root, got) {
		t.Errorf("suggested %q does not resolve to a runnable leaf", got)
	}
}
