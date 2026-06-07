// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVendorCasePageDecoding(t *testing.T) {
	// vendorCaseId arrives as a JSON number; json.Number must preserve it
	// without scientific-notation corruption.
	payload := `{"vendorCases":[{"vendorCaseId":123456789012,"vendorDomain":"Acme-Supplies.com","firstObservedTime":"2026-06-01T00:00:00Z"}],"nextPageNumber":2}`
	var pageDoc struct {
		VendorCases    []vendorCaseRow `json:"vendorCases"`
		NextPageNumber *int            `json:"nextPageNumber"`
	}
	if err := json.Unmarshal([]byte(payload), &pageDoc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(pageDoc.VendorCases) != 1 {
		t.Fatalf("expected 1 vendor case, got %d", len(pageDoc.VendorCases))
	}
	vc := pageDoc.VendorCases[0]
	if vc.VendorCaseID.String() != "123456789012" {
		t.Errorf("vendorCaseId mangled: %s", vc.VendorCaseID.String())
	}
	if pageDoc.NextPageNumber == nil || *pageDoc.NextPageNumber != 2 {
		t.Errorf("nextPageNumber lost: %v", pageDoc.NextPageNumber)
	}
}

func TestVendorRiskViewEmptyCollectionsMarshalAsArrays(t *testing.T) {
	view := vendorRiskView{
		VendorDomain:  "acme.com",
		OpenCases:     make([]vendorCaseRow, 0),
		FetchFailures: make([]vendorRiskFailure, 0),
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !json.Valid(data) {
		t.Fatalf("invalid json: %s", s)
	}
	if want := `"open_cases":[]`; !strings.Contains(s, want) {
		t.Errorf("open_cases should marshal as [], got %s", s)
	}
}
