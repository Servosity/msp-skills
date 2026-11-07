// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestAuthLoginFailsWhenProbeReadFails (handfixes.json:
// auth-login-fails-when-probe-fails): `auth login` mints the token and then
// proves it with one read. A token that mints but cannot read (an API user
// without device-management access, an outage) used to print "token minted,
// but the probe call failed" and exit 0, so `auth login && sync` carried on.
func TestAuthLoginFailsWhenProbeReadFails(t *testing.T) {
	for _, tc := range []struct {
		name    string
		probe   int
		wantErr bool
		code    int
	}{
		{"probe ok", http.StatusOK, false, 0},
		{"probe 403", http.StatusForbidden, true, 4},
		{"probe 401", http.StatusUnauthorized, true, 4},
		{"probe 404 (not the ESET API)", http.StatusNotFound, true, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			respondClearHarness(t)
			withTokenServer(t)
			var probes int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/device_groups" {
					atomic.AddInt32(&probes, 1)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.probe)
				_, _ = io.WriteString(w, `{"deviceGroups":[],"nextPageToken":""}`)
			}))
			t.Cleanup(api.Close)
			t.Setenv("ESET_PROTECT_BASE_URL", api.URL)
			t.Setenv("ESET_PROTECT_REGION", "eu")
			t.Setenv("ESET_PROTECT_TOKEN", "")
			t.Setenv("ESET_PROTECT_USERNAME", "api@msp.example")
			t.Setenv("ESET_PROTECT_PASSWORD", "s3cret")

			for _, mode := range [][]string{{"--json"}, {}} {
				cmd := RootCmd()
				var out, errOut bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&errOut)
				cmd.SetArgs(append([]string{"auth", "login"}, mode...))
				err := cmd.Execute()
				if (err != nil) != tc.wantErr {
					t.Fatalf("auth login %v: err = %v, want error = %v\nstdout: %s\nstderr: %s", mode, err, tc.wantErr, out.String(), errOut.String())
				}
				if tc.wantErr && ExitCode(err) != tc.code {
					t.Fatalf("auth login %v: exit code %d, want %d (%v)", mode, ExitCode(err), tc.code, err)
				}
				if tc.wantErr && !strings.Contains(err.Error(), "GET /v1/device_groups") {
					t.Fatalf("auth login %v: error does not name the failed probe: %v", mode, err)
				}
				if len(mode) == 1 {
					var got map[string]any
					if jerr := json.Unmarshal(out.Bytes(), &got); jerr != nil {
						t.Fatalf("auth login --json printed no JSON object: %v\n%s", jerr, out.String())
					}
					if got["probe_ok"] != !tc.wantErr {
						t.Fatalf("probe_ok = %v, want %v", got["probe_ok"], !tc.wantErr)
					}
				}
			}
			if n := atomic.LoadInt32(&probes); n < 2 {
				t.Fatalf("the probe read reached device-management %d times for 2 logins", n)
			}
		})
	}
}
