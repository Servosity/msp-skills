// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func agreementRows(t *testing.T, csv string, tree []gzCompany, cur map[string]usageCounts) []billingRow {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agreements.csv")
	if err := os.WriteFile(p, []byte(csv), 0o600); err != nil {
		t.Fatal(err)
	}
	agreed, err := readAgreements(p)
	if err != nil {
		t.Fatal(err)
	}
	return buildBillingRows(tree, cur, cur, nil, nil, agreed, true)
}

// The print review's input: one company listed once by id and once by name.
// Both rows are its agreement, so they add up (as duplicate rows already do).
func TestAgreementRowsByIDAndNameAddUp(t *testing.T) {
	tree := []gzCompany{{ID: "aaaaaaaaaaaaaaaaaaaaaaa2", Name: "Acme", Type: 1}}
	cur := map[string]usageCounts{"aaaaaaaaaaaaaaaaaaaaaaa2": {0: {"endpointMonthlyUsage": 12}}}
	rows := agreementRows(t, "company,quantity\naaaaaaaaaaaaaaaaaaaaaaa2,5\nAcme,7\n", tree, cur)
	if len(rows) != 1 || rows[0].AgreementQty == nil || *rows[0].AgreementQty != 12 {
		t.Fatalf("rows = %+v, want agreement_quantity 12 (5 by id + 7 by name)", rows)
	}
	if f := strings.Join(rows[0].Flags, ","); strings.Contains(f, "agreement_mismatch") {
		t.Fatalf("flags = %q: 12 billed against 12 agreed is not a mismatch", f)
	}
}

// A name two companies share cannot say which one it means: it is applied to
// neither, and both rows say why instead of each claiming the whole quantity.
func TestAgreementNameSharedByTwoCompaniesIsNotApplied(t *testing.T) {
	tree := []gzCompany{{ID: "c1", Name: "Acme", Type: 1}, {ID: "c2", Name: "acme", Type: 1}}
	cur := map[string]usageCounts{"c1": {0: {"endpointMonthlyUsage": 7}}, "c2": {0: {"endpointMonthlyUsage": 3}}}
	rows := agreementRows(t, "company,quantity\nAcme,7\nc2,3\n", tree, cur)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want two", rows)
	}
	for _, r := range rows {
		f := strings.Join(r.Flags, ",")
		if !strings.Contains(f, "agreement_name_ambiguous") {
			t.Fatalf("%s flags = %q, want agreement_name_ambiguous", r.CompanyID, f)
		}
		if r.CompanyID == "c1" && r.AgreementQty != nil {
			t.Fatalf("c1 agreement = %d, want none applied from an ambiguous name", *r.AgreementQty)
		}
		if r.CompanyID == "c2" && (r.AgreementQty == nil || *r.AgreementQty != 3) {
			t.Fatalf("c2 agreement = %v, want 3 from its id row only", r.AgreementQty)
		}
	}
}
