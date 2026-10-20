// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

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
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"saas-alerts-pp-cli/internal/cliutil/testenv"
	"saas-alerts-pp-cli/internal/store"
)

// rqtAPIKey is the partner key the tests configure; rqtTokens are the
// Respond access tokens the fake mints, in order. All are identifier-shaped
// so a filter that only hides odd-looking strings would still leak them.
const rqtAPIKey = "PartnerKeyQ9Z"

var rqtTokens = []string{"MintedTokenA1", "MintedTokenB2"}

// rqtNull makes the fake API answer with a literal JSON null body.
var rqtNull = json.RawMessage("null")

// rqtList is a fake collection body: unset means an empty array.
func rqtList(v any) any {
	if v == nil {
		return []any{}
	}
	return v
}

// rqtNow is the pinned clock every Respond test computes deadlines against.
var rqtNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func rqtMS(d time.Duration) int64 { return rqtNow.Add(d).UnixMilli() }

func rqtPinClock(t *testing.T) {
	t.Helper()
	prev := rqNow
	rqNow = func() time.Time { return rqtNow }
	t.Cleanup(func() { rqNow = prev })
}

func rqtRule(id, name, severity string, actions ...string) map[string]any {
	ras := make([]any, 0, len(actions))
	for _, a := range actions {
		ras = append(ras, map[string]any{"applicationType": "MS", "responseAction": a})
	}
	return map[string]any{
		"id": id, "name": name, "alertSeverity": severity, "ruleTriggerType": "MANUAL",
		"responseActions": ras,
		// Shape check: the spec's rule.accounts is an object, not an array.
		"accounts": map[string]any{"accounts": []any{}, "type": "ALL"},
	}
}

func rqtAccount(id, email string) []any {
	return []any{map[string]any{"id": id, "email": email, "fullName": email, "product": map[string]any{"type": "MS"}}}
}

// rqtTriggers is the Respond fixture set, in the order the fake API lists them.
//
//	RT_A2  PENDING  RULE_A  -1h            -> expires +11h (12h)
//	RT_B   PENDING  RULE_B  -11h30m        -> expires +30m (12h)
//	RT_C   REMEDIATION_FAILED, failed -30m, 2 reminders -> auto-ignore +15m
//	RT_A   PENDING  RULE_A  -3h, RT_A2 newer -> superseded, expired -1h
//	RT_D   PENDING  RULE_D  -2h, fires Delete User -> expires +10h
//	RT_E   PENDING  RULE_E  no triggeredAt, createdAt -14h -> expired -2h
//	RT_OLD APPROVED RULE_A  -5h (excluded by the default status filter)
func rqtTriggers() []map[string]any {
	return []map[string]any{
		{"id": "RT_A2", "status": "PENDING", "triggeredAt": rqtMS(-time.Hour), "createdAt": rqtMS(-time.Hour),
			"organization": map[string]any{"id": "ORG_1", "name": "Contoso"},
			"accounts":     rqtAccount("ACC_JANE", "jane.doe@example.com"),
			"rule":         rqtRule("RULE_A", "Impossible travel", "critical", "Block Sign In"),
			"history":      []any{map[string]any{"type": "TRIGGERED", "timestamp": rqtMS(-time.Hour)}}},
		{"id": "RT_B", "status": "PENDING", "triggeredAt": rqtMS(-11*time.Hour - 30*time.Minute),
			"organization": map[string]any{"id": "ORG_1", "name": "Contoso"},
			"accounts":     rqtAccount("ACC_BOB", "bob@example.com"),
			"rule":         rqtRule("RULE_B", "Mass download", "medium", "Expire Account Logins")},
		{"id": "RT_C", "status": "REMEDIATION_FAILED", "triggeredAt": rqtMS(-40 * time.Minute), "updatedAt": rqtMS(0),
			"organization": map[string]any{"id": "ORG_2", "name": "Fabrikam"},
			"accounts":     rqtAccount("ACC_CAROL", "carol@example.com"),
			"rule":         rqtRule("RULE_C", "Inbox forwarding rule", "critical", "Block Sign In"),
			"history": []any{
				map[string]any{"type": "TRIGGERED", "timestamp": rqtMS(-40 * time.Minute)},
				map[string]any{"type": "APPROVED", "timestamp": rqtMS(-35 * time.Minute)},
				map[string]any{"type": "REMEDIATION_FAILED", "timestamp": rqtMS(-30 * time.Minute),
					"details": []any{map[string]any{"applicationType": "MS", "action": "Block Sign In", "status": "FAILED", "error": map[string]any{"message": "connection broken"}}}},
				map[string]any{"type": "REMEDIATION_FAILED_REMINDER", "timestamp": rqtMS(-15 * time.Minute)},
				map[string]any{"type": "REMEDIATION_FAILED_REMINDER", "timestamp": rqtMS(0)},
			}},
		{"id": "RT_A", "status": "PENDING", "triggeredAt": rqtMS(-3 * time.Hour),
			"organization": map[string]any{"id": "ORG_1", "name": "Contoso"},
			"accounts":     rqtAccount("ACC_JANE", "jane.doe@example.com"),
			"rule":         rqtRule("RULE_A", "Impossible travel", "critical", "Block Sign In")},
		{"id": "RT_D", "status": "PENDING", "triggeredAt": rqtMS(-2 * time.Hour),
			"organization": map[string]any{"id": "ORG_1", "name": "Contoso"},
			"accounts":     rqtAccount("ACC_DAVE", "dave@example.com"),
			"rule":         rqtRule("RULE_D", "Compromised account", "critical", "Block Sign In", "Delete User")},
		{"id": "RT_E", "status": "PENDING", "createdAt": rqtMS(-14 * time.Hour),
			"organization": map[string]any{"id": "ORG_2", "name": "Fabrikam"},
			"accounts":     rqtAccount("ACC_ERIN", "erin@example.com"),
			"rule":         rqtRule("RULE_E", "Legacy authentication", "low", "Alert Only")},
		{"id": "RT_OLD", "status": "APPROVED", "triggeredAt": rqtMS(-5 * time.Hour),
			"organization": map[string]any{"id": "ORG_1", "name": "Contoso"},
			"accounts":     rqtAccount("ACC_JANE", "jane.doe@example.com"),
			"rule":         rqtRule("RULE_A", "Impossible travel", "critical", "Block Sign In")},
	}
}

