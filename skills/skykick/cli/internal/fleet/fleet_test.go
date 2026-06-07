// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the fleet novel-feature logic.
package fleet

import (
	"encoding/json"
	"testing"
	"time"
)

func boolPtr(b bool) *bool           { return &b }
func intPtr(i int) *int              { return &i }
func timePtr(t time.Time) *time.Time { return &t }

var now = time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

func TestParseSubscriptions(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantLen int
		wantID  string
		wantCo  string
		wantPID string
	}{
		{
			name:    "bare array PascalCase",
			raw:     `[{"Id":"abc-123","CompanyName":"Contoso Ltd","PartnerId":"p-9"}]`,
			wantLen: 1, wantID: "abc-123", wantCo: "Contoso Ltd", wantPID: "p-9",
		},
		{
			name:    "wrapped value array camelCase",
			raw:     `{"value":[{"id":"s1","companyName":"Fabrikam"}]}`,
			wantLen: 1, wantID: "s1", wantCo: "Fabrikam",
		},
		{
			name:    "nested customer info",
			raw:     `[{"subscriptionId":"s2","CustomerInformation":{"CompanyName":"Northwind"}}]`,
			wantLen: 1, wantID: "s2", wantCo: "Northwind",
		},
		{
			name:    "rows without ids skipped",
			raw:     `[{"companyName":"NoId Inc"},{"id":"s3"}]`,
			wantLen: 1, wantID: "s3",
		},
		{name: "empty array", raw: `[]`, wantLen: 0},
		{name: "not json array or wrapper", raw: `{"message":"nope"}`, wantLen: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subs := ParseSubscriptions(json.RawMessage(tc.raw))
			if len(subs) != tc.wantLen {
				t.Fatalf("len=%d want %d", len(subs), tc.wantLen)
			}
			if tc.wantLen == 0 {
				return
			}
			if subs[0].ID != tc.wantID {
				t.Errorf("id=%q want %q", subs[0].ID, tc.wantID)
			}
			if tc.wantCo != "" && subs[0].CompanyName != tc.wantCo {
				t.Errorf("company=%q want %q", subs[0].CompanyName, tc.wantCo)
			}
			if tc.wantPID != "" && subs[0].PartnerID != tc.wantPID {
				t.Errorf("partner=%q want %q", subs[0].PartnerID, tc.wantPID)
			}
		})
	}
}

func TestParseSettings(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantCo   string
		wantExch *bool
		wantSP   *bool
	}{
		{
			name:     "explicit booleans",
			raw:      `{"CustomerInformation":{"CompanyName":"Contoso"},"ExchangeBackupEnabled":true,"SharePointBackupEnabled":false}`,
			wantCo:   "Contoso",
			wantExch: boolPtr(true), wantSP: boolPtr(false),
		},
		{
			name:     "string states",
			raw:      `{"exchangeState":"Enabled","sharePointState":"Disabled"}`,
			wantExch: boolPtr(true), wantSP: boolPtr(false),
		},
		{
			name: "unknown fields stay nil",
			raw:  `{"somethingElse":1}`,
		},
		{name: "non-object", raw: `[1,2]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := ParseSettings("sub-1", json.RawMessage(tc.raw))
			if s.SubscriptionID != "sub-1" {
				t.Fatalf("subscription id lost")
			}
			if s.CompanyName != tc.wantCo {
				t.Errorf("company=%q want %q", s.CompanyName, tc.wantCo)
			}
			checkTriState(t, "exchange", s.ExchangeEnabled, tc.wantExch)
			checkTriState(t, "sharepoint", s.SharePointEnabled, tc.wantSP)
		})
	}
}

func checkTriState(t *testing.T, what string, got, want *bool) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s: got %v want nil", what, *got)
	case want != nil && got == nil:
		t.Errorf("%s: got nil want %v", what, *want)
	case want != nil && got != nil && *want != *got:
		t.Errorf("%s: got %v want %v", what, *got, *want)
	}
}

func TestParseRetention(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantExch *int
		wantSP   *int
	}{
		{
			name:     "upstream misspelling honored",
			raw:      `{"ExchangeRentionPeriodInDays":365,"SharePointRetentionPeriodInDays":180}`,
			wantExch: intPtr(365), wantSP: intPtr(180),
		},
		{
			name:     "corrected spelling also matches",
			raw:      `{"ExchangeRetentionPeriodInDays":"30"}`,
			wantExch: intPtr(30),
		},
		{name: "absent stays nil", raw: `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ParseRetention("sub-1", json.RawMessage(tc.raw))
			if (r.ExchangeDays == nil) != (tc.wantExch == nil) || (r.ExchangeDays != nil && *r.ExchangeDays != *tc.wantExch) {
				t.Errorf("exchange=%v want %v", r.ExchangeDays, tc.wantExch)
			}
			if (r.SharePointDays == nil) != (tc.wantSP == nil) || (r.SharePointDays != nil && *r.SharePointDays != *tc.wantSP) {
				t.Errorf("sharepoint=%v want %v", r.SharePointDays, tc.wantSP)
			}
		})
	}
}

