// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Hand-fix (handfixes.json: expiry-unknown-never-read-as-not-expiring, Codex
// round 1, class "incomplete fetch read as complete"): an allocation row that
// carries no expiry date, no daysUntilExpiry and no allocationExpiryType
// "never" is expiry-unknown, never "not expiring". Shapes are the spec's own:
// GET /rest/platform/allocation/v2/{accountId}/assets/allocations/{resourceType}
// (AllocassetAllocationObject; example AllocfireboxAssetsAllocation1 is a
// per-account quantity with no date, AllocWESAssetsAllocation1 ends with an
// account-only placeholder row) and .../assets/summary/{resourceType}
// (AllocinventorySumObject; AllocfireboxSampleDataAllocated is per device with
// accountId and expiry fields, AllocWESInventory2's subscription row has none).
func expiryStub(t *testing.T, tomorrow string) *httptest.Server {
	t.Helper()
	return expiryStubWith(t, `{"data":[{"productName":"firebox","accountId":"WGC-1-123abc456","accountName":"Subscriber1","serialOrLicense":"ABCD123-EFGH","modelName":"M200","allocationExpiryType":"never","licenseExpiryDate":"`+tomorrow+`"},
			 {"productName":"firebox","accountId":"WGC-1-123abc4567","accountName":"Subscriber2","serialOrLicense":"ABCD124-EFGH","modelName":"M400","allocationExpiryType":"never","licenseExpiryDate":"2099-12-31"}]}`)
}

