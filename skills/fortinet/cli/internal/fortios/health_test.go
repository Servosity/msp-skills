package fortios

import (
	"strings"
	"testing"
	"time"
)

func TestHAChecksumsInSync(t *testing.T) {
	if got := HAChecksumsInSync(objs(t, `[{"checksum":{"all":"a"}}]`)); got != "n/a" {
		t.Errorf("standalone = %s", got)
	}
	if got := HAChecksumsInSync(objs(t, `[{"checksum":{"all":"a"}},{"checksum":{"all":"a"}}]`)); got != "in-sync" {
		t.Errorf("same = %s", got)
	}
	if got := HAChecksumsInSync(objs(t, `[{"checksum":{"all":"a"}},{"checksum":{"all":"b"}}]`)); got != "out-of-sync" {
		t.Errorf("different = %s", got)
	}
	if got := HAChecksumsInSync(objs(t, `[{"serial_no":"x"},{"serial_no":"y"}]`)); got != "unknown" {
		t.Errorf("no checksum objects = %s", got)
	}
	for _, empty := range []string{`null`, `{}`, `"x"`, `{"all":null,"root":null}`, `{"all":"","global":" "}`} {
		if got := HAChecksumsInSync(objs(t, `[{"checksum":`+empty+`},{"checksum":`+empty+`}]`)); got != "unknown" {
			t.Errorf("checksum %s on every member = %s, want unknown", empty, got)
		}
	}
}

func TestIPsecDown(t *testing.T) {
	tun := objs(t, `[{"name":"a","proxyid":[{"status":"up"}]},{"name":"b","proxyid":[{"status":"down"},{"status":"down"}]},{"name":"c","proxyid":[]}]`)
	if d, n := IPsecDown(tun); d != 1 || n != 2 {
		t.Fatalf("IPsecDown = %d/%d", d, n)
	}
}

func TestSDWANDown(t *testing.T) {
	v := obj(t, `{"sla1":{"wan1":{"status":"up","latency":3},"wan2":{"status":"dead"}},"sla2":{"wan1":{"status":"up"}}}`)
	if d, n := SDWANDown(v); d != 1 || n != 3 {
		t.Fatalf("SDWANDown = %d/%d", d, n)
	}
	if d, n := SDWANDown(obj(t, `{"x":{"latency":1}}`)); d != 0 || n != 0 {
		t.Fatalf("no status fields = %d/%d", d, n)
	}
}

func TestCurrentUsageAndExpiringCerts(t *testing.T) {
	if v, ok := CurrentUsage(obj(t, `{"cpu":[{"current":42}]}`), "cpu"); !ok || v != 42 {
		t.Fatalf("CurrentUsage = %v,%v", v, ok)
	}
	if _, ok := CurrentUsage(Obj{}, "mem"); ok {
		t.Fatal("CurrentUsage on empty payload")
	}
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	certs := objs(t, `[{"name":"vpn","valid_to":`+itoa(now.Unix()+5*86400)+`},{"name":"Fortinet_Factory","is_built_in":true,"valid_to":1},{"name":"far","valid_to":`+itoa(now.Unix()+900*86400)+`}]`)
	got := ExpiringCerts(certs, now, 30*24*time.Hour)
	if len(got) != 1 || !strings.HasPrefix(got[0], "vpn") {
		t.Fatalf("ExpiringCerts = %v", got)
	}
}

func TestHealthGrade(t *testing.T) {
	cpu := 95.0
	h := HealthRow{HASync: "in-sync", TunnelsDown: 1, TunnelsTotal: 4, CPUPercent: &cpu, CertsExpiring: []string{}}
	h.Grade(90, 90)
	if h.Status != Warn || len(h.Signals) != 2 {
		t.Fatalf("grade = %+v", h)
	}
	h = HealthRow{HASync: "out-of-sync", CertsExpiring: []string{}}
	h.Grade(90, 90)
	if h.Status != Fail {
		t.Fatalf("HA out of sync not FAIL: %+v", h)
	}
}

func TestHealthGradeUnreadableIsNeverPass(t *testing.T) {
	h := HealthRow{HASync: "in-sync", CertsExpiring: []string{}, Unreadable: []string{"ipsec root (HTTP 403)"}}
	h.Grade(90, 90)
	if h.Status != Unknown {
		t.Fatalf("unreadable signal graded %s, want UNKNOWN: %+v", h.Status, h)
	}
	h.TunnelsDown, h.TunnelsTotal = 1, 2
	h.Grade(90, 90)
	if h.Status != Warn {
		t.Fatalf("WARN must outrank UNKNOWN: %+v", h)
	}
}

// A certificate whose expiry cannot be read is reported, not silently dropped.
func TestUndatedCertsAreReported(t *testing.T) {
	certs := objs(t, `[
		{"name":"vpn-cert","valid_to":1900000000},
		{"name":"odd-cert","valid_to":"2027-01-01"},
		{"name":"bare-cert"},
		{"name":"Fortinet_Factory","is_built_in":true},
		{"name":"root-ca","is_ca":true}]`)
	got := UndatedCerts(certs)
	if len(got) != 2 || got[0] != "odd-cert" || got[1] != "bare-cert" {
		t.Fatalf("UndatedCerts = %v, want [odd-cert bare-cert]", got)
	}
}
