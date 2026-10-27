// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: class "a write acts on a stale local snapshot". sync pulls
// approvals with statusId=1 only, so a request a technician approves or
// ignores upstream stops coming back, and a plain resync leaves its local row
// marked pending. approve-batch must re-confirm every target against the live
// API, under that target's own tenant, before it writes, and approve only the
// requests the live API still reports pending with the requested hash. Any
// other answer skips the target and is never approved.
//
// Driven through the real command tree (sync, then approve-batch) against a
// local fixture shaped like the adversarial reviewer's stub: three tenants,
// six pending requests for one file hash. No tenant data, no real credentials.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"threatlocker-pp-cli/internal/cliutil/testenv"
	"threatlocker-pp-cli/internal/store"
)

const (
	tlStaleOrgA     = "aaaaaaaa-0000-4000-8000-00000000000a"
	tlStaleOrgB     = "bbbbbbbb-0000-4000-8000-00000000000b"
	tlStaleOrgC     = "cccccccc-0000-4000-8000-00000000000c"
	tlStaleGetByID  = "/ApprovalRequest/ApprovalRequestGetById"
	tlStaleList     = "/ApprovalRequest/ApprovalRequestGetByParameters"
	tlStalePermit   = "/ApprovalRequest/ApprovalRequestPermitApplication"
	tlStaleHashText = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
)

var tlStaleOrgNames = map[string]string{tlStaleOrgA: "Tenant A", tlStaleOrgB: "Tenant B", tlStaleOrgC: "Tenant C"}

// tlAnswerRecheckPending wraps a recorder-fixture handler so the live
// re-check approve-batch makes before each write (GetById) is answered with a
// pending row for the fixture hash "aaaa"; every other path is unchanged.
func tlAnswerRecheckPending(next func(path string, body map[string]any) (int, any)) func(path string, body map[string]any) (int, any) {
	return func(path string, body map[string]any) (int, any) {
		if strings.HasSuffix(path, tlStaleGetByID) {
			return 200, map[string]any{"statusId": 1, "status": "Pending", "hash": "aaaa"}
		}
		return next(path, body)
	}
}

type tlLiveRequest struct {
	Method, Path, RequestID, Org, OverrideOrg string
	Body                                      map[string]any
}

// tlLiveStub is a fixture ThreatLocker Portal API holding the vendor's live
// approval rows. The list endpoint honours statusId, as the real one does, so
// a request that leaves Pending stops coming back to sync. GetById answers
// only under the row's own tenant header (a wrong tenant is a 404). A permit
// write moves the matching pending rows to Approved.
type tlLiveStub struct {
	mu   sync.Mutex
	rows []map[string]any
	reqs []tlLiveRequest
	// getByID, when set, may replace the honest GetById answer for a row
	// found under the right tenant; ok=false keeps the honest answer.
	getByID func(row map[string]any) (status int, body []byte, ok bool)
	// permit, when set, may replace the permit answer for a tenant.
	permit func(org string) (status int, body any, ok bool)
}

func newTLLiveStub(t *testing.T) (*tlLiveStub, string) {
	t.Helper()
	stub := &tlLiveStub{}
	for i := 0; i < 6; i++ {
		org := []string{tlStaleOrgA, tlStaleOrgB, tlStaleOrgC}[i%3]
		stub.rows = append(stub.rows, map[string]any{
			"approvalRequestId": fmt.Sprintf("d0000000-0000-4000-8000-%012d", i),
			"organizationId":    org,
			"organizationName":  tlStaleOrgNames[org],
			"computerId":        fmt.Sprintf("c0000000-0000-4000-8000-%012d", i),
			"computerName":      fmt.Sprintf("PC-%03d", i),
			"fileName":          "setup.exe",
			"fullPath":          `c:\setup.exe`,
			"hash":              tlStaleHashText,
			"statusId":          1,
			"status":            "Pending",
			"dateRequested":     "2026-10-26T09:00:00Z",
			"requestedBy":       "u",
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(stub.serve))
	t.Cleanup(srv.Close)
	return stub, srv.URL
}

func tlStubInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	}
	return 0, false
}