func TestParseMailboxStats(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantLen  int
		wantBox  string
		wantSnap bool // expect a parsed snapshot time
	}{
		{
			name:    "known field RFC3339",
			raw:     `[{"SmtpAddress":"a@contoso.com","LastSnapshotDate":"2026-06-01T08:00:00Z"}]`,
			wantLen: 1, wantBox: "a@contoso.com", wantSnap: true,
		},
		{
			name:    "odata date via fragment scan",
			raw:     `{"value":[{"EmailAddress":"b@contoso.com","MostRecentSnapshotTime":"/Date(1717200000000)/"}]}`,
			wantLen: 1, wantBox: "b@contoso.com", wantSnap: true,
		},
		{
			name:    "mailbox with no time still recorded",
			raw:     `[{"MailboxName":"c@contoso.com"}]`,
			wantLen: 1, wantBox: "c@contoso.com", wantSnap: false,
		},
		{
			name:    "empty rows skipped",
			raw:     `[{}]`,
			wantLen: 0,
		},
		{
			name:    "unnamed row with timestamp skipped (store-key collision guard)",
			raw:     `[{"LastSnapshotDate":"2026-06-01T08:00:00Z"},{"LastSnapshotDate":"2026-06-02T08:00:00Z"}]`,
			wantLen: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stats := ParseMailboxStats("sub-1", json.RawMessage(tc.raw))
			if len(stats) != tc.wantLen {
				t.Fatalf("len=%d want %d", len(stats), tc.wantLen)
			}
			if tc.wantLen == 0 {
				return
			}
			if stats[0].Mailbox != tc.wantBox {
				t.Errorf("mailbox=%q want %q", stats[0].Mailbox, tc.wantBox)
			}
			if (stats[0].LastSnapshot != nil) != tc.wantSnap {
				t.Errorf("snapshot parsed=%v want %v", stats[0].LastSnapshot != nil, tc.wantSnap)
			}
		})
	}
}

func TestParseMailboxesAndSites(t *testing.T) {
	mailboxes := ParseMailboxes("s1", json.RawMessage(
		`{"IndividualMailboxes":[{"MailboxId":"m1","SmtpAddress":"a@x.com","IsEnabled":true},{"MailboxId":"m2","SmtpAddress":"b@x.com","IsEnabled":false},{"MailboxId":"m3","SmtpAddress":"c@x.com"}]}`))
	if len(mailboxes) != 3 {
		t.Fatalf("mailboxes len=%d want 3", len(mailboxes))
	}
	if mailboxes[0].Enabled == nil || !*mailboxes[0].Enabled {
		t.Errorf("m1 should be enabled")
	}
	if mailboxes[1].Enabled == nil || *mailboxes[1].Enabled {
		t.Errorf("m2 should be disabled")
	}
	if mailboxes[2].Enabled != nil {
		t.Errorf("m3 enablement should be unknown (nil)")
	}

	sites := ParseSites("s1", json.RawMessage(
		`[{"Url":"https://x.sharepoint.com/sites/a","BackupEnabled":"Enabled"},{"Url":"https://x.sharepoint.com/sites/b","BackupEnabled":"Disabled"}]`))
	if len(sites) != 2 {
		t.Fatalf("sites len=%d want 2", len(sites))
	}
	if sites[0].Enabled == nil || !*sites[0].Enabled {
		t.Errorf("site a should be enabled")
	}
	if sites[1].Enabled == nil || *sites[1].Enabled {
		t.Errorf("site b should be disabled")
	}
}