type rqtCall struct {
	Method string
	Path   string
	Query  map[string][]string
	Body   string
}

// rqtAPI is a fake SaaS Alerts backend covering the Respond and Webhooks
// endpoints these commands call. It records every request.
type rqtAPI struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	calls    []rqtCall
	triggers []map[string]any
	pageSize int
	// decisionFail maps a trigger id to the HTTP status its decision POST returns.
	decisionFail map[string]int

	jobs          []map[string]any
	jobGets       map[string]int
	jobFailAction map[string]bool
	hideJobs      bool
	execCount     int
	// execBy is the executedBy identity the execute call reports and the
	// createdBy of the job it creates.
	execBy map[string]any
	// jobOverride merges fields into the job created for an action.
	jobOverride map[string]map[string]any
	// jobDelay hides an action's own job for that many job listings.
	jobDelay map[string]int
	// jobVisibleAt maps a job id to the listing count it first appears at.
	jobVisibleAt map[string]int
	jobLists     int
	// execBody/execCode, when set, replace the execute response (and no job
	// is created).
	execBody any
	execCode int
	// execHook can add other jobs (other operators, action-less, ...) when an
	// action executes; it runs under the lock.
	execHook func(a *rqtAPI, action string, executedAt int64)

	// decisionBody replaces a decision POST's 200 answer; the decision only
	// changes the trigger when decisionApplies is set for that id.
	decisionBody    map[string]any
	decisionApplies map[string]bool
	// Token minting: currentToken is the accepted one; rejectFirstToken
	// refuses rqtTokens[0] once so the client re-mints rqtTokens[1].
	currentToken     string
	mints            int
	rejectFirstToken bool
	// decisionStatus overrides the status a decision leaves; decisionLag keeps
	// the status and only appends the history entry.
	decisionStatus map[string]string
	decisionLag    map[string]bool
	// triggerGet overrides the 200 body of GET /rule-triggers/{id}.
	triggerGet map[string]any
	// rules are the GET /rules/{id} bodies; an unknown id is a 404.
	rules map[string]any
	// jobListSeq / jobDetailSeq give a job a different body on each list
	// read or detail read (the last one repeats).
	jobListSeq   map[string][]map[string]any
	jobDetailSeq map[string][]map[string]any
	jobListReads map[string]int
	// jobHideAt drops a job from listings from that listing count on.
	jobHideAt map[string]int
	// jobNoComplete keeps every job running (never completed).
	jobNoComplete bool
	// jobDetail replaces a GET /response-actions/jobs/{id} body (the listed
	// job still completes on its second detail read).
	jobDetail map[string]any
	// jobsBody, when set, replaces the GET /response-actions/jobs body.
	jobsBody any
	// listResponses, when set, are served in order by GET /rule-triggers.
	listResponses []any
	listCalls     int

	// Collections default to [] when unset; set one to rqtNull to serve a
	// literal JSON null.
	subs         any
	disabledSubs any
	domains      any
	domainsCode  int
	queue        any
	channelQueue map[string]any
	patchFail    map[string]int
	// onPatch runs (under the lock) when a PATCH arrives, before it is
	// answered; tests use it to change what later reads return.
	onPatch func(a *rqtAPI, ch string)
	// patchBody replaces a PATCH answer; the subscription is then only
	// enabled when patchApplies is set for it.
	patchBody    map[string]any
	patchApplies map[string]bool
	// testEventResp is the sendTestEvent 200 body (default true).
	testEventResp any
}

func newRQTAPI(t *testing.T) *rqtAPI {
	t.Helper()
	api := &rqtAPI{
		t:               t,
		triggers:        rqtTriggers(),
		pageSize:        3,
		decisionFail:    map[string]int{},
		jobGets:         map[string]int{},
		jobFailAction:   map[string]bool{},
		channelQueue:    map[string]any{},
		triggerGet:      map[string]any{},
		decisionBody:    map[string]any{},
		decisionApplies: map[string]bool{},
		rules:           map[string]any{},
		jobDetail:       map[string]any{},
		jobListSeq:      map[string][]map[string]any{},
		jobDetailSeq:    map[string][]map[string]any{},
		jobListReads:    map[string]int{},
		jobHideAt:       map[string]int{},
		execBy:          map[string]any{"id": "OP_ME", "email": "operator@example.com"},
		jobOverride:     map[string]map[string]any{},
		jobDelay:        map[string]int{},
		jobVisibleAt:    map[string]int{},
		decisionStatus:  map[string]string{},
		decisionLag:     map[string]bool{},
		patchFail:       map[string]int{},
		patchBody:       map[string]any{},
		patchApplies:    map[string]bool{},
		testEventResp:   true,
	}
	api.srv = httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(api.srv.Close)
	return api
}