func (s *tlLiveStub) serve(w http.ResponseWriter, req *http.Request) {
	raw, _ := io.ReadAll(req.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	s.mu.Lock()
	defer s.mu.Unlock()
	org := req.Header.Get(TenantOrgHeader)
	s.reqs = append(s.reqs, tlLiveRequest{req.Method, req.URL.Path, req.URL.Query().Get("approvalRequestId"), org, req.Header.Get(OverrideTenantOrgHeader), body})
	writeJSON := func(status int, v any) {
		b, _ := json.Marshal(v)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}
	if req.Header.Get("Authorization") != tlFixtureToken {
		writeJSON(401, map[string]any{"message": "Unauthorized"})
		return
	}
	switch {
	case req.Method == http.MethodPost && req.URL.Path == tlStaleList:
		want := 1
		if n, ok := tlStubInt(body["statusId"]); ok {
			want = n
		}
		page, size := 1, 100
		if n, ok := tlStubInt(body["pageNumber"]); ok && n > 0 {
			page = n
		}
		if n, ok := tlStubInt(body["pageSize"]); ok && n > 0 {
			size = n
		}
		matched := []map[string]any{}
		for _, r := range s.rows {
			if n, _ := tlStubInt(r["statusId"]); n == want {
				matched = append(matched, r)
			}
		}
		lo, hi := (page-1)*size, page*size
		if lo > len(matched) {
			lo = len(matched)
		}
		if hi > len(matched) {
			hi = len(matched)
		}
		writeJSON(200, matched[lo:hi])
	case req.Method == http.MethodGet && req.URL.Path == tlStaleGetByID:
		id := req.URL.Query().Get("approvalRequestId")
		for _, r := range s.rows {
			if r["approvalRequestId"] != id || r["organizationId"] != org {
				continue
			}
			if s.getByID != nil {
				if status, b, ok := s.getByID(r); ok {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = w.Write(b)
					return
				}
			}
			writeJSON(200, r)
			return
		}
		writeJSON(404, map[string]any{"message": "Approval request not found"})
	case req.Method == http.MethodPost && req.URL.Path == tlStalePermit:
		bodyOrg, _ := body["organizationId"].(string)
		if s.permit != nil {
			if status, out, ok := s.permit(bodyOrg); ok {
				writeJSON(status, out)
				return
			}
		}
		for _, r := range s.rows {
			if n, _ := tlStubInt(r["statusId"]); n == 1 && r["organizationId"] == bodyOrg && r["computerId"] == body["computerId"] {
				r["statusId"], r["status"] = 4, "Approved"
			}
		}
		writeJSON(200, map[string]any{"success": true, "message": "ok"})
	default:
		writeJSON(200, []any{})
	}
}

// ignore marks every request in org Ignored upstream (statusId 10), as a
// technician acting in the portal would.
func (s *tlLiveStub) ignore(org string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rows {
		if r["organizationId"] == org {
			r["statusId"], r["status"] = 10, "Ignored"
		}
	}
}

func (s *tlLiveStub) requests() []tlLiveRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]tlLiveRequest(nil), s.reqs...)
}

func (s *tlLiveStub) rowByID(id string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rows {
		if r["approvalRequestId"] == id {
			return r
		}
	}
	return nil
}

// permitsByTenant counts permit writes per tenant, failing the test when a
// write's tenant header disagrees with the tenant its body names.
func (s *tlLiveStub) permitsByTenant(t *testing.T, since int) map[string]int {
	t.Helper()
	got := map[string]int{}
	for _, r := range s.requests()[since:] {
		if r.Path != tlStalePermit {
			continue
		}
		if r.Org != r.Body["organizationId"] || r.OverrideOrg != r.Org {
			t.Fatalf("permit for %v sent under tenant headers %q / %q", r.Body["organizationId"], r.Org, r.OverrideOrg)
		}
		got[r.Org]++
	}
	return got
}

