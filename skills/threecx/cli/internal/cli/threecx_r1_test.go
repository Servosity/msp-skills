// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"threecx-pp-cli/internal/cliutil/testenv"
	"threecx-pp-cli/internal/config"
)

func writeFixture(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Class unknown-telemetry-reported-as-healthy (round 1 input): GetUpdates
// answers IsMaintananceExpired null (Pbx.UpdateList, nullable) while
// SystemStatus says maintenance ended on 2026-10-01; --behind must keep the
// PBX and report maintenance as expired, never drop it as current.
func TestFleetVersionsBehindKeepsUnknownAndExpiredMaintenance(t *testing.T) {
	dir := setupFleet(t)
	writeFixture(t, dir, "acme_SystemStatus.json", `{"Version":"20.0.10.1479","MaintenanceExpiresAt":"2026-10-01T00:00:00Z"}`)
	writeFixture(t, dir, "acme_GetUpdates__.json", `{"Entries":[],"IsMaintananceExpired":null,"UpdatesRunning":false}`)
	out, err := runCLI(t, "fleet", "versions", "--only", "acme", "--behind", "--json")
	if err != nil {
		t.Fatalf("fleet versions: %v\n%s", err, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0]["pbx"] != "acme" || rows[0]["maintenance_expired"] != true {
		t.Fatalf("want acme kept with maintenance_expired true, got %s", out)
	}

	// An update entry whose OutOfDate is null, and a PBX that reported no
	// build, are not known to be current either.
	writeFixture(t, dir, "acme_SystemStatus.json", `{"MaintenanceExpiresAt":"2099-01-01T00:00:00Z"}`)
	writeFixture(t, dir, "acme_GetUpdates__.json", `{"Entries":[{"Name":"PBX","OutOfDate":null,"Ignore":null}],"IsMaintananceExpired":false,"UpdatesRunning":null}`)
	out, err = runCLI(t, "fleet", "versions", "--only", "acme", "--behind", "--json")
	if err != nil {
		t.Fatalf("fleet versions: %v\n%s", err, out)
	}
	rows = nil
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("want acme kept (no build reported, an update of unknown state), got %s", out)
	}
}

// Class unknown-telemetry-reported-as-healthy: a PBX that reported neither
// expiry date (Pbx.LicenseStatus and Pbx.SystemStatus, both nullable) is not
// known to be outside the renewal window, and an inactive licence is a
// renewal problem whatever its date.
func TestFleetLicencesWithinKeepsUnknownAndInactive(t *testing.T) {
	dir := setupFleet(t)
	writeFixture(t, dir, "acme_LicenseStatus.json", `{"CompanyName":"Acme","LicenseActive":true}`)
	writeFixture(t, dir, "acme_SystemStatus.json", `{"Version":"20.0.10.1479"}`)
	writeFixture(t, dir, "globex_LicenseStatus.json", `{"CompanyName":"Globex","LicenseActive":false,"ExpirationDate":"2099-01-01T00:00:00Z","MaintenanceExpiresAt":"2099-01-01T00:00:00Z"}`)
	out, err := runCLI(t, "fleet", "licences", "--only", "acme,globex", "--within", "30", "--json")
	if err != nil {
		t.Fatalf("fleet licences: %v\n%s", err, out)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r["pbx"].(string)] = true
	}
	if !got["acme"] || !got["globex"] {
		t.Fatalf("want acme (no expiry reported) and globex (inactive) kept, got %s", out)
	}
}

// Class preview-sends-anyway, both transports: a client built under
// --dry-run (every API client through guardClient, the token-endpoint client
// through guardTokenClient) refuses every request, whatever calls it, and the
// token client is pinned to the configured phone system's origin.
func TestDryRunClientsRefuseEveryRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok-dryrun-1234567890","expires_in":3600}`)
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("THREECX_BASE_URL", srv.URL+"/xapi/v1")
	t.Setenv("THREECX_CLIENT_ID", "900")
	t.Setenv("THREECX_CLIENT_SECRET", "dryRunSecret-0a1b2c3d")

	flags := &rootFlags{dryRun: true, timeout: 5 * time.Second}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.HTTPClient.Get(srv.URL + "/xapi/v1/SystemStatus"); !errors.Is(err, ErrDryRunSend) {
		t.Errorf("API client under --dry-run: want ErrDryRunSend, got %v", err)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	hc, err := flags.oauthHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hc.Post(srv.URL+"/connect/token", "application/x-www-form-urlencoded", strings.NewReader("grant_type=client_credentials")); !errors.Is(err, ErrDryRunSend) {
		t.Errorf("token client under --dry-run: want ErrDryRunSend, got %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("--dry-run clients sent %d request(s)", hits.Load())
	}

	// Without --dry-run the token client reaches the configured origin only.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(100) }))
	t.Cleanup(other.Close)
	live := &rootFlags{timeout: 5 * time.Second}
	hc, err = live.oauthHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hc.Post(other.URL+"/connect/token", "application/x-www-form-urlencoded", strings.NewReader("grant_type=client_credentials")); !errors.Is(err, ErrOriginChanged) {
		t.Errorf("token client to another host: want ErrOriginChanged, got %v", err)
	}
	if resp, err := hc.Post(srv.URL+"/connect/token", "application/x-www-form-urlencoded", strings.NewReader("grant_type=client_credentials")); err != nil {
		t.Errorf("token client to the configured origin: %v", err)
	} else {
		_ = resp.Body.Close()
	}
	if hits.Load() != 1 {
		t.Fatalf("want exactly one request, to the configured origin; got hits=%d", hits.Load())
	}
}