func (a *rqtAPI) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *rqtAPI) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, rqtCall{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: string(body)})

	const respond = "/respondApi/api/v1"
	const webhooks = "/outgoingWebhookApi/api/v1"
	p := r.URL.Path
	if strings.HasPrefix(p, respond) && p != respond+"/auth" {
		tok := r.Header.Get("accesstoken")
		if a.rejectFirstToken && tok == rqtTokens[0] {
			// The first minted token is refused once, so the client re-mints:
			// both a previous and a current token are then real values.
			a.writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "Access token is invalid"})
			return
		}
		if tok == "" || tok != a.currentToken {
			a.writeJSON(w, http.StatusBadRequest, map[string]any{"message": "Only secure origins are allowed"})
			return
		}
	}
	switch {
	case r.Method == http.MethodPost && p == respond+"/auth":
		if r.Header.Get("api_key") != rqtAPIKey {
			a.writeJSON(w, http.StatusUnauthorized, map[string]any{"message": "bad api key"})
			return
		}
		a.currentToken = rqtTokens[min(a.mints, len(rqtTokens)-1)]
		a.mints++
		a.writeJSON(w, http.StatusOK, map[string]any{"accessToken": a.currentToken})
	case r.Method == http.MethodGet && p == respond+"/rule-triggers":
		a.listTriggers(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(p, respond+"/rule-triggers/"):
		id := strings.TrimPrefix(p, respond+"/rule-triggers/")
		if body, ok := a.triggerGet[id]; ok {
			a.writeJSON(w, http.StatusOK, body)
			return
		}
		if tr := a.findTrigger(id); tr != nil {
			a.writeJSON(w, http.StatusOK, map[string]any{"ruleTrigger": tr, "events": []any{}})
			return
		}
		a.writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
	case r.Method == http.MethodPost && strings.HasPrefix(p, respond+"/rule-triggers/"):
		rest := strings.TrimPrefix(p, respond+"/rule-triggers/")
		id, decision, _ := strings.Cut(rest, "/")
		if code := a.decisionFail[id]; code != 0 {
			a.writeJSON(w, code, map[string]any{"message": "rule trigger cannot be decided"})
			return
		}
		tr := a.findTrigger(id)
		if tr == nil {
			a.writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
			return
		}
		next := map[string]string{"approve": "APPROVED", "reject": "REJECTED", "ignore": "IGNORED", "manually-remediate": "MANUALLY_REMEDIATED"}[decision]
		if body, ok := a.decisionBody[id]; ok {
			if a.decisionApplies[id] {
				tr["status"] = next
			}
			a.writeJSON(w, http.StatusOK, body)
			return
		}
		if a.decisionLag[id] {
			hist, _ := tr["history"].([]any)
			tr["history"] = append(hist, map[string]any{"type": next, "timestamp": rqtMS(0)})
		} else if override := a.decisionStatus[id]; override != "" {
			tr["status"] = override
		} else {
			tr["status"] = next
		}
		a.writeJSON(w, http.StatusOK, tr)
	case r.Method == http.MethodPost && p == respond+"/response-actions/execute":
		var req struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(body, &req)
		if a.execBody != nil {
			code := a.execCode
			if code == 0 {
				code = http.StatusOK
			}
			a.writeJSON(w, code, a.execBody)
			return
		}
		a.execCount++
		executedAt := rqtMS(time.Duration(a.execCount) * time.Minute)
		failed := 0
		if a.jobFailAction[req.Action] {
			failed = 1
		}
		id := fmt.Sprintf("JOB_%d", a.execCount)
		job := map[string]any{"id": id, "createdAt": executedAt + 500, "updatedAt": executedAt + 500, "total": 1,
			"success": 1 - failed, "failed": failed, "missed": nil, "completed": false, "action": req.Action,
			"createdBy": a.execBy, "partnerId": "PARTNER_1"}
		for k, v := range a.jobOverride[req.Action] {
			job[k] = v
		}
		a.jobVisibleAt[id] = a.jobLists + a.jobDelay[req.Action]
		a.jobs = append(a.jobs, job)
		if a.execHook != nil {
			a.execHook(a, req.Action, executedAt)
		}
		a.writeJSON(w, http.StatusAccepted, map[string]any{"executedAt": executedAt, "status": "PROCESSING", "eventIds": []string{"EVENT_1"},
			"executedBy": a.execBy, "executedByIp": "203.0.113.7"})
	case r.Method == http.MethodGet && strings.HasPrefix(p, respond+"/rules/"):
		if body, ok := a.rules[strings.TrimPrefix(p, respond+"/rules/")]; ok {
			a.writeJSON(w, http.StatusOK, body)
			return
		}
		a.writeJSON(w, http.StatusNotFound, map[string]any{"message": "rule not found"})
	case r.Method == http.MethodGet && p == respond+"/response-actions/jobs" && a.jobsBody != nil:
		a.writeJSON(w, http.StatusOK, a.jobsBody)
	case r.Method == http.MethodGet && p == respond+"/response-actions/jobs":
		a.jobLists++
		visible := make([]map[string]any, 0, len(a.jobs))
		for _, j := range a.jobs {
			if a.hideJobs || a.jobLists <= a.jobVisibleAt[fmt.Sprint(j["id"])] {
				continue
			}
			if at := a.jobHideAt[fmt.Sprint(j["id"])]; at > 0 && a.jobLists >= at {
				continue
			}
			id := fmt.Sprint(j["id"])
			if seq := a.jobListSeq[id]; len(seq) > 0 {
				n := a.jobListReads[id]
				a.jobListReads[id]++
				visible = append(visible, seq[min(n, len(seq)-1)])
				continue
			}
			visible = append(visible, j)
		}
		a.writeJSON(w, http.StatusOK, visible)
	case r.Method == http.MethodGet && strings.HasPrefix(p, respond+"/response-actions/jobs/"):
		id := strings.TrimPrefix(p, respond+"/response-actions/jobs/")
		for _, j := range a.jobs {
			if j["id"] == id {
				a.jobGets[id]++
				// Completed on the second read so the poll loop is exercised.
				if a.jobGets[id] >= 2 && !a.jobNoComplete {
					j["completed"] = true
				}
				if seq := a.jobDetailSeq[id]; len(seq) > 0 {
					a.writeJSON(w, http.StatusOK, seq[min(a.jobGets[id]-1, len(seq)-1)])
					return
				}
				if body, ok := a.jobDetail[id]; ok {
					a.writeJSON(w, http.StatusOK, body)
					return
				}
				a.writeJSON(w, http.StatusOK, j)
				return
			}
		}
		a.writeJSON(w, http.StatusNotFound, map[string]any{"message": "not found"})
	case r.Method == http.MethodGet && p == webhooks+"/subscriptions":
		a.writeJSON(w, http.StatusOK, rqtList(a.subs))
	case r.Method == http.MethodGet && p == webhooks+"/subscriptions/disabled":
		a.writeJSON(w, http.StatusOK, rqtList(a.disabledSubs))
	case r.Method == http.MethodGet && p == webhooks+"/domains":
		if a.domainsCode != 0 {
			a.writeJSON(w, a.domainsCode, map[string]any{"message": "forbidden"})
			return
		}
		a.writeJSON(w, http.StatusOK, rqtList(a.domains))
	case r.Method == http.MethodGet && p == webhooks+"/queue":
		a.writeJSON(w, http.StatusOK, rqtList(a.queue))
	case r.Method == http.MethodGet && strings.HasPrefix(p, webhooks+"/queue/channels/"):
		a.writeJSON(w, http.StatusOK, rqtList(a.channelQueue[strings.TrimPrefix(p, webhooks+"/queue/channels/")]))
	case r.Method == http.MethodPatch && strings.HasPrefix(p, webhooks+"/subscriptions/"):
		ch := strings.TrimPrefix(p, webhooks+"/subscriptions/")
		if code := a.patchFail[ch]; code != 0 {
			a.writeJSON(w, code, map[string]any{"message": "update rejected"})
			return
		}
		if a.onPatch != nil {
			a.onPatch(a, ch)
		}
		body, overridden := a.patchBody[ch]
		var updated map[string]any
		if !overridden || a.patchApplies[ch] {
			updated = a.enableSubscription(ch)
		}
		if overridden {
			a.writeJSON(w, http.StatusOK, body)
			return
		}
		a.writeJSON(w, http.StatusOK, updated)
	case r.Method == http.MethodPost && p == webhooks+"/misc/sendTestEvent":
		a.writeJSON(w, http.StatusOK, a.testEventResp)
	default:
		a.writeJSON(w, http.StatusNotFound, map[string]any{"message": "unexpected " + r.Method + " " + p})
	}
}

