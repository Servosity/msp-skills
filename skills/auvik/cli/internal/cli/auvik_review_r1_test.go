// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written: regression tests for the 4.35.1 reprint review, round 1. Each
// test reproduces a vendor-documented response shape that made a novel report a
// wrong answer an MSP would act on. The shapes are cited from Auvik's own
// OpenAPI documents (auvik-v1/v2), not invented.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"auvik-pp-cli/internal/store"
)

// deviceDetailsAttributes.discoveryStatus is an OBJECT of four probe enums
// (auvik-v1 #/components/schemas/deviceDetailsAttributes_discoveryStatus). A
// device whose only discovery evidence is its detail record, with SNMP
// credentials rejected, is a gap, not "fully discovered".
func TestDiscoveryGapsReadsTheDetailProbeObject(t *testing.T) {
	devices := []auvikRow{device("d1", "core-sw", "switch", "m", "t1")}
	detail := auvikRow{ID: "d1", Data: map[string]any{
		"id": "d1", "type": "deviceDetail",
		"attributes": map[string]any{
			"discoveryStatus": map[string]any{
				"snmp": "notAuthorized", "login": "disabled", "wmi": "disabled", "vmware": "disabled",
			},
			"manageStatus": true,
		},
	}}
	got := buildDiscoveryGapsReport(devices, []auvikRow{detail}, nil, tenantMap())
	if got.FullyDiscovered != 0 || len(got.Gaps) != 1 {
		t.Fatalf("detail probe snmp=notAuthorized must be a gap: fully=%d gaps=%+v", got.FullyDiscovered, got.Gaps)
	}
	g := got.Gaps[0]
	if len(g.MissingWhat) != 1 || g.MissingWhat[0] != "snmp" {
		t.Errorf("credentials_rejected = %v, want [snmp]", g.MissingWhat)
	}
	if strings.Contains(g.Status, "map[") || strings.Contains(g.Reason, "map[") {
		t.Errorf("a Go map dump leaked into the report: status=%q reason=%q", g.Status, g.Reason)
	}
}

// When both sources speak, any failure signal wins: a v2 status that still says
// "authorized" cannot hide the detail record's rejected credential.
func TestDiscoveryGapsAnyFailureSignalWins(t *testing.T) {
	devices := []auvikRow{device("d1", "core-sw", "switch", "m", "t1")}
	status := auvikRow{ID: "d1", Data: map[string]any{
		"id": "d1", "type": "deviceDiscoveryStatus",
		"attributes": map[string]any{"snmp": "authorized", "login": "authorized"},
	}}
	detail := auvikRow{ID: "d1", Data: map[string]any{
		"id": "d1", "type": "deviceDetail",
		"attributes": map[string]any{"discoveryStatus": map[string]any{"snmp": "notAuthorized"}},
	}}
	got := buildDiscoveryGapsReport(devices, []auvikRow{detail}, []auvikRow{status}, tenantMap())
	if len(got.Gaps) != 1 || len(got.Gaps[0].MissingWhat) != 1 || got.Gaps[0].MissingWhat[0] != "snmp" {
		t.Fatalf("a rejected probe in either source must be a gap, got %+v", got.Gaps)
	}
}