// expiryStubWith serves expiryStub's responses with fireboxAllocated as the
// allocated Firebox summary (Allocv2DeviceInfoObject rows).
func expiryStubWith(t *testing.T, fireboxAllocated string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p, q := r.URL.Path, r.URL.Query()
		write := func(s string) { _, _ = w.Write([]byte(s)) }
		switch {
		case p == "/oauth/token":
			write(`{"access_token":"tok","expires_in":3600}`)
		case strings.HasSuffix(p, "/audiences"):
			body, _ := io.ReadAll(r.Body)
			var in struct {
				AccountID string `json:"accountId"`
			}
			_ = json.Unmarshal(body, &in)
			write(`{"audience":"aud-` + in.AccountID + `"}`)
		case strings.HasSuffix(p, "/ACC-SP/children"):
			write(`{"items":[{"accountId":"WGC-1-123abc456","name":"Subscriber1"},{"accountId":"WGC-1-123abc4567","name":"Subscriber2"}],"pageControls":{"totalItems":2}}`)
		case strings.Contains(p, "/firebox/management/v1/info/"):
			write(`{"count":1,"data":[{"id":"FB","state":"online"}]}`)
		case strings.HasSuffix(p, "/devicesprotectionstatus"):
			write(`{"data":[],"total_items":0}`)
		case strings.HasSuffix(p, "/incidents"):
			write(`{"items":[],"count":0}`)
		case strings.HasSuffix(p, "/assets/allocations/firebox"): // AllocfireboxAssetsAllocation1
			write(`{"data":[{"productName":"firebox","accountId":"WGC-1-123abc456","accountName":"Subscriber1","quantityTotal":10},
			 {"productName":"firebox","accountId":"WGC-1-123abc4567","accountName":"Subscriber2","quantityTotal":20},
			 {"accountId":"WGC-1-123abc4568","accountName":"Subscriber3"}]}`)
		case strings.HasSuffix(p, "/assets/summary/firebox") && q.Get("allocationStatus") == "allocated": // AllocfireboxSampleDataAllocated
			write(fireboxAllocated)
		case strings.HasSuffix(p, "/assets/allocations/endPoint"): // AllocWESAssetsAllocation1 rows
			write(`{"data":[{"allocationExpiryDate":"2099-12-12","allocationExpiryType":"custom","productName":"WES_EDR","accountId":"WGC-1-123abc456","accountName":"Subscriber1","quantityTotal":10,"licenseType":"term","usage":10},
			 {"allocationExpiryType":"never","productName":"WES_EDR","accountId":"WGC-1-123abc4567","accountName":"Subscriber2","subscriptionTotal":"unlimited","licenseType":"subscription"},
			 {"accountId":"WGC-1-123abc4569","accountName":"Subscriber4"}]}`)
		case strings.HasSuffix(p, "/assets/allocations/authPoint"): // a row with no expiry evidence
			write(`{"data":[{"productName":"AuthPoint_MFA","accountId":"WGC-1-123abc456","accountName":"Subscriber1","quantityTotal":5,"licenseType":"term"}]}`)
		case strings.HasSuffix(p, "/assets/summary/endPoint"): // AllocWESInventory2
			write(`{"data":[{"productName":"WES_EDR","allocationType":"term","allocationExpiryDate":"2099-12-10","quantityTotal":45,"quantityAvailable":30,"quantityAllocated":15,"usage":10},
			 {"productName":"WES_EDR","allocationType":"subscription","subscriptionTotal":50,"usage":10}]}`)
		default:
			write(`{"data":[],"items":[],"pageControls":{"totalItems":0}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAccountsHealthFireboxExpiryIsReadPerDevice(t *testing.T) {
	tomorrow := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	spEnv(t, expiryStub(t, tomorrow))
	// Codex's failing input: the per-account Firebox aggregate has no dates.
	doc, code := runJSON(t, "accounts", "health", "--accounts", "WGC-1-123abc456", "--license-types", "firebox", "--json")
	row := healthRowFor(doc, "WGC-1-123abc456")
	if row == nil {
		t.Fatalf("no row: exit %d, %v", code, doc)
	}
	if row["licenses_expiring"] != float64(1) || row["complete"] != true || code != 0 {
		t.Fatalf("Firebox whose license expires %s: exit %d, row %v (want licenses_expiring 1, complete true, exit 0)", tomorrow, code, row)
	}
}

func TestAccountsHealthExpiryUnknownIsIncomplete(t *testing.T) {
	spEnv(t, expiryStub(t, "2099-01-01"))
	doc, code := runJSON(t, "accounts", "health", "--license-types", "endPoint,authPoint", "--json")
	a, b := healthRowFor(doc, "WGC-1-123abc456"), healthRowFor(doc, "WGC-1-123abc4567")
	if a == nil || b == nil {
		t.Fatalf("missing rows: %v", doc)
	}
	if a["licenses_expiry_unknown"] != float64(1) || a["complete"] != false {
		t.Fatalf("AuthPoint row with no expiry evidence: %v (want licenses_expiry_unknown 1, complete false)", a)
	}
	// A "never" row and the account-only placeholder row are known, not unknown.
	if b["licenses_expiry_unknown"] != float64(0) || b["complete"] != true || b["licenses_expiring"] != float64(0) {
		t.Fatalf("never-expiring row: %v (want 0 unknown, complete true)", b)
	}
	if code != 6 {
		t.Fatalf("an account with expiry-unknown allocations: exit %d, want 6 (partial)", code)
	}
	if _, code := runJSON(t, "accounts", "health", "--license-types", "endPoint,authPoint", "--json", "--allow-partial-failure"); code != 0 {
		t.Fatalf("--allow-partial-failure: exit %d, want 0", code)
	}
}

func TestReconcileListsExpiryUnknown(t *testing.T) {
	spEnv(t, expiryStub(t, "2099-01-01"))
	doc, code := runJSON(t, "allocations", "reconcile", "--types", "endPoint", "--json")
	if code != 0 {
		t.Fatalf("reconcile: exit %d", code)
	}
	unk, ok := doc["expiry_unknown"].([]any)
	if !ok || len(unk) != 1 {
		t.Fatalf("expiry_unknown = %v, want the AllocWESInventory2 subscription row", doc["expiry_unknown"])
	}
	if u := unk[0].(map[string]any); u["product"] != "WES_EDR" || u["resource_type"] != "endPoint" {
		t.Fatalf("expiry_unknown[0] = %v", u)
	}
}

func TestExpiryOfStates(t *testing.T) {
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		row  string
		want expiryState
	}{
		{`{"productName":"WES_EDR","allocationExpiryDate":"2026-12-12"}`, expiryDated},
		{`{"productName":"firebox","allocationExpiryType":"never","licenseExpiryDate":"2027-12-31"}`, expiryDated},
		{`{"productName":"WES_EDR","daysUntilExpiry":5}`, expiryDated},
		{`{"productName":"WES_EDR","allocationExpiryDate":"not a date","licenseExpiryDate":"2027-12-31"}`, expiryUnknown},
		{`{"productName":"WES_EDR","allocationExpiryType":" Never "}`, expiryNever},
		{`{"accountId":"A","accountName":"Sub4"}`, expiryNoAllocation},
		{`{"accountId":"A","accountName":"Sub4","productName":null}`, expiryNoAllocation},
		{`{"productName":"firebox","accountId":"A","quantityTotal":10}`, expiryUnknown},
		{`{"productName":"WES_EDR","allocationExpiryDate":"12-12-2222"}`, expiryUnknown},
		{`{"productName":"WES_EDR","allocationExpiryType":"custom"}`, expiryUnknown},
	} {
		var m map[string]any
		if err := json.Unmarshal([]byte(tc.row), &m); err != nil {
			t.Fatal(err)
		}
		if _, _, _, got := expiryOf(m, now); got != tc.want {
			t.Errorf("expiryOf(%s) = %v, want %v", tc.row, got, tc.want)
		}
	}
}

// Hand-fix (handfixes.json: expiry-reads-the-earliest-evidence, Codex round 2,
// class "wrong expiry field read"): a row expires at the EARLIEST of its
// documented expiry evidence. Allocv2DeviceInfoObject (GET
// /rest/platform/allocation/v2/{accountId}/assets/summary/{resourceType})
// documents allocationExpiryDate and licenseExpiryDate independently, so an
// earlier license expiry must never hide behind a later allocation date.
func earliestExpiryRow(tomorrow string) string {
	return `{"data":[{"productName":"firebox","accountId":"WGC-1-123abc456","accountName":"Subscriber1","serialOrLicense":"SERIAL-A","allocationExpiryType":"custom","allocationExpiryDate":"2027-12-31","licenseExpiryDate":"` + tomorrow + `"}]}`
}

func TestAccountsHealthReadsTheEarliestExpiry(t *testing.T) {
	tomorrow := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	spEnv(t, expiryStubWith(t, earliestExpiryRow(tomorrow)))
	// Codex round 2's failing input.
	doc, code := runJSON(t, "accounts", "health", "--accounts", "WGC-1-123abc456", "--license-types", "firebox", "--expiring-within", "30d", "--json")
	row := healthRowFor(doc, "WGC-1-123abc456")
	if row == nil {
		t.Fatalf("no row: exit %d, %v", code, doc)
	}
	if row["licenses_expiring"] != float64(1) || row["complete"] != true || code != 0 {
		t.Fatalf("license expiring %s behind a 2027 allocation date: exit %d, row %v (want licenses_expiring 1, complete true, exit 0)", tomorrow, code, row)
	}
}

func TestReconcileReadsTheEarliestExpiry(t *testing.T) {
	tomorrow := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	spEnv(t, expiryStubWith(t, earliestExpiryRow(tomorrow)))
	doc, code := runJSON(t, "allocations", "reconcile", "--types", "endPoint", "--json")
	if code != 0 {
		t.Fatalf("reconcile: exit %d", code)
	}
	exp, _ := doc["expiring"].([]any)
	var hit map[string]any
	for _, e := range exp {
		if m, _ := e.(map[string]any); m["serial_or_license"] == "SERIAL-A" {
			hit = m
		}
	}
	if hit == nil || hit["expiry_date"] != tomorrow || hit["expiry_field"] != "licenseExpiryDate" {
		t.Fatalf("expiring = %v, want SERIAL-A expiring %s by licenseExpiryDate", doc["expiring"], tomorrow)
	}
}

func TestExpiryOfTakesTheEarliestEvidence(t *testing.T) {
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		row, wantDate, wantField string
		want                     expiryState
	}{
		{`{"productName":"firebox","allocationExpiryDate":"2027-12-31","licenseExpiryDate":"2026-11-03"}`, "2026-11-03", "licenseExpiryDate", expiryDated},
		{`{"productName":"firebox","allocationExpiryDate":"2026-11-03","licenseExpiryDate":"2027-12-31"}`, "2026-11-03", "allocationExpiryDate", expiryDated},
		{`{"productName":"WES_EDR","allocationExpiryDate":"2027-12-31","licenseExpiryDate":"2027-06-30","daysUntilExpiry":3}`, "2026-11-05T00:00:00Z", "daysUntilExpiry", expiryDated},
		{`{"productName":"WES_EDR","allocationExpiryDate":"2026-11-04","daysUntilExpiry":30}`, "2026-11-04", "allocationExpiryDate", expiryDated},
		// Present but unreadable evidence: the earliest is unknowable, so fail closed.
		{`{"productName":"WES_EDR","allocationExpiryDate":"not a date","licenseExpiryDate":"2027-12-31"}`, "", "", expiryUnknown},
		{`{"productName":"WES_EDR","licenseExpiryDate":"2027-12-31","daysUntilExpiry":"soon"}`, "", "", expiryUnknown},
	} {
		var m map[string]any
		if err := json.Unmarshal([]byte(tc.row), &m); err != nil {
			t.Fatal(err)
		}
		got, _ := expiringWithin("endPoint", []map[string]any{m}, 3650*24*time.Hour, now)
		if tc.want != expiryDated {
			if len(got) != 0 {
				t.Errorf("%s: expiring %+v, want expiry-unknown", tc.row, got)
			}
			continue
		}
		if len(got) != 1 || got[0].ExpiryDate != tc.wantDate || got[0].ExpiryField != tc.wantField {
			t.Errorf("%s: got %+v, want %s by %s", tc.row, got, tc.wantDate, tc.wantField)
		}
	}
}

// The same class on reconcile's product rows (AllocinventoryObject documents
// allocationExpiryDate and daysUntilExpiry): the shown expiry is the earliest.
func TestReconcileProductShowsTheEarliestExpiry(t *testing.T) {
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	var m map[string]any
	if err := json.Unmarshal([]byte(`{"productName":"WES_EDR","allocationType":"term","allocationExpiryDate":"2099-12-10","daysUntilExpiry":"3","quantityTotal":45}`), &m); err != nil {
		t.Fatal(err)
	}
	if p := productFromRow("endPoint", m, now); p.ExpiryDate != "2026-11-05T00:00:00Z" || p.ExpiryField != "daysUntilExpiry" {
		t.Fatalf("product = %+v, want expiry 2026-11-05T00:00:00Z by daysUntilExpiry", p)
	}
}