// enableSubscription makes a PATCH real: the channel moves from the
// disabled list to the enabled one with enabled:true. It returns the
// updated subscription, or nil when the channel is not a disabled one.
func (a *rqtAPI) enableSubscription(ch string) map[string]any {
	disabled, ok := a.disabledSubs.([]any)
	if !ok {
		return nil
	}
	for i, item := range disabled {
		sub, _ := item.(map[string]any)
		if sub == nil || sub["channelId"] != ch {
			continue
		}
		sub["enabled"] = true
		a.disabledSubs = append(append([]any{}, disabled[:i]...), disabled[i+1:]...)
		enabled, _ := a.subs.([]any)
		a.subs = append(enabled, sub)
		return sub
	}
	return nil
}

func (a *rqtAPI) findTrigger(id string) map[string]any {
	for _, tr := range a.triggers {
		if tr["id"] == id {
			return tr
		}
	}
	return nil
}

// listTriggers applies the spec's array filters and cursor paging.
func (a *rqtAPI) listTriggers(w http.ResponseWriter, r *http.Request) {
	if len(a.listResponses) > 0 {
		i := a.listCalls
		if i >= len(a.listResponses) {
			i = len(a.listResponses) - 1
		}
		a.listCalls++
		a.writeJSON(w, http.StatusOK, a.listResponses[i])
		return
	}
	q := r.URL.Query()
	in := func(key, v string) bool {
		vals := q[key]
		if len(vals) == 0 {
			return true
		}
		for _, x := range vals {
			if x == v {
				return true
			}
		}
		return false
	}
	matched := make([]map[string]any, 0)
	for _, tr := range a.triggers {
		org, _ := tr["organization"].(map[string]any)
		rule, _ := tr["rule"].(map[string]any)
		if in("statuses[]", fmt.Sprint(tr["status"])) && in("organizationIds[]", fmt.Sprint(org["id"])) && in("ruleIds[]", fmt.Sprint(rule["id"])) {
			matched = append(matched, tr)
		}
	}
	start := 0
	if after := q["startAfter"]; len(after) == 2 {
		for i, tr := range matched {
			if tr["id"] == after[1] {
				start = i + 1
			}
		}
	}
	end := start + a.pageSize
	if end > len(matched) {
		end = len(matched)
	}
	page := matched[start:end]
	pagination := map[string]any{"hasNextPage": end < len(matched)}
	if len(page) > 0 {
		last := page[len(page)-1]
		ts := last["triggeredAt"]
		if ts == nil {
			ts = last["createdAt"]
		}
		pagination["cursor"] = []any{ts, last["id"]}
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"ruleTriggers": page, "pagination": pagination})
}

