// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
)

// TestNovelContainHelpWires smoke-tests that the contain command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelContainHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"contain", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("contain --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "contain"} {
		if !strings.Contains(help, want) {
			t.Fatalf("contain --help missing %q in output:\n%s", want, help)
		}
	}
}

type rqtContainOut struct {
	Mode       string            `json:"mode"`
	Email      string            `json:"email"`
	Resolution containResolution `json:"resolution"`
	Actions    []struct {
		Order           int               `json:"order"`
		Action          string            `json:"action"`
		Path            string            `json:"path"`
		Body            json.RawMessage   `json:"body"`
		PropagationNote string            `json:"propagation_note"`
		Status          string            `json:"status"`
		Submitted       *containSubmitted `json:"submitted"`
		JobMatch        *containJobMatch  `json:"job_match"`
		Error           string            `json:"error"`
	} `json:"actions"`
	PropagationNotes []string       `json:"propagation_notes"`
	Verified         bool           `json:"verified"`
	VerificationNote string         `json:"verification_note"`
	Summary          containSummary `json:"summary"`
}

func rqtContainActions(out rqtContainOut) []string {
	got := make([]string, 0, len(out.Actions))
	for _, a := range out.Actions {
		got = append(got, a.Action)
	}
	return got
}

func rqtFastPoll(t *testing.T) {
	t.Helper()
	prev := containPollInterval
	containPollInterval = time.Millisecond
	t.Cleanup(func() { containPollInterval = prev })
}

func TestContainPreviewOrdersActionsWithoutRequests(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	out, stderr, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "mfa,reset,block,force-change",
		"--org-id", "ORG_1", "--account-id", "ACC_JANE", "--json")
	if err != nil {
		t.Fatalf("contain preview: %v\nstderr: %s", err, stderr)
	}
	res := rqtDecode[rqtContainOut](t, out)
	want := "Block Sign In|Force User to Change Password on Next Sign In|Reset User Password|Setup User MFA"
	if got := strings.Join(rqtContainActions(res), "|"); got != want {
		t.Fatalf("action order = %s, want %s", got, want)
	}
	if res.Mode != "preview" || res.Resolution.Source != "flags" || res.Resolution.Inferred {
		t.Fatalf("preview/resolution wrong: %+v", res)
	}
	wantBody := `{"accounts":[{"application":"MS","organizationId":"ORG_1","account":{"id":"ACC_JANE","email":"jane.doe@example.com"}}],"action":"Block Sign In","contacts":null}`
	var compact bytes.Buffer
	if err := json.Compact(&compact, res.Actions[0].Body); err != nil || compact.String() != wantBody {
		t.Fatalf("execute body = %s\nwant %s", res.Actions[0].Body, wantBody)
	}
	if res.Actions[0].Path != "/respondApi/api/v1/response-actions/execute" || res.Actions[0].Order != 1 || res.Actions[0].Status != "planned" {
		t.Fatalf("first step wrong: %+v", res.Actions[0])
	}
	if !strings.Contains(strings.Join(res.PropagationNotes, " "), "24 hours") {
		t.Fatalf("Block Sign In propagation note missing: %v", res.PropagationNotes)
	}
	if api.callCount() != 0 {
		t.Fatalf("preview made %d requests", api.callCount())
	}

	// Default actions: block, expire, reset.
	out, _, err = rqtRun(t, "contain", "jane.doe@example.com", "--org-id", "ORG_1", "--account-id", "ACC_JANE", "--json")
	if err != nil {
		t.Fatalf("contain default preview: %v", err)
	}
	res = rqtDecode[rqtContainOut](t, out)
	if got := strings.Join(rqtContainActions(res), "|"); got != "Block Sign In|Expire Account Logins|Reset User Password" {
		t.Fatalf("default actions = %s", got)
	}
	if len(res.PropagationNotes) != 2 || !strings.Contains(res.PropagationNotes[1], "15 minutes") {
		t.Fatalf("propagation notes = %v", res.PropagationNotes)
	}
}

func TestContainResolvesFromStore(t *testing.T) {
	rqtEnv(t, nil)
	users := []map[string]any{
		{"id": "USER_FRANK_MS", "email": "frank@example.com", "name": "Frank", "parent_id": "CUST_9", "customers_id": "CUST_9",
			"product": map[string]any{"id": "P_MS", "name": "Microsoft 365", "type": "MS"}},
		{"id": "dbmid:FRANK", "email": "frank@example.com", "name": "Frank", "parent_id": "CUST_9", "customers_id": "CUST_9",
			"product": map[string]any{"id": "P_DBX", "name": "Dropbox", "type": "DROP_BOX"}},
	}
	dbPath := rqtSeedStore(t, rqtTriggers(), users)

	out, _, err := rqtRun(t, "contain", "JANE.DOE@example.com", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("contain via triggers: %v", err)
	}
	res := rqtDecode[rqtContainOut](t, out)
	r := res.Resolution
	if r.Source != "rule_trigger" || r.OrganizationID != "ORG_1" || r.AccountID != "ACC_JANE" || r.TriggerID != "RT_A2" || r.Inferred {
		t.Fatalf("trigger resolution wrong: %+v", r)
	}

	out, _, err = rqtRun(t, "contain", "frank@example.com", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("contain via users: %v", err)
	}
	res = rqtDecode[rqtContainOut](t, out)
	r = res.Resolution
	if r.Source != "users" || r.OrganizationID != "CUST_9" || r.AccountID != "USER_FRANK_MS" || !r.Inferred || !strings.Contains(r.Note, "assumed") {
		t.Fatalf("users resolution wrong: %+v", r)
	}

	// A lone --account-id selects among synced candidates; it is not an
	// override, so an id the store does not know resolves to nothing.
	out, _, err = rqtRun(t, "contain", "jane.doe@example.com", "--db", dbPath, "--account-id", "ACC_JANE", "--json")
	if err != nil {
		t.Fatalf("contain with a matching --account-id: %v", err)
	}
	if r := rqtDecode[rqtContainOut](t, out).Resolution; r.Source != "rule_trigger" || r.OrganizationID != "ORG_1" || r.AccountID != "ACC_JANE" {
		t.Fatalf("selector resolution wrong: %+v", r)
	}
	_, _, err = rqtRun(t, "contain", "jane.doe@example.com", "--db", dbPath, "--account-id", "ACC_OVERRIDE", "--json")
	if rqtExit(err) != 2 || !strings.Contains(err.Error(), "with account id ACC_OVERRIDE") {
		t.Fatalf("an unknown --account-id must not be paired with another account's org: exit=%d err=%v", rqtExit(err), err)
	}
}

func TestContainUnresolvedAndBadActionsExit2(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	dbPath := rqtSeedStore(t, rqtTriggers(), nil)
	_, stderr, err := rqtRun(t, "contain", "nobody@example.com", "--db", dbPath)
	if rqtExit(err) != 2 || !strings.Contains(err.Error(), "--org-id") || !strings.Contains(err.Error(), "--account-id") {
		t.Fatalf("unresolved email: exit=%d err=%v stderr=%s", rqtExit(err), err, stderr)
	}
	_, _, err = rqtRun(t, "contain", "nobody@example.com", "--db", filepath.Join(t.TempDir(), "missing.db"))
	if rqtExit(err) != 2 {
		t.Fatalf("missing store: exit=%d err=%v", rqtExit(err), err)
	}
	for _, actions := range []string{"block,teleport", "delete", "block,delete"} {
		_, _, err = rqtRun(t, "contain", "jane.doe@example.com", "--org-id", "O", "--account-id", "A", "--actions", actions)
		if rqtExit(err) != 2 {
			t.Fatalf("--actions %s: exit=%d err=%v", actions, rqtExit(err), err)
		}
	}
	_, _, err = rqtRun(t, "contain", "--actions", "block")
	if rqtExit(err) != 2 {
		t.Fatalf("missing email: exit=%d err=%v", rqtExit(err), err)
	}
	if api.callCount() != 0 {
		t.Fatalf("rejected invocations reached the API %d times", api.callCount())
	}
}

