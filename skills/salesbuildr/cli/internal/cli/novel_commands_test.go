// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"salesbuildr-pp-cli/internal/store"
)

// seedNovelStore creates a temp store populated with a small, deterministic
// corpus the novel-command tests share: 3 quotes (stale approved, fresh sent,
// declined), 3 catalog products, 1 pricing book with one drifted entry, 3
// opportunities (won, lost, open), and 2 companies.
func seedNovelStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "novel.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	fresh := time.Now().UTC().Add(-2 * 24 * time.Hour).Format(time.RFC3339)

	type doc struct {
		resource string
		id       string
		body     string
	}
	docs := []doc{
		{"quote", "q1", fmt.Sprintf(`{"id":"q1","number":"Q-001","title":"Server refresh","company":{"id":"c1","name":"Acme Managed IT"},"status":"approved","createdAt":%q,"sentAt":%q,"approvedAt":%q,"items":[{"id":"i1","name":"Rack Server","mpn":"SRV-100","product":{"id":"p1"},"price":1000,"cost":900,"markup":11.1,"quantity":2},{"id":"i2","name":"Switch","mpn":"SW-20","product":{"id":"p2"},"price":200,"cost":100,"markup":100,"quantity":1}]}`, old, old, old)},
		{"quote", "q2", fmt.Sprintf(`{"id":"q2","number":"Q-002","title":"Backup bundle","company":{"id":"c2","name":"Beta Corp"},"status":"sent","createdAt":%q,"sentAt":%q,"items":[{"id":"i3","name":"Backup Agent","mpn":"BA-1","product":{"id":"p3"},"price":50,"cost":10,"quantity":10}]}`, fresh, fresh)},
		{"quote", "q3", fmt.Sprintf(`{"id":"q3","number":"Q-003","title":"Declined deal","company":{"id":"c1","name":"Acme Managed IT"},"status":"declined","createdAt":%q,"sentAt":%q,"declinedAt":%q,"items":[{"id":"i4","name":"Firewall","mpn":"FW-9","product":{"id":"p2"},"price":500,"cost":400,"quantity":1}]}`, old, old, fresh)},
		{"product", "p1", `{"id":"p1","name":"Rack Server","mpn":"SRV-100","price":1000,"cost":900,"markup":11.1,"msrp":1200}`},
		{"product", "p2", `{"id":"p2","name":"Switch","mpn":"SW-20","price":200,"cost":100,"markup":100,"msrp":250}`},
		{"product", "p3", `{"id":"p3","name":"Backup Agent","mpn":"BA-1","price":50,"cost":10,"markup":400,"msrp":60,"externalIdentifier":"AT-77"}`},
		{"pricing-book", "pb1", `{"id":"pb1","name":"Acme Book","products":[{"productId":"p1","price":950,"cost":900},{"productId":"p2","price":200,"cost":100}]}`},
		{"opportunity", "o1", fmt.Sprintf(`{"id":"o1","name":"Won deal","company":{"name":"Acme Managed IT"},"owner":{"name":"Marcus"},"pipelineStage":{"name":"Closed"},"statusId":"won","closedAt":%q,"salesCycleDurationDays":20,"probability":100,"monthlyRevenue":100,"externalIdentifier":"AT-1"}`, fresh)},
		{"opportunity", "o2", fmt.Sprintf(`{"id":"o2","name":"Lost deal","company":{"name":"Beta Corp"},"owner":{"name":"Marcus"},"pipelineStage":{"name":"Closed"},"statusId":"lost","closedAt":%q,"salesCycleDurationDays":40}`, fresh)},
		{"opportunity", "o3", fmt.Sprintf(`{"id":"o3","name":"Open deal","company":{"name":"Acme Managed IT"},"owner":{"name":"Priya"},"pipelineStage":{"name":"Proposal"},"statusId":"open","probability":50,"monthlyRevenue":1000,"monthlyProfit":400,"onetimeRevenue":2000,"monthlyChurn":10,"stageUpdatedAt":%q}`, old)},
		{"company", "c1", `{"id":"c1","name":"Acme Managed IT","externalIdentifier":"AT-100"}`},
		{"company", "c2", `{"id":"c2","name":"Beta Corp"}`},
		{"contact", "ct1", `{"id":"ct1","firstName":"Jane","lastName":"Doe","externalIdentifier":"AT-CT-1"}`},
		{"contact", "ct2", `{"id":"ct2","firstName":"John","lastName":"Smith"}`},
	}
	for _, d := range docs {
		if err := db.Upsert(d.resource, d.id, json.RawMessage(d.body)); err != nil {
			t.Fatalf("seed %s/%s: %v", d.resource, d.id, err)
		}
	}
	return dbPath
}

