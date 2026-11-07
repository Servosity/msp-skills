// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex r1 (salesbuildr 4.34.2 reprint review) wrong-result classes, each
// test written to fail on the code before its fix:
//   - documented-fraction-read-as-percent: OpportunityResponseDto.probability
//     is documented 0-1 (spec.json UpsertOpportunityDto.probability, minimum
//     0, maximum 1); the forecast divided it by 100.
//   - documented-zero-read-as-missing: QuoteItemResponseDto.quantity and
//     .markup are required numbers and 0 is a value (CreateQuoteProductDto
//     allows quantity 0); an explicit 0 was read as "absent".
//   - identifier-namespaces-merged: a company target matched internal id,
//     external identifier and name in one pass, first row wins, so another
//     company's external identifier (or a shared name) shadowed an exact id.

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"salesbuildr-pp-cli/internal/store"
)

func seedR1Store(t *testing.T, docs [][3]string) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "r1.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for _, d := range docs {
		if err := db.Upsert(d[0], d[1], json.RawMessage(d[2])); err != nil {
			t.Fatalf("seed %s/%s: %v", d[0], d[1], err)
		}
	}
	return dbPath
}

func TestR1ForecastReadsProbabilityAsDocumentedFraction(t *testing.T) {
	dbPath := seedR1Store(t, [][3]string{
		{"opportunity", "o1", `{"id":"o1","name":"Half","statusId":"open","probability":0.5,"monthlyRevenue":1000,"monthlyChurn":0}`},
		{"opportunity", "o2", `{"id":"o2","name":"Certain","statusId":"open","probability":1,"monthlyRevenue":100,"monthlyChurn":0}`},
	})
	report := runNovel(t, newNovelOpportunityMrrForecastCmd, "--db", dbPath)
	if mrr := report["weightedMrr"].(float64); mrr != 600 {
		t.Fatalf("weightedMrr = %v, want 600 (1000 x 0.5 + 100 x 1)", mrr)
	}
}

// A probability outside the documented 0-1 range is not guessed at: it is
// left out of the weighted totals and counted.
func TestR1ForecastLeavesOutOfRangeProbabilityOut(t *testing.T) {
	dbPath := seedR1Store(t, [][3]string{
		{"opportunity", "o1", `{"id":"o1","name":"Half","statusId":"open","probability":0.5,"monthlyRevenue":1000}`},
		{"opportunity", "o2", `{"id":"o2","name":"Percent","statusId":"open","probability":50,"monthlyRevenue":1000}`},
		{"opportunity", "o3", `{"id":"o3","name":"Negative","statusId":"open","probability":-0.2,"monthlyRevenue":1000}`},
	})
	report := runNovel(t, newNovelOpportunityMrrForecastCmd, "--db", dbPath)
	if mrr := report["weightedMrr"].(float64); mrr != 500 {
		t.Fatalf("weightedMrr = %v, want 500 (only the in-range deal)", mrr)
	}
	if n, _ := report["probabilityOutOfRange"].(float64); n != 2 {
		t.Fatalf("probabilityOutOfRange = %v, want 2: %#v", report["probabilityOutOfRange"], report)
	}
}

func TestR1FunnelCountsExplicitZeroQuantityAsZero(t *testing.T) {
	dbPath := seedR1Store(t, [][3]string{
		{"quote", "q1", `{"id":"q1","number":"Q-1","status":"sent","sentAt":"2026-01-01T00:00:00Z","items":[{"id":"i1","name":"Optional server","price":1000,"cost":800,"markup":25,"quantity":0}]}`},
	})
	report := runNovel(t, newNovelQuoteFunnelCmd, "--db", dbPath)
	for _, s := range report["stages"].([]any) {
		st := s.(map[string]any)
		if st["stage"] == "sent" && st["value"].(float64) != 0 {
			t.Fatalf("sent-stage value = %v, want 0 (the only line has quantity 0)", st["value"])
		}
	}
}