func TestContainConfirmSubmitsInOrderAndNeverClaimsVerified(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	rqtFastPoll(t)
	out, stderr, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "reset,expire,block",
		"--org-id", "ORG_1", "--account-id", "ACC_JANE", "--confirm", "--wait", "5s", "--json")
	if err != nil {
		t.Fatalf("contain confirm: %v\nstderr: %s\n%s", err, stderr, out)
	}
	res := rqtDecode[rqtContainOut](t, out)

	execs := api.callsMatching("POST", "/respondApi/api/v1/response-actions/execute")
	sent := make([]string, 0, len(execs))
	for _, c := range execs {
		var body containExecuteBody
		if err := json.Unmarshal([]byte(c.Body), &body); err != nil {
			t.Fatalf("execute body: %v", err)
		}
		if len(body.Accounts) != 1 || body.Accounts[0].OrganizationID != "ORG_1" || body.Accounts[0].Account.ID != "ACC_JANE" {
			t.Fatalf("execute account wrong: %s", c.Body)
		}
		sent = append(sent, body.Action)
	}
	if got := strings.Join(sent, "|"); got != "Block Sign In|Expire Account Logins|Reset User Password" {
		t.Fatalf("execute order = %s", got)
	}
	if res.Mode != "executed" || res.Verified || !strings.Contains(res.VerificationNote, "no job id") {
		t.Fatalf("contain must never claim verification: verified=%v note=%q", res.Verified, res.VerificationNote)
	}
	if s := res.Summary; s.Submitted != 3 || s.ExecuteErrors != 0 || s.JobsHeuristic != 3 || s.JobWarnings != 0 {
		t.Fatalf("summary = %+v", s)
	}
	for i, a := range res.Actions {
		wantJob := fmt.Sprintf("JOB_%d", i+1)
		if a.Status != "submitted" || a.Submitted == nil || a.Submitted.Status != "PROCESSING" || len(a.Submitted.EventIDs) != 1 ||
			a.Submitted.ExecutedAt != rqFormatTime(rqtNow.Add(time.Duration(i+1)*time.Minute)) {
			t.Fatalf("step %d submission wrong: %+v / %+v", i, a, a.Submitted)
		}
		m := a.JobMatch
		if m == nil || m.Confidence != "heuristic" || strings.Join(m.JobIDs, ",") != wantJob || !m.Completed ||
			m.Success == nil || *m.Success != 1 || !strings.Contains(m.Note, "matched by action, operator, and time only") {
			t.Fatalf("step %d job_match = %+v, want heuristic %s", i, m, wantJob)
		}
	}
	for _, id := range []string{"JOB_1", "JOB_2", "JOB_3"} {
		if n := len(api.callsMatching("GET", "/respondApi/api/v1/response-actions/jobs/"+id)); n != 2 {
			t.Fatalf("%s polled %d times, want 2", id, n)
		}
	}
}

func TestContainHeuristicJobFailureWarnsAndExitsNonZero(t *testing.T) {
	api := newRQTAPI(t)
	api.jobFailAction["Reset User Password"] = true
	rqtEnv(t, api)
	rqtFastPoll(t)
	out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--org-id", "ORG_1", "--account-id", "ACC_JANE", "--confirm", "--wait", "5s", "--json")
	if rqtExit(err) == 0 || !strings.Contains(err.Error(), "may be another job") {
		t.Fatalf("a matched job reporting failures must exit non-zero with a caveat: err=%v\n%s", err, out)
	}
	res := rqtDecode[rqtContainOut](t, out)
	last := res.Actions[len(res.Actions)-1]
	if last.Action != "Reset User Password" || last.Status != "submitted" || last.JobMatch == nil ||
		!strings.Contains(last.JobMatch.Warning, "failed=1") || !strings.Contains(last.JobMatch.Warning, "may be another job") {
		t.Fatalf("reset step should carry a heuristic warning: %+v", last)
	}
	if res.Summary.Submitted != 3 || res.Summary.JobWarnings != 1 || res.Verified {
		t.Fatalf("summary = %+v verified=%v", res.Summary, res.Verified)
	}
}

func TestContainUnmatchedJobStillExitsZero(t *testing.T) {
	api := newRQTAPI(t)
	api.hideJobs = true
	rqtEnv(t, api)
	rqtFastPoll(t)
	out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
		"--confirm", "--wait", "50ms", "--json")
	if err != nil {
		t.Fatalf("an accepted action with no job match must exit 0: %v\n%s", err, out)
	}
	res := rqtDecode[rqtContainOut](t, out)
	m := res.Actions[0].JobMatch
	if res.Actions[0].Status != "submitted" || m == nil || m.Confidence != "none" || len(m.JobIDs) != 0 || !strings.Contains(m.Note, "no candidate job appeared") {
		t.Fatalf("unmatched job not reported: %+v", res.Actions[0])
	}
	if res.Summary.JobsUnmatched != 1 || res.Verified {
		t.Fatalf("summary = %+v", res.Summary)
	}
}

func TestContainExecuteFailureExitsNonZero(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	rqtFastPoll(t)
	// A dead server: every execute fails before anything is accepted.
	t.Setenv("SAAS_ALERTS_BASE_URL", api.srv.URL+"/nowhere")
	out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
		"--confirm", "--wait", "50ms", "--json")
	if rqtExit(err) == 0 {
		t.Fatalf("a refused execute must exit non-zero:\n%s", out)
	}
	res := rqtDecode[rqtContainOut](t, out)
	if res.Actions[0].Status != "error" || res.Actions[0].Submitted != nil || res.Summary.ExecuteErrors != 1 {
		t.Fatalf("execute failure not reported: %+v / %+v", res.Actions[0], res.Summary)
	}
}

func TestContainHarnessRefusesConfirm(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--org-id", "ORG_1", "--account-id", "ACC_JANE", "--confirm", "--json")
	if err != nil || !strings.Contains(out, `"refused"`) || api.callCount() != 0 {
		t.Fatalf("harness should refuse with no calls: err=%v out=%s calls=%d", err, out, api.callCount())
	}
}

// rqtContainBlock runs a confirmed single-action (Block Sign In) contain.
func rqtContainBlock(t *testing.T) (rqtContainOut, error) {
	t.Helper()
	out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
		"--confirm", "--wait", "2s", "--json")
	return rqtDecode[rqtContainOut](t, out), err
}

func rqtAddJob(a *rqtAPI, job map[string]any) {
	a.jobs = append(a.jobs, job)
	a.jobVisibleAt[fmt.Sprint(job["id"])] = a.jobLists
}

// rqtHeuristic asserts a step was matched heuristically to wantJob and that
// nothing claims verification.
func rqtHeuristic(t *testing.T, res rqtContainOut, wantJob string) {
	t.Helper()
	m := res.Actions[0].JobMatch
	if res.Verified || m == nil || m.Confidence != "heuristic" || strings.Join(m.JobIDs, ",") != wantJob {
		t.Fatalf("want a heuristic (unverified) match to %s, got verified=%v %+v", wantJob, res.Verified, m)
	}
}

func TestContainIgnoresAnotherOperatorsConcurrentJob(t *testing.T) {
	api := newRQTAPI(t)
	api.execHook = func(a *rqtAPI, action string, executedAt int64) {
		// Same action and window as ours (created at +500ms), but run by
		// someone else: one just before ours, one just after.
		other := map[string]any{"id": "OP_OTHER", "email": "other@example.com"}
		rqtAddJob(a, map[string]any{"id": "JOB_OTHER", "createdAt": executedAt + 2000, "total": 1, "success": 1, "failed": 0,
			"completed": true, "action": action, "createdBy": other})
		rqtAddJob(a, map[string]any{"id": "JOB_OTHER_EARLY", "createdAt": executedAt + 200, "total": 1, "success": 1, "failed": 0,
			"completed": true, "action": action, "createdBy": other})
	}
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if err != nil {
		t.Fatalf("contain: %v", err)
	}
	rqtHeuristic(t, res, "JOB_1")
}

func TestContainSingleCandidateIsHeuristicNotVerified(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if err != nil {
		t.Fatalf("contain: %v", err)
	}
	rqtHeuristic(t, res, "JOB_1")
	m := res.Actions[0].JobMatch
	if !m.Completed || m.Note != containJobMatchNote || !strings.Contains(res.VerificationNote, "never proves") {
		t.Fatalf("a lone candidate is still only a guess: %+v / %q", m, res.VerificationNote)
	}
}