// assertRecheckedBeforeWrite proves every permit write was preceded, in the
// same run, by a GetById for that very request under that request's tenant.
func (s *tlLiveStub) assertRecheckedBeforeWrite(t *testing.T, since int) {
	t.Helper()
	confirmed := map[string]bool{} // org|computerId
	for _, r := range s.requests()[since:] {
		switch r.Path {
		case tlStaleGetByID:
			row := s.rowByID(r.RequestID)
			if row == nil {
				t.Fatalf("GetById for unknown request %q", r.RequestID)
			}
			if r.Org != row["organizationId"] || r.OverrideOrg != row["organizationId"] {
				t.Fatalf("GetById for %s (tenant %v) sent under tenant headers %q / %q", r.RequestID, row["organizationId"], r.Org, r.OverrideOrg)
			}
			confirmed[fmt.Sprint(row["organizationId"], "|", row["computerId"])] = true
		case tlStalePermit:
			key := fmt.Sprint(r.Body["organizationId"], "|", r.Body["computerId"])
			if !confirmed[key] {
				t.Fatalf("permit write for %s was not preceded by a live GetById re-check of that request", key)
			}
		}
	}
}

// tlLiveRunner isolates the user directories once and returns a runner that
// executes the real command tree against baseURL, so successive runs share
// one home (one local store, one response cache) the way an operator's do.
// --rate-limit 0 only drops the client's 2 req/s warm-up pacing, which would
// otherwise cost seconds per run against a local fixture.
func tlLiveRunner(t *testing.T, baseURL string) func(args ...string) (string, error) {
	t.Helper()
	testenv.Isolate(t)
	t.Setenv("THREATLOCKER_API_KEY", tlFixtureToken)
	t.Setenv(TenantOrgEnvVar, "")
	t.Setenv("THREATLOCKER_BASE_URL", baseURL)
	return func(args ...string) (string, error) {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append(append([]string(nil), args...), "--rate-limit", "0"))
		err := cmd.ExecuteContext(context.Background())
		return out.String(), err
	}
}

func tlSyncApprovals(t *testing.T, run func(args ...string) (string, error), dbPath string) {
	t.Helper()
	if out, err := run("sync", "--resources", "approvals", "--db", dbPath); err != nil {
		t.Fatalf("sync --resources approvals: %v\n%s", err, out)
	}
}

