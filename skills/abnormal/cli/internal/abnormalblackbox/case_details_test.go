// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package abnormalblackbox

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Codex r1 class wrong-field-read-from-a-documented-response: the case lists
// answer list rows only (spec.json AbnormalCase, VendorCases); employee-risk
// and vendor-risk must match on each case's detail (AbnormalCaseDetails,
// VendorCaseDetails), and a detail that cannot be read is reported.
func TestRiskNovelsMatchOnTheCaseDetail(t *testing.T) {
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimPrefix(r.URL.Path, "/v1")
		switch {
		case strings.HasSuffix(p, "/logins"):
			w.Header().Set("Content-Type", "text/csv")
			fmt.Fprint(w, "timestamp,ip address,country or region,status\n2026-10-27T10:00:00Z,203.0.113.5,US,success\n")
		case strings.HasPrefix(p, "/employee/"):
			fmt.Fprint(w, `{"name":"First Last","email":"first.last@example.com"}`)
		case p == "/cases":
			fmt.Fprint(w, `{"cases":[{"caseId":"1234","severity_level":"HIGH"},{"caseId":5678},{"caseId":"91"}],"pageNumber":1}`)
		case p == "/cases/91":
			fmt.Fprint(w, `{}`)
		case p == "/cases/1234":
			fmt.Fprint(w, `{"caseId":"1234","case_status":"Action Required","severity":"Potential Account Takeover","affectedEmployee":"First Last","firstObserved":"2026-10-26T09:00:00Z"}`)
		case p == "/cases/5678":
			fmt.Fprint(w, `{"caseId":5678,"case_status":"Acknowledged (Attack resolved)","severity":"Potential Account Takeover","affectedEmployee":"First Last"}`)
		case strings.HasPrefix(p, "/vendors/"):
			fmt.Fprint(w, `{"vendorDomain":"supplier.example.com"}`)
		case p == "/vendor-cases":
			fmt.Fprint(w, `{"vendorCases":[{"vendorCaseId":7},{"vendorCaseId":8},{"vendorCaseId":9},{"vendorCaseId":10}],"pageNumber":1}`)
		case p == "/vendor-cases/10":
			fmt.Fprint(w, `null`)
		case p == "/vendor-cases/7":
			fmt.Fprint(w, `{"vendorCaseId":7,"vendorDomain":"supplier.example.com","firstObservedTime":"2026-10-20T00:00:00Z"}`)
		case p == "/vendor-cases/9":
			fmt.Fprint(w, `{"vendorCaseId":9,"vendorDomain":"other.example.net"}`)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"boom"}`)
		}
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, &server{headers: map[string]int{}, url: srv.URL + "/v1", caFile: ca}, credEnv)

	var emp struct {
		OpenCases []struct {
			CaseID string `json:"caseId"`
			Status string `json:"case_status"`
		} `json:"open_cases"`
		ScannedCases  int `json:"scanned_cases"`
		FetchFailures []struct {
			Source string `json:"source"`
		} `json:"fetch_failures"`
	}
	out := e.run(t, "employee-risk", "first.last@example.com", "--json", "--no-learn")
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&emp); err != nil {
		t.Fatalf("employee-risk: %v\n%s", err, out)
	}
	if len(emp.FetchFailures) != 1 || !strings.Contains(emp.FetchFailures[0].Source, "/cases/91") {
		t.Errorf("an empty case detail must be a fetch failure, never no match: %+v", emp.FetchFailures)
	}
	if emp.ScannedCases != 3 || len(emp.OpenCases) != 1 || emp.OpenCases[0].CaseID != "1234" || emp.OpenCases[0].Status != "Action Required" {
		t.Errorf("employee-risk must keep the open case named in its detail (1234), not the resolved one: %+v\n%s", emp, out)
	}

	var ven struct {
		OpenCases []struct {
			ID json.Number `json:"vendorCaseId"`
		} `json:"open_cases"`
		FetchFailures []struct {
			Source string `json:"source"`
		} `json:"fetch_failures"`
		Note string `json:"note"`
	}
	out = e.run(t, "vendor-risk", "supplier.example.com", "--json", "--no-learn")
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&ven); err != nil {
		t.Fatalf("vendor-risk: %v\n%s", err, out)
	}
	if len(ven.OpenCases) != 1 || ven.OpenCases[0].ID.String() != "7" {
		t.Errorf("vendor-risk must keep the case whose detail names the domain (7): %+v\n%s", ven, out)
	}
	srcs := ""
	for _, f := range ven.FetchFailures {
		srcs += f.Source + " "
	}
	if len(ven.FetchFailures) != 2 || !strings.Contains(srcs, "/vendor-cases/8") || !strings.Contains(srcs, "/vendor-cases/10") || !strings.Contains(ven.Note, "incomplete") {
		t.Errorf("an unreadable case detail must be reported, never read as no match: %+v", ven)
	}
}