func (a *rqtAPI) callsMatching(method, prefix string) []rqtCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]rqtCall, 0)
	for _, c := range a.calls {
		if c.Method == method && strings.HasPrefix(c.Path, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// writes are every mutating request except the Respond token mint.
func (a *rqtAPI) writes() []rqtCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]rqtCall, 0)
	for _, c := range a.calls {
		if c.Method == http.MethodGet || c.Path == "/respondApi/api/v1/auth" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (a *rqtAPI) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

// rqtEnv sandboxes the test, points the CLI at api, and pins the clock.
func rqtEnv(t *testing.T, api *rqtAPI) {
	t.Helper()
	testenv.Isolate(t)
	if api != nil {
		t.Setenv("SAAS_ALERTS_BASE_URL", api.srv.URL)
	}
	t.Setenv("SAAS_ALERTS_API_KEY", rqtAPIKey)
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	rqtPinClock(t)
}

// rqtExit is the process exit code for err. The generated ExitCode maps
// every non-cliError, including nil, to 1, so "exit non-zero" assertions must
// not call it on a nil error.
func rqtExit(err error) int {
	if err == nil {
		return 0
	}
	return ExitCode(err)
}

func rqtRun(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	// --rate-limit 0: the auto limiter starts at 2 req/s, which only slows tests.
	cmd.SetArgs(append([]string{"--no-learn", "--rate-limit", "0"}, args...))
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func rqtDecode[T any](t *testing.T, out string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("output is not the expected JSON: %v\n%s", err, out)
	}
	return v
}

// rqtSeedStore writes the fixture triggers (and any extra users) into a
// fresh store the way sync would, and returns its path.
func rqtSeedStore(t *testing.T, triggers []map[string]any, users []map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	seed := func(resource string, rows []map[string]any) {
		if len(rows) == 0 {
			return
		}
		raws := make([]json.RawMessage, 0, len(rows))
		for _, r := range rows {
			b, _ := json.Marshal(r)
			raws = append(raws, b)
		}
		if _, _, err := db.UpsertBatch(resource, raws); err != nil {
			t.Fatalf("seed %s: %v", resource, err)
		}
		if err := db.SaveSyncState(resource, "", len(rows)); err != nil {
			t.Fatalf("sync state %s: %v", resource, err)
		}
	}
	seed("rule-triggers", triggers)
	seed("users", users)
	return path
}

func rqtTriggerFixtures(t *testing.T) []rqTrigger {
	t.Helper()
	out := make([]rqTrigger, 0)
	for _, raw := range rqtTriggers() {
		b, _ := json.Marshal(raw)
		var tr rqTrigger
		if err := json.Unmarshal(b, &tr); err != nil {
			t.Fatalf("decode fixture: %v", err)
		}
		out = append(out, tr)
	}
	return out
}

func TestRespondDeadlinesTable(t *testing.T) {
	triggers := rqtTriggerFixtures(t)
	items := rqAnnotate(rqFilter{Statuses: []string{"PENDING", "REMEDIATION_FAILED"}}.apply(triggers), rqtNow)
	byID := map[string]rqQueueItem{}
	for _, it := range items {
		byID[it.TriggerID] = it
	}
	cases := []struct {
		id           string
		kind         string
		deadline     time.Time
		timeLeft     int64
		expired      bool
		superseded   bool
		reason       string
		reminders    int
		triggeredSrc string
	}{
		{id: "RT_A2", kind: rqKindExpiresAt, deadline: rqtNow.Add(11 * time.Hour), timeLeft: 11 * 3600, reason: "12h", triggeredSrc: "triggeredAt"},
		{id: "RT_B", kind: rqKindExpiresAt, deadline: rqtNow.Add(30 * time.Minute), timeLeft: 1800, reason: "12h", triggeredSrc: "triggeredAt"},
		{id: "RT_C", kind: rqKindAutoIgnoreAt, deadline: rqtNow.Add(15 * time.Minute), timeLeft: 900, reminders: 2, triggeredSrc: "triggeredAt"},
		{id: "RT_A", kind: rqKindExpiresAt, deadline: rqtNow.Add(-time.Hour), timeLeft: -3600, expired: true, superseded: true, reason: "superseded", triggeredSrc: "triggeredAt"},
		{id: "RT_D", kind: rqKindExpiresAt, deadline: rqtNow.Add(10 * time.Hour), timeLeft: 10 * 3600, reason: "12h", triggeredSrc: "triggeredAt"},
		{id: "RT_E", kind: rqKindExpiresAt, deadline: rqtNow.Add(-2 * time.Hour), timeLeft: -7200, expired: true, reason: "12h", triggeredSrc: "createdAt"},
	}
	if len(items) != len(cases) {
		t.Fatalf("annotated %d triggers, want %d", len(items), len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			it, ok := byID[tc.id]
			if !ok {
				t.Fatalf("missing %s", tc.id)
			}
			if it.DeadlineKind != tc.kind {
				t.Errorf("deadline_kind = %q, want %q", it.DeadlineKind, tc.kind)
			}
			if it.Deadline == nil || *it.Deadline != rqFormatTime(tc.deadline) {
				t.Errorf("deadline = %v, want %s", it.Deadline, rqFormatTime(tc.deadline))
			}
			if it.TimeLeft == nil || *it.TimeLeft != tc.timeLeft {
				t.Errorf("time_left = %v, want %d", it.TimeLeft, tc.timeLeft)
			}
			if it.Expired != tc.expired || it.Superseded != tc.superseded {
				t.Errorf("expired/superseded = %v/%v, want %v/%v", it.Expired, it.Superseded, tc.expired, tc.superseded)
			}
			if it.ExpiryReason != tc.reason {
				t.Errorf("expiry_reason = %q, want %q", it.ExpiryReason, tc.reason)
			}
			if tc.kind == rqKindAutoIgnoreAt {
				if it.RemindersSent == nil || *it.RemindersSent != tc.reminders {
					t.Errorf("reminders_sent = %v, want %d", it.RemindersSent, tc.reminders)
				}
				if it.AutoIgnoreAt == nil || it.ExpiresAt != nil {
					t.Errorf("auto_ignore_at/expires_at = %v/%v, want set/nil", it.AutoIgnoreAt, it.ExpiresAt)
				}
			} else if it.RemindersSent != nil || it.AutoIgnoreAt != nil {
				t.Errorf("PENDING trigger carries failed-remediation fields: %+v", it)
			}
			if it.TriggeredAtSource != tc.triggeredSrc {
				t.Errorf("triggered_at_source = %q, want %q", it.TriggeredAtSource, tc.triggeredSrc)
			}
		})
	}
	if byID["RT_A"].SupersededBy != "RT_A2" {
		t.Errorf("RT_A superseded_by = %q, want RT_A2", byID["RT_A"].SupersededBy)
	}
	if !byID["RT_D"].Destructive || byID["RT_B"].Destructive {
		t.Errorf("destructive flags wrong: RT_D=%v RT_B=%v", byID["RT_D"].Destructive, byID["RT_B"].Destructive)
	}
}