// tlLocalPending counts the rows the local store still marks pending for the
// stale-snapshot hash, per tenant.
func tlLocalPending(t *testing.T, dbPath string) map[string]int {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.DB().Query(`SELECT organization_id, count(*) FROM approvals WHERE status_id = 1 AND lower(hash) = ? GROUP BY organization_id`, tlStaleHashText)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var org string
		var n int
		if err := rows.Scan(&org, &n); err != nil {
			t.Fatal(err)
		}
		got[org] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

type tlBatchReport struct {
	Approved          int `json:"approved"`
	SkippedNotPending int `json:"skipped_not_pending"`
	Failed            int `json:"failed"`
	Total             int `json:"total"`
	Results           []struct {
		ApprovalRequestID string `json:"approvalRequestId"`
		OrganizationID    string `json:"organizationId"`
		Approved          bool   `json:"approved"`
		Outcome           string `json:"outcome"`
		Reason            string `json:"reason"`
		Error             string `json:"error"`
	} `json:"results"`
}

func decodeTLBatchReport(t *testing.T, out string) tlBatchReport {
	t.Helper()
	var r tlBatchReport
	if err := json.NewDecoder(strings.NewReader(out)).Decode(&r); err != nil {
		t.Fatalf("decode approve-batch --json output: %v\n%s", err, out)
	}
	return r
}

// The reported failing input: six pending requests synced across tenants A,
// B and C; the vendor then marks B's two requests Ignored; approve-batch
// --all-tenants --yes. B must receive zero permit writes, and A and C must
// still be approved, each only after a live re-check under its own tenant.
// "stale since the last sync" keeps the snapshot stale no matter how sync
// evolves; "after a plain resync" is the exact sequence the reviewer ran.
func TestApproveBatchSkipsRequestsNoLongerPendingUpstream(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resync   bool
		wantSkip int // -1: whatever the plain resync left pending locally for B
	}{
		{"stale since the last sync", false, 2},
		{"after a plain resync", true, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub, base := newTLLiveStub(t)
			run := tlLiveRunner(t, base)
			dbPath := filepath.Join(t.TempDir(), "tl.db")
			tlSyncApprovals(t, run, dbPath)
			stub.ignore(tlStaleOrgB)
			if tc.resync {
				tlSyncApprovals(t, run, dbPath)
			}
			local := tlLocalPending(t, dbPath)
			wantSkip := tc.wantSkip
			if wantSkip < 0 {
				wantSkip = local[tlStaleOrgB]
				t.Logf("after a plain resync the local store still marks %d of B's requests pending", wantSkip)
			}
			if local[tlStaleOrgA] != 2 || local[tlStaleOrgC] != 2 || local[tlStaleOrgB] != wantSkip {
				t.Fatalf("precondition: local pending rows per tenant = %v", local)
			}

			since := len(stub.requests())
			out, err := run("approvals", "approve-batch", "--hash", tlStaleHashText, "--all-tenants", "--yes", "--json", "--db", dbPath)
			if err != nil {
				t.Fatalf("approve-batch with only not-pending skips must exit 0: %v\n%s", err, out)
			}
			got := stub.permitsByTenant(t, since)
			if got[tlStaleOrgB] != 0 {
				t.Fatalf("approve-batch sent %d permit write(s) under tenant B, whose requests were Ignored upstream\n%s", got[tlStaleOrgB], out)
			}
			if got[tlStaleOrgA] != 2 || got[tlStaleOrgC] != 2 || len(got) != 2 {
				t.Fatalf("permit writes per tenant = %v, want A:2 and C:2\n%s", got, out)
			}
			stub.assertRecheckedBeforeWrite(t, since)

			rep := decodeTLBatchReport(t, out)
			if rep.Approved != 4 || rep.SkippedNotPending != wantSkip || rep.Failed != 0 || rep.Total != 4+wantSkip {
				t.Fatalf("report approved=%d skipped_not_pending=%d failed=%d total=%d, want 4/%d/0/%d\n%s", rep.Approved, rep.SkippedNotPending, rep.Failed, rep.Total, wantSkip, 4+wantSkip, out)
			}
			for _, r := range rep.Results {
				if r.OrganizationID != tlStaleOrgB {
					continue
				}
				if r.Approved || r.Outcome != "skipped_not_pending" || !strings.Contains(r.Reason, "pending") {
					t.Fatalf("tenant B result %+v, want an unapproved skipped_not_pending with a reason\n%s", r, out)
				}
			}
		})
	}

	// Text output names every skipped request and still exits 0.
	t.Run("text output", func(t *testing.T) {
		stub, base := newTLLiveStub(t)
		run := tlLiveRunner(t, base)
		dbPath := filepath.Join(t.TempDir(), "tl.db")
		tlSyncApprovals(t, run, dbPath)
		stub.ignore(tlStaleOrgB)
		since := len(stub.requests())
		out, err := run("approvals", "approve-batch", "--hash", tlStaleHashText, "--all-tenants", "--yes", "--db", dbPath)
		if err != nil {
			t.Fatalf("approve-batch: %v\n%s", err, out)
		}
		if got := stub.permitsByTenant(t, since); got[tlStaleOrgB] != 0 {
			t.Fatalf("text run sent %d permit write(s) under tenant B\n%s", got[tlStaleOrgB], out)
		}
		for _, id := range []string{"d0000000-0000-4000-8000-000000000001", "d0000000-0000-4000-8000-000000000004"} {
			if !strings.Contains(out, id) {
				t.Fatalf("text output does not name skipped request %s:\n%s", id, out)
			}
		}
		if !strings.Contains(out, "no longer pending") {
			t.Fatalf("text output does not say why B was skipped:\n%s", out)
		}
	})
}

