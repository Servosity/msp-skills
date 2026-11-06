// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cometblackbox

// comet-backup handfix (doctor-verdict-tracks-reality, issue #282): `doctor`
// is what every guide tells an operator to run. It must make one
// authenticated Comet call the way every command does (a POST to
// /api/v1/admin/... with Username, AuthType and Password form fields), and its
// verdict must track reality: positive only when that call proved the admin
// login works, negative when the login is refused (HTTP 401/403 or a Comet
// failure envelope on HTTP 200), when base_url is not a Comet Server (404, an
// HTML page, JSON that is not Comet's), and when the shipped placeholder base
// URL is still in force; the last two name the variables that fix it. Its own
// report keys ("credentials") are not credential fields, so the verdicts are
// printed as written, and the configured password is never printed.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// doctorNegative mirrors tools/maintainer/check_doctor_truth.py: a verdict is
// negative when it opens with one of these markers or contains one of these
// phrases; everything else claims health.
var (
	doctorNegativePrefixes = []string{"ERROR", "WARN", "INFO", "FAIL"}
	doctorNegativePhrases  = []string{"not verified", "not configured", "unreachable", "invalid", "rejected",
		"scope-limited", "skipped", "placeholder", "not there", "error", "refused", "not loaded", "transport level only"}
)

func doctorPositive(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	s = strings.TrimSpace(s)
	for _, p := range doctorNegativePrefixes {
		if strings.HasPrefix(s, p) {
			return false
		}
	}
	low := strings.ToLower(s)
	for _, p := range doctorNegativePhrases {
		if strings.Contains(low, p) {
			return false
		}
	}
	return true
}

// cometStub answers every request the same way for one state and records the
// admin login each Comet API call carried.
type cometStub struct {
	mu     sync.Mutex
	logins []url.Values
}

func newCometStub(t *testing.T, state string) (*cometStub, string, string) {
	t.Helper()
	st := &cometStub{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			_ = r.ParseForm()
			st.mu.Lock()
			st.logins = append(st.logins, r.PostForm)
			st.mu.Unlock()
		}
		send := func(code int, ctype, body string) {
			w.Header().Set("Content-Type", ctype)
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}
		const html = "<!doctype html><html><head><title>Comet</title></head><body>app</body></html>"
		switch state {
		case "healthy":
			if r.URL.Path == "/api/v1/admin/meta/version" {
				send(200, "application/json", `{"Version":"24.6.3","VersionCodename":"Voyager","ServerStartTime":1700000000}`)
			} else {
				send(200, "text/html", html)
			}
		case "rejected-http":
			send(403, "application/json", `{"Status":403,"Message":"Incorrect password"}`)
		case "rejected-envelope":
			send(200, "application/json", `{"Status":403}`)
		case "expired":
			send(401, "application/json", `{"error":"unauthorized"}`)
		case "wrong-base-404":
			if r.URL.Path == "/" {
				send(200, "text/html", html)
			} else {
				send(404, "application/json", `{"error":"not found"}`)
			}
		case "wrong-base-html":
			send(200, "text/html", html)
		case "not-comet-json":
			send(200, "application/json", `{"data":[],"items":[],"results":[]}`)
		}
	}))
	t.Cleanup(srv.Close)
	return st, srv.URL, tlsCAFile(t, srv)
}

func runDoctor(t *testing.T, e *bbEnv) (map[string]any, string) {
	t.Helper()
	out := e.run(t, "doctor", "--json")
	start := strings.Index(out, "{")
	if start < 0 {
		t.Fatalf("doctor printed no JSON:\n%s", out)
	}
	var report map[string]any
	dec := json.NewDecoder(strings.NewReader(out[start:]))
	if err := dec.Decode(&report); err != nil {
		t.Fatalf("doctor JSON: %v\n%s", err, out)
	}
	return report, out
}