func TestRespondDeadlineEdgeCases(t *testing.T) {
	cases := []struct {
		name     string
		trigger  rqTrigger
		others   []rqTrigger
		kind     string
		deadline time.Time
		reason   string
	}{
		{
			name:     "newer trigger after the 12h window keeps the 12h deadline but is superseded",
			trigger:  rqTrigger{ID: "X1", Status: "PENDING", TriggeredAt: rqMillis(rqtMS(-20 * time.Hour)), Rule: rqRule{ID: "R"}},
			others:   []rqTrigger{{ID: "X2", Status: "PENDING", TriggeredAt: rqMillis(rqtMS(-2 * time.Hour)), Rule: rqRule{ID: "R"}}},
			kind:     rqKindExpiresAt,
			deadline: rqtNow.Add(-8 * time.Hour),
			reason:   "12h",
		},
		{
			name:     "failed remediation without a REMEDIATION_FAILED entry falls back to updatedAt",
			trigger:  rqTrigger{ID: "F1", Status: "REMEDIATION_FAILED", TriggeredAt: rqMillis(rqtMS(-time.Hour)), UpdatedAt: rqMillis(rqtMS(-10 * time.Minute))},
			kind:     rqKindAutoIgnoreAt,
			deadline: rqtNow.Add(35 * time.Minute),
		},
		{
			name:    "other statuses have no deadline",
			trigger: rqTrigger{ID: "O1", Status: "APPROVED", TriggeredAt: rqMillis(rqtMS(-time.Hour))},
			kind:    rqKindNone,
		},
		{
			name:    "a trigger with no timestamps has no deadline",
			trigger: rqTrigger{ID: "N1", Status: "PENDING"},
			kind:    rqKindNone,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := rqAnnotate(append([]rqTrigger{tc.trigger}, tc.others...), rqtNow)
			it := items[0]
			if it.DeadlineKind != tc.kind {
				t.Fatalf("deadline_kind = %q, want %q", it.DeadlineKind, tc.kind)
			}
			if tc.kind == rqKindNone {
				if it.Deadline != nil || it.TimeLeft != nil || it.Expired {
					t.Fatalf("expected no deadline, got %+v", it)
				}
				return
			}
			if it.Deadline == nil || *it.Deadline != rqFormatTime(tc.deadline) {
				t.Fatalf("deadline = %v, want %s", it.Deadline, rqFormatTime(tc.deadline))
			}
			if it.ExpiryReason != tc.reason {
				t.Fatalf("expiry_reason = %q, want %q", it.ExpiryReason, tc.reason)
			}
		})
	}
}