func TestParseAlerts(t *testing.T) {
	alerts := ParseAlerts("s1", json.RawMessage(
		`[{"Id":"al-1","Severity":"Critical","Status":"Active","Description":"Backup failed","CreatedDate":"2026-06-05T01:00:00Z"},{"NoId":true}]`))
	if len(alerts) != 1 {
		t.Fatalf("alerts len=%d want 1 (no-id row must be skipped)", len(alerts))
	}
	a := alerts[0]
	if a.ID != "al-1" || a.Severity != "Critical" || a.Status != "Active" || a.Description != "Backup failed" || a.Created == nil {
		t.Errorf("unexpected alert extraction: %+v", a)
	}
}

func TestBuildPostures(t *testing.T) {
	state := FleetState{
		Subscriptions: []Subscription{
			{ID: "s1", CompanyName: "Contoso", PartnerID: "p1"},
			{ID: "s2", CompanyName: "Fabrikam"},
		},
		Settings: []Settings{
			{SubscriptionID: "s1", ExchangeEnabled: boolPtr(true), SharePointEnabled: boolPtr(false)},
			{SubscriptionID: "s2", ExchangeEnabled: boolPtr(true), SharePointEnabled: boolPtr(true)},
		},
		Retention: []Retention{
			{SubscriptionID: "s1", ExchangeDays: intPtr(365)},
		},
		Autodiscover: []Autodiscover{
			{SubscriptionID: "s1", ExchangeOn: boolPtr(false), SharePointOn: boolPtr(false)},
			{SubscriptionID: "s2", ExchangeOn: boolPtr(true)},
		},
		Stats: []MailboxStat{
			{SubscriptionID: "s1", Mailbox: "a@contoso.com", LastSnapshot: timePtr(now.Add(-100 * time.Hour))},
			{SubscriptionID: "s2", Mailbox: "z@fabrikam.com", LastSnapshot: timePtr(now.Add(-1 * time.Hour))},
		},
		Mailboxes: []Mailbox{
			{SubscriptionID: "s1", Email: "a@contoso.com", Enabled: boolPtr(true)},
			{SubscriptionID: "s1", Email: "b@contoso.com", Enabled: boolPtr(false)},
			{SubscriptionID: "s2", Email: "z@fabrikam.com", Enabled: boolPtr(true)},
		},
		Sites: []Site{
			{SubscriptionID: "s2", URL: "https://f.sharepoint.com/sites/hq", Enabled: boolPtr(true)},
		},
	}
	postures := BuildPostures(state, now)
	if len(postures) != 2 {
		t.Fatalf("postures len=%d want 2", len(postures))
	}
	// s1 has more gaps so sorts first.
	p1 := postures[0]
	if p1.SubscriptionID != "s1" {
		t.Fatalf("expected s1 first (most gaps), got %s", p1.SubscriptionID)
	}
	wantGaps := map[string]bool{"sharepoint_backup_off": true, "autodiscover_off": true, "unprotected_mailboxes": true, "stale_backup": true}
	if len(p1.Gaps) != len(wantGaps) {
		t.Errorf("s1 gaps=%v want %v", p1.Gaps, wantGaps)
	}
	for _, g := range p1.Gaps {
		if !wantGaps[g] {
			t.Errorf("unexpected gap %q", g)
		}
	}
	if p1.MailboxesEnabled != 1 || p1.MailboxesTotal != 2 {
		t.Errorf("s1 mailbox counts %d/%d want 1/2", p1.MailboxesEnabled, p1.MailboxesTotal)
	}
	p2 := postures[1]
	if len(p2.Gaps) != 0 {
		t.Errorf("s2 gaps=%v want none", p2.Gaps)
	}
	if p2.SitesEnabled != 1 || p2.SitesTotal != 1 {
		t.Errorf("s2 site counts %d/%d want 1/1", p2.SitesEnabled, p2.SitesTotal)
	}
}