// clientUsageAttributes.monitoredServers / monitoredWorkstations are ARRAYS of
// daily counts (auvik-v1 #/components/schemas/clientUsageAttributes). Reading
// them as a scalar silently reported zero servers on every invoice row.
func TestReconcileReadsTheDocumentedDailyUsageArrays(t *testing.T) {
	devices := []auvikRow{device("dev-1", "sw-1", "switch", "m", "t1")}
	billing := []auvikRow{specRow("t1", map[specField]any{
		fUsageDomainPrefix: "acme",
		fUsageMonServers: []any{
			map[string]any{"date": "2026-10-31", "totalServers": float64(1)},
			map[string]any{"date": "2026-11-01", "totalServers": float64(2)},
		},
		fUsageMonWorkstations: []any{
			map[string]any{"date": "2026-11-01", "totalEndpoints": float64(3)},
		},
	}, nil)}
	got := buildReconcileReport(devices, billing, tenantMap())
	if len(got.Clients) != 1 {
		t.Fatalf("want one client row, got %+v", got.Clients)
	}
	b, err := json.Marshal(got.Clients[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"monitored_servers":2`, `"monitored_workstations":3`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("row %s is missing %s (the latest day's documented count)", b, want)
		}
	}
}

// A legacy partition an older print wrote must not resurrect a record once the
// current partition has completed a sync that no longer returns it. Auvik's
// device list simply omits a decommissioned device (auvik-v1
// #/paths/~1v1~1inventory~1device~1info/get), so the completed current
// partition is the whole truth.
func TestLegacyPartitionDoesNotResurrectRemovedRecords(t *testing.T) {
	db := seedPartitionStore(t, []seedRow{
		{"inventory", "d0", "device", "2026-10-01 10:00:00"},
		{"inventory-device-info", "d1", "device", "2026-11-01 10:00:00"},
	})
	if _, err := db.DB().Exec(`INSERT INTO sync_state (resource_type, last_synced_at, total_count, last_attempt_complete)
		VALUES ('inventory-device-info', '2026-11-01T10:00:00Z', 1, 1)`); err != nil {
		t.Fatalf("seed sync_state: %v", err)
	}
	got, err := loadResources(context.Background(), db, rtDevices)
	if err != nil {
		t.Fatal(err)
	}
	if ids := strings.Join(rowIDs(got), ","); ids != "d1" {
		t.Fatalf("device read returned %q, want d1 only (d0 is gone from a completed current sync)", ids)
	}
}

// deviceLifecycleAttributes enums include "unpublished" and "empty" (auvik-v1
// #/components/schemas/deviceLifecycleAttributes). Neither says the device is
// supported; reporting it as "supported" turns missing vendor data into a
// support verdict.
func TestEolUnpublishedLifecycleIsNotSupported(t *testing.T) {
	devices := []auvikRow{device("d1", "edge-rtr", "router", "ISR", "t1")}
	v1 := []auvikRow{specRow("d1", map[specField]any{
		fLifecycleLastSupportStatus: "unpublished",
		fLifecycleSalesAvail:        "unpublished",
		fLifecycleSwMaint:           "unpublished",
		fLifecycleSecSwMaint:        "unpublished",
	}, nil)}
	got := buildEolReport(devices, v1, nil, nil, tenantMap(), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	if r := got.Rows[0]; r.Bucket != "unknown" {
		t.Fatalf("unpublished lifecycle bucketed %q (source %q), want unknown", r.Bucket, r.Source)
	}
}

// An empty device list is a documented response (auvik-v1
// #/components/schemas/deviceInfoReadMultiple, data: []). A snapshot of an
// empty fleet must still become the newest baseline, or every later diff keeps
// comparing against the old one and reports the same removals forever.
func TestEmptySnapshotBecomesTheNewestBaseline(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWithContext(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(`INSERT INTO auvik_device_snapshots
		(snapshot_at, device_id, tenant_id, device_name, fingerprint) VALUES ('2026-10-01T00:00:00Z', 'd1', 't1', 'sw', 'fp')`); err != nil {
		t.Fatal(err)
	}
	if err := saveDeviceSnapshot(ctx, db, map[string]deviceSnapshotRow{}); err != nil {
		t.Fatal(err)
	}
	at, base, err := loadLatestSnapshot(ctx, db, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !at.After(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || len(base) != 0 {
		t.Fatalf("newest baseline = %s with %d devices, want the empty snapshot just saved", at, len(base))
	}
}

// ASM licenses are application-scoped: /v1/asm/license/info REQUIRES
// filter[applicationId] (auvik-v1 #/paths/~1v1~1asm~1license~1info/get) and the
// license record names no application (#/components/schemas/asmLicenseResourceObject).
// A license held for app A must not mark app B as licensed.
func TestAsmShadowDoesNotCountOneAppsLicenseForAnother(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/v1/asm/license/info" {
			http.NotFound(w, r)
			return
		}
		data := []any{}
		if r.URL.Query().Get("filter[applicationId]") == "appA" && r.URL.Query().Get("filter[clientId]") == "c1" {
			data = append(data, map[string]any{"type": "asmLicense", "id": "L1",
				"attributes": map[string]any{"email": "u@example.com", "externalUserId": "U", "licenseType": "full"}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	isolateCLIEnv(t, srv.URL)

	dbPath := filepath.Join(t.TempDir(), "data.db")
	seedRawRows(t, dbPath, []rawSeed{
		{"asm", "c1", `{"id":"c1","type":"asmClient","attributes":{"name":"acme"}}`},
		{"asm-app-info", "appA", `{"id":"appA","type":"asmApp","attributes":{"name":"Figma"},"relationships":{"client":{"data":{"type":"asmClient","id":"c1"}},"users":{"data":[{"type":"asmAppUser","id":"U"}]}}}`},
		{"asm-app-info", "appB", `{"id":"appB","type":"asmApp","attributes":{"name":"Notion"},"relationships":{"client":{"data":{"type":"asmClient","id":"c1"}},"users":{"data":[{"type":"asmAppUser","id":"U"}]}}}`},
		{"asm-user-info", "U", `{"id":"U","type":"asmUser","attributes":{"email":"u@example.com","active":true,"disabled":false}}`},
		{"asm-license-info", "L1", `{"id":"L1","type":"asmLicense","attributes":{"email":"u@example.com","externalUserId":"U","licenseType":"full"}}`},
	})

	out := runCLI(t, "asm", "shadow", "--db", dbPath, "--agent", "--no-cache")
	var report struct {
		Findings []shadowFinding `json:"findings"`
	}
	if err := json.Unmarshal(extractJSONReport(t, out), &report); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	var notion, figma *shadowFinding
	for i := range report.Findings {
		switch report.Findings[i].Application {
		case "Notion":
			notion = &report.Findings[i]
		case "Figma":
			figma = &report.Findings[i]
		}
	}
	if notion == nil || notion.Finding != findingUnlicensed {
		t.Fatalf("Notion has an active user and no license of its own; findings = %+v", report.Findings)
	}
	if figma != nil {
		t.Errorf("Figma is balanced (1 active user, 1 license of its own), got %+v", *figma)
	}
}

// ---- helpers shared by the command-level tests below and above ----

type rawSeed struct{ partition, id, data string }

func seedRawRows(t *testing.T, dbPath string, rows []rawSeed) {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for _, r := range rows {
		if _, err := db.DB().Exec(`INSERT INTO resources (id, resource_type, data) VALUES (?, ?, ?)`,
			r.id, r.partition, r.data); err != nil {
			t.Fatalf("seed %v: %v", r, err)
		}
	}
}

// isolateCLIEnv points the CLI at a fake vendor and keeps every config, cache
// and state root inside the test's temp dir.
func isolateCLIEnv(t *testing.T, baseURL string) {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": filepath.Join(home, "cfg"),
		"XDG_CACHE_HOME": filepath.Join(home, "cache"), "XDG_DATA_HOME": filepath.Join(home, "data"),
		"XDG_STATE_HOME": filepath.Join(home, "state"), "AUVIK_CONFIG": filepath.Join(home, "none.toml"),
		"AUVIK_BASE_URL": baseURL, "AUVIK_USERNAME": "ops@example.com", "AUVIK_API_KEY": "test-key-not-real",
	} {
		t.Setenv(k, v)
	}
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\nstdout: %s\nstderr: %s", args, err, out.String(), errOut.String())
	}
	return out.String()
}

// extractJSONReport returns the report object, unwrapping an agent envelope
// when the output carries one.
func extractJSONReport(t *testing.T, out string) []byte {
	t.Helper()
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &probe); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	for _, k := range []string{"results", "data", "result"} {
		if inner, ok := probe[k]; ok && len(inner) > 0 && inner[0] == '{' {
			return inner
		}
	}
	return []byte(out)
}

// ---- class members, pinned after the round-1 fixes ----

// The EOL "supported" verdict is an allowlist: only an affirmative status
// (covered / available, or a warranty "Covered") earns it without a date.
func TestEolSupportedNeedsAnAffirmativeStatus(t *testing.T) {
	asOf := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	devices := []auvikRow{
		device("d1", "a", "switch", "m", "t1"),
		device("d2", "b", "switch", "m", "t1"),
		device("d3", "c", "switch", "m", "t1"),
		device("d4", "d", "switch", "m", "t1"),
	}
	v1 := []auvikRow{
		specRow("d1", map[specField]any{fLifecycleLastSupportStatus: "covered"}, nil),
		specRow("d3", map[specField]any{fLifecycleLastSupportStatus: "empty"}, nil),
		specRow("d4", map[specField]any{fLifecycleSecSwMaint: "expired", fLifecycleLastSupportStatus: "covered"}, nil),
	}
	warranties := []auvikRow{specRow("d2", map[specField]any{fWarrantyCoverageStatus: "Not Covered"}, nil)}
	got := buildEolReport(devices, v1, nil, warranties, tenantMap(), asOf)
	want := map[string]string{"d1": bucketSupported, "d2": bucketUnknown, "d3": bucketUnknown, "d4": bucketExpired}
	for _, r := range got.Rows {
		if r.Bucket != want[r.DeviceID] {
			t.Errorf("%s bucketed %q (source %q), want %q", r.DeviceID, r.Bucket, r.Source, want[r.DeviceID])
		}
	}
	if got.Buckets[bucketUnknown] != 2 {
		t.Errorf("unknown bucket = %d, want 2", got.Buckets[bucketUnknown])
	}
}

// A string-shaped detail status is kept as a signal; it never becomes a dump.
func TestDiscoveryGapsKeepsAStringDetailStatus(t *testing.T) {
	devices := []auvikRow{device("d1", "core-sw", "switch", "m", "t1")}
	detail := auvikRow{ID: "d1", Data: map[string]any{"id": "d1", "type": "deviceDetail",
		"attributes": map[string]any{"discoveryStatus": "partial"}}}
	got := buildDiscoveryGapsReport(devices, []auvikRow{detail}, nil, tenantMap())
	if got.NoEvidence != 0 || got.FullyDiscovered != 1 {
		t.Fatalf("a string status is evidence, got %+v", got)
	}
}

// Absent counts are null, never zero; the usage record's own id is its tenant
// when domainPrefix matches no synced tenant; billed device names come from
// the documented relationship attribute "name".
func TestReconcileUnknownIsNullAndTenantComesFromTheRecordID(t *testing.T) {
	devices := []auvikRow{device("dev-1", "sw-1", "switch", "m", "t9")}
	billing := []auvikRow{{ID: "t9", Data: map[string]any{
		"id": "t9", "type": "clientUsage",
		"attributes": map[string]any{"domainPrefix": "unsynced-prefix"},
		"relationships": map[string]any{
			"clients": map[string]any{"data": []any{map[string]any{"id": "child-1"}}},
			"devices": map[string]any{"data": []any{
				map[string]any{"id": "dev-1", "attributes": map[string]any{"name": "sw-1"}},
				map[string]any{"id": "dev-2", "attributes": map[string]any{"name": "gone-ap"}},
			}},
		},
	}}}
	got := buildReconcileReport(devices, billing, map[string]string{})
	if len(got.Clients) != 1 {
		t.Fatalf("usage must join its own tenant t9, got %+v", got.Clients)
	}
	c := got.Clients[0]
	if c.TenantID != "t9" || c.BilledCount == nil || *c.BilledCount != 2 {
		t.Fatalf("row = %+v, want tenant t9 with 2 billed", c)
	}
	if len(c.BilledNotSeen) != 1 || c.BilledNotSeen[0] != "gone-ap" {
		t.Errorf("billed_not_seen = %v, want the documented device name gone-ap", c.BilledNotSeen)
	}
	b, _ := json.Marshal(c)
	for _, want := range []string{`"monitored_servers":null`, `"max_monitored_servers":null`, `"billable_days":null`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("row %s: want %s for an absent count", b, want)
		}
	}
}

// An empty baseline is a real baseline: what appears after it is an addition.
func TestEmptyBaselineReportsAdditions(t *testing.T) {
	after := map[string]deviceSnapshotRow{"d2": snap("d2", "t1", "new", "A", "fp")}
	got := buildInventoryDiff(map[string]deviceSnapshotRow{}, after, tenantMap())
	if got.Counts["added"] != 1 {
		t.Fatalf("want d2 added against an empty baseline, got %+v", got)
	}
}

// Two snapshots in the same second are one snapshot: the second replaces the
// first instead of merging device rows into it.
func TestSameSecondSnapshotReplacesNotMerges(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWithContext(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for {
		start := time.Now().UTC().Truncate(time.Second)
		if err := saveDeviceSnapshot(ctx, db, map[string]deviceSnapshotRow{
			"d1": snap("d1", "t1", "a", "A", "fp1"), "d2": snap("d2", "t1", "b", "B", "fp2")}); err != nil {
			t.Fatal(err)
		}
		if err := saveDeviceSnapshot(ctx, db, map[string]deviceSnapshotRow{"d1": snap("d1", "t1", "a", "A", "fp1")}); err != nil {
			t.Fatal(err)
		}
		if time.Now().UTC().Truncate(time.Second).Equal(start) {
			break
		}
	}
	_, base, err := loadLatestSnapshot(ctx, db, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(base) != 1 {
		t.Fatalf("same-second snapshot merged: %d devices, want 1", len(base))
	}
}

// License pagination follows the cursor against the configured host only; a
// next link pointing at another host is never dialed. A rejected credential
// stops the walk and leaves every application without a finding.
func TestAsmShadowLicensePagingStaysOnTheConfiguredHost(t *testing.T) {
	var pages, offHost int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		pages++
		if r.URL.Query().Get("page[after]") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":  []any{map[string]any{"type": "asmLicense", "id": "L1", "attributes": map[string]any{"email": "u@example.com"}}},
				"links": map[string]any{"next": "https://elsewhere.example/v1/asm/license/info?page%5Bafter%5D=CUR2"},
			})
			return
		}
		if r.URL.Query().Get("page[after]") != "CUR2" {
			offHost++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{map[string]any{"type": "asmLicense", "id": "L2", "attributes": map[string]any{"email": "v@example.com"}}},
		})
	}))
	defer srv.Close()
	isolateCLIEnv(t, srv.URL)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	seedRawRows(t, dbPath, []rawSeed{
		{"asm-app-info", "appA", `{"id":"appA","type":"asmApp","attributes":{"name":"Figma"},"relationships":{"client":{"data":{"id":"c1"}},"users":{"data":[{"id":"U"}]}}}`},
		{"asm-user-info", "U", `{"id":"U","type":"asmUser","attributes":{"email":"u@example.com","active":true}}`},
	})
	out := runCLI(t, "asm", "shadow", "--db", dbPath, "--agent", "--no-cache")
	var report shadowReport
	if err := json.Unmarshal(extractJSONReport(t, out), &report); err != nil {
		t.Fatal(err)
	}
	if pages != 2 || offHost != 0 || report.Licenses != 2 {
		t.Fatalf("pages=%d offHost=%d licenses=%d, want 2/0/2", pages, offHost, report.Licenses)
	}
	// v@example.com holds a Figma license but is not an active Figma user.
	if report.Counts[findingUnusedSeat] != 1 || report.Counts[findingUnlicensed] != 0 {
		t.Errorf("counts = %+v, want one unused_licenses finding", report.Counts)
	}
}

func TestAsmShadowRejectedCredentialYieldsNoFinding(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, `{"errors":[{"detail":"bad credential"}]}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	isolateCLIEnv(t, srv.URL)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	seedRawRows(t, dbPath, []rawSeed{
		{"asm-app-info", "appA", `{"id":"appA","type":"asmApp","attributes":{"name":"Figma"},"relationships":{"client":{"data":{"id":"c1"}},"users":{"data":[{"id":"U"}]}}}`},
		{"asm-app-info", "appB", `{"id":"appB","type":"asmApp","attributes":{"name":"Notion"},"relationships":{"client":{"data":{"id":"c1"}},"users":{"data":[{"id":"U"}]}}}`},
		{"asm-user-info", "U", `{"id":"U","type":"asmUser","attributes":{"email":"u@example.com","active":true}}`},
		{"asm-license-info", "L9", `{"id":"L9","type":"asmLicense","attributes":{"email":"u@example.com"}}`},
	})
	out := runCLI(t, "asm", "shadow", "--db", dbPath, "--agent", "--no-cache")
	var report shadowReport
	if err := json.Unmarshal(extractJSONReport(t, out), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 0 || len(report.LicenseLookupFailed) != 2 {
		t.Fatalf("a failed license read must yield no finding and list both apps: %+v", report)
	}
	if calls == 0 || calls > 4 {
		t.Errorf("license reads after a 401 = %d; the walk must stop once the credential is rejected", calls)
	}
	if strings.Contains(out, "bad credential") {
		t.Errorf("a response body was echoed into the report: %s", out)
	}
}

// Review r2: salesAvailability describes point-of-sale availability (auvik-v1
// #/components/schemas/deviceLifecycleAttributes/properties/salesAvailability),
// not support. "available" for sale with unpublished support data is no
// support verdict.
func TestEolSalesAvailabilityIsNotASupportVerdict(t *testing.T) {
	devices := []auvikRow{device("d1", "edge-rtr", "router", "ISR", "t1")}
	v1 := []auvikRow{specRow("d1", map[specField]any{
		fLifecycleLastSupportStatus: "unpublished",
		fLifecycleSalesAvail:        "available",
		fLifecycleSwMaint:           "unpublished",
		fLifecycleSecSwMaint:        "unpublished",
	}, nil)}
	got := buildEolReport(devices, v1, nil, nil, tenantMap(), time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	if r := got.Rows[0]; r.Bucket != bucketUnknown {
		t.Fatalf("sales availability alone bucketed %q (source %q), want unknown", r.Bucket, r.Source)
	}
}

// Review r2 (P2): a 200 whose body carries no data member is not a completed
// license read; the application must be listed as failed, not unlicensed.
func TestAsmShadowLicenseBodyWithoutDataIsAFailedRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()
	isolateCLIEnv(t, srv.URL)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	seedRawRows(t, dbPath, []rawSeed{
		{"asm-app-info", "appA", `{"id":"appA","type":"asmApp","attributes":{"name":"Figma"},"relationships":{"client":{"data":{"id":"c1"}},"users":{"data":[{"id":"U"}]}}}`},
		{"asm-user-info", "U", `{"id":"U","type":"asmUser","attributes":{"email":"u@example.com","active":true}}`},
	})
	out := runCLI(t, "asm", "shadow", "--db", dbPath, "--agent", "--no-cache")
	var report shadowReport
	if err := json.Unmarshal(extractJSONReport(t, out), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 0 || len(report.LicenseLookupFailed) != 1 {
		t.Fatalf("a body with no data member must be a failed read, got %+v", report)
	}
}