func TestContainUnrelatedJobBeforeDelayedRealJobNeverVerifies(t *testing.T) {
	api := newRQTAPI(t)
	// Our own job shows up one listing late; an unrelated, still-running job
	// for the same action (no creator recorded) shows up first.
	api.jobDelay["Block Sign In"] = 1
	api.execHook = func(a *rqtAPI, action string, executedAt int64) {
		rqtAddJob(a, map[string]any{"id": "JOB_UNRELATED", "createdAt": executedAt + 300, "total": 1, "success": 0, "failed": 0,
			"completed": false, "action": action})
	}
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if err != nil {
		t.Fatalf("an ambiguous match leaves the accepted action at exit 0: %v", err)
	}
	m := res.Actions[0].JobMatch
	if res.Verified || m == nil || m.Confidence != "ambiguous" || strings.Join(m.JobIDs, ",") != "JOB_UNRELATED,JOB_1" || m.Completed {
		t.Fatalf("the late real job must downgrade the match to ambiguous, never verified: verified=%v %+v", res.Verified, m)
	}
}

func TestContainIgnoresJobsOutsideTheWindow(t *testing.T) {
	api := newRQTAPI(t)
	api.jobDelay["Block Sign In"] = 1000 // our own job never shows up
	api.execHook = func(a *rqtAPI, action string, executedAt int64) {
		// Same action and operator, but stamped 3 minutes after the call.
		rqtAddJob(a, map[string]any{"id": "JOB_FAR", "createdAt": executedAt + 180000, "total": 1, "success": 1, "failed": 0,
			"completed": true, "action": action, "createdBy": a.execBy})
	}
	rqtEnv(t, api)
	rqtFastPoll(t)
	out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
		"--confirm", "--wait", "50ms", "--json")
	if err != nil {
		t.Fatalf("contain: %v", err)
	}
	if m := rqtDecode[rqtContainOut](t, out).Actions[0].JobMatch; m == nil || m.Confidence != "none" || len(m.JobIDs) != 0 {
		t.Fatalf("a job outside [executedAt-5s, executedAt+2m] was matched: %+v", m)
	}
}

func TestContainSameOperatorTwinIsAmbiguous(t *testing.T) {
	api := newRQTAPI(t)
	api.execHook = func(a *rqtAPI, action string, executedAt int64) {
		rqtAddJob(a, map[string]any{"id": "JOB_LATER", "createdAt": executedAt + 5000, "total": 1, "success": 1, "failed": 0,
			"completed": true, "action": action, "createdBy": a.execBy})
	}
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if err != nil {
		t.Fatalf("contain: %v", err)
	}
	m := res.Actions[0].JobMatch
	if m == nil || m.Confidence != "ambiguous" || strings.Join(m.JobIDs, ",") != "JOB_1,JOB_LATER" || m.Completed || m.Success != nil || res.Verified {
		t.Fatalf("two fitting jobs must be ambiguous, with no job's counts reported: %+v", m)
	}
	if res.Summary.JobsAmbiguous != 1 {
		t.Fatalf("summary = %+v", res.Summary)
	}
}

func TestContainExcludesPreexistingRecentJob(t *testing.T) {
	api := newRQTAPI(t)
	// Created 2s before the call (inside the skew window), same action and
	// operator: it existed before the call, so it cannot be this call's job.
	api.jobs = append(api.jobs, map[string]any{"id": "JOB_PRE", "createdAt": rqtMS(time.Minute) - 2000, "total": 1, "success": 1,
		"failed": 0, "completed": true, "action": "Block Sign In", "createdBy": api.execBy})
	api.jobDelay["Block Sign In"] = 1
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if err != nil {
		t.Fatalf("contain: %v", err)
	}
	rqtHeuristic(t, res, "JOB_1")
}

func TestContainJobWithoutActionNeverMatches(t *testing.T) {
	api := newRQTAPI(t)
	api.jobDelay["Block Sign In"] = 1
	api.execHook = func(a *rqtAPI, action string, executedAt int64) {
		rqtAddJob(a, map[string]any{"id": "JOB_NOACTION", "createdAt": executedAt + 100, "total": 1, "success": 1, "failed": 0,
			"completed": true, "createdBy": a.execBy})
	}
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if err != nil {
		t.Fatalf("contain: %v", err)
	}
	rqtHeuristic(t, res, "JOB_1")
}

func TestContainMissedJobWarns(t *testing.T) {
	api := newRQTAPI(t)
	api.jobOverride["Block Sign In"] = map[string]any{"success": 0, "failed": 0, "missed": 1}
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if rqtExit(err) == 0 {
		t.Fatalf("a matched job that missed the account must exit non-zero")
	}
	m := res.Actions[0].JobMatch
	if m == nil || m.Missed == nil || *m.Missed != 1 || !strings.Contains(m.Warning, "missed=1") {
		t.Fatalf("missed account not surfaced: %+v", m)
	}
}

func TestContainSameEmailInTwoOrganizations(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	rqtFastPoll(t)
	triggers := append(rqtTriggers(),
		map[string]any{"id": "RT_MULTI_1", "status": "PENDING", "triggeredAt": rqtMS(-time.Hour),
			"organization": map[string]any{"id": "ORG_1", "name": "Contoso"},
			"accounts":     rqtAccount("ACC_SAM_1", "sam@example.com"),
			"rule":         rqtRule("RULE_A", "Impossible travel", "critical", "Block Sign In")},
		map[string]any{"id": "RT_MULTI_2", "status": "PENDING", "triggeredAt": rqtMS(-2 * time.Hour),
			"organization": map[string]any{"id": "ORG_2", "name": "Fabrikam"},
			"accounts":     rqtAccount("ACC_SAM_2", "sam@example.com"),
			"rule":         rqtRule("RULE_A", "Impossible travel", "critical", "Block Sign In")},
		// The same pair again on an older trigger is not a second candidate.
		map[string]any{"id": "RT_MULTI_1_OLD", "status": "APPROVED", "triggeredAt": rqtMS(-9 * time.Hour),
			"organization": map[string]any{"id": "ORG_1", "name": "Contoso"},
			"accounts":     rqtAccount("ACC_SAM_1", "sam@example.com"),
			"rule":         rqtRule("RULE_A", "Impossible travel", "critical", "Block Sign In")},
	)
	dbPath := rqtSeedStore(t, triggers, nil)

	_, _, err := rqtRun(t, "contain", "sam@example.com", "--db", dbPath, "--confirm", "--json")
	if rqtExit(err) != 2 {
		t.Fatalf("two organizations must stop with exit 2, got exit=%d err=%v", rqtExit(err), err)
	}
	for _, want := range []string{"2 Microsoft 365 accounts", "--org-id ORG_1 --account-id ACC_SAM_1", "--org-id ORG_2 --account-id ACC_SAM_2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("candidate list missing %q: %v", want, err)
		}
	}
	if n := len(api.callsMatching("POST", "/respondApi/api/v1/response-actions/execute")); n != 0 {
		t.Fatalf("an ambiguous target sent %d execute requests", n)
	}

	out, _, err := rqtRun(t, "contain", "sam@example.com", "--db", dbPath, "--org-id", "ORG_2", "--actions", "block", "--confirm", "--wait", "2s", "--json")
	if err != nil {
		t.Fatalf("--org-id selecting one organization should proceed: %v\n%s", err, out)
	}
	if r := rqtDecode[rqtContainOut](t, out).Resolution; r.OrganizationID != "ORG_2" || r.AccountID != "ACC_SAM_2" || r.TriggerID != "RT_MULTI_2" {
		t.Fatalf("selected pair wrong: %+v", r)
	}
	execs := api.callsMatching("POST", "/respondApi/api/v1/response-actions/execute")
	if len(execs) != 1 || !strings.Contains(execs[0].Body, `"organizationId":"ORG_2"`) || !strings.Contains(execs[0].Body, `"id":"ACC_SAM_2"`) {
		t.Fatalf("execute did not target the selected pair: %+v", execs)
	}
}