// runNovel executes a novel command constructor against the seeded store and
// returns its decoded JSON output.
func runNovel(t *testing.T, build func(*rootFlags) *cobra.Command, args ...string) map[string]any {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := build(flags)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %s %v: %v", cmd.Use, args, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("output not a JSON object: %v\n%s", err, out.String())
	}
	return decoded
}

func listLen(t *testing.T, m map[string]any, key string) int {
	t.Helper()
	v, ok := m[key].([]any)
	if !ok {
		t.Fatalf("field %q is not an array: %#v", key, m[key])
	}
	return len(v)
}

func TestQuoteStaleFindsAgingApprovedQuote(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelQuoteStaleCmd, "--days", "14", "--db", dbPath)
	if got := listLen(t, report, "quotes"); got != 1 {
		t.Fatalf("expected exactly 1 stale quote (q1), got %d: %#v", got, report["quotes"])
	}
	q := report["quotes"].([]any)[0].(map[string]any)
	if q["number"] != "Q-001" {
		t.Fatalf("expected Q-001, got %v", q["number"])
	}
	// at-risk = 1000*2 + 200*1 = 2200
	if v := report["totalAtRisk"].(float64); v != 2200 {
		t.Fatalf("expected totalAtRisk 2200, got %v", v)
	}
	if q["ageFrom"] != "approvedAt" {
		t.Fatalf("expected age measured from approvedAt, got %v", q["ageFrom"])
	}
}

func TestQuoteStaleExcludesFreshAndDeclined(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelQuoteStaleCmd, "--days", "0", "--db", dbPath)
	// q3 is declined; only q1 (30d) and q2 (2d) qualify at --days 0.
	if got := listLen(t, report, "quotes"); got != 2 {
		t.Fatalf("expected 2 open quotes at --days 0, got %d", got)
	}
}

func TestQuoteThinFlagsBelowFloorOnly(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelQuoteThinCmd, "--floor", "20", "--db", dbPath)
	// q1/i1 markup 11.1 < 20; q1/i2 markup 100; q2/i3 computed (50-10)/10*100=400; q3 declined.
	if got := listLen(t, report, "lines"); got != 1 {
		t.Fatalf("expected 1 thin line, got %d: %#v", got, report["lines"])
	}
	line := report["lines"].([]any)[0].(map[string]any)
	if line["mpn"] != "SRV-100" {
		t.Fatalf("expected SRV-100, got %v", line["mpn"])
	}
}

func TestQuoteFunnelBucketsAllStages(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelQuoteFunnelCmd, "--db", dbPath)
	stages := map[string]map[string]any{}
	for _, raw := range report["stages"].([]any) {
		s := raw.(map[string]any)
		stages[s["stage"].(string)] = s
	}
	if stages["approved"]["count"].(float64) != 1 || stages["sent"]["count"].(float64) != 1 || stages["declined"]["count"].(float64) != 1 {
		t.Fatalf("unexpected funnel counts: %#v", stages)
	}
	// approved vs declined = 1/(1+1) = 50%
	if pct := report["sentToApprovedPct"].(float64); pct != 50 {
		t.Fatalf("expected 50%% approved-vs-declined, got %v", pct)
	}
}

func TestPricingDriftReportsOnlyDivergedEntries(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelPricingDriftCmd, "--db", dbPath)
	if got := listLen(t, report, "rows"); got != 1 {
		t.Fatalf("expected 1 drift row (p1), got %d: %#v", got, report["rows"])
	}
	row := report["rows"].([]any)[0].(map[string]any)
	if row["productId"] != "p1" || row["priceDelta"].(float64) != -50 {
		t.Fatalf("unexpected drift row: %#v", row)
	}
}

func TestOpportunityVelocityAggregates(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelOpportunityVelocityCmd, "--db", dbPath)
	if report["total"].(float64) != 3 {
		t.Fatalf("expected 3 opportunities, got %v", report["total"])
	}
	// cycles: 20 and 40 → avg 30, median 30
	if avg := report["avgCycleDays"].(float64); avg != 30 {
		t.Fatalf("expected avg cycle 30, got %v", avg)
	}
}

func TestOpportunityWinrateByOwner(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelOpportunityWinrateCmd, "--by", "owner", "--db", dbPath)
	rows := map[string]map[string]any{}
	for _, raw := range report["rows"].([]any) {
		r := raw.(map[string]any)
		rows[r["group"].(string)] = r
	}
	marcus := rows["Marcus"]
	if marcus["won"].(float64) != 1 || marcus["lost"].(float64) != 1 || marcus["winRatePct"].(float64) != 50 {
		t.Fatalf("unexpected Marcus winrate row: %#v", marcus)
	}
	if rows["Priya"]["open"].(float64) != 1 {
		t.Fatalf("expected Priya to have 1 open deal: %#v", rows["Priya"])
	}
}

