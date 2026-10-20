// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "doctor-verdict-tracks-reality" entry.

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type doctorNAS struct {
	srv    *httptest.Server
	logins atomic.Int32
}

// newDoctorNAS serves mode: "dsm" (a faithful DSM: SYNO.API.Info, a login for
// msp-admin/s3cretpw, SYNO.DSM.Info), "otp" (DSM demanding a one-time code),
// "json" (any other server answering 200 JSON everywhere) or "html" (a web
// page everywhere).
func newDoctorNAS(t *testing.T, mode string) *doctorNAS {
	t.Helper()
	d := &doctorNAS{}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch mode {
		case "json":
			if r.Form.Get("passwd") != "" {
				d.logins.Add(1)
			}
			_, _ = w.Write([]byte(`{"data":[],"items":[],"results":[]}`))
			return
		case "html":
			if r.Form.Get("passwd") != "" {
				d.logins.Add(1)
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!doctype html><html><body>Synology DiskStation</body></html>"))
			return
		}
		switch r.Form.Get("api") + "/" + r.Form.Get("method") {
		case "SYNO.API.Info/query":
			_, _ = w.Write([]byte(`{"success":true,"data":{"SYNO.API.Auth":{"path":"entry.cgi","minVersion":1,"maxVersion":7}}}`))
		case "SYNO.API.Auth/login":
			d.logins.Add(1)
			switch {
			case mode == "otp":
				_, _ = w.Write([]byte(`{"success":false,"error":{"code":403}}`))
			case r.Form.Get("account") == "msp-admin" && r.Form.Get("passwd") == "s3cretpw":
				_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`))
			default:
				_, _ = w.Write([]byte(`{"success":false,"error":{"code":400}}`))
			}
		case "SYNO.API.Auth/logout":
			_, _ = w.Write([]byte(`{"success":true}`))
		case "SYNO.DSM.Info/getinfo":
			if r.Form.Get("_sid") != "sid-1" {
				_, _ = w.Write([]byte(`{"success":false,"error":{"code":119}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"model":"DS1621+"}}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
		}
	}))
	t.Cleanup(d.srv.Close)
	return d
}

func runDoctor(t *testing.T) map[string]any {
	t.Helper()
	out, _, code := sweepExecute(t, "doctor", "--json")
	if code != 0 {
		t.Fatalf("doctor exited %d: %s", code, out)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("doctor output is not JSON: %v\n%s", err, out)
	}
	return report
}

func verdict(report map[string]any, key string) string {
	s, _ := report[key].(string)
	return s
}

// TestDoctorVerdictTracksReality: the generated doctor called any answer to
// GET / "reachable" (DSM's own login page included) and never verified the
// DSM login; with SYNOLOGY_ACCOUNT/PASSWORD set it printed no credentials row
// and auth "not configured".
func TestDoctorVerdictTracksReality(t *testing.T) {
	cases := []struct {
		name, mode, password string
		api, creds           string // required prefixes
		loginSent            bool
	}{
		{"working DSM login", "dsm", "s3cretpw", "reachable", "valid", true},
		{"wrong password", "dsm", "wrong", "reachable", "ERROR rejected", true},
		{"two-factor code needed", "otp", "s3cretpw", "reachable", "WARN not verified", true},
		{"a JSON server that is not DSM", "json", "s3cretpw", "ERROR", "ERROR not verified", false},
		{"a web page", "html", "s3cretpw", "ERROR", "ERROR not verified", false},
		{"DSM without a login configured", "dsm", "", "reachable", "ERROR not configured", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sweepIsolate(t)
			nas := newDoctorNAS(t, tc.mode)
			t.Setenv("SYNOLOGY_BASE_URL", nas.srv.URL)
			if tc.password != "" {
				t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
				t.Setenv("SYNOLOGY_PASSWORD", tc.password)
			}
			report := runDoctor(t)
			if api := verdict(report, "api"); !strings.HasPrefix(api, tc.api) {
				t.Fatalf("api = %q, want prefix %q", api, tc.api)
			}
			if creds := verdict(report, "credentials"); !strings.HasPrefix(creds, tc.creds) {
				t.Fatalf("credentials = %q, want prefix %q", creds, tc.creds)
			}
			if sent := nas.logins.Load() > 0; sent != tc.loginSent {
				t.Fatalf("login sent = %v, want %v (the password must never reach a host that is not DSM)", sent, tc.loginSent)
			}
			if tc.password == "s3cretpw" && tc.mode == "dsm" && verdict(report, "auth") != "configured" {
				t.Fatalf("auth = %q with SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD set", verdict(report, "auth"))
			}
		})
	}
	t.Run("placeholder address names SYNOLOGY_BASE_URL", func(t *testing.T) {
		sweepIsolate(t)
		t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
		t.Setenv("SYNOLOGY_PASSWORD", "s3cretpw")
		report := runDoctor(t)
		api, creds := verdict(report, "api"), verdict(report, "credentials")
		if !strings.HasPrefix(api, "ERROR") || !strings.Contains(api, "SYNOLOGY_BASE_URL") || strings.HasPrefix(creds, "valid") {
			t.Fatalf("placeholder: api=%q credentials=%q", api, creds)
		}
	})
}