func TestContainMicrosoftAccountsOnly(t *testing.T) {
	rqtEnv(t, nil)
	triggers := rqtTriggers()
	triggers = append(triggers,
		map[string]any{"id": "RT_GOOGLE", "status": "PENDING", "triggeredAt": rqtMS(-time.Hour),
			"organization": map[string]any{"id": "ORG_3", "name": "Northwind"},
			"accounts":     []any{map[string]any{"id": "ACC_GINA", "email": "gina@example.com", "product": map[string]any{"type": "G_SUITE"}}},
			"rule":         rqtRule("RULE_G", "Google sign-in", "low", "Alert Only")},
		map[string]any{"id": "RT_HANK_GOOGLE", "status": "PENDING", "triggeredAt": rqtMS(-time.Hour),
			"organization": map[string]any{"id": "ORG_3", "name": "Northwind"},
			"accounts":     []any{map[string]any{"id": "ACC_HANK_G", "email": "hank@example.com", "product": map[string]any{"type": "G_SUITE"}}},
			"rule":         rqtRule("RULE_G", "Google sign-in", "low", "Alert Only")},
	)
	users := []map[string]any{
		{"id": "dbmid:GAIL", "email": "gail@example.com", "parent_id": "CUST_5", "customers_id": "CUST_5",
			"product": map[string]any{"id": "P_DBX", "name": "Dropbox", "type": "DROP_BOX"}},
		{"id": "GOOGLE_HANK", "email": "hank@example.com", "parent_id": "CUST_7", "customers_id": "CUST_7",
			"product": map[string]any{"id": "P_GWS", "name": "Google Workspace", "type": "G_SUITE"}},
		{"id": "MS_HANK", "email": "hank@example.com", "parent_id": "CUST_8", "customers_id": "CUST_8",
			"product": map[string]any{"id": "P_MS", "name": "Microsoft 365", "type": "MS"}},
	}
	dbPath := rqtSeedStore(t, triggers, users)

	for _, email := range []string{"gina@example.com", "gail@example.com"} {
		_, _, err := rqtRun(t, "contain", email, "--db", dbPath, "--json")
		if rqtExit(err) != 2 || !strings.Contains(err.Error(), "Microsoft 365 accounts only") {
			t.Fatalf("%s (non-Microsoft only): exit=%d err=%v", email, rqtExit(err), err)
		}
	}

	// hank has a Google trigger account and two user rows: only the
	// Microsoft 365 user row may be used.
	out, _, err := rqtRun(t, "contain", "hank@example.com", "--db", dbPath, "--json")
	if err != nil {
		t.Fatalf("hank: %v", err)
	}
	r := rqtDecode[rqtContainOut](t, out).Resolution
	if r.Source != "users" || r.OrganizationID != "CUST_8" || r.AccountID != "MS_HANK" || !r.Inferred {
		t.Fatalf("hank should resolve to the Microsoft 365 user row: %+v", r)
	}
}

func TestContainHelpStatesScopeAndPreview(t *testing.T) {
	testenv.Isolate(t)
	out, _, err := rqtRun(t, "contain", "--help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{"Microsoft 365 accounts only", "--org-id and --account-id", "sync --resources rule-triggers,users",
		"Reports customer id equals the Respond organization id", "inferred:true", "without --confirm", "--dry-run only prints a stub",
		"returns no job id", "never proves completion", "verified:false", "more than one Microsoft 365 account"} {
		if !strings.Contains(out, want) {
			t.Fatalf("contain --help missing %q", want)
		}
	}
}

func TestContainExecuteAcceptanceIsValidated(t *testing.T) {
	cases := []struct {
		name    string
		code    int
		body    any // nil: the fake's documented 202 acceptance
		outcome string
	}{
		{name: "200 error-shaped object", code: 200, body: map[string]any{"message": "backend failure"}, outcome: "unknown_outcome"},
		{name: "200 null", code: 200, body: rqtNull, outcome: "unknown_outcome"},
		{name: "200 string", code: 200, body: "ok", outcome: "unknown_outcome"},
		{name: "500 server error", code: 500, body: map[string]any{"message": "internal"}, outcome: "unknown_outcome"},
		{name: "202 status ERROR", code: 202, body: map[string]any{"status": "ERROR", "executedAt": rqtMS(time.Minute)}, outcome: "unknown_outcome"},
		{name: "202 status FAILED", code: 202, body: map[string]any{"status": "FAILED"}, outcome: "unknown_outcome"},
		{name: "202 executedAt with an error field", code: 202, body: map[string]any{"executedAt": rqtMS(time.Minute), "error": "account locked"}, outcome: "unknown_outcome"},
		{name: "202 executedAt with a failure message", code: 202, body: map[string]any{"executedAt": rqtMS(time.Minute), "message": "Request failed"}, outcome: "unknown_outcome"},
		{name: "202 accepted status with errors", code: 202, body: map[string]any{"status": "PROCESSING", "errors": []any{"x"}}, outcome: "unknown_outcome"},
		{name: "202 PROCESSING with success:false", code: 202, body: map[string]any{"status": "PROCESSING", "success": false, "executedAt": rqtMS(time.Minute)}, outcome: "unknown_outcome"},
		{name: "202 PROCESSING with ok:false", code: 202, body: map[string]any{"status": "PROCESSING", "ok": false, "executedAt": rqtMS(time.Minute)}, outcome: "unknown_outcome"},
		{name: "202 PROCESSING with code 500", code: 202, body: map[string]any{"status": "PROCESSING", "code": 500, "executedAt": rqtMS(time.Minute)}, outcome: "unknown_outcome"},
		{name: "202 lowercase processing", code: 202, body: map[string]any{"status": "processing"}, outcome: "submitted"},
		{name: "202 executedAt only", code: 202, body: map[string]any{"executedAt": rqtMS(time.Minute)}, outcome: "submitted"},
		{name: "202 accepted status with empty errors", code: 202, body: map[string]any{"status": "QUEUED", "errors": []any{}}, outcome: "submitted"},
		{name: "202 status only", code: 202, body: map[string]any{"status": "PROCESSING"}, outcome: "submitted"},
		{name: "202 documented acceptance", outcome: "submitted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.execBody, api.execCode = tc.body, tc.code
			rqtEnv(t, api)
			rqtFastPoll(t)
			out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
				"--confirm", "--wait", "50ms", "--json")
			res := rqtDecode[rqtContainOut](t, out)
			execs := len(api.callsMatching("POST", "/respondApi/api/v1/response-actions/execute"))
			first := res.Actions[0]
			if first.Status != tc.outcome {
				t.Fatalf("first action status = %q, want %q (%s)", first.Status, tc.outcome, first.Error)
			}
			if tc.outcome == "submitted" {
				if err != nil || execs != 3 || res.Summary.Submitted != 3 {
					t.Fatalf("an accepted execute should proceed: err=%v execs=%d summary=%+v", err, execs, res.Summary)
				}
				return
			}
			if rqtExit(err) == 0 {
				t.Fatalf("an unknown outcome must exit non-zero")
			}
			// One execute, never replayed, and nothing after it.
			if execs != 1 || first.Submitted != nil || !strings.Contains(first.Error, "not retried") {
				t.Fatalf("execs=%d submitted=%v error=%q", execs, first.Submitted, first.Error)
			}
			for _, a := range res.Actions[1:] {
				if a.Status != "not_sent" {
					t.Fatalf("actions after an unknown outcome must not be sent: %+v", a)
				}
			}
			if res.Summary.UnknownOutcomes != 1 || res.Summary.NotSent != 2 || res.Summary.Submitted != 0 {
				t.Fatalf("summary = %+v", res.Summary)
			}
		})
	}
}