func TestDoctorVerdictTracksReality(t *testing.T) {
	t.Parallel()
	cases := []struct {
		state       string
		positive    bool // credentials verdict claims health
		apiNegative bool // api verdict must not claim health
		namesEnv    bool // the remedy names COMET_BACKUP_SERVER and COMET_BACKUP_BASE_URL
	}{
		{state: "healthy", positive: true},
		{state: "rejected-http"},
		{state: "rejected-envelope"},
		{state: "expired"},
		{state: "wrong-base-404", apiNegative: true, namesEnv: true},
		{state: "wrong-base-html", apiNegative: true, namesEnv: true},
		{state: "not-comet-json", apiNegative: true, namesEnv: true},
		{state: "placeholder", apiNegative: true, namesEnv: true},
	}
	verdicts := map[string]string{}
	for _, c := range cases {
		t.Run(c.state, func(t *testing.T) {
			set := map[string]string{"COMET_BACKUP_BASE_URL": "", "COMET_BACKUP_SERVER": ""}
			var st *cometStub
			if c.state != "placeholder" {
				var u string
				var ca string
				st, u, ca = newCometStub(t, c.state)
				set["COMET_BACKUP_BASE_URL"], set["COMET_BACKUP_CA_CERT"], set["SSL_CERT_FILE"] = u, ca, ca
			}
			report, out := runDoctor(t, isolatedEnv(t, set))
			assertNoEcho(t, "doctor output", out)
			creds, api := report["credentials"], report["api"]
			verdicts[c.state], _ = creds.(string)
			if creds == nil {
				t.Fatalf("no credentials verdict:\n%s", out)
			}
			if s, _ := creds.(string); s == "[redacted]" || strings.Contains(s, "text withheld") {
				t.Fatalf("doctor's own credentials verdict was redacted: %q", s)
			}
			if s, _ := api.(string); strings.Contains(s, "text withheld") {
				t.Fatalf("doctor's api verdict was withheld: %q", s)
			}
			if got := doctorPositive(creds); got != c.positive {
				t.Errorf("credentials positive=%v, want %v: %q", got, c.positive, creds)
			}
			if c.apiNegative && doctorPositive(api) {
				t.Errorf("api claims health in state %s: %q", c.state, api)
			}
			if c.namesEnv {
				remedy, _ := api.(string)
				for _, name := range []string{"COMET_BACKUP_SERVER", "COMET_BACKUP_BASE_URL"} {
					if !strings.Contains(remedy, name) {
						t.Errorf("api remedy does not name %s: %q", name, remedy)
					}
				}
			}
			if c.state == "healthy" {
				if s, _ := api.(string); !strings.Contains(s, "24.6.3") {
					t.Errorf("api verdict does not name the Comet Server version: %q", s)
				}
				if a, _ := report["agentcookie"].(string); a == "[redacted]" {
					t.Errorf("doctor's own agentcookie row was redacted")
				}
			}
			if st == nil || c.state == "wrong-base-html" {
				return
			}
			// The probe is authenticated the Comet way: the admin login rides
			// in the POST form body.
			st.mu.Lock()
			defer st.mu.Unlock()
			sent := false
			for _, f := range st.logins {
				if f.Get("Username") == "admin" && f.Get("AuthType") == "Password" && f.Get("Password") == bbPassword {
					sent = true
				}
			}
			if !sent {
				t.Errorf("doctor never made an authenticated Comet call (form logins seen: %d)", len(st.logins))
			}
		})
	}
	for _, broken := range []string{"rejected-http", "rejected-envelope", "expired", "wrong-base-404", "not-comet-json"} {
		if verdicts[broken] != "" && verdicts[broken] == verdicts["healthy"] {
			t.Errorf("doctor reports the same credentials verdict when healthy and when %s: %q", broken, verdicts[broken])
		}
	}
}

// With no admin login configured, doctor says so instead of reporting a
// credential it never sent as valid.
func TestDoctorWithoutLoginIsNotValid(t *testing.T) {
	t.Parallel()
	_, u, ca := newCometStub(t, "healthy")
	report, out := runDoctor(t, isolatedEnv(t, map[string]string{
		"COMET_BACKUP_BASE_URL": u, "COMET_BACKUP_CA_CERT": ca, "SSL_CERT_FILE": ca, "COMET_BACKUP_USERNAME": "", "COMET_BACKUP_PASSWORD": "",
	}))
	if doctorPositive(report["credentials"]) {
		t.Fatalf("no login configured, yet credentials claim health: %q\n%s", report["credentials"], out)
	}
	if s, _ := report["credentials"].(string); !strings.Contains(s, "COMET_BACKUP_USERNAME") {
		t.Errorf("the remedy does not name COMET_BACKUP_USERNAME: %q", s)
	}
}
