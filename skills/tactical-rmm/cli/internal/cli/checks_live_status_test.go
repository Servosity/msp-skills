// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): tactical-rmm handfix check-status-from-agent-checks.

package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tactical-rmm-pp-cli/internal/store"
)

// Tactical RMM keeps a check's status on the per-agent CheckResult, not on the
// Check row. GET /checks/ (CheckSerializer over Check.objects.filter_by_role)
// carries no status, and its check_result is {} because no agent's result is
// attached there (vendor-src/checks/views.py GetAddChecks.get). Only GET
// /agents/{agent_id}/checks/ (agent.get_checks_with_policies()) attaches the
// agent's result, nested as check_result.status. These tests serve the vendor
// shapes and fail when a check report reads a status the source never sends.

// checkStub is a fake Tactical RMM that serves the agent list, the bare check
// list, and each agent's checks with results. It records every path it saw.
type checkStub struct {
	mu       sync.Mutex
	agents   string
	checks   string
	perAgent map[string]stubReply
	seen     map[string]int
}

type stubReply struct {
	status int
	body   string
}

func (s *checkStub) set(agentID string, status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.perAgent[agentID] = stubReply{status, body}
}

func (s *checkStub) hits(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[path]
}

func newCheckStub(t *testing.T) *checkStub {
	t.Helper()
	s := &checkStub{perAgent: map[string]stubReply{}, seen: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.seen[r.URL.Path]++
		agents, checks := s.agents, s.checks
		per := s.perAgent
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch {
		case r.URL.Path == "/agents/":
			_, _ = w.Write([]byte(agents))
		case r.URL.Path == "/checks/":
			_, _ = w.Write([]byte(checks))
		case strings.HasPrefix(r.URL.Path, "/agents/") && strings.HasSuffix(r.URL.Path, "/checks/"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/agents/"), "/checks/")
			s.mu.Lock()
			rep, ok := per[id]
			s.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"detail":"Not found."}`))
				return
			}
			if rep.status != 0 {
				w.WriteHeader(rep.status)
			}
			_, _ = w.Write([]byte(rep.body))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TACTICAL_RMM_BASE_URL", srv.URL)
	t.Setenv("TRMM_API_KEY", "test-key")
	return s
}

// Agent rows in AgentTableSerializer shape (the fields the reports read).
const checkStubAgents = `[
 {"agent_id":"AGENTID001","hostname":"HOST1","client_name":"Acme","site_name":"HQ","status":"online","checks":{"total":3,"passing":2,"failing":1,"warning":0,"info":0,"has_failing_checks":true}},
 {"agent_id":"AGENTID002","hostname":"HOST2","client_name":"Acme","site_name":"Branch","status":"offline","checks":{"total":3,"passing":3,"failing":0,"warning":0,"info":0,"has_failing_checks":false}},
 {"agent_id":"AGENTID003","hostname":"HOST3","client_name":"Beta","site_name":"Main","status":"overdue","checks":{"total":3,"passing":1,"failing":2,"warning":0,"info":0,"has_failing_checks":true}},
 {"agent_id":"AGENTID004","hostname":"HOST4","client_name":"Beta","site_name":"Main","status":"online","checks":{"total":3,"passing":3,"failing":0,"warning":0,"info":0,"has_failing_checks":false}}]`

// GET /checks/ rows: no status anywhere a top-level read could find it; one
// carries check_result {} and one a check_result.status, which must not be
// trusted either (it is not the per-agent result the reports need).
const checkStubCheckList = `[
 {"id":5,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","name":null,"check_result":{},"agent":1,"policy":null},
 {"id":6,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","name":null,"check_result":{"id":60,"status":"failing","agent":3,"assigned_check":6},"agent":3,"policy":null},
 {"id":7,"readable_desc":"Ping: gw","check_type":"ping","name":null,"check_result":{},"agent":3,"policy":null}]`

type worstEnvelope struct {
	Items []struct {
		Check         string   `json:"check"`
		Type          string   `json:"check_type"`
		AgentsFailing int      `json:"agents_failing"`
		Agents        []string `json:"agents"`
	} `json:"items"`
	ScannedAgents   int    `json:"scanned_agents"`
	CandidateAgents int    `json:"candidate_agents"`
	FetchFailures   int    `json:"fetch_failures"`
	StatusUnknown   int    `json:"status_unknown"`
	FirstError      string `json:"first_error"`
	Note            string `json:"note"`
}

func decodeWorst(t *testing.T, out string) worstEnvelope {
	t.Helper()
	var probe any
	assertJSON(t, out, &probe)
	if _, ok := probe.(map[string]any); !ok {
		t.Fatalf("checks worst printed %s; want an envelope {items, scanned_agents, fetch_failures} built from each agent's check results", strings.TrimSpace(out))
	}
	var w worstEnvelope
	assertJSON(t, out, &w)
	return w
}

// The executed repro: after a real sync, fleet health reports 3 failing checks
// on 2 agents, and checks worst must rank those failures, not print [].
func TestChecksWorstRanksPerAgentCheckResults(t *testing.T) {
	novelTestEnv(t)
	stub := newCheckStub(t)
	stub.agents = checkStubAgents
	stub.checks = checkStubCheckList
	stub.set("AGENTID001", 0, `[
 {"id":5,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","name":null,"check_result":{"id":50,"status":"failing","agent":1,"assigned_check":5}},
 {"id":8,"readable_desc":"CPU Load > 90%","check_type":"cpuload","name":null,"check_result":{"id":80,"status":"passing","agent":1,"assigned_check":8}},
 {"id":9,"readable_desc":"Ping: gw","check_type":"ping","name":null,"check_result":{}}]`)
	stub.set("AGENTID003", 0, `[
 {"id":6,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","name":null,"check_result":{"id":60,"status":"failing","agent":3,"assigned_check":6}},
 {"id":7,"readable_desc":"Ping: gw","check_type":"ping","name":null,"check_result":{"id":70,"status":"failing","agent":3,"assigned_check":7}},
 {"id":10,"readable_desc":"","check_type":"memory","name":null,"check_result":{"id":100,"status":"passing","agent":3,"assigned_check":10}}]`)
	stub.set("AGENTID002", 0, `[{"id":11,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","check_result":{"status":"passing"}}]`)
	stub.set("AGENTID004", 0, `[{"id":12,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","check_result":{"status":"passing"}}]`)

	if _, stderr, err := runNovel(t, "sync", "--resources", "agents,checks"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	out, stderr, err := runNovel(t, "fleet", "health")
	if err != nil {
		t.Fatalf("fleet health: %v\n%s", err, stderr)
	}
	var fh struct {
		Checks struct {
			Failing           int `json:"failing"`
			AgentsWithFailing int `json:"agents_with_failing"`
		} `json:"checks"`
	}
	assertJSON(t, out, &fh)
	if fh.Checks.Failing != 3 || fh.Checks.AgentsWithFailing != 2 {
		t.Fatalf("fleet health checks = %+v, want 3 failing on 2 agents (fixture drift)", fh.Checks)
	}

	out, stderr, err = runNovel(t, "checks", "worst")
	if err != nil {
		t.Fatalf("checks worst: %v\n%s", err, stderr)
	}
	w := decodeWorst(t, out)
	if len(w.Items) != 2 {
		t.Fatalf("checks worst items = %+v, want Disk Space (2 agents) then Ping (1 agent)", w.Items)
	}
	if w.Items[0].Check != "Disk Space Drive C: > 90%" || w.Items[0].Type != "diskspace" || w.Items[0].AgentsFailing != 2 {
		t.Errorf("top item = %+v, want Disk Space Drive C: > 90%% / diskspace failing on 2 agents", w.Items[0])
	}
	if strings.Join(w.Items[0].Agents, ",") != "HOST1,HOST3" {
		t.Errorf("top item agents = %v, want [HOST1 HOST3]", w.Items[0].Agents)
	}
	if w.Items[1].Check != "Ping: gw" || w.Items[1].AgentsFailing != 1 {
		t.Errorf("second item = %+v, want Ping: gw failing on 1 agent (HOST1's Ping has no result: unknown, not failing)", w.Items[1])
	}
	if w.ScannedAgents != 2 || w.CandidateAgents != 2 || w.FetchFailures != 0 || w.StatusUnknown != 1 {
		t.Errorf("envelope = scanned %d candidates %d failures %d unknown %d, want 2/2/0/1", w.ScannedAgents, w.CandidateAgents, w.FetchFailures, w.StatusUnknown)
	}
	for _, p := range []string{"/agents/AGENTID001/checks/", "/agents/AGENTID003/checks/"} {
		if stub.hits(p) != 1 {
			t.Errorf("GET %s seen %d times, want 1", p, stub.hits(p))
		}
	}
	for _, p := range []string{"/agents/AGENTID002/checks/", "/agents/AGENTID004/checks/"} {
		if stub.hits(p) != 0 {
			t.Errorf("GET %s fetched for an agent with no failing checks", p)
		}
	}

	// --max-scan-agents caps the fan-out and says so.
	out, stderr, err = runNovel(t, "checks", "worst", "--max-scan-agents", "1")
	if err != nil {
		t.Fatalf("checks worst --max-scan-agents 1: %v\n%s", err, stderr)
	}
	w = decodeWorst(t, out)
	if w.ScannedAgents != 1 || w.CandidateAgents != 2 || !strings.Contains(w.Note, "max-scan-agents") {
		t.Errorf("capped envelope = scanned %d candidates %d note %q, want 1/2 and a cap note", w.ScannedAgents, w.CandidateAgents, w.Note)
	}
}

// A fetch error or a body that is not a check array is a failure, never an
// empty result: all failed is an error, some failed is a warning.
func TestChecksWorstFetchFailuresAreNotEmptyResults(t *testing.T) {
	db := novelTestEnv(t)
	seedResource(t, db, "agents", "a1", `{"agent_id":"a1","hostname":"h1","checks":{"total":1,"failing":1}}`)
	seedResource(t, db, "agents", "a2", `{"agent_id":"a2","hostname":"h2","checks":{"total":1,"failing":1}}`)
	seedResource(t, db, "agents", "a3", `{"agent_id":"a3","hostname":"h3","checks":{"total":1,"failing":1}}`)
	stub := newCheckStub(t)
	stub.set("a1", http.StatusForbidden, `{"detail":"You do not have permission to perform this action."}`)
	stub.set("a2", 0, `{"detail":"not a list"}`)
	stub.set("a3", 0, `null`)

	out, _, err := runNovel(t, "checks", "worst")
	if err == nil {
		t.Fatalf("checks worst succeeded with every fetch failed; output %s", strings.TrimSpace(out))
	}
	w := decodeWorst(t, out)
	if w.FetchFailures != 3 || w.ScannedAgents != 3 || len(w.Items) != 0 || w.FirstError == "" {
		t.Errorf("all-failed envelope = %+v, want 3 of 3 failures, a first_error, no items", w)
	}

	stub.set("a3", 0, `[{"id":1,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","check_result":{"status":"failing"}}]`)
	out, stderr, err := runNovel(t, "checks", "worst")
	if err != nil {
		t.Fatalf("checks worst with one good agent: %v\n%s", err, stderr)
	}
	w = decodeWorst(t, out)
	if w.FetchFailures != 2 || len(w.Items) != 1 || w.Items[0].AgentsFailing != 1 {
		t.Errorf("partial envelope = %+v, want 2 failures and Disk Space failing on 1 agent", w)
	}
	if !strings.Contains(stderr, "2 of 3") {
		t.Errorf("stderr = %q, want a warning naming 2 of 3 failed fetches", stderr)
	}
}

// --snapshot records each agent's real check status, skips checks with no
// result, and two snapshots with a changed status are reported as a flip.
func TestChecksFlappingSnapshotRecordsPerAgentStatus(t *testing.T) {
	db := novelTestEnv(t)
	seedResource(t, db, "agents", "a1", `{"agent_id":"a1","hostname":"h1","checks":{"total":2,"failing":0}}`)
	seedResource(t, db, "agents", "a2", `{"agent_id":"a2","hostname":"h2","checks":{"total":1,"failing":1}}`)
	// What sync stores from GET /checks/: no status.
	seedResource(t, db, "checks", "5", `{"id":5,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","check_result":{},"agent":1}`)
	stub := newCheckStub(t)
	stub.set("a1", 0, `[
 {"id":5,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","check_result":{"status":"passing"}},
 {"id":9,"readable_desc":"Ping: gw","check_type":"ping","check_result":{}}]`)
	stub.set("a2", 0, `[{"id":6,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","check_result":{"status":"failing"}}]`)

	out, stderr, err := runNovel(t, "checks", "flapping", "--snapshot")
	if err != nil {
		t.Fatalf("checks flapping --snapshot: %v\n%s", err, stderr)
	}
	var res map[string]any
	assertJSON(t, out, &res)
	if res["snapshot_recorded"] != float64(2) {
		t.Errorf("snapshot_recorded = %v, want 2 (a1 Disk passing, a2 Disk failing; a1 Ping has no result)", res["snapshot_recorded"])
	}

	s, err := store.Open(db)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	got := map[string]string{}
	rows, err := s.DB().Query("SELECT check_id, agent_id, status FROM check_snapshots")
	if err != nil {
		t.Fatalf("read check_snapshots: %v", err)
	}
	for rows.Next() {
		var c, a, st string
		if err := rows.Scan(&c, &a, &st); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[c+"@"+a] = st
	}
	_ = rows.Close()
	_ = s.Close()
	want := map[string]string{"5@a1": "passing", "6@a2": "failing"}
	if len(got) != len(want) || got["5@a1"] != "passing" || got["6@a2"] != "failing" {
		t.Fatalf("check_snapshots = %v, want %v (no empty or unknown statuses)", got, want)
	}

	stub.set("a1", 0, `[{"id":5,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","check_result":{"status":"failing"}}]`)
	if _, stderr, err := runNovel(t, "checks", "flapping", "--snapshot"); err != nil {
		t.Fatalf("second snapshot: %v\n%s", err, stderr)
	}
	out, stderr, err = runNovel(t, "checks", "flapping", "--min-flips", "1", "--window", "24h")
	if err != nil {
		t.Fatalf("checks flapping: %v\n%s", err, stderr)
	}
	var flips []map[string]any
	assertJSON(t, out, &flips)
	if len(flips) != 1 || flips[0]["check_id"] != "5" || flips[0]["agent_id"] != "a1" || flips[0]["flips"] != float64(1) {
		t.Errorf("flapping = %v, want check 5 on a1 with 1 flip", flips)
	}

	// Every fetch failing is an error, not "0 recorded".
	stub.set("a1", http.StatusForbidden, `{"detail":"denied"}`)
	stub.set("a2", 0, `{"detail":"not a list"}`)
	if out, _, err := runNovel(t, "checks", "flapping", "--snapshot"); err == nil {
		t.Errorf("snapshot with every fetch failed succeeded: %s", strings.TrimSpace(out))
	}
}

// Tactical RMM's agent checks summary counts a failing check under failing,
// warning or info by its alert severity (has_failing_checks is failing or
// warning); ping, service and event-log checks default to warning. An agent
// whose failing checks are all warning or info severity is still a candidate,
// and every failing status counts the same.
func TestChecksWorstScansWarningAndInfoSeverityFailures(t *testing.T) {
	db := novelTestEnv(t)
	seedResource(t, db, "agents", "w1", `{"agent_id":"w1","hostname":"hw1","checks":{"total":2,"passing":1,"failing":0,"warning":1,"info":0,"has_failing_checks":true}}`)
	seedResource(t, db, "agents", "w2", `{"agent_id":"w2","hostname":"hw2","checks":{"total":1,"passing":0,"failing":0,"warning":1,"info":0,"has_failing_checks":true}}`)
	seedResource(t, db, "agents", "i1", `{"agent_id":"i1","hostname":"hi1","checks":{"total":1,"passing":0,"failing":0,"warning":0,"info":1,"has_failing_checks":false}}`)
	seedResource(t, db, "agents", "e1", `{"agent_id":"e1","hostname":"he1","checks":{"total":1,"passing":0,"failing":1,"warning":0,"info":0,"has_failing_checks":true}}`)
	seedResource(t, db, "agents", "ok", `{"agent_id":"ok","hostname":"hok","checks":{"total":1,"passing":1,"failing":0,"warning":0,"info":0,"has_failing_checks":false}}`)
	svc := `{"id":%d,"readable_desc":"Service Check - Spooler","check_type":"winsvc","alert_severity":"warning","check_result":{"status":"failing","alert_severity":"warning"}}`
	stub := newCheckStub(t)
	stub.set("w1", 0, "["+fmt.Sprintf(svc, 1)+`,{"id":2,"readable_desc":"Ping: gw","check_type":"ping","check_result":{"status":"passing"}}]`)
	stub.set("w2", 0, "["+fmt.Sprintf(svc, 3)+"]")
	stub.set("i1", 0, "["+fmt.Sprintf(svc, 4)+"]")
	stub.set("e1", 0, `[{"id":5,"readable_desc":"Disk Space Drive C: > 90%","check_type":"diskspace","check_result":{"status":"failing","alert_severity":"error"}}]`)

	out, stderr, err := runNovel(t, "checks", "worst")
	if err != nil {
		t.Fatalf("checks worst: %v\n%s", err, stderr)
	}
	w := decodeWorst(t, out)
	if w.CandidateAgents != 4 || w.ScannedAgents != 4 {
		t.Errorf("candidates %d scanned %d, want 4/4 (every agent with a failing check of any severity, not the passing one)", w.CandidateAgents, w.ScannedAgents)
	}
	if len(w.Items) != 2 || w.Items[0].Check != "Service Check - Spooler" || w.Items[0].AgentsFailing != 3 {
		t.Errorf("items = %+v, want Service Check - Spooler failing on 3 agents first, then the disk check", w.Items)
	}
}