func TestContainTargetResolutionIgnoresDecisionStatus(t *testing.T) {
	samRow := func(id, org string, status any) map[string]any {
		return map[string]any{"id": id, "status": status, "triggeredAt": rqtMS(-time.Hour),
			"organization": map[string]any{"id": org, "name": org},
			"accounts":     rqtAccount("ACC_"+org, "sam@example.com"),
			"rule":         rqtRule("RULE_A", "Impossible travel", "critical", "Block Sign In")}
	}
	cases := []struct {
		name    string
		second  map[string]any
		args    []string
		wantErr string // "" means resolution proceeds
		wantOrg string
	}{
		{name: "second organization's row has a null status", second: samRow("RT_SAM_2", "ORG_2", nil),
			wantErr: "2 Microsoft 365 accounts"},
		{name: "second row's organization is unreadable", second: func() map[string]any {
			r := samRow("RT_SAM_2", "ORG_2", "PENDING")
			r["organization"] = "ORG_2"
			return r
		}(), wantErr: "unreadable organization or account identity"},
		{name: "second row's account id is unreadable", second: func() map[string]any {
			r := samRow("RT_SAM_2", "ORG_2", "PENDING")
			r["accounts"] = []any{map[string]any{"id": 5, "email": "sam@example.com", "product": map[string]any{"type": "MS"}}}
			return r
		}(), wantErr: "unreadable organization or account identity"},
		{name: "--org-id selects past a null-status row", second: samRow("RT_SAM_2", "ORG_2", nil),
			args: []string{"--org-id", "ORG_1"}, wantOrg: "ORG_1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			rqtEnv(t, api)
			rqtFastPoll(t)
			dbPath := rqtSeedStore(t, []map[string]any{samRow("RT_SAM_1", "ORG_1", "PENDING"), tc.second}, nil)
			args := append([]string{"contain", "sam@example.com", "--actions", "block", "--db", dbPath, "--confirm", "--wait", "2s", "--json"}, tc.args...)
			out, _, err := rqtRun(t, args...)
			execs := len(api.callsMatching("POST", "/respondApi/api/v1/response-actions/execute"))
			if tc.wantErr != "" {
				if rqtExit(err) != 2 || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want exit 2 containing %q, got exit=%d err=%v\n%s", tc.wantErr, rqtExit(err), err, out)
				}
				if execs != 0 {
					t.Fatalf("an unresolved target sent %d execute calls", execs)
				}
				return
			}
			if err != nil {
				t.Fatalf("contain: %v\n%s", err, out)
			}
			if r := rqtDecode[rqtContainOut](t, out).Resolution; r.OrganizationID != tc.wantOrg || execs != 1 {
				t.Fatalf("resolution=%+v execs=%d", r, execs)
			}
		})
	}
}

func TestContainJobDetailMustMatchTheListedJob(t *testing.T) {
	detail := func(mutate func(d map[string]any)) map[string]any {
		d := map[string]any{"id": "JOB_1", "createdAt": rqtMS(time.Minute) + 500, "total": 1, "success": 1, "failed": 0, "missed": 0,
			"completed": true, "action": "Block Sign In", "createdBy": map[string]any{"id": "OP_ME", "email": "operator@example.com"}}
		mutate(d)
		return d
	}
	cases := map[string]map[string]any{
		"mismatched id":        detail(func(d map[string]any) { d["id"] = "JOB_OTHER" }),
		"contradictory action": detail(func(d map[string]any) { d["action"] = "Delete User" }),
		"contradictory operator": detail(func(d map[string]any) {
			d["createdBy"] = map[string]any{"id": "OP_OTHER", "email": "other@example.com"}
		}),
		"contradictory time":       detail(func(d map[string]any) { d["createdAt"] = rqtMS(time.Minute) + 10*60*1000 }),
		"detail without an action": detail(func(d map[string]any) { delete(d, "action") }),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			api := newRQTAPI(t)
			// The listed job reports a failure; the contradicting detail claims success.
			api.jobFailAction["Block Sign In"] = true
			api.jobDetail["JOB_1"] = body
			rqtEnv(t, api)
			rqtFastPoll(t)
			res, err := rqtContainBlock(t)
			if rqtExit(err) == 0 {
				t.Fatalf("the listed job's failure must keep the exit non-zero")
			}
			m := res.Actions[0].JobMatch
			if m == nil || strings.Join(m.JobIDs, ",") != "JOB_1" || m.Failed == nil || *m.Failed != 1 || !strings.Contains(m.Warning, "failed=1") {
				t.Fatalf("the vetted list entry (failed=1) must stay visible: %+v", m)
			}
			// A detail that answers for another job id, or contradicts the
			// listing, did not answer clearly: both are warning evidence.
			want := "contradicts the listed job"
			if name == "mismatched id" {
				want = "answered for a different job id"
			}
			if !strings.Contains(m.Warning, want) {
				t.Fatalf("want %q in the warning: %+v", want, m)
			}
		})
	}
}

func TestContainUnknownProductOrUnreadableUserBlocksResolution(t *testing.T) {
	samRow := func(id, org string, product any) map[string]any {
		return map[string]any{"id": id, "status": "PENDING", "triggeredAt": rqtMS(-time.Hour),
			"organization": map[string]any{"id": org, "name": org},
			"accounts":     []any{map[string]any{"id": "ACC_" + org, "email": "sam@example.com", "product": product}},
			"rule":         rqtRule("RULE_A", "Impossible travel", "critical", "Block Sign In")}
	}
	ms := map[string]any{"type": "MS"}
	frank := func(id, customer string, product any) map[string]any {
		return map[string]any{"id": id, "email": "frank@example.com", "name": "Frank", "parent_id": customer, "customers_id": customer, "product": product}
	}
	msUser := map[string]any{"id": "P_MS", "name": "Microsoft 365", "type": "MS"}
	cases := []struct {
		name     string
		triggers []map[string]any
		users    []map[string]any
		email    string
		proceeds bool // a positively non-Microsoft second row must not block
	}{
		{name: "second candidate has a type outside the Respond enum", email: "sam@example.com",
			triggers: []map[string]any{samRow("RT_SAM_1", "ORG_1", ms), samRow("RT_SAM_2", "ORG_2", map[string]any{"type": "SLACK"})}},
		{name: "a user row whose product is an opaque id only", email: "frank@example.com",
			users: []map[string]any{frank("USER_FRANK_MS", "CUST_9", msUser), frank("USER_FRANK_2", "CUST_8", map[string]any{"id": "opaque-product-id"})}},
		{name: "a user row with an unrecognized product type", email: "frank@example.com",
			users: []map[string]any{frank("USER_FRANK_MS", "CUST_9", msUser), frank("USER_FRANK_2", "CUST_8", map[string]any{"id": "P_X", "name": "X", "type": "NOT_A_PRODUCT"})}},
		{name: "a user row under a Microsoft-family monitor", email: "frank@example.com",
			users: []map[string]any{frank("USER_FRANK_MS", "CUST_9", msUser), frank("USER_FRANK_2", "CUST_8", map[string]any{"id": "P_DEF", "name": "Defender", "type": "MS_DEFENDER"})}},
		{name: "a G_SUITE user row is positively non-Microsoft and does not block", email: "frank@example.com", proceeds: true,
			users: []map[string]any{frank("USER_FRANK_MS", "CUST_9", msUser), frank("USER_FRANK_2", "CUST_8", map[string]any{"id": "P_GWS", "name": "Google Workspace", "type": "G_SUITE"})}},
		{name: "second candidate has a null product", email: "sam@example.com",
			triggers: []map[string]any{samRow("RT_SAM_1", "ORG_1", ms), samRow("RT_SAM_2", "ORG_2", nil)}},
		{name: "second candidate has a malformed product", email: "sam@example.com",
			triggers: []map[string]any{samRow("RT_SAM_1", "ORG_1", ms), samRow("RT_SAM_2", "ORG_2", "MS")}},
		{name: "second candidate has a non-string product type", email: "sam@example.com",
			triggers: []map[string]any{samRow("RT_SAM_1", "ORG_1", ms), samRow("RT_SAM_2", "ORG_2", map[string]any{"type": 5})}},
		{name: "a malformed matching user row", email: "frank@example.com",
			users: []map[string]any{frank("USER_FRANK_MS", "CUST_9", map[string]any{"id": "P_MS", "name": "Microsoft 365", "type": "MS"}),
				frank("USER_FRANK_2", "CUST_8", "Microsoft 365")}},
		{name: "a matching user row with a null product", email: "frank@example.com",
			users: []map[string]any{frank("USER_FRANK_MS", "CUST_9", map[string]any{"id": "P_MS", "name": "Microsoft 365", "type": "MS"}),
				frank("USER_FRANK_2", "CUST_8", nil)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			rqtEnv(t, api)
			rqtFastPoll(t)
			dbPath := rqtSeedStore(t, tc.triggers, tc.users)
			out, _, err := rqtRun(t, "contain", tc.email, "--actions", "block", "--db", dbPath, "--confirm", "--wait", "2s", "--json")
			if tc.proceeds {
				execs := len(api.callsMatching("POST", "/respondApi/api/v1/response-actions/execute"))
				if err != nil || execs != 1 || rqtDecode[rqtContainOut](t, out).Resolution.AccountID != "USER_FRANK_MS" {
					t.Fatalf("a positively non-Microsoft row must not block: err=%v execs=%d\n%s", err, execs, out)
				}
				return
			}
			if rqtExit(err) != 2 || !strings.Contains(err.Error(), "--org-id and --account-id") {
				t.Fatalf("an unknown product or unreadable row must block resolution (exit 2): exit=%d err=%v\n%s", rqtExit(err), err, out)
			}
			if n := len(api.callsMatching("POST", "/respondApi/api/v1/response-actions/execute")); n != 0 {
				t.Fatalf("a blocked resolution sent %d execute calls", n)
			}
		})
	}
}