func TestRespondQueueSortOrder(t *testing.T) {
	items := rqAnnotate(rqFilter{Statuses: []string{"PENDING", "REMEDIATION_FAILED"}}.apply(rqtTriggerFixtures(t)), rqtNow)
	items = append(items, rqAnnotate([]rqTrigger{{ID: "NODEADLINE", Status: "REMEDIATION_PARTIAL", TriggeredAt: rqMillis(rqtMS(-time.Hour))}}, rqtNow)...)
	rqSortQueue(items)
	got := make([]string, 0, len(items))
	for _, it := range items {
		got = append(got, it.TriggerID)
	}
	want := []string{"RT_C", "RT_B", "RT_D", "RT_A2", "NODEADLINE", "RT_E", "RT_A"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestRespondMillisDecoding(t *testing.T) {
	cases := map[string]int64{
		`1680539636183`:          1680539636183,
		`1.680539636183e+12`:     1680539636183,
		`"1680539636183"`:        1680539636183,
		`"2023-04-03T16:33:56Z"`: 1680539636000,
		`null`:                   0,
		`{"_seconds":1}`:         0,
	}
	keys := make([]string, 0, len(cases))
	for k := range cases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, in := range keys {
		var m rqMillis
		if err := json.Unmarshal([]byte(in), &m); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if int64(m) != cases[in] {
			t.Errorf("%s decoded to %d, want %d", in, int64(m), cases[in])
		}
	}
}

// TestNoRawBodyEcho feeds malformed 200 bodies carrying a credential-looking
// value through every decode path and checks it never reaches stdout,
// stderr, or the returned error.
func TestNoRawBodyEcho(t *testing.T) {
	const secret = "secret-api-key-XYZ"
	run := func(t *testing.T, api *rqtAPI, args ...string) {
		t.Helper()
		rqtEnv(t, api)
		rqtFastPoll(t)
		out, stderr, err := rqtRun(t, args...)
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		for where, text := range map[string]string{"stdout": out, "stderr": stderr, "error": errText} {
			if strings.Contains(text, secret) {
				t.Fatalf("%s echoes the response body:\n%s", where, text)
			}
		}
		if out == "" && stderr == "" && errText == "" {
			t.Fatalf("expected some output")
		}
	}
	t.Run("queue list envelope", func(t *testing.T) {
		api := newRQTAPI(t)
		api.listResponses = []any{map[string]any{"message": secret}}
		run(t, api, "rule-triggers", "queue", "--json")
	})
	t.Run("queue ruleTriggers not an array", func(t *testing.T) {
		api := newRQTAPI(t)
		api.listResponses = []any{map[string]any{"ruleTriggers": secret}}
		run(t, api, "rule-triggers", "queue", "--json")
	})
	t.Run("decide --ids direct lookup", func(t *testing.T) {
		api := newRQTAPI(t)
		api.triggerGet["RT_X"] = map[string]any{"message": secret}
		run(t, api, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_X", "--json")
	})
	t.Run("decide approve rule lookup", func(t *testing.T) {
		api := newRQTAPI(t)
		api.triggers = append(api.triggers, rqtPendingWithRule("RT_X", map[string]any{"id": "RULE_S"}))
		api.rules["RULE_S"] = map[string]any{"message": secret}
		run(t, api, "rule-triggers", "decide", "--decision", "approve", "--ids", "RT_X", "--json")
	})
	t.Run("contain execute", func(t *testing.T) {
		api := newRQTAPI(t)
		api.execBody = map[string]any{"message": secret}
		run(t, api, "contain", "jane.doe@example.com", "--org-id", "ORG_1", "--account-id", "ACC_JANE", "--confirm", "--wait", "50ms", "--json")
	})
	t.Run("contain execute string body", func(t *testing.T) {
		api := newRQTAPI(t)
		api.execBody = secret
		run(t, api, "contain", "jane.doe@example.com", "--org-id", "ORG_1", "--account-id", "ACC_JANE", "--confirm", "--wait", "50ms", "--json")
	})
	t.Run("contain jobs list", func(t *testing.T) {
		api := newRQTAPI(t)
		api.jobsBody = map[string]any{"message": secret}
		run(t, api, "contain", "jane.doe@example.com", "--actions", "block", "--org-id", "ORG_1", "--account-id", "ACC_JANE", "--confirm", "--wait", "50ms", "--json")
	})
	t.Run("subscriptions list", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.subs = map[string]any{"message": secret}
		run(t, api, "subscriptions", "health", "--json")
	})
	t.Run("domains and queues", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.domains = map[string]any{"message": secret}
		api.queue = []any{map[string]any{"id": "Q1"}}
		api.channelQueue["CH_S4"] = map[string]any{"message": secret}
		run(t, api, "subscriptions", "health", "--json")
	})
	t.Run("queue object", func(t *testing.T) {
		api := newRQTAPI(t)
		rqtWebhookFixtures(api)
		api.queue = map[string]any{"message": secret}
		run(t, api, "subscriptions", "health", "--json")
	})
	t.Run("test event error string and IWhError", func(t *testing.T) {
		for _, resp := range []any{"error: " + secret, []any{map[string]any{"channelId": secret, "message": secret}}} {
			api := newRQTAPI(t)
			rqtWebhookFixtures(api)
			api.testEventResp = resp
			run(t, api, "subscriptions", "health", "--reenable", "--confirm", "--json")
		}
	})
}

// TestNoCredentialEchoInDiagnostics places the configured API key and both
// minted access tokens (previous and current) into each response field that
// used to reach a diagnostic, and checks none of them is printed.
func TestNoCredentialEchoInDiagnostics(t *testing.T) {
	secrets := append([]string{rqtAPIKey}, rqtTokens...)
	check := func(t *testing.T, api *rqtAPI, args ...string) {
		t.Helper()
		out, stderr, err := rqtRun(t, args...)
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		for _, secret := range secrets {
			for where, text := range map[string]string{"stdout": out, "stderr": stderr, "error": errText} {
				if strings.Contains(strings.ToLower(text), strings.ToLower(secret)) {
					t.Fatalf("%s leaks %q:\n%s", where, secret, text)
				}
			}
		}
	}
	for _, v := range secrets {
		t.Run("trigger id mismatch/"+v, func(t *testing.T) {
			api := newRQTAPI(t)
			api.rejectFirstToken = true
			api.triggerGet["RT_X"] = rqtPendingWithRule(v, rqtRule("RULE_X", "x", "low", "Alert Only"))
			rqtEnv(t, api)
			check(t, api, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_X", "--json")
			if api.mints != 2 {
				t.Fatalf("the fake should have minted a previous and a current token, minted %d", api.mints)
			}
		})
		t.Run("object key names/"+v, func(t *testing.T) {
			api := newRQTAPI(t)
			api.listResponses = []any{map[string]any{v: 1, "message": "backend failure"}}
			rqtEnv(t, api)
			check(t, api, "rule-triggers", "queue", "--json")
		})
		t.Run("execute status/"+v, func(t *testing.T) {
			api := newRQTAPI(t)
			api.execBody, api.execCode = map[string]any{"status": v}, 202
			rqtEnv(t, api)
			rqtFastPoll(t)
			check(t, api, "contain", "jane.doe@example.com", "--org-id", "ORG_1", "--account-id", "ACC_JANE", "--confirm", "--wait", "50ms", "--json")
		})
		t.Run("execute key names/"+v, func(t *testing.T) {
			api := newRQTAPI(t)
			api.execBody, api.execCode = map[string]any{v: "x"}, 202
			rqtEnv(t, api)
			rqtFastPoll(t)
			check(t, api, "contain", "jane.doe@example.com", "--org-id", "ORG_1", "--account-id", "ACC_JANE", "--confirm", "--wait", "50ms", "--json")
		})
		t.Run("repeated cursor/"+v, func(t *testing.T) {
			api := newRQTAPI(t)
			api.listResponses = []any{map[string]any{"ruleTriggers": []any{rqtTriggers()[1]},
				"pagination": map[string]any{"hasNextPage": true, "cursor": []any{1, v}}}}
			rqtEnv(t, api)
			check(t, api, "rule-triggers", "queue", "--json")
		})
		t.Run("credential status on a direct lookup/"+v, func(t *testing.T) {
			api := newRQTAPI(t)
			body := rqtPendingWithRule("RT_X", rqtRule("RULE_X", "x", "low", "Alert Only"))
			body["status"] = v
			api.triggerGet["RT_X"] = body
			rqtEnv(t, api)
			check(t, api, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_X", "--json")
		})
		t.Run("credential status on the verification read/"+v, func(t *testing.T) {
			api := newRQTAPI(t)
			body := rqtTriggers()[1]
			body["status"] = v
			api.triggerGet["RT_B"] = body
			rqtEnv(t, api)
			check(t, api, "rule-triggers", "decide", "--decision", "reject", "--ids", "RT_B", "--confirm",
				"--db", filepath.Join(t.TempDir(), "r.db"), "--json")
		})
		t.Run("failedAttempts message/"+v, func(t *testing.T) {
			api := newRQTAPI(t)
			rqtWebhookFixtures(api)
			api.subs = []any{map[string]any{"channelId": "CH_F1", "url": "https://hooks.example.com/saas/f1", "partnerId": "P1", "enabled": true,
				"alertStatuses": []any{"critical"}, "failedAttempts": []any{map[string]any{"message": "401 for key " + v, "time": 1}}}}
			rqtEnv(t, api)
			check(t, api, "subscriptions", "health", "--json")
		})
	}
}

func rqtFailureEnvelopes() map[string]any {
	pag := map[string]any{"hasNextPage": false}
	return map[string]any{
		"error":                          map[string]any{"ruleTriggers": []any{}, "error": "backend failure", "pagination": pag},
		"errors":                         map[string]any{"ruleTriggers": []any{}, "errors": []any{"x"}, "pagination": pag},
		"success":                        map[string]any{"ruleTriggers": []any{}, "success": false, "pagination": pag},
		"message":                        map[string]any{"ruleTriggers": []any{}, "message": "backend failure", "pagination": pag},
		"status ERROR":                   map[string]any{"ruleTriggers": []any{}, "status": "ERROR", "pagination": pag},
		"ok false":                       map[string]any{"ruleTriggers": []any{}, "ok": false, "pagination": pag},
		"success true with status ERROR": map[string]any{"ruleTriggers": []any{}, "success": true, "status": "ERROR", "pagination": pag},
		"ok true with an error":          map[string]any{"ruleTriggers": []any{}, "ok": true, "error": "x", "pagination": pag},
		"code 500":                       map[string]any{"ruleTriggers": []any{}, "code": 500, "pagination": pag},
	}
}

func TestRuleTriggersEnvelopeFailureIndicatorsAreErrors(t *testing.T) {
	for name, body := range rqtFailureEnvelopes() {
		t.Run("queue/"+name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.listResponses = []any{body}
			rqtEnv(t, api)
			out, _, err := rqtRun(t, "rule-triggers", "queue", "--json")
			if rqtExit(err) == 0 || strings.Contains(out, `"items"`) {
				t.Fatalf("a failure envelope must be an error, not an empty queue: err=%v out=%s", err, out)
			}
		})
		t.Run("decide/"+name, func(t *testing.T) {
			api := newRQTAPI(t)
			api.listResponses = []any{body}
			rqtEnv(t, api)
			out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--rule", "RULE_B", "--confirm",
				"--db", filepath.Join(t.TempDir(), "r.db"), "--json")
			if rqtExit(err) == 0 || len(api.writes()) != 0 {
				t.Fatalf("a failure envelope must fail decide before any write: err=%v writes=%v out=%s", err, api.writes(), out)
			}
		})
	}
	t.Run("empty indicators are not failures", func(t *testing.T) {
		api := newRQTAPI(t)
		api.listResponses = []any{map[string]any{"ruleTriggers": []any{}, "errors": []any{}, "error": nil, "message": "",
			"status": nil, "success": true, "ok": true, "code": 200, "pagination": map[string]any{"hasNextPage": false}}}
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "rule-triggers", "queue", "--json")
		if err != nil || len(rqtDecode[rqQueueEnvelope](t, out).Items) != 0 {
			t.Fatalf("an empty page with empty indicators is a complete empty queue: err=%v out=%s", err, out)
		}
	})
}