func TestR1ThinReportsExplicitZeroMarkup(t *testing.T) {
	dbPath := seedR1Store(t, [][3]string{
		{"quote", "q1", `{"id":"q1","number":"Q-1","status":"sent","sentAt":"2026-01-01T00:00:00Z","items":[{"id":"i1","name":"Free hardware","price":0,"cost":100,"markup":0,"quantity":1}]}`},
	})
	report := runNovel(t, newNovelQuoteThinCmd, "--floor", "20", "--db", dbPath)
	if got := listLen(t, report, "lines"); got != 1 {
		t.Fatalf("lines = %d, want 1 (markup 0 is below the 20%% floor): %#v", got, report)
	}
}

// Absent numbers keep their documented fallbacks: no quantity counts one
// unit, no markup is computed from price and cost.
func TestR1AbsentNumbersKeepFallbacks(t *testing.T) {
	q := parseNovelQuote(json.RawMessage(`{"id":"q","items":[{"price":120,"cost":100}]}`))
	if got := q.Items[0].effectiveQuantity(); got != 1 {
		t.Fatalf("absent quantity = %v, want 1", got)
	}
	if got, ok := q.Items[0].effectiveMarkup(); !ok || got < 19.999 || got > 20.001 {
		t.Fatalf("absent markup = (%v, %v), want (20, true)", got, ok)
	}
	z := parseNovelQuote(json.RawMessage(`{"id":"q","items":[{"price":0,"cost":100,"quantity":0,"markup":0}]}`))
	if got := z.Items[0].effectiveQuantity(); got != 0 {
		t.Fatalf("explicit quantity 0 read as %v", got)
	}
	if got, ok := z.Items[0].effectiveMarkup(); !ok || got != 0 {
		t.Fatalf("explicit markup 0 read as (%v, %v)", got, ok)
	}
}

func TestR1ResolveCompanyExactIDBeatsAnotherCompanysExternalID(t *testing.T) {
	companies := []novelCompany{
		{ID: "5678", Name: "Alpha", ExternalIdentifier: "1234"},
		{ID: "1234", Name: "Beta", ExternalIdentifier: "9876"},
	}
	c, err := resolveCompany(companies, "1234")
	if err != nil || c.ID != "1234" {
		t.Fatalf("resolveCompany(1234) = %+v, %v; want Beta (internal id 1234)", c, err)
	}
	// The external identifier still resolves when no internal id matches.
	if c, err := resolveCompany(companies, "9876"); err != nil || c.ID != "1234" {
		t.Fatalf("resolveCompany(9876) = %+v, %v; want Beta by external identifier", c, err)
	}
}

func TestR1ResolveCompanyRefusesAmbiguousNamespaces(t *testing.T) {
	companies := []novelCompany{
		{ID: "c1", Name: "Acme", ExternalIdentifier: "EXT-1"},
		{ID: "c2", Name: "acme", ExternalIdentifier: "EXT-1"},
	}
	if c, err := resolveCompany(companies, "EXT-1"); err == nil {
		t.Fatalf("a shared external identifier resolved to %+v; want an ambiguity error", c)
	}
	if c, err := resolveCompany(companies, "Acme"); err == nil {
		t.Fatalf("a shared name resolved to %+v; want an ambiguity error", c)
	}
	if c, err := resolveCompany(companies, "c2"); err != nil || c.ID != "c2" {
		t.Fatalf("exact id c2 = %+v, %v", c, err)
	}
}

func TestR1WhitespaceCommandUsesExactInternalID(t *testing.T) {
	dbPath := seedR1Store(t, [][3]string{
		{"company", "5678", `{"id":"5678","name":"Alpha","externalIdentifier":"1234"}`},
		{"company", "1234", `{"id":"1234","name":"Beta","externalIdentifier":"9876"}`},
		{"product", "p1", `{"id":"p1","name":"Widget","price":10}`},
	})
	flags := &rootFlags{asJSON: true}
	cmd := newNovelCompanyWhitespaceCmd(flags)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"1234", "--db", dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var report map[string]any
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	if report["companyId"] != "1234" {
		t.Fatalf("whitespace ran for company %v, want 1234 (Beta)", report["companyId"])
	}
}
