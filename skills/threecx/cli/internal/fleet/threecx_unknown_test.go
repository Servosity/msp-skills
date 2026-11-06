// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Class unknown-telemetry-reported-as-healthy: every boolean or number a 3CX
// body carries is nullable in vendor-docs/3cx-swagger.yaml (Pbx.SystemStatus,
// Pbx.UpdateList, Pbx.UpdateItem, Pbx.BackupSettings, Pbx.BackupContents,
// Pbx.Backups, Pbx.Trunk, Pbx.User, Pbx.LicenseStatus). A plain Go bool or
// number decodes null as false or 0, a healthy-looking value, so every such
// field the fleet reads is a pointer (nil = not reported).
func TestNoVendorFactDecodesNullAsAValue(t *testing.T) {
	var walk func(path string, typ reflect.Type)
	walk = func(path string, typ reflect.Type) {
		switch typ.Kind() {
		case reflect.Ptr:
			if e := typ.Elem(); e.Kind() == reflect.Struct {
				walk(path, e)
			}
			return
		case reflect.Slice:
			walk(path+"[]", typ.Elem())
			return
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				walk(path+"."+f.Name, f.Type)
			}
			return
		case reflect.Bool, reflect.Int, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64:
			t.Errorf("%s is a plain %s: a null from the phone system would read as %v", path, typ.Kind(), reflect.Zero(typ))
		}
	}
	for _, v := range []any{SystemStatus{}, LicenseStatus{}, UpdateList{}, BackupSettings{}, Backup{}, Trunk{}, RoutingGraph{}} {
		walk(reflect.TypeOf(v).Name(), reflect.TypeOf(v))
	}
}

func mustDecode(t *testing.T, body string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("%s: %v", body, err)
	}
}

// A null or missing fact never yields a passing verdict: health reports the
// check as unknown (so the PBX is not "pass"), backups keep unknown contents
// and sizes unknown and flag an unknown schedule, and the routing audit
// reports a reference to a user whose state was not reported.
func TestUnknownTelemetryIsNeverHealthy(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	p := PBX{Name: "acme"}

	var s SystemStatus
	mustDecode(t, `{"Version":"20.0.10.1479","LicenseActive":true,"ExpirationDate":"2027-06-01T00:00:00Z",
		"MaintenanceExpiresAt":"2027-06-01T00:00:00Z","LastBackupDateTime":"2026-10-27T06:00:00Z","BackupScheduled":true,
		"TrunksRegistered":1,"TrunksTotal":1,"TotalDiskSpace":100,"FreeDiskSpace":60,"HasNotRunningServices":null}`, &s)
	row := EvaluateHealth(p, s, HealthOptions{Now: now, WarnDays: 30, BackupStale: 48 * time.Hour})
	if row.Status == Pass {
		t.Errorf("health with unreported services, extensions, quotas, firmware and capacity is %q: %+v", row.Status, row.Checks)
	}
	var services string
	for _, c := range row.Checks {
		if c.Name == "services" {
			services = string(c.Level)
		}
	}
	if services != "unknown" {
		t.Errorf("services check = %q, want unknown (HasNotRunningServices is null)", services)
	}

	var set BackupSettings
	mustDecode(t, `{"ScheduleEnabled":null,"Contents":{"Recordings":null,"EncryptBackup":null}}`, &set)
	var files []Backup
	mustDecode(t, `[{"FileName":"new.zip","CreationTime":"2026-10-27T06:00:00Z","Size":null},{"FileName":"old.zip","CreationTime":"2026-10-26T06:00:00Z","Size":5000000}]`, &files)
	var bs SystemStatus
	mustDecode(t, `{"LastBackupDateTime":"2026-10-27T06:00:00Z","BackupScheduled":null}`, &bs)
	b := BackupRowFor(p, bs, set, files, 48*time.Hour, now)
	if b.IncludesRecords != nil || b.Encrypted != nil {
		t.Errorf("null backup contents read as %v/%v, want unknown", deref(b.IncludesRecords), deref(b.Encrypted))
	}
	if b.NewestSizeMB != nil {
		t.Errorf("a null backup size read as %v MB, want unknown", *b.NewestSizeMB)
	}
	if b.Status == "ok" || !strings.Contains(strings.Join(b.Flags, ","), "SCHEDULE_UNKNOWN") {
		t.Errorf("unreported schedule: status %q flags %v, want SCHEDULE_UNKNOWN", b.Status, b.Flags)
	}
	if !strings.Contains(strings.Join(b.Flags, ","), "SIZE_UNKNOWN") {
		t.Errorf("unreported newest backup size: flags %v, want SIZE_UNKNOWN", b.Flags)
	}

	// Health: every fact reported but whether a backup is scheduled.
	full := s
	for _, f := range []**bool{&full.HasNotRunningServices, &full.HasUnregisteredSystemExtensions, &full.RecordingStopped,
		&full.RecordingQuotaReached, &full.VoicemailStopped, &full.VoicemailQuotaReached, &full.Firmware3CXOutdated} {
		no := false
		*f = &no
	}
	calls, maxCalls := 1, 8
	full.CallsActive, full.MaxSimCalls, full.BackupScheduled = &calls, &maxCalls, nil
	for _, c := range EvaluateHealth(p, full, HealthOptions{Now: now, WarnDays: 30, BackupStale: 48 * time.Hour}).Checks {
		if c.Name == "backup" && c.Level != Unknown {
			t.Errorf("backup with an unreported schedule = %+v, want unknown", c)
		}
	}

	var g RoutingGraph
	mustDecode(t, `{"Users":[{"Number":"100","Enabled":null}],"RingGroups":[{"Number":"800","Name":"Sales","Members":[{"Number":"100"}]}]}`, &g)
	var unknownRef bool
	for _, f := range Audit("acme", g) {
		if f.Target == "100" && f.Problem == "unknown" {
			unknownRef = true
		}
	}
	if !unknownRef {
		t.Errorf("a ring group member whose enabled state is null was not reported: %+v", Audit("acme", g))
	}
}

func deref(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}