// A re-check that does not positively confirm "pending, this hash, this
// request, this tenant" approves nothing for that target: an error, a 404, an
// unparsable or ambiguous body, a missing statusId, or a row that is not the
// one asked for. A and C still go through, so the guard is not a blanket
// refusal, and the run exits non-zero because a target was skipped for an
// error rather than merely being no longer pending.
func TestApproveBatchUnconfirmedRecheckApprovesNothing(t *testing.T) {
	rowJSON := func(row map[string]any, edit func(map[string]any)) []byte {
		cp := map[string]any{}
		for k, v := range row {
			cp[k] = v
		}
		if edit != nil {
			edit(cp)
		}
		b, _ := json.Marshal(cp)
		return b
	}
	for _, tc := range []struct {
		name   string
		answer func(row map[string]any) (int, []byte)
	}{
		{"404", func(map[string]any) (int, []byte) { return 404, []byte(`{"message":"Not Found"}`) }},
		{"403", func(map[string]any) (int, []byte) { return 403, []byte(`{"message":"Forbidden"}`) }},
		{"missing statusId", func(r map[string]any) (int, []byte) {
			return 200, rowJSON(r, func(m map[string]any) { delete(m, "statusId") })
		}},
		{"null statusId", func(r map[string]any) (int, []byte) {
			return 200, rowJSON(r, func(m map[string]any) { m["statusId"] = nil })
		}},
		{"string statusId", func(r map[string]any) (int, []byte) {
			return 200, rowJSON(r, func(m map[string]any) { m["statusId"] = "1" })
		}},
		{"fractional statusId", func(r map[string]any) (int, []byte) {
			return 200, rowJSON(r, func(m map[string]any) { m["statusId"] = 1.5 })
		}},
		{"duplicate statusId", func(r map[string]any) (int, []byte) {
			b := rowJSON(r, func(m map[string]any) { delete(m, "statusId") })
			return 200, append([]byte(`{"statusId":10,`), append(b[1:len(b)-1], []byte(`,"statusId":1}`)...)...)
		}},
		{"case-variant duplicate statusId", func(r map[string]any) (int, []byte) {
			b := rowJSON(r, func(m map[string]any) { m["statusId"] = 10 })
			return 200, append(b[:len(b)-1], []byte(`,"StatusId":1}`)...)
		}},
		{"unparsable body", func(map[string]any) (int, []byte) { return 200, []byte(`<html>gateway</html>`) }},
		{"empty body", func(map[string]any) (int, []byte) { return 200, nil }},
		{"array body", func(r map[string]any) (int, []byte) {
			return 200, append(append([]byte(`[`), rowJSON(r, nil)...), ']')
		}},
		{"trailing data", func(r map[string]any) (int, []byte) { return 200, append(rowJSON(r, nil), []byte(`{}`)...) }},
		{"in-band failure", func(map[string]any) (int, []byte) {
			return 200, []byte(`{"success":false,"message":"no access"}`)
		}},
		{"another request's row", func(r map[string]any) (int, []byte) {
			return 200, rowJSON(r, func(m map[string]any) { m["approvalRequestId"] = "d0000000-0000-4000-8000-999999999999" })
		}},
		{"another tenant's row", func(r map[string]any) (int, []byte) {
			return 200, rowJSON(r, func(m map[string]any) { m["organizationId"] = tlStaleOrgA })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub, base := newTLLiveStub(t)
			stub.getByID = func(row map[string]any) (int, []byte, bool) {
				if row["organizationId"] != tlStaleOrgB {
					return 0, nil, false
				}
				status, b := tc.answer(row)
				return status, b, true
			}
			run := tlLiveRunner(t, base)
			dbPath := filepath.Join(t.TempDir(), "tl.db")
			tlSyncApprovals(t, run, dbPath)
			since := len(stub.requests())
			out, err := run("approvals", "approve-batch", "--hash", tlStaleHashText, "--all-tenants", "--yes", "--json", "--db", dbPath)
			if err == nil {
				t.Fatalf("approve-batch exited 0 although B's re-check was %s\n%s", tc.name, out)
			}
			got := stub.permitsByTenant(t, since)
			if got[tlStaleOrgB] != 0 {
				t.Fatalf("approve-batch sent %d permit write(s) under tenant B on a %s re-check\n%s", got[tlStaleOrgB], tc.name, out)
			}
			if got[tlStaleOrgA] != 2 || got[tlStaleOrgC] != 2 {
				t.Fatalf("permit writes per tenant = %v, want A:2 and C:2 (only B is unconfirmed)\n%s", got, out)
			}
			stub.assertRecheckedBeforeWrite(t, since)
			rep := decodeTLBatchReport(t, out)
			if rep.Approved != 4 || rep.SkippedNotPending != 0 || rep.Failed != 2 || rep.Total != 6 {
				t.Fatalf("report approved=%d skipped_not_pending=%d failed=%d total=%d, want 4/0/2/6\n%s", rep.Approved, rep.SkippedNotPending, rep.Failed, rep.Total, out)
			}
			for _, r := range rep.Results {
				if r.OrganizationID == tlStaleOrgB && (r.Approved || r.Outcome == "approved" || r.Outcome == "skipped_not_pending" || r.Reason == "" || r.Error == "") {
					t.Fatalf("tenant B result %+v, want unapproved, counted failed, with a reason\n%s", r, out)
				}
			}
		})
	}

	// Text output names each unconfirmed request too.
	t.Run("text output", func(t *testing.T) {
		stub, base := newTLLiveStub(t)
		stub.getByID = func(row map[string]any) (int, []byte, bool) {
			if row["organizationId"] != tlStaleOrgB {
				return 0, nil, false
			}
			return 404, []byte(`{"message":"Not Found"}`), true
		}
		run := tlLiveRunner(t, base)
		dbPath := filepath.Join(t.TempDir(), "tl.db")
		tlSyncApprovals(t, run, dbPath)
		out, err := run("approvals", "approve-batch", "--hash", tlStaleHashText, "--all-tenants", "--yes", "--db", dbPath)
		if err == nil {
			t.Fatalf("text run exited 0 although B's re-check failed\n%s", out)
		}
		for _, id := range []string{"d0000000-0000-4000-8000-000000000001", "d0000000-0000-4000-8000-000000000004"} {
			if !strings.Contains(out, id) {
				t.Fatalf("text output does not name unconfirmed request %s:\n%s", id, out)
			}
		}
	})
}