func TestContainJobDetailMissingCountsKeepTheListedOnes(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{"id": "JOB_1", "createdAt": rqtMS(time.Minute) + 500, "total": 1, "success": 1, "completed": true,
			"action": "Block Sign In", "createdBy": map[string]any{"id": "OP_ME", "email": "operator@example.com"}}
	}
	cases := map[string]map[string]any{
		"failed missing":           func() map[string]any { d := base(); d["missed"] = 0; return d }(),
		"failed null":              func() map[string]any { d := base(); d["failed"], d["missed"] = nil, nil; return d }(),
		"failed 0 and null missed": func() map[string]any { d := base(); d["failed"], d["missed"] = 0, nil; return d }(),
	}
	for name, detail := range cases {
		t.Run(name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.jobFailAction["Block Sign In"] = true // the listed job reports failed:1
			api.jobDetail["JOB_1"] = detail
			rqtEnv(t, api)
			rqtFastPoll(t)
			res, err := rqtContainBlock(t)
			if rqtExit(err) == 0 {
				t.Fatalf("the listed failure must keep the exit non-zero")
			}
			m := res.Actions[0].JobMatch
			if m == nil || m.Failed == nil || *m.Failed != 1 || !strings.Contains(m.Warning, "failed=1") {
				t.Fatalf("the listed failed:1 must stay visible: %+v", m)
			}
		})
	}
}

func TestContainJobDetailWithoutCompletionKeepsTheListed(t *testing.T) {
	detail := func(completed any, present bool) map[string]any {
		d := map[string]any{"id": "JOB_1", "createdAt": rqtMS(time.Minute) + 500, "total": 1, "success": 1, "failed": 0, "missed": 0,
			"action": "Block Sign In", "createdBy": map[string]any{"id": "OP_ME", "email": "operator@example.com"}}
		if present {
			d["completed"] = completed
		}
		return d
	}
	cases := map[string]map[string]any{
		"completed omitted": detail(nil, false),
		"completed null":    detail(nil, true),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.jobFailAction["Block Sign In"] = true // the listed job reports failed:1
			api.jobDetail["JOB_1"] = body
			rqtEnv(t, api)
			rqtFastPoll(t)
			out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
				"--confirm", "--wait", "300ms", "--json")
			if rqtExit(err) == 0 {
				t.Fatalf("a known failure must exit non-zero:\n%s", out)
			}
			m := rqtDecode[rqtContainOut](t, out).Actions[0].JobMatch
			if m == nil || m.Failed == nil || *m.Failed != 1 || !strings.Contains(m.Warning, "failed=1") || !m.Completed {
				t.Fatalf("want the listed completion and failed=1 with a warning: %+v", m)
			}
		})
	}
}

func TestContainKnownFailureWarnsBeforeCompletion(t *testing.T) {
	api := newRQTAPI(t)
	api.jobFailAction["Block Sign In"] = true // failed:1 is known while the job still runs
	api.jobNoComplete = true
	rqtEnv(t, api)
	rqtFastPoll(t)
	out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
		"--confirm", "--wait", "100ms", "--json")
	if rqtExit(err) == 0 {
		t.Fatalf("a known failure on a still-running job must exit non-zero:\n%s", out)
	}
	m := rqtDecode[rqtContainOut](t, out).Actions[0].JobMatch
	if m == nil || m.Completed || !strings.Contains(m.Warning, "failed=1") {
		t.Fatalf("want an incomplete job with a failed=1 warning: %+v", m)
	}
}

func TestContainWorstCountsSurviveLaterSparsePolls(t *testing.T) {
	job := func(fields map[string]any) map[string]any {
		j := map[string]any{"id": "JOB_1", "createdAt": rqtMS(time.Minute) + 500, "total": 1, "action": "Block Sign In",
			"createdBy": map[string]any{"id": "OP_ME", "email": "operator@example.com"}}
		for k, v := range fields {
			j[k] = v
		}
		return j
	}
	cases := map[string]struct{ list, detail []map[string]any }{
		"poll 2 omits failed": {
			list:   []map[string]any{job(map[string]any{"success": 0, "failed": 1, "completed": false}), job(map[string]any{"success": 1, "completed": true})},
			detail: []map[string]any{job(map[string]any{"success": 0, "failed": 1, "completed": false}), job(map[string]any{"success": 1, "completed": true})},
		},
		"poll 2 nulls failed": {
			list:   []map[string]any{job(map[string]any{"success": 0, "failed": 1, "completed": false}), job(map[string]any{"success": 1, "failed": nil, "completed": true})},
			detail: []map[string]any{job(map[string]any{"success": 0, "failed": 1, "completed": false}), job(map[string]any{"success": 1, "failed": nil, "missed": nil, "completed": true})},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.jobListSeq["JOB_1"], api.jobDetailSeq["JOB_1"] = tc.list, tc.detail
			rqtEnv(t, api)
			rqtFastPoll(t)
			res, err := rqtContainBlock(t)
			if rqtExit(err) == 0 {
				t.Fatalf("a failure seen on an earlier poll must keep the exit non-zero")
			}
			m := res.Actions[0].JobMatch
			if m == nil || !m.Completed || m.Failed == nil || *m.Failed != 1 || !strings.Contains(m.Warning, "failed=1") {
				t.Fatalf("want the completed job with failed=1 and a warning: %+v", m)
			}
			if n := len(api.callsMatching("GET", "/respondApi/api/v1/response-actions/jobs/JOB_1")); n != 2 {
				t.Fatalf("detail reads = %d, want 2 polls", n)
			}
		})
	}
}

func TestContainWarningDroppedWhenAttributionTurnsAmbiguous(t *testing.T) {
	api := newRQTAPI(t)
	// An unrelated, still-running job that reports a failure shows up first;
	// our own job appears one listing later, so attribution turns ambiguous.
	api.jobDelay["Block Sign In"] = 1
	api.execHook = func(a *rqtAPI, action string, executedAt int64) {
		rqtAddJob(a, map[string]any{"id": "JOB_UNRELATED", "createdAt": executedAt + 300, "total": 1, "success": 0, "failed": 1,
			"completed": false, "action": action})
	}
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if err != nil {
		t.Fatalf("an ambiguous match leaves the accepted action at exit 0: %v", err)
	}
	m := res.Actions[0].JobMatch
	if m == nil || m.Confidence != "ambiguous" || m.Warning != "" || m.Failed != nil {
		t.Fatalf("the earlier warning must be dropped once attribution is ambiguous: %+v", m)
	}
}

func TestContainCandidateSwapAcrossPollsIsAmbiguous(t *testing.T) {
	api := newRQTAPI(t)
	// Listing 1 is the pre-call snapshot. Poll 1 (listing 2) shows only our
	// job A (JOB_1: failed:1, still running); poll 2 (listing 3) shows only
	// B, a completed same-action job from the same operator.
	api.jobFailAction["Block Sign In"] = true
	api.jobHideAt["JOB_1"] = 3
	api.execHook = func(a *rqtAPI, action string, executedAt int64) {
		rqtAddJob(a, map[string]any{"id": "JOB_B", "createdAt": executedAt + 800, "total": 1, "success": 1, "failed": 0,
			"completed": true, "action": action, "createdBy": a.execBy})
		a.jobVisibleAt["JOB_B"] = a.jobLists + 1
	}
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if err != nil {
		t.Fatalf("an ambiguous match leaves the accepted action at exit 0: %v", err)
	}
	m := res.Actions[0].JobMatch
	if m == nil || m.Confidence != "ambiguous" || strings.Join(m.JobIDs, ",") != "JOB_1,JOB_B" || m.Completed || m.Failed != nil || m.Warning != "" {
		t.Fatalf("two distinct candidates across polls must be ambiguous, not a match to JOB_B: %+v", m)
	}
	if n := len(api.callsMatching("GET", "/respondApi/api/v1/response-actions/jobs/JOB_B")); n != 0 {
		t.Fatalf("JOB_B was followed as if it were this call's job (%d detail reads)", n)
	}
}