func TestRuleTriggersRowsWithoutAValidStatusAreRejected(t *testing.T) {
	rows := func() []any {
		null := rqtTriggers()[1]
		null["id"], null["status"] = "RT_NULL", nil
		missing := rqtTriggers()[1]
		missing["id"] = "RT_MISSING"
		delete(missing, "status")
		unknown := rqtTriggers()[1]
		unknown["id"], unknown["status"] = "RT_UNKNOWN", "WEIRD"
		return []any{null, missing, unknown}
	}
	page := map[string]any{"ruleTriggers": rows(), "pagination": map[string]any{"hasNextPage": false}}

	t.Run("queue live", func(t *testing.T) {
		api := newRQTAPI(t)
		api.listResponses = []any{page}
		rqtEnv(t, api)
		out, stderr, err := rqtRun(t, "rule-triggers", "queue", "--json")
		env := rqtDecode[rqQueueEnvelope](t, out)
		if rqtExit(err) == 0 || env.Undecoded != 3 || len(env.Items) != 0 || !strings.Contains(stderr, "rejected 3") {
			t.Fatalf("rows without a valid status must make the queue incomplete: err=%v undecoded=%d stderr=%q", err, env.Undecoded, stderr)
		}
	})
	t.Run("queue local", func(t *testing.T) {
		rqtEnv(t, nil)
		seeded := make([]map[string]any, 0)
		for _, r := range rows() {
			seeded = append(seeded, r.(map[string]any))
		}
		dbPath := rqtSeedStore(t, seeded, nil)
		out, _, err := rqtRun(t, "rule-triggers", "queue", "--data-source", "local", "--db", dbPath, "--json")
		if env := rqtDecode[rqQueueEnvelope](t, out); rqtExit(err) == 0 || env.Undecoded != 3 {
			t.Fatalf("local rows without a valid status must make the queue incomplete: err=%v %s", err, out)
		}
	})
	t.Run("decide", func(t *testing.T) {
		api := newRQTAPI(t)
		api.listResponses = []any{page}
		rqtEnv(t, api)
		out, _, err := rqtRun(t, "rule-triggers", "decide", "--decision", "reject", "--rule", "RULE_B", "--json")
		if res := rqtDecode[rtDecideResult](t, out); rqtExit(err) == 0 || res.Summary.Undecoded != 3 {
			t.Fatalf("decide must fail on rows without a valid status: err=%v %s", err, out)
		}
	})
}