// The live row must carry the requested hash. A different, absent, null or
// non-string hash approves nothing for that target; hex case is not a
// difference, so an upper-case live hash still matches.
func TestApproveBatchLiveHashMismatchApprovesNothing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		edit    func(m map[string]any)
		matches bool
	}{
		{"different hash", func(m map[string]any) { m["hash"] = strings.Repeat("d", 64) }, false},
		{"missing hash", func(m map[string]any) { delete(m, "hash") }, false},
		{"null hash", func(m map[string]any) { m["hash"] = nil }, false},
		{"numeric hash", func(m map[string]any) { m["hash"] = 7 }, false},
		{"upper-case hash", func(m map[string]any) { m["hash"] = strings.ToUpper(tlStaleHashText) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub, base := newTLLiveStub(t)
			stub.getByID = func(row map[string]any) (int, []byte, bool) {
				if row["organizationId"] != tlStaleOrgB {
					return 0, nil, false
				}
				cp := map[string]any{}
				for k, v := range row {
					cp[k] = v
				}
				tc.edit(cp)
				b, _ := json.Marshal(cp)
				return 200, b, true
			}
			run := tlLiveRunner(t, base)
			dbPath := filepath.Join(t.TempDir(), "tl.db")
			tlSyncApprovals(t, run, dbPath)
			since := len(stub.requests())
			out, err := run("approvals", "approve-batch", "--hash", tlStaleHashText, "--all-tenants", "--yes", "--json", "--db", dbPath)
			got := stub.permitsByTenant(t, since)
			rep := decodeTLBatchReport(t, out)
			stub.assertRecheckedBeforeWrite(t, since)
			if tc.matches {
				if err != nil || got[tlStaleOrgB] != 2 || rep.Approved != 6 || rep.Failed != 0 {
					t.Fatalf("%s: err=%v permits=%v approved=%d failed=%d, want all six approved\n%s", tc.name, err, got, rep.Approved, rep.Failed, out)
				}
				return
			}
			if err == nil {
				t.Fatalf("approve-batch exited 0 on a %s re-check\n%s", tc.name, out)
			}
			if got[tlStaleOrgB] != 0 {
				t.Fatalf("approve-batch sent %d permit write(s) under tenant B on a %s\n%s", got[tlStaleOrgB], tc.name, out)
			}
			if got[tlStaleOrgA] != 2 || got[tlStaleOrgC] != 2 || rep.Approved != 4 || rep.Failed != 2 {
				t.Fatalf("permits=%v approved=%d failed=%d, want A:2 C:2, 4 approved, 2 failed\n%s", got, rep.Approved, rep.Failed, out)
			}
		})
	}
}