// rqtJob1 is a JOB_1 body that passes the candidate checks for the first
// execute call (action, operator, time), plus the given fields.
func rqtJob1(fields map[string]any) map[string]any {
	j := map[string]any{"id": "JOB_1", "createdAt": rqtMS(time.Minute) + 500, "total": 1, "action": "Block Sign In",
		"createdBy": map[string]any{"id": "OP_ME", "email": "operator@example.com"}}
	for k, v := range fields {
		j[k] = v
	}
	return j
}

func TestContainFailureFlaggedJobResponsesAreNotEvidence(t *testing.T) {
	flaggedDone := map[string]any{"completed": true, "success": 1, "failed": 0, "ok": false, "error": "backend failure"}
	cases := []struct {
		name  string
		setup func(a *rqtAPI)
		check func(t *testing.T, a *rqtAPI, m *containJobMatch)
	}{
		{name: "detail with a failure indicator", setup: func(a *rqtAPI) {
			a.jobNoComplete = true // the listing never says completed
			a.jobDetail["JOB_1"] = rqtJob1(flaggedDone)
		}, check: func(t *testing.T, a *rqtAPI, m *containJobMatch) {
			if m.Confidence != "heuristic" || strings.Join(m.JobIDs, ",") != "JOB_1" {
				t.Fatalf("the vetted listing should still be the match: %+v", m)
			}
		}},
		{name: "list row with a failure indicator", setup: func(a *rqtAPI) {
			a.jobListSeq["JOB_1"] = []map[string]any{rqtJob1(flaggedDone)}
		}, check: func(t *testing.T, a *rqtAPI, m *containJobMatch) {
			if m.Confidence != "none" || len(m.JobIDs) != 0 {
				t.Fatalf("a flagged, never-vetted row must not be attributed: %+v", m)
			}
			if n := len(a.callsMatching("GET", "/respondApi/api/v1/response-actions/jobs/JOB_1")); n != 0 {
				t.Fatalf("the flagged row was followed (%d detail reads)", n)
			}
		}},
		{name: "list wrapper carrying an error after the call", setup: func(a *rqtAPI) {
			a.execHook = func(a *rqtAPI, action string, executedAt int64) {
				a.jobsBody = map[string]any{"jobs": []any{rqtJob1(map[string]any{"completed": true, "success": 1, "failed": 0})}, "error": "backend failure"}
			}
		}, check: func(t *testing.T, a *rqtAPI, m *containJobMatch) {}},
		{name: "list wrapper carrying an error in the pre-call snapshot", setup: func(a *rqtAPI) {
			a.jobsBody = map[string]any{"jobs": []any{}, "error": "backend failure"}
		}, check: func(t *testing.T, a *rqtAPI, m *containJobMatch) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			tc.setup(api)
			rqtEnv(t, api)
			rqtFastPoll(t)
			out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
				"--confirm", "--wait", "100ms", "--json")
			if rqtExit(err) == 0 {
				t.Fatalf("a failure-flagged job response must make the exit non-zero:\n%s", out)
			}
			res := rqtDecode[rqtContainOut](t, out)
			m := res.Actions[0].JobMatch
			if m == nil || m.Completed || !strings.Contains(m.Warning, "failure indicator") || res.Verified {
				t.Fatalf("no completion may come from a failure-flagged response, and it must warn: %+v", m)
			}
			tc.check(t, api, m)
		})
	}
}

func TestContainNumericSuccessIsNotAFailureFlag(t *testing.T) {
	api := newRQTAPI(t)
	api.jobListSeq["JOB_1"] = []map[string]any{rqtJob1(map[string]any{"completed": true, "success": 0, "failed": 0, "missed": 0})}
	api.jobDetail["JOB_1"] = rqtJob1(map[string]any{"completed": true, "success": 0, "failed": 0, "missed": 0})
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if err != nil {
		t.Fatalf("success:0 is a counter, not a failure flag: %v", err)
	}
	if m := res.Actions[0].JobMatch; m == nil || m.Confidence != "heuristic" || !m.Completed || m.Warning != "" {
		t.Fatalf("want a clean completed heuristic match: %+v", m)
	}
}

func TestContainBooleanSuccessFalseAndUndecodableJobsWarn(t *testing.T) {
	boolFalse := map[string]any{"completed": true, "success": false, "failed": 0, "error": "backend failure"}
	undecodable := map[string]any{"completed": "yes", "success": 1, "failed": 0}
	cases := []struct {
		name  string
		setup func(a *rqtAPI)
		want  string // warning fragment
	}{
		{name: "list row with success:false", want: "carried a failure indicator",
			setup: func(a *rqtAPI) { a.jobListSeq["JOB_1"] = []map[string]any{rqtJob1(boolFalse)} }},
		{name: "detail with success:false", want: "carried a failure indicator",
			setup: func(a *rqtAPI) { a.jobNoComplete = true; a.jobDetail["JOB_1"] = rqtJob1(boolFalse) }},
		{name: "undecodable list row", want: "could not be decoded",
			setup: func(a *rqtAPI) { a.jobListSeq["JOB_1"] = []map[string]any{rqtJob1(undecodable)} }},
		{name: "undecodable detail", want: "could not be decoded",
			setup: func(a *rqtAPI) { a.jobNoComplete = true; a.jobDetail["JOB_1"] = rqtJob1(undecodable) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			tc.setup(api)
			rqtEnv(t, api)
			rqtFastPoll(t)
			out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
				"--confirm", "--wait", "100ms", "--json")
			if rqtExit(err) == 0 {
				t.Fatalf("must exit non-zero, not drop to a note:\n%s", out)
			}
			m := rqtDecode[rqtContainOut](t, out).Actions[0].JobMatch
			if m == nil || m.Completed || !strings.Contains(m.Warning, tc.want) {
				t.Fatalf("want a warning containing %q and no completion: %+v", tc.want, m)
			}
		})
	}
}

// rqtJobFaults makes the job list, or one job's detail, answer with an HTTP
// status. It wraps the shared fake API's handler, so the fake stays as is.
type rqtJobFaults struct {
	mu     sync.Mutex
	list   int
	detail map[string]int
}

