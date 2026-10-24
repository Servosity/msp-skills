// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// TestDetectCippResultsFailureShapes locks the payload shapes CIPP actually
// returns. The bare-string case is Invoke-AddScheduledItem; the array case is
// Invoke-EditUser.
func TestDetectCippResultsFailureShapes(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantFail bool
	}{
		{"array with failure", `{"Results":["Could not remove license for user@example.com"]}`, true},
		{"array all success", `{"Results":["Successfully created user","Assigned license"]}`, false},
		{"bare string failure", `{"Results":"Failed to schedule item"}`, true},
		{"bare string success", `{"Results":"Successfully scheduled item"}`, false},
		{"mixed success and failure", `{"Results":["Successfully created user","Unable to assign license"]}`, true},
		{"absent Results", `{"Data":[1,2,3]}`, false},
		{"empty body", ``, false},
		{"not json", `<html>challenge</html>`, false},
		{"null Results", `{"Results":null}`, false},
		{"lowercase results key", `{"results":["Error: mailbox does not exist"]}`, true},
		{"already exists", `{"Results":["Group already exists"]}`, true},
		{"not permitted", `{"Results":["Not permitted to perform this action"]}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectCippResultsFailure([]byte(c.body))
			if c.wantFail && got == nil {
				t.Fatalf("expected a partial-failure report for %s, got nil", c.body)
			}
			if !c.wantFail && got != nil {
				t.Fatalf("expected no partial-failure report for %s, got %+v", c.body, got)
			}
			if got != nil && got.Field != "Results" {
				t.Errorf("Field = %q, want %q", got.Field, "Results")
			}
		})
	}
}

// TestDetectCippResultsFailureIgnoresListPayloads is the regression guard that
// keeps read paths working. ListTenants legitimately returns
// {"Results":[{...tenant objects...}]}; flagging those would make every
// fan-out read look like a failed write. Only failure PROSE counts.
func TestDetectCippResultsFailureIgnoresListPayloads(t *testing.T) {
	body := `{"Results":[
		{"customerId":"aaa","defaultDomainName":"contoso.onmicrosoft.com","displayName":"Contoso"},
		{"customerId":"bbb","defaultDomainName":"fabrikam.onmicrosoft.com","displayName":"Fabrikam"}
	]}`
	if got := detectCippResultsFailure([]byte(body)); got != nil {
		t.Fatalf("tenant list payload must not be flagged as a failure, got %+v", got)
	}
}

// TestDetectPartialFailureDelegatesToCipp asserts the one-line hand-fix inside
// the generated detectPartialFailure() is present and reached. Without it the
// generated detector only matches Google Ads' partialFailureError object and a
// failed CIPP write exits 0.
func TestDetectPartialFailureDelegatesToCipp(t *testing.T) {
	report := detectPartialFailure([]byte(`{"Results":["Could not convert mailbox"]}`))
	if report == nil {
		t.Fatal("detectPartialFailure did not delegate to the CIPP Results detector")
	}
	if !strings.Contains(report.Message, "Could not convert mailbox") {
		t.Errorf("report message lost the failure text: %q", report.Message)
	}
}

// TestInterpretCippResultsCountsEntries asserts every entry is preserved in
// Details, not just the failing ones, so callers can see which operations
// in a batch did succeed.
func TestInterpretCippResultsCountsEntries(t *testing.T) {
	results, failures, _ := interpretCippResults([]any{
		"Successfully created user",
		"Could not assign license",
		"Successfully revoked sessions",
	})
	if len(results) != 3 {
		t.Errorf("results = %d entries, want 3", len(results))
	}
	if len(failures) != 1 {
		t.Errorf("failures = %d entries, want 1", len(failures))
	}
}