func TestOpportunityWinrateRejectsBadDimension(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelOpportunityWinrateCmd(flags)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--by", "moon-phase"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected usage error for invalid --by dimension")
	}
}

func TestMrrForecastWeightsOpenPipelineOnly(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelOpportunityMrrForecastCmd, "--db", dbPath)
	if report["openOpportunities"].(float64) != 1 {
		t.Fatalf("expected 1 open opportunity, got %v", report["openOpportunities"])
	}
	// o3: 1000 * 0.5 = 500 weighted; churn 10% → 450 adjusted.
	if mrr := report["weightedMrr"].(float64); mrr != 500 {
		t.Fatalf("expected weighted MRR 500, got %v", mrr)
	}
	if adj := report["churnAdjustedMrr"].(float64); adj != 450 {
		t.Fatalf("expected churn-adjusted MRR 450, got %v", adj)
	}
}

func TestCompanyWhitespaceSetDifference(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelCompanyWhitespaceCmd, "Acme Managed IT", "--db", dbPath)
	// Acme quotes: q1 (p1, p2) + q3 (p2, declined still counts as "ever quoted").
	// Catalog: p1, p2, p3 → whitespace = p3 only.
	if got := listLen(t, report, "whitespace"); got != 1 {
		t.Fatalf("expected 1 whitespace product, got %d: %#v", got, report["whitespace"])
	}
	p := report["whitespace"].([]any)[0].(map[string]any)
	if p["id"] != "p3" {
		t.Fatalf("expected p3 in whitespace, got %v", p["id"])
	}
}

func TestCompanyWhitespaceRequiresArgument(t *testing.T) {
	flags := &rootFlags{asJSON: true}
	cmd := newNovelCompanyWhitespaceCmd(flags)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--limit", "5"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected usage error when company argument is missing")
	}
}

func TestProductVelocityRanksByValue(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelProductVelocityCmd, "--db", dbPath)
	rows := report["rows"].([]any)
	if len(rows) != 3 {
		t.Fatalf("expected 3 ranked products, got %d", len(rows))
	}
	top := rows[0].(map[string]any)
	// p1: 1000*2 = 2000 is the top value.
	if top["productId"] != "p1" || top["totalValue"].(float64) != 2000 {
		t.Fatalf("unexpected top velocity row: %#v", top)
	}
	// p2 appears on q1 and q3 → quoteCount 2.
	for _, raw := range rows {
		r := raw.(map[string]any)
		if r["productId"] == "p2" && r["quoteCount"].(float64) != 2 {
			t.Fatalf("expected p2 on 2 quotes: %#v", r)
		}
	}
}

func TestReconcilePsaCountsMissingExternalIDs(t *testing.T) {
	dbPath := seedNovelStore(t)
	report := runNovel(t, newNovelReconcilePsaCmd, "--db", dbPath)
	gaps := map[string]map[string]any{}
	for _, raw := range report["resources"].([]any) {
		g := raw.(map[string]any)
		gaps[g["resource"].(string)] = g
	}
	// companies: c2 missing; contacts: ct2 missing; products: p1,p2 missing; opportunities: o2,o3 missing.
	if gaps["company"]["missingExt"].(float64) != 1 ||
		gaps["contact"]["missingExt"].(float64) != 1 ||
		gaps["product"]["missingExt"].(float64) != 2 ||
		gaps["opportunity"]["missingExt"].(float64) != 2 {
		t.Fatalf("unexpected missing-ext counts: %#v", gaps)
	}
	if report["totalMissing"].(float64) != 6 {
		t.Fatalf("expected 6 total missing, got %v", report["totalMissing"])
	}
}

func TestSQLCommandRejectsMutations(t *testing.T) {
	for _, q := range []string{"DELETE FROM quote", "drop table quote", "insert into quote values(1)", "with x as (select 1) update quote set id=1", "select 1; analyze", "select 1; reindex", "SELECT 1; INSERT INTO quote VALUES(1)"} {
		if isReadOnlyQuery(q) {
			t.Fatalf("query should be rejected: %s", q)
		}
	}
	for _, q := range []string{"SELECT * FROM quote", "with x as (select 1) select * from x", "select count(*) from resources"} {
		if !isReadOnlyQuery(q) {
			t.Fatalf("query should be allowed: %s", q)
		}
	}
}