func rqtWrapJobFaults(api *rqtAPI) *rqtJobFaults {
	f := &rqtJobFaults{detail: map[string]int{}}
	inner := api.srv.Config.Handler
	const jobs = "/respondApi/api/v1/response-actions/jobs"
	api.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		list, detail := f.list, f.detail[strings.TrimPrefix(r.URL.Path, jobs+"/")]
		f.mu.Unlock()
		code := 0
		switch {
		case r.Method == http.MethodGet && r.URL.Path == jobs:
			code = list
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, jobs+"/"):
			code = detail
		}
		if code != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"job read unavailable"}`))
			return
		}
		inner.ServeHTTP(w, r)
	})
	return f
}

func (f *rqtJobFaults) set(fn func(f *rqtJobFaults)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// TestContainMonitoringReadProblemsWarn: once the action is accepted, a job
// read that does not answer clearly is a warning with a non-zero exit.
func TestContainMonitoringReadProblemsWarn(t *testing.T) {
	cases := []struct {
		name  string
		setup func(a *rqtAPI, f *rqtJobFaults)
		want  string
	}{
		{name: "job list answers success:false with an error", want: "failure indicator",
			setup: func(a *rqtAPI, f *rqtJobFaults) {
				a.execHook = func(a *rqtAPI, action string, executedAt int64) {
					a.jobsBody = map[string]any{"success": false, "error": "backend failure"}
				}
			}},
		{name: "job list answers success:false with jobs:null", want: "failure indicator",
			setup: func(a *rqtAPI, f *rqtJobFaults) {
				a.execHook = func(a *rqtAPI, action string, executedAt int64) {
					a.jobsBody = map[string]any{"success": false, "error": "backend failure", "jobs": nil}
				}
			}},
		{name: "job list answers an object with jobs:null", want: "expected a job array",
			setup: func(a *rqtAPI, f *rqtJobFaults) {
				a.execHook = func(a *rqtAPI, action string, executedAt int64) { a.jobsBody = map[string]any{"jobs": nil} }
			}},
		{name: "job list answers HTTP 500", want: "listing jobs after the call was accepted failed",
			setup: func(a *rqtAPI, f *rqtJobFaults) {
				a.execHook = func(a *rqtAPI, action string, executedAt int64) {
					f.set(func(f *rqtJobFaults) { f.list = http.StatusInternalServerError })
				}
			}},
		// 404 rather than 500 only to skip the client's 5xx retry backoff.
		{name: "job detail answers an HTTP error", want: "reading the job detail for JOB_1 failed",
			setup: func(a *rqtAPI, f *rqtJobFaults) {
				f.set(func(f *rqtJobFaults) { f.detail["JOB_1"] = http.StatusNotFound })
			}},
		{name: "job list unreadable before executing", want: "could not be read before executing",
			setup: func(a *rqtAPI, f *rqtJobFaults) {
				f.set(func(f *rqtJobFaults) { f.list = http.StatusNotFound })
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newRQTAPI(t)
			tc.setup(api, rqtWrapJobFaults(api))
			rqtEnv(t, api)
			rqtFastPoll(t)
			out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
				"--confirm", "--wait", "100ms", "--json")
			if rqtExit(err) == 0 {
				t.Fatalf("a monitoring read problem must exit non-zero, not drop to a note:\n%s", out)
			}
			res := rqtDecode[rqtContainOut](t, out)
			if res.Actions[0].Status != "submitted" {
				t.Fatalf("the action itself was accepted: %+v", res.Actions[0])
			}
			if m := res.Actions[0].JobMatch; m == nil || m.Completed || !strings.Contains(m.Warning, tc.want) {
				t.Fatalf("want a warning containing %q and no completion: %+v", tc.want, m)
			}
		})
	}
}

// TestContainVettedJobLaterFlaggedRowWarns: poll 1 vets JOB_1; poll 2 lists
// JOB_1 again with a failure indicator and unreadable metadata. That row is
// warning evidence for the vetted job whatever its metadata says.
func TestContainVettedJobLaterFlaggedRowWarns(t *testing.T) {
	clean := rqtJob1(map[string]any{"completed": false, "success": 0, "failed": 0})
	omitted := rqtJob1(map[string]any{"completed": true, "success": false, "error": "backend failure"})
	delete(omitted, "action")
	malformed := rqtJob1(map[string]any{"completed": true, "success": false, "error": "backend failure", "action": 5})
	for name, poll2 := range map[string]map[string]any{"action omitted": omitted, "action malformed": malformed} {
		t.Run(name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.jobNoComplete = true
			api.jobListSeq["JOB_1"] = []map[string]any{clean, poll2}
			rqtEnv(t, api)
			rqtFastPoll(t)
			out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
				"--confirm", "--wait", "100ms", "--json")
			res := rqtDecode[rqtContainOut](t, out)
			if rqtExit(err) == 0 {
				t.Fatalf("the vetted job's later failure must exit non-zero")
			}
			m := res.Actions[0].JobMatch
			if m == nil || m.Confidence != "heuristic" || strings.Join(m.JobIDs, ",") != "JOB_1" || m.Completed ||
				!strings.Contains(m.Warning, "the job list row for JOB_1") {
				t.Fatalf("want JOB_1 kept, not completed, with a warning about its row: %+v", m)
			}
		})
	}
}

func TestContainDetailForAnotherJobWarns(t *testing.T) {
	api := newRQTAPI(t)
	// The listing is clean and complete; the detail answers for another job.
	api.jobListSeq["JOB_1"] = []map[string]any{rqtJob1(map[string]any{"completed": true, "success": 1, "failed": 0, "missed": 0})}
	api.jobDetail["JOB_1"] = rqtJob1(map[string]any{"id": "JOB_OTHER", "completed": true, "success": 1, "failed": 0})
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if rqtExit(err) == 0 {
		t.Fatalf("a detail for a different job id must make the exit non-zero")
	}
	m := res.Actions[0].JobMatch
	if m == nil || strings.Join(m.JobIDs, ",") != "JOB_1" || !strings.Contains(m.Warning, "answered for a different job id") {
		t.Fatalf("want the vetted listing kept with a warning: %+v", m)
	}
}

func TestContainAmbiguityKeepsMonitoringProblems(t *testing.T) {
	api := newRQTAPI(t)
	faults := rqtWrapJobFaults(api)
	faults.set(func(f *rqtJobFaults) { f.detail["JOB_1"] = http.StatusNotFound })
	// Poll 1 vets JOB_1 (still running; its detail read fails). Poll 2 also
	// lists JOB_B, a second candidate, so attribution turns ambiguous.
	api.execHook = func(a *rqtAPI, action string, executedAt int64) {
		rqtAddJob(a, map[string]any{"id": "JOB_B", "createdAt": executedAt + 800, "total": 1, "success": 1, "failed": 0,
			"completed": true, "action": action, "createdBy": a.execBy})
		a.jobVisibleAt["JOB_B"] = a.jobLists + 1
	}
	rqtEnv(t, api)
	rqtFastPoll(t)
	res, err := rqtContainBlock(t)
	if rqtExit(err) == 0 {
		t.Fatalf("a monitoring problem collected before ambiguity must still make the exit non-zero")
	}
	m := res.Actions[0].JobMatch
	if m == nil || m.Confidence != "ambiguous" || strings.Join(m.JobIDs, ",") != "JOB_1,JOB_B" || m.Completed || m.Failed != nil ||
		!strings.Contains(m.Warning, "reading the job detail for JOB_1 failed") {
		t.Fatalf("want an ambiguous result that still warns about the failed read: %+v", m)
	}
}

func TestContainUnreadableNewFlaggedRowWarns(t *testing.T) {
	t.Run("action unreadable: cannot be ruled out", func(t *testing.T) {
		api := newRQTAPI(t)
		// The pre-call snapshot is empty; the first post-call listing shows a
		// new JOB_1 with matching time and operator, success:false, an error
		// and an unreadable action.
		api.jobListSeq["JOB_1"] = []map[string]any{rqtJob1(map[string]any{"completed": true, "success": false, "error": "backend failure", "action": 5})}
		rqtEnv(t, api)
		rqtFastPoll(t)
		out, _, err := rqtRun(t, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE",
			"--confirm", "--wait", "100ms", "--json")
		if rqtExit(err) == 0 {
			t.Fatalf("an unusable new row that cannot be ruled out must warn and exit non-zero:\n%s", out)
		}
		m := rqtDecode[rqtContainOut](t, out).Actions[0].JobMatch
		if m == nil || m.Confidence != "none" || len(m.JobIDs) != 0 || m.Completed || !strings.Contains(m.Warning, "cannot be read to rule it out") {
			t.Fatalf("want a warning and no attribution: %+v", m)
		}
	})
	t.Run("readable different action: ruled out and ignored", func(t *testing.T) {
		api := newRQTAPI(t)
		api.execHook = func(a *rqtAPI, action string, executedAt int64) {
			rqtAddJob(a, map[string]any{"id": "JOB_OTHER_ACTION", "createdAt": executedAt + 300, "total": 1, "success": false,
				"error": "backend failure", "completed": true, "action": "Delete User", "createdBy": a.execBy})
		}
		rqtEnv(t, api)
		rqtFastPoll(t)
		res, err := rqtContainBlock(t)
		if err != nil {
			t.Fatalf("a flagged row that is clearly another action must not affect the run: %v", err)
		}
		if m := res.Actions[0].JobMatch; m == nil || m.Confidence != "heuristic" || strings.Join(m.JobIDs, ",") != "JOB_1" || m.Warning != "" {
			t.Fatalf("want a clean heuristic match to JOB_1: %+v", m)
		}
	})
}