// The re-check must read the live API, never the response cache: a cached
// "pending" from an earlier run is itself a stale snapshot. Run 1 re-checks
// B (pending) and its write is refused with HTTP 409, which leaves the cache
// standing; the vendor then marks B Ignored; run 2, in the same home, must
// see Ignored and send nothing.
func TestApproveBatchRecheckBypassesTheResponseCache(t *testing.T) {
	stub, base := newTLLiveStub(t)
	stub.permit = func(string) (int, any, bool) { return 409, map[string]any{"message": "conflict"}, true }
	run := tlLiveRunner(t, base)
	dbPath := filepath.Join(t.TempDir(), "tl.db")
	tlSyncApprovals(t, run, dbPath)
	if out, err := run("approvals", "approve-batch", "--hash", tlStaleHashText, "--org", tlStaleOrgB, "--yes", "--json", "--db", dbPath); err == nil {
		t.Fatalf("run 1 exited 0 although every write was refused\n%s", out)
	}
	stub.mu.Lock()
	stub.permit = nil
	stub.mu.Unlock()
	stub.ignore(tlStaleOrgB)
	since := len(stub.requests())
	out, err := run("approvals", "approve-batch", "--hash", tlStaleHashText, "--org", tlStaleOrgB, "--yes", "--json", "--db", dbPath)
	if got := stub.permitsByTenant(t, since); got[tlStaleOrgB] != 0 {
		t.Fatalf("run 2 sent %d permit write(s) for B's Ignored requests (re-check served from cache?)\n%s", got[tlStaleOrgB], out)
	}
	if err != nil {
		t.Fatalf("run 2 with only not-pending skips must exit 0: %v\n%s", err, out)
	}
	if rep := decodeTLBatchReport(t, out); rep.SkippedNotPending != 2 || rep.Approved != 0 {
		t.Fatalf("run 2 approved=%d skipped_not_pending=%d, want 0/2\n%s", rep.Approved, rep.SkippedNotPending, out)
	}
}

// --dry-run stays network-free and tells the operator the live run re-checks
// each request first; the Long help says the same.
func TestApproveBatchDryRunSaysTheLiveRunRechecks(t *testing.T) {
	stub, base := newTLLiveStub(t)
	run := tlLiveRunner(t, base)
	dbPath := filepath.Join(t.TempDir(), "tl.db")
	tlSyncApprovals(t, run, dbPath)
	since := len(stub.requests())

	out, err := run("approvals", "approve-batch", "--hash", tlStaleHashText, "--all-tenants", "--dry-run", "--db", dbPath)
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "re-checks each request") {
		t.Fatalf("dry-run text does not say the live run re-checks each request first:\n%s", out)
	}
	out, err = run("approvals", "approve-batch", "--hash", tlStaleHashText, "--all-tenants", "--dry-run", "--json", "--db", dbPath)
	if err != nil {
		t.Fatalf("dry-run --json: %v\n%s", err, out)
	}
	var plan struct {
		WouldApprove int    `json:"wouldApprove"`
		LiveRecheck  string `json:"liveRecheck"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode dry-run plan: %v\n%s", err, out)
	}
	if plan.WouldApprove != 6 || !strings.Contains(plan.LiveRecheck, "re-checks each request") {
		t.Fatalf("dry-run plan wouldApprove=%d liveRecheck=%q, want 6 and the re-check note\n%s", plan.WouldApprove, plan.LiveRecheck, out)
	}
	if n := len(stub.requests()) - since; n != 0 {
		t.Fatalf("dry-run sent %d request(s); it must stay network-free", n)
	}

	help, err := run("approvals", "approve-batch", "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(strings.Join(strings.Fields(help), " "), "re-checks each request") {
		t.Fatalf("Long help does not say the live run re-checks each request:\n%s", help)
	}
}
