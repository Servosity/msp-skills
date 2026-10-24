// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cove-pp-cli/internal/cliutil"
	"cove-pp-cli/internal/cliutil/testenv"
	"cove-pp-cli/internal/config"
)

// TestDoctorTreatsCovesGlobalHostAsReal: Cove has one global JSON-RPC
// endpoint, so the shipped base_url is the operator's real API. The shipped
// 4.24 build named it a placeholder, so every healthy install got
// "ERROR base_url is still the shipped placeholder" and no Login probe.
func TestDoctorTreatsCovesGlobalHostAsReal(t *testing.T) {
	t.Setenv("COVE_BASE_URL", "")
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{cfg.BaseURL, "https://api.backup.management", "https://api.backup.management/"} {
		if doctorBaseURLIsPlaceholder(base) || doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor calls Cove's real API host %q a placeholder", base)
		}
	}
	// The placeholder refusal itself still works.
	for _, base := range []string{"https://{tenant}.example.com/api", "https://api.example.com", "https://x/YOUR_PARTNER"} {
		if !doctorBaseURLIsPlaceholder(base) && !doctorBaseURLIsShippedPlaceholder(base) {
			t.Errorf("doctor dials placeholder %q", base)
		}
	}
}

// TestDoctorRunsTheCoveLoginEndToEnd drives the real doctor command: with the
// three Cove credentials set, the Credentials row is the Login verdict.
func TestDoctorRunsTheCoveLoginEndToEnd(t *testing.T) {
	var logins int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "Login" {
			logins++
			fmt.Fprint(w, `{"id":"jsonrpc","jsonrpc":"2.0","visa":"doctor-truth-visa-0123456789","result":{"result":{"PartnerId":9000}}}`)
			return
		}
		fmt.Fprint(w, `{"id":"jsonrpc","jsonrpc":"2.0","error":{"code":-32601,"data":0,"message":"method not found"}}`)
	}))
	defer srv.Close()
	fleetTestEnv(t, srv)

	out, err := runCoveCmd(t, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &report); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	if report["auth"] != coveDoctorAuth {
		t.Errorf("auth row = %v, want %q", report["auth"], coveDoctorAuth)
	}
	if report["credentials"] != "ok (JSON-RPC Login accepted)" {
		t.Errorf("credentials row = %v, want the Login verdict", report["credentials"])
	}
	if logins != 1 {
		t.Errorf("doctor performed %d Logins, want 1", logins)
	}
}

// TestDoctorWithheldReachAnswerStillRunsTheLogin: a host whose reachability
// answer repeats a held credential has still answered. The doctor reports it
// reachable and runs the Login probe; the reprint called it unreachable and
// skipped the Login, so no credential was ever checked (class harness,
// auth-wire).
func TestDoctorWithheldReachAnswerStillRunsTheLogin(t *testing.T) {
	const password = "doctor-reach-echo-password-0123456789"
	var logins int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			fmt.Fprint(w, `{"error":"bad request","echo":"`+password+`"}`)
			return
		}
		logins++
		fmt.Fprint(w, `{"id":"jsonrpc","jsonrpc":"2.0","visa":"doctor-reach-visa-0123456789","result":{"result":{"PartnerId":9000}}}`)
	}))
	defer srv.Close()
	fleetTestEnv(t, srv)
	t.Setenv("COVE_PASSWORD", password)

	out, _ := runCoveCmd(t, "doctor", "--json")
	var report map[string]any
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &report); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, out)
	}
	if api, _ := report["api"].(string); !strings.HasPrefix(api, "reachable") {
		t.Errorf("api row = %q, want reachable (the host answered)", api)
	}
	if logins != 1 || report["credentials"] != "ok (JSON-RPC Login accepted)" {
		t.Errorf("logins = %d, credentials row = %v; want the Login verdict", logins, report["credentials"])
	}
	if strings.Contains(out, password) {
		t.Fatalf("doctor showed the password: %s", out)
	}
}
