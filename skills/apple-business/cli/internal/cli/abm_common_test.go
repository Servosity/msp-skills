package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCoverageSummary(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cov := func(status, end string, canceled bool) map[string]any {
		var e any
		if end != "" {
			e = end
		}
		return map[string]any{"attributes": map[string]any{"status": status, "endDateTime": e, "isCanceled": canceled, "description": status + end}}
	}
	body := func(c ...map[string]any) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"data": c})
		return b
	}
	cases := []struct {
		name                string
		raw                 json.RawMessage
		expiring, uncovered bool
	}{
		{"ends in 30d", body(cov("ACTIVE", "2026-01-31T00:00:00Z", false)), true, false},
		{"latest end wins", body(cov("ACTIVE", "2026-01-31T00:00:00Z", false), cov("ACTIVE", "2027-06-01T00:00:00Z", false)), false, false},
		{"open-ended never expires", body(cov("ACTIVE", "2026-01-31T00:00:00Z", false), cov("ACTIVE", "", false)), false, false},
		{"canceled ignored", body(cov("ACTIVE", "2026-01-31T00:00:00Z", true)), false, true},
		{"expired only", body(cov("EXPIRED", "2025-01-31T00:00:00Z", false)), false, true},
		{"no records", body(), false, true},
	}
	for _, c := range cases {
		v := coverageSummary(c.raw)
		if got := v.expiringWithin(now, 90*24*time.Hour); got != c.expiring {
			t.Errorf("%s: expiring = %v, want %v", c.name, got, c.expiring)
		}
		if got := v.uncovered(now); got != c.uncovered {
			t.Errorf("%s: uncovered = %v, want %v", c.name, got, c.uncovered)
		}
	}
}

func TestFamilyKeyAndHW(t *testing.T) {
	if familyKey("AppleTV") != familyKey("APPLE_TV") || familyKey("Mac") != familyKey("MAC") {
		t.Fatal("family spellings must fold together")
	}
	if normalizeHW("AA:BB-cc.dd") != "aabbccdd" {
		t.Fatalf("normalizeHW = %q", normalizeHW("AA:BB-cc.dd"))
	}
}

func TestResolveServer(t *testing.T) {
	mk := func(id, name string) mdmServer {
		var s mdmServer
		s.ID, s.Attributes.ServerName = id, name
		return s
	}
	servers := []mdmServer{mk("A1", "Jamf Prod"), mk("B2", "Intune"), mk("C3", "intune")}
	if s, err := resolveServer(servers, "jamf prod"); err != nil || s.ID != "A1" {
		t.Fatalf("name match: %v %v", s.ID, err)
	}
	if s, err := resolveServer(servers, "B2"); err != nil || s.ID != "B2" {
		t.Fatalf("id match: %v %v", s.ID, err)
	}
	if _, err := resolveServer(servers, "Intune"); err == nil {
		t.Fatal("ambiguous name must error")
	}
	if _, err := resolveServer(servers, "Kandji"); err == nil {
		t.Fatal("unknown name must error")
	}
}

func TestActivityBody(t *testing.T) {
	b, _ := json.Marshal(activityBody("ASSIGN_DEVICES_WITH_MDM_MIGRATION_DEADLINE", "SRV", []string{"S1", "S2"}, "2026-03-15T17:00:00Z"))
	want := `{"data":{"attributes":{"activityType":"ASSIGN_DEVICES_WITH_MDM_MIGRATION_DEADLINE","activityTypeMetadata":{"mdmMigrationDeadlineDateTime":"2026-03-15T17:00:00Z"}},"relationships":{"devices":{"data":[{"id":"S1","type":"orgDevices"},{"id":"S2","type":"orgDevices"}]},"mdmServer":{"data":{"id":"SRV","type":"mdmServers"}}},"type":"orgDeviceActivities"}}`
	if string(b) != want {
		t.Fatalf("body =\n%s\nwant\n%s", b, want)
	}
}

func TestReadSerialsJSONAndLines(t *testing.T) {
	cases := map[string][]string{
		`[{"serial":"S1","family":"Mac"},{"serial":"S2"}]`: {"S1", "S2"},
		`{"devices":[{"serial":"S3"}],"not_found":[]}`:     {"S3"},
		`["S4","S5"]`:            {"S4", "S5"},
		"serial\nS6\nS7,extra\n": {"S6", "S7"},
	}
	for in, want := range cases {
		got, err := readSerials([]string{"S0"}, "-", strings.NewReader(in))
		if err != nil {
			t.Fatal(err)
		}
		want = append([]string{"S0"}, want...)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}