func TestFindStale(t *testing.T) {
	companies := map[string]string{"s1": "Contoso"}
	stats := []MailboxStat{
		{SubscriptionID: "s1", Mailbox: "fresh@x.com", LastSnapshot: timePtr(now.Add(-2 * time.Hour))},
		{SubscriptionID: "s1", Mailbox: "stale@x.com", LastSnapshot: timePtr(now.Add(-72 * time.Hour))},
		{SubscriptionID: "s1", Mailbox: "never@x.com"},
	}
	rows := FindStale(stats, companies, 48*time.Hour, now)
	if len(rows) != 2 {
		t.Fatalf("stale len=%d want 2", len(rows))
	}
	if !rows[0].NeverSeen || rows[0].Mailbox != "never@x.com" {
		t.Errorf("never-snapshotted row should sort first, got %+v", rows[0])
	}
	if rows[1].Mailbox != "stale@x.com" || rows[1].AgeHours == nil || *rows[1].AgeHours < 71 {
		t.Errorf("unexpected stale row %+v", rows[1])
	}
	// Absence-of-correctness: all-fresh input returns empty, not fabricated.
	none := FindStale(stats[:1], companies, 48*time.Hour, now)
	if len(none) != 0 {
		t.Errorf("fresh-only input produced %d stale rows", len(none))
	}
}

func TestCoverageGaps(t *testing.T) {
	companies := map[string]string{"s1": "Contoso"}
	mailboxes := []Mailbox{
		{SubscriptionID: "s1", Email: "on@x.com", Enabled: boolPtr(true)},
		{SubscriptionID: "s1", Email: "off@x.com", Enabled: boolPtr(false)},
		{SubscriptionID: "s1", Email: "unk@x.com"},
	}
	sites := []Site{
		{SubscriptionID: "s1", URL: "https://x/sites/off", Enabled: boolPtr(false)},
	}
	gaps, unknown := CoverageGaps(mailboxes, sites, companies, "all")
	if len(gaps) != 2 {
		t.Fatalf("gaps len=%d want 2", len(gaps))
	}
	if unknown != 1 {
		t.Errorf("unknown=%d want 1 (nil enablement is NOT a gap)", unknown)
	}
	gapsOnlyMail, _ := CoverageGaps(mailboxes, sites, companies, "mailboxes")
	if len(gapsOnlyMail) != 1 || gapsOnlyMail[0].Kind != "mailbox" {
		t.Errorf("type filter failed: %+v", gapsOnlyMail)
	}
}

func TestRetentionAudit(t *testing.T) {
	companies := map[string]string{}
	rows := RetentionAudit([]Retention{
		{SubscriptionID: "pass", ExchangeDays: intPtr(400), SharePointDays: intPtr(365)},
		{SubscriptionID: "under", ExchangeDays: intPtr(30)},
		{SubscriptionID: "unknown"},
	}, companies, 365)
	if len(rows) != 3 {
		t.Fatalf("rows=%d want 3", len(rows))
	}
	byID := map[string]string{}
	for _, r := range rows {
		byID[r.SubscriptionID] = r.Status
	}
	if byID["pass"] != "pass" || byID["under"] != "under_floor" || byID["unknown"] != "unknown" {
		t.Errorf("statuses wrong: %v", byID)
	}
	if rows[0].Status != "under_floor" {
		t.Errorf("under_floor should sort first, got %s", rows[0].Status)
	}
}

func TestAutodiscoverAudit(t *testing.T) {
	states := []Autodiscover{
		{SubscriptionID: "on", ExchangeOn: boolPtr(true), SharePointOn: boolPtr(true)},
		{SubscriptionID: "off", ExchangeOn: boolPtr(false), SharePointOn: boolPtr(false)},
		{SubscriptionID: "partial", ExchangeOn: boolPtr(true), SharePointOn: boolPtr(false)},
		{SubscriptionID: "unknown"},
	}
	all := AutodiscoverAudit(states, map[string]string{}, false)
	if len(all) != 4 {
		t.Fatalf("all len=%d want 4", len(all))
	}
	if all[0].Status != "off" {
		t.Errorf("off should sort first, got %s", all[0].Status)
	}
	onlyOff := AutodiscoverAudit(states, map[string]string{}, true)
	if len(onlyOff) != 2 {
		t.Fatalf("onlyOff len=%d want 2 (off+partial)", len(onlyOff))
	}
}

func TestDrift(t *testing.T) {
	prev := FleetState{
		Subscriptions: []Subscription{{ID: "s1", CompanyName: "Contoso"}, {ID: "gone"}},
		Settings:      []Settings{{SubscriptionID: "s1", ExchangeEnabled: boolPtr(true)}},
		Retention:     []Retention{{SubscriptionID: "s1", ExchangeDays: intPtr(365)}},
		Stats: []MailboxStat{
			{SubscriptionID: "s1", Mailbox: "a@x.com", LastSnapshot: timePtr(now.Add(-1 * time.Hour))},
		},
		Mailboxes: []Mailbox{{SubscriptionID: "s1", Email: "a@x.com", Enabled: boolPtr(true)}},
	}
	cur := FleetState{
		Subscriptions: []Subscription{{ID: "s1", CompanyName: "Contoso"}, {ID: "new"}},
		Settings:      []Settings{{SubscriptionID: "s1", ExchangeEnabled: boolPtr(false)}},
		Retention:     []Retention{{SubscriptionID: "s1", ExchangeDays: intPtr(30)}},
		Stats: []MailboxStat{
			{SubscriptionID: "s1", Mailbox: "a@x.com", LastSnapshot: timePtr(now.Add(-100 * time.Hour))},
		},
		Mailboxes: []Mailbox{{SubscriptionID: "s1", Email: "a@x.com", Enabled: boolPtr(false)}},
	}
	rep := Drift(prev, cur, 48*time.Hour, now)
	if len(rep.AddedSubscriptions) != 1 || rep.AddedSubscriptions[0] != "new" {
		t.Errorf("added=%v", rep.AddedSubscriptions)
	}
	if len(rep.RemovedSubscriptions) != 1 || rep.RemovedSubscriptions[0] != "gone" {
		t.Errorf("removed=%v", rep.RemovedSubscriptions)
	}
	if len(rep.EnablementFlips) != 1 || rep.EnablementFlips[0].What != "exchange_backup" || rep.EnablementFlips[0].To != "off" {
		t.Errorf("flips=%+v", rep.EnablementFlips)
	}
	if len(rep.NewlyStaleMailboxes) != 1 || rep.NewlyStaleMailboxes[0].Mailbox != "a@x.com" {
		t.Errorf("newly stale=%+v", rep.NewlyStaleMailboxes)
	}
	if len(rep.MailboxFlips) != 1 || rep.MailboxFlips[0].To != "off" {
		t.Errorf("mailbox flips=%+v", rep.MailboxFlips)
	}
	if len(rep.RetentionChanges) != 1 || *rep.RetentionChanges[0].To != 30 {
		t.Errorf("retention changes=%+v", rep.RetentionChanges)
	}

	// Absence-of-correctness: identical states produce an empty report.
	same := Drift(cur, cur, 48*time.Hour, now)
	if !same.Empty() {
		t.Errorf("identical states should produce empty drift, got %+v", same)
	}
	// unknown->known transitions are NOT flips.
	repUnknown := Drift(
		FleetState{Settings: []Settings{{SubscriptionID: "s1"}}},
		FleetState{Settings: []Settings{{SubscriptionID: "s1", ExchangeEnabled: boolPtr(false)}}},
		48*time.Hour, now)
	if len(repUnknown.EnablementFlips) != 0 {
		t.Errorf("unknown->known must not count as flip: %+v", repUnknown.EnablementFlips)
	}
}

func TestPartnerRollup(t *testing.T) {
	postures := []TenantPosture{
		{SubscriptionID: "s1", PartnerID: "p1", Gaps: []string{"stale_backup"}, MailboxesTotal: 5, MailboxesEnabled: 3},
		{SubscriptionID: "s2", PartnerID: "p1", Gaps: []string{}},
		{SubscriptionID: "s3", Gaps: []string{"unprotected_sites"}, SitesTotal: 2, SitesEnabled: 1},
	}
	rollup := PartnerRollup(postures)
	if len(rollup) != 2 {
		t.Fatalf("rollup len=%d want 2", len(rollup))
	}
	byID := map[string]PartnerSummary{}
	for _, r := range rollup {
		byID[r.PartnerID] = r
	}
	p1 := byID["p1"]
	if p1.Tenants != 2 || p1.TenantsWithGaps != 1 || p1.UnprotectedBoxes != 2 || p1.StaleTenants != 1 {
		t.Errorf("p1=%+v", p1)
	}
	unk := byID["(unknown)"]
	if unk.Tenants != 1 || unk.UnprotectedSites != 1 {
		t.Errorf("(unknown)=%+v", unk)
	}
}

func TestRankAlerts(t *testing.T) {
	early := now.Add(-48 * time.Hour)
	late := now.Add(-1 * time.Hour)
	alerts := []Alert{
		{ID: "info-late", Severity: "Info", Created: &late},
		{ID: "crit-early", Severity: "Critical", Created: &early},
		{ID: "warn", Severity: "Warning", Created: &late},
		{ID: "crit-late", Severity: "Critical", Created: &late},
	}
	ranked := RankAlerts(alerts)
	wantOrder := []string{"crit-late", "crit-early", "warn", "info-late"}
	for i, want := range wantOrder {
		if ranked[i].ID != want {
			t.Fatalf("rank[%d]=%s want %s (full: %v)", i, ranked[i].ID, want, ids(ranked))
		}
	}
}

func ids(alerts []Alert) []string {
	out := make([]string, len(alerts))
	for i, a := range alerts {
		out[i] = a.ID
	}
	return out
}

func TestOperationTerminal(t *testing.T) {
	cases := []struct {
		status   string
		terminal bool
		ok       bool
	}{
		{"Completed", true, true},
		{"succeeded", true, true},
		{"FAILED", true, false},
		{"Cancelled", true, false},
		{"InProgress", false, false},
		{"Queued", false, false},
		{"", false, false},
	}
	for _, tc := range cases {
		term, ok := OperationTerminal(tc.status)
		if term != tc.terminal || ok != tc.ok {
			t.Errorf("OperationTerminal(%q)=(%v,%v) want (%v,%v)", tc.status, term, ok, tc.terminal, tc.ok)
		}
	}
}

func TestExtractOperationStatusAndID(t *testing.T) {
	status, opID := ExtractOperationStatus(json.RawMessage(`{"Id":"op-1","Status":"InProgress"}`))
	if status != "InProgress" || opID != "op-1" {
		t.Errorf("got (%q,%q)", status, opID)
	}
	if got := ExtractOperationID(json.RawMessage(`{"operationId":"op-9"}`)); got != "op-9" {
		t.Errorf("op id=%q", got)
	}
	if got := ExtractOperationID(json.RawMessage(`"bare-guid"`)); got != "bare-guid" {
		t.Errorf("bare guid=%q", got)
	}
}

func TestCompanyIndex(t *testing.T) {
	idx := CompanyIndex(
		[]Subscription{{ID: "s1", CompanyName: "FromList"}, {ID: "s2"}},
		[]Settings{{SubscriptionID: "s1", CompanyName: "FromSettings"}, {SubscriptionID: "s2", CompanyName: "OnlySettings"}},
	)
	if idx["s1"] != "FromSettings" {
		t.Errorf("settings name should win: %q", idx["s1"])
	}
	if idx["s2"] != "OnlySettings" {
		t.Errorf("s2=%q", idx["s2"])
	}
}

func TestParseTimeLooseFormats(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"rfc3339", `"2026-06-01T08:00:00Z"`, true},
		{"dotnet no zone", `"2026-06-01T08:00:00"`, true},
		{"odata ms", `"/Date(1717200000000)/"`, true},
		{"epoch seconds", `1717200000`, true},
		{"epoch millis", `1717200000000`, true},
		{"garbage", `"not a date"`, false},
		{"empty", `""`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTimeLoose(json.RawMessage(tc.raw))
			if (got != nil) != tc.ok {
				t.Errorf("parseTimeLoose(%s) parsed=%v want %v", tc.raw, got != nil, tc.ok)
			}
		})
	}
}