func TestSQLCommandQueriesStore(t *testing.T) {
	dbPath := seedNovelStore(t)
	flags := &rootFlags{asJSON: true}
	cmd := newSQLCmd(flags)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"SELECT resource_type, count(*) AS n FROM resources GROUP BY resource_type ORDER BY resource_type", "--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sql execute: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("sql output not JSON: %v\n%s", err, out.String())
	}
	if len(rows) < 5 {
		t.Fatalf("expected at least 5 resource types, got %d: %#v", len(rows), rows)
	}
}

func TestQuoteLifecycleStageClassification(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		quote novelQuote
		want  string
	}{
		{"declined wins over approved", novelQuote{ApprovedAt: now, DeclinedAt: now}, "declined"},
		{"expired", novelQuote{SentAt: now, ExpiredAt: now}, "expired"},
		{"approved", novelQuote{SentAt: now, ApprovedAt: now}, "approved"},
		{"sent", novelQuote{SentAt: now}, "sent"},
		{"draft", novelQuote{}, "draft"},
		{"status fallback", novelQuote{Status: "Declined"}, "declined"},
	}
	for _, tc := range cases {
		if got := tc.quote.lifecycleStage(); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestEffectiveMarkupFallsBackToComputed(t *testing.T) {
	cases := []struct {
		item   novelQuoteItem
		want   float64
		wantOK bool
	}{
		{novelQuoteItem{Markup: 25}, 25, true},
		{novelQuoteItem{Price: 120, Cost: 100}, 20, true},
		{novelQuoteItem{Price: 120}, 0, false},
		{novelQuoteItem{}, 0, false},
	}
	for i, tc := range cases {
		got, ok := tc.item.effectiveMarkup()
		if ok != tc.wantOK || (ok && (got < tc.want-0.001 || got > tc.want+0.001)) {
			t.Errorf("case %d: got (%v, %v) want (%v, %v)", i, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestResolveCompanyMatchingOrder(t *testing.T) {
	companies := []novelCompany{
		{ID: "c1", Name: "Acme Managed IT", ExternalIdentifier: "AT-100"},
		{ID: "c2", Name: "Acme Cloud"},
		{ID: "c3", Name: "Beta Corp"},
	}
	if c, err := resolveCompany(companies, "AT-100"); err != nil || c.ID != "c1" {
		t.Fatalf("external id match failed: %v %v", c, err)
	}
	if c, err := resolveCompany(companies, "beta corp"); err != nil || c.ID != "c3" {
		t.Fatalf("case-insensitive name match failed: %v %v", c, err)
	}
	if _, err := resolveCompany(companies, "acme"); err == nil {
		t.Fatal("expected ambiguity error for 'acme'")
	}
	if _, err := resolveCompany(companies, "zeta"); err == nil {
		t.Fatal("expected not-found error for 'zeta'")
	}
}

func TestSQLCommandRefusesNeverSyncedEmptyStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "phantom.db")
	flags := &rootFlags{asJSON: true}
	cmd := newSQLCmd(flags)
	cmd.SetOut(&bytes.Buffer{})
	errBuf := &bytes.Buffer{}
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"SELECT count(*) FROM resources", "--db", dbPath})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error querying a never-synced empty store")
	}
}

func TestProductVelocityMergesIDAndMPNLines(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "velocity.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	docs := map[string]string{
		"p1": `{"id":"p1","name":"Rack Server","mpn":"SRV-100","price":1000,"cost":900}`,
	}
	if err := db.Upsert("product", "p1", json.RawMessage(docs["p1"])); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	// Same physical product: q1 links by product id, q2 carries only the MPN.
	q1 := `{"id":"q1","number":"Q-1","status":"sent","sentAt":"2026-01-01T00:00:00Z","items":[{"id":"i1","name":"Rack Server","product":{"id":"p1"},"price":1000,"quantity":1}]}`
	q2 := `{"id":"q2","number":"Q-2","status":"sent","sentAt":"2026-01-02T00:00:00Z","items":[{"id":"i2","name":"Rack Server (alt)","mpn":"srv-100","price":1000,"quantity":1}]}`
	if err := db.Upsert("quote", "q1", json.RawMessage(q1)); err != nil {
		t.Fatalf("seed q1: %v", err)
	}
	if err := db.Upsert("quote", "q2", json.RawMessage(q2)); err != nil {
		t.Fatalf("seed q2: %v", err)
	}
	db.Close()

	report := runNovel(t, newNovelProductVelocityCmd, "--db", dbPath)
	rows := report["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("expected id-linked and mpn-only lines to merge into 1 row, got %d: %#v", len(rows), rows)
	}
	top := rows[0].(map[string]any)
	if top["quoteCount"].(float64) != 2 || top["totalValue"].(float64) != 2000 {
		t.Fatalf("expected merged row quoteCount=2 totalValue=2000: %#v", top)
	}
}
