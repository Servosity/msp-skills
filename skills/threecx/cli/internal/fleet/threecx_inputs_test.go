// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// Class unknown-telemetry-reported-as-healthy, the one rule: a verdict is
// healthy only when every input it uses was reported; a missing input makes
// it unknown (or keeps a worse verdict), never pass, never warn.
const inputsHealthyStatus = `{"ExtensionsRegistered":8,"ExtensionsTotal":10,"Version":"20.0.10.1479","ProductCode":"3CXPS","LicenseActive":true,
"ExpirationDate":"2099-01-01T00:00:00Z","MaintenanceExpiresAt":"2099-01-01T00:00:00Z",
"LastBackupDateTime":"%s","BackupScheduled":true,"TrunksRegistered":2,"TrunksTotal":2,
"TotalDiskSpace":100,"FreeDiskSpace":60,"HasNotRunningServices":false,"HasUnregisteredSystemExtensions":false,
"RecordingStopped":false,"RecordingQuotaReached":false,"VoicemailStopped":false,"VoicemailQuotaReached":false,
"Firmware3CXOutdated":false,"MaxSimCalls":8,"CallsActive":1}`

// healthInputs maps every SystemStatus field a health check reads to it.
var healthInputs = map[string]string{
	"LicenseActive": "license", "ExpirationDate": "license", "MaintenanceExpiresAt": "maintenance",
	"LastBackupDateTime": "backup", "BackupScheduled": "backup", "TrunksRegistered": "trunks", "TrunksTotal": "trunks",
	"TotalDiskSpace": "disk", "FreeDiskSpace": "disk", "HasNotRunningServices": "services",
	"HasUnregisteredSystemExtensions": "system_extensions", "RecordingStopped": "storage_quota",
	"RecordingQuotaReached": "storage_quota", "VoicemailStopped": "storage_quota", "VoicemailQuotaReached": "storage_quota",
	"Firmware3CXOutdated": "phone_firmware", "MaxSimCalls": "capacity", "CallsActive": "capacity",
}

func evaluate(t *testing.T, body map[string]any) HealthRow {
	t.Helper()
	b, _ := json.Marshal(body)
	var s SystemStatus
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return EvaluateHealth(PBX{Name: "acme"}, s, HealthOptions{Now: time.Now().UTC(), WarnDays: 30, BackupStale: 36 * time.Hour})
}

func healthyBody(t *testing.T) map[string]any {
	t.Helper()
	var body map[string]any
	recent := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if err := json.Unmarshal([]byte(sprintf(inputsHealthyStatus, recent)), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestHealthIsPassOnlyWhenEveryInputWasReported(t *testing.T) {
	if row := evaluate(t, healthyBody(t)); row.Status != Pass {
		t.Fatalf("fully reported healthy PBX: status %s (%s)", row.Status, row.Problems)
	}
	for field, check := range healthInputs {
		for _, missing := range []any{nil, "drop"} {
			body := healthyBody(t)
			if missing == nil {
				body[field] = nil
			} else {
				delete(body, field)
			}
			row := evaluate(t, body)
			var got *Check
			for i := range row.Checks {
				if row.Checks[i].Name == check {
					got = &row.Checks[i]
				}
			}
			if got == nil {
				t.Errorf("%s missing: check %s dropped", field, check)
				continue
			}
			if !Worse(got.Level, Warn) {
				t.Errorf("%s missing: check %s is %s, want unknown or worse", field, check, got.Level)
			}
			if !Worse(row.Status, Warn) {
				t.Errorf("%s missing: row status %s, want unknown or worse", field, row.Status)
			}
		}
	}
	// A limit reported as 0 says nothing about capacity: unknown, not dropped.
	body := healthyBody(t)
	body["MaxSimCalls"] = 0
	row := evaluate(t, body)
	found := false
	for _, c := range row.Checks {
		if c.Name == "capacity" {
			found = true
			if !Worse(c.Level, Warn) {
				t.Errorf("MaxSimCalls 0: capacity %s, want unknown", c.Level)
			}
		}
	}
	if !found {
		t.Error("MaxSimCalls 0: capacity check dropped")
	}
}

// A licence row is outside a renewal window only when every date and the
// activation it depends on were reported.
func TestLicenceOutsideWindowNeedsEveryInput(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	active, inactive := true, false
	far := "2099-01-01T00:00:00Z"
	for name, c := range map[string]struct {
		l    LicenseStatus
		keep bool
	}{
		"all reported and far":          {LicenseStatus{LicenseActive: &active, ExpirationDate: far, MaintenanceExpiresAt: far}, false},
		"maintenance expiry null":       {LicenseStatus{LicenseActive: &active, ExpirationDate: far}, true},
		"licence expiry null":           {LicenseStatus{LicenseActive: &active, MaintenanceExpiresAt: far}, true},
		"activation null":               {LicenseStatus{ExpirationDate: far, MaintenanceExpiresAt: far}, true},
		"inactive":                      {LicenseStatus{LicenseActive: &inactive, ExpirationDate: far, MaintenanceExpiresAt: far}, true},
		"maintenance inside the window": {LicenseStatus{LicenseActive: &active, ExpirationDate: far, MaintenanceExpiresAt: "2026-11-01T00:00:00Z"}, true},
	} {
		row := LicenceRowFor(PBX{Name: "acme"}, c.l, SystemStatus{}, "", now)
		if got := !row.OutsideWindow(30); got != c.keep {
			t.Errorf("%s: kept %v, want %v (row %+v)", name, got, c.keep, row)
		}
		if c.keep && name != "inactive" && name != "maintenance inside the window" && len(row.Unknown) == 0 {
			t.Errorf("%s: the row does not name what was not reported", name)
		}
	}
}

// Maintenance is not expired only when both sources reported it current;
// a missing update list is not "no pending updates".
func TestVersionVerdictsNeedEveryInput(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	f := false
	var u UpdateList
	if err := json.Unmarshal([]byte(`{"Entries":[],"IsMaintananceExpired":false}`), &u); err != nil {
		t.Fatal(err)
	}
	row := VersionRowFor(PBX{Name: "acme"}, SystemStatus{Version: "20.0.10.1479"}, u, now)
	if row.MaintenanceExpired != nil && !*row.MaintenanceExpired {
		t.Error("maintenance expiry date not reported, yet maintenance reads as current")
	}
	if !row.NeedsAttention() {
		t.Error("partial maintenance facts: the row reads as current")
	}
	u = UpdateList{IsMaintananceExpired: &f}
	row = VersionRowFor(PBX{Name: "acme"}, SystemStatus{Version: "20.0.10.1479", MaintenanceExpiresAt: "2099-01-01T00:00:00Z"}, u, now)
	if !row.NeedsAttention() {
		t.Error("no update list reported (Entries null), yet the row reads as current")
	}
	if err := json.Unmarshal([]byte(`{"Entries":[],"IsMaintananceExpired":false}`), &u); err != nil {
		t.Fatal(err)
	}
	row = VersionRowFor(PBX{Name: "acme"}, SystemStatus{Version: "20.0.10.1479", MaintenanceExpiresAt: "2099-01-01T00:00:00Z"}, u, now)
	row.Behind = &f
	if row.NeedsAttention() {
		t.Errorf("fully reported current PBX needs attention: %+v", row)
	}
}

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }
