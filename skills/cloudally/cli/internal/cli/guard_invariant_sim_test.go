// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// guard_invariant_sim_test.go drives the restore and download guards with
// random sequences of real command invocations (in process, cancelled part
// way, as a child process killed with SIGKILL, and two child processes at
// once) against one fake API, portal, and object store that keep the true
// state, and checks five invariants after every operation:
//
//	I1 no restore identity is created twice unless a --new-batch op
//	   targeted it after the first creation;
//	I2 the same for export archive identities (a job that ended in failure
//	   may be created once more: the documented JOB_FAILED path);
//	I3 no command exits 0 while something it was asked for is missing,
//	   failed, or unknown;
//	I4 no message steers to --new-batch outside the documented
//	   person-decision cases;
//	I5 no row is EMPTY unless the fake created that job empty.
//
// A second creation the CLI had no evidence against (the first job was not
// listed yet when the second was sent) is the documented risk of an
// unverifiable create; it is reported as an evidence gap, not a violation.
//
// Knobs: GUARD_SIM_SEEDS (default 0: a default run is the regression replays), GUARD_SIM_OPS (default 10),
// GUARD_SIM_SEED (run only this seed), GUARD_SIM_FIRST_SEED (default 1),
// GUARD_SIM_NO_CHILD=1 (no child processes; the default on Windows).
// A default run (no seed knob set) replays the two seeds that found the
// settlement bugs (4, 1014); GUARD_SIM_ALL_REGRESSION=1 replays every seed in
// simRegressionSeeds, one per class found. A soak sets GUARD_SIM_SEEDS and
// GUARD_SIM_FIRST_SEED (thousands of seeds, sharded over processes).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/config"
	"cloudally-pp-cli/internal/portal"
)

// ---------------------------------------------------------------- world

const (
	simAccount = "acct-1"
	simTask    = "task-1" // MS365: restores and download submit
	simGTask   = "task-2" // GMAIL: download start
	simGEntity = "g@contoso.com"
	simGSnap   = int64(20251231042000)
)

var simEntities = []string{"a@contoso.com", "b@contoso.com"}

// simSubs are the MS365 sub-sources each mailbox backs up, with the offset
// added to each mailbox's own timestamp.
var simSubs = []struct {
	name string
	base int64
}{{"O365MAIL", 20260930042000}, {"O365CALENDAR", 20260930041000}}

func simMailboxIndex(entity string) int64 {
	for i, e := range simEntities {
		if strings.EqualFold(e, entity) {
			return int64(i)
		}
	}
	return 0
}

// simRestoreResolved is what 'restore submit --resolve last-backup' picks
// for a mailbox: its newest sub-source backup.
func simRestoreResolved(entity string) int64 {
	return simSubs[0].base + simMailboxIndex(entity)
}

// simExport is one create the portal received.
type simExport struct {
	ID, Identity, Entity, Sub, Task, Label, Decision string
	Snapshot                                         int64
	Answered                                         bool // the create (or its poll) answered success
	Created, Empty                                   bool
	At, VisibleAt, FinalAt, ExpireAt                 time.Time
	FinalStatus                                      string
	Body                                             []byte
}

// simRestore is one restore POST the API received.
type simRestore struct {
	ID, Identity, Entity, Decision string
	Resolved                       int64
	Answered, Created              bool
	At, VisibleAt, FinalAt         time.Time
	FinalStatus                    string
}

type simWorld struct {
	seed     int64
	mu       sync.Mutex
	exports  []*simExport
	restores []*simRestore
	pending  map[string]*simExport // deferred reqId
	planned  map[string]string     // reqId -> poll outcome
	tries    map[string]int
	pageCap  int
	nextID   int

	api, portal, storage *httptest.Server
}

// simDraw is a deterministic random source for one decision: the same seed,
// identity, attempt, and purpose always draw the same value, whatever order
// concurrent requests arrive in.
func (w *simWorld) simDraw(parts ...any) *rand.Rand {
	h := fnv.New64a()
	fmt.Fprint(h, w.seed)
	for _, p := range parts {
		fmt.Fprint(h, "|", p)
	}
	return rand.New(rand.NewSource(int64(h.Sum64())))
}

// simPick returns one name from weighted choices.
func simPick(r *rand.Rand, choices []simChoice) string {
	total := 0
	for _, c := range choices {
		total += c.weight
	}
	n := r.Intn(total)
	for _, c := range choices {
		if n < c.weight {
			return c.name
		}
		n -= c.weight
	}
	return choices[0].name
}

type simChoice struct {
	name   string
	weight int
}

var simExportDecisions = []simChoice{
	{"accept", 55}, {"acceptDrop", 7}, {"acceptEmpty", 7}, {"refuse401", 4}, {"refuse403", 3}, {"refuse400", 3},
	{"dropCreated", 6}, {"drop", 6}, {"deferred", 5}, {"throttleCreated", 2}, {"throttle", 2},
	{"timeoutCreated", 2}, {"timeout", 2},
}

var simRestoreDecisions = []simChoice{
	{"accept", 62}, {"acceptDrop", 5}, {"refuse401", 5}, {"refuse400", 5},
	{"err502Created", 5}, {"err502", 5}, {"dropCreated", 6}, {"drop", 7},
	{"timeoutCreated", 3}, {"timeout", 3},
}

func newSimWorld(t *testing.T, seed int64) *simWorld {
	w := &simWorld{seed: seed, pending: map[string]*simExport{}, planned: map[string]string{}, tries: map[string]int{}}
	if w.simDraw("pagecap").Intn(5) == 0 {
		w.pageCap = 3
	}
	w.api = httptest.NewServer(http.HandlerFunc(w.serveAPI))
	t.Cleanup(w.api.Close)
	w.portal = httptest.NewServer(http.HandlerFunc(w.servePortal))
	t.Cleanup(w.portal.Close)
	w.storage = httptest.NewServer(http.HandlerFunc(w.serveStorage))
	t.Cleanup(w.storage.Close)
	return w
}

func simHijackClose(rw http.ResponseWriter) {
	if hj, ok := rw.(http.Hijacker); ok {
		if conn, _, err := hj.Hijack(); err == nil {
			_ = conn.Close()
			return
		}
	}
	rw.WriteHeader(http.StatusBadGateway)
}

func simJSON(rw http.ResponseWriter, v any) {
	rw.Header().Set("Content-Type", "application/json")
	raw, _ := json.Marshal(v)
	_, _ = rw.Write(raw)
}

func (w *simWorld) taskRecord(task string) map[string]any {
	if task == simGTask {
		return map[string]any{"id": simGTask, "account": simAccount, "type": "BACKUP", "source": "GMAIL", "status": "ACTIVE",
			"numOfEntities": 1, "entities": []map[string]any{{"id": "g1", "taskId": simGTask, "entity": simGEntity,
				"extId": "ext-g", "status": "ACTIVE", "lastBackupDates": map[string]any{"GMAIL": simGSnap}}}}
	}
	ents := []map[string]any{}
	for i, e := range simEntities {
		dates := map[string]any{}
		for _, s := range simSubs {
			dates[s.name] = s.base + int64(i)
		}
		ents = append(ents, map[string]any{"id": fmt.Sprintf("e%d", i), "taskId": simTask, "entity": e, "extId": "ext-" + e,
			"status": "ACTIVE", "lastBackupDates": dates})
	}
	return map[string]any{"id": simTask, "account": simAccount, "type": "BACKUP", "source": "MS365", "status": "ACTIVE",
		"numOfEntities": len(ents), "entities": ents}
}

func (w *simWorld) serveAPI(rw http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/partner-access-token"):
		simJSON(rw, map[string]any{"accessToken": "handoff-token"})
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/backup-tasks/"+simTask):
		simJSON(rw, w.taskRecord(simTask))
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/backup-tasks/"+simGTask):
		simJSON(rw, w.taskRecord(simGTask))
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/backup-tasks"):
		w.serveRestoreFeed(rw, r)
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/recovery/restore"):
		w.serveRestoreCreate(rw, r)
	default:
		rw.WriteHeader(http.StatusNotFound)
	}
}

func (w *simWorld) serveRestoreCreate(rw http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	entity, _ := body["account"].(string)
	task, _ := body["taskId"].(string)
	date := int64(0)
	if f, ok := body["date"].(float64); ok {
		date = int64(f)
	}
	identity := fmt.Sprintf("%s|%s|%d", strings.ToLower(task), strings.ToLower(entity), date)
	w.mu.Lock()
	k := w.tries["r|"+identity]
	w.tries["r|"+identity]++
	dec := simPick(w.simDraw("restore", identity, k), simRestoreDecisions)
	w.nextID++
	now := time.Now()
	rs := &simRestore{ID: fmt.Sprintf("rt-%d", w.nextID), Identity: identity, Entity: entity, Resolved: date, Decision: dec, At: now}
	d := w.simDraw("restore-life", identity, k)
	switch dec {
	case "accept", "err502Created", "dropCreated", "timeoutCreated":
		rs.Created = true
		rs.VisibleAt = now.Add(simLag(d))
		rs.FinalAt = rs.VisibleAt.Add(time.Duration(50+d.Intn(250)) * time.Millisecond)
		rs.FinalStatus = simPick(d, []simChoice{{"COMPLETED", 85}, {"FAILED", 10}, {"PARTIALLY_SUCCEEDED", 5}})
	}
	rs.Answered = dec == "accept" || dec == "acceptDrop"
	w.restores = append(w.restores, rs)
	w.mu.Unlock()
	switch dec {
	case "accept", "acceptDrop":
		simJSON(rw, map[string]any{"needRedirect": false})
	case "refuse401":
		rw.WriteHeader(http.StatusUnauthorized)
	case "refuse400":
		rw.WriteHeader(http.StatusBadRequest)
	case "err502Created", "err502":
		rw.WriteHeader(http.StatusBadGateway)
	case "timeoutCreated", "timeout":
		simHoldPastTimeout(rw, r)
	default:
		simHijackClose(rw)
	}
}

// simHoldPastTimeout answers nothing until the client gives up (its
// per-request --timeout, or the portal session's), then drops the
// connection: a create that timed out, before or after the server made it.
func simHoldPastTimeout(rw http.ResponseWriter, r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(5 * time.Second):
	}
	simHijackClose(rw)
}

// simLag is how long a new job or task takes to be listed: usually well
// inside the reconcile wait, sometimes far beyond it.
func simLag(d *rand.Rand) time.Duration {
	if d.Intn(20) == 0 {
		return 2500 * time.Millisecond
	}
	return time.Duration(d.Intn(100)) * time.Millisecond
}

// simPage cuts a newest-first listing into the page asked for. With a page
// cap the portal returns fewer rows than asked and says so in totalPages.
func (w *simWorld) simPage(n, page, rows int) (from, to, totalPages int) {
	size := rows
	if w.pageCap > 0 && w.pageCap < size {
		size = w.pageCap
	}
	if size <= 0 {
		size = 100
	}
	totalPages = (n + size - 1) / size
	if page < 1 {
		page = 1
	}
	from = (page - 1) * size
	to = from + size
	if from > n {
		from = n
	}
	if to > n {
		to = n
	}
	return from, to, totalPages
}

func (w *simWorld) serveRestoreFeed(rw http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.URL.Query().Get("type"), "RESTORE") {
		simJSON(rw, map[string]any{"data": []any{}})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	now := time.Now()
	w.mu.Lock()
	var shown []*simRestore
	for _, rs := range w.restores {
		if rs.Created && !now.Before(rs.VisibleAt) {
			shown = append(shown, rs)
		}
	}
	sort.SliceStable(shown, func(i, j int) bool { return shown[i].At.After(shown[j].At) })
	from, to, totalPages := w.simPage(len(shown), page, rows)
	data := []map[string]any{}
	for _, rs := range shown[from:to] {
		status := "INPROCESS"
		if !now.Before(rs.FinalAt) {
			status = rs.FinalStatus
		}
		data = append(data, map[string]any{"id": rs.ID, "account": rs.Entity, "type": "RESTORE", "source": "MS365",
			"status": status, "backupTaskId": simTask, "snapshotDate": rs.Resolved, "createDate": rs.At.UTC().Format(time.RFC3339)})
	}
	w.mu.Unlock()
	if page < 1 {
		page = 1
	}
	simJSON(rw, map[string]any{"page": page, "totalPages": totalPages, "total": len(shown), "data": data})
}

func (w *simWorld) servePortal(rw http.ResponseWriter, r *http.Request) {
	const shell = `<html><head><meta name="_csrf" content="csrf-1"/>` +
		`<meta name="_csrf_header" content="X-CSRF-TOKEN"/></head></html>`
	if r.URL.Path == "/newapp" {
		http.SetCookie(rw, &http.Cookie{Name: "JSESSIONID", Value: "authed"})
		_, _ = rw.Write([]byte(shell))
		return
	}
	op := strings.TrimPrefix(r.URL.Path, "/application/service/")
	envelope := func(data any) { simJSON(rw, map[string]any{"success": true, "data": data}) }
	switch op {
	case "startRestoreOrDownload":
		if id := r.URL.Query().Get("reqId"); id != "" {
			w.servePoll(rw, id)
			return
		}
		w.serveExportCreate(rw, r)
	case "getRestoreDownloadStatus":
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Rows int `json:"rows"`
			Page int `json:"page"`
		}
		_ = json.Unmarshal(raw, &body)
		now := time.Now()
		w.mu.Lock()
		var shown []*simExport
		for _, e := range w.exports {
			if w.listed(e, now) {
				shown = append(shown, e)
			}
		}
		sort.SliceStable(shown, func(i, j int) bool { return shown[i].At.After(shown[j].At) })
		from, to, totalPages := w.simPage(len(shown), body.Page, body.Rows)
		rows := []map[string]any{}
		for _, e := range shown[from:to] {
			rows = append(rows, map[string]any{"taskId": e.ID, "type": "EXPORT", "status": e.statusAt(now), "account": e.Entity,
				"source": map[bool]string{true: "GMAIL", false: "MS365"}[e.Task == simGTask], "subsource": e.Sub,
				"snapshotDate": e.Snapshot, "jobLabel": e.Label, "initDate": simCompact(e.At)})
		}
		w.mu.Unlock()
		envelope(map[string]any{"page": max(body.Page, 1), "totalPages": totalPages, "total": len(shown), "data": rows})
	case "getDownloadInfo":
		id := r.URL.Query().Get("taskId")
		files := []portal.DownloadFile{}
		w.mu.Lock()
		for _, e := range w.exports {
			if e.ID == id && e.Created && !e.Empty && e.statusAt(time.Now()) == portal.StatusReadyForDownload {
				files = append(files, portal.DownloadFile{ExportDataID: e.ID + "-f", FileName: e.ID + ".zip", Size: fmt.Sprintf("%d Bytes", len(e.Body))})
			}
		}
		w.mu.Unlock()
		envelope(files)
	case "getDownloadUrl":
		id := strings.TrimSuffix(r.URL.Query().Get("taskId"), "-f")
		w.mu.Lock()
		var hit *simExport
		for _, e := range w.exports {
			if e.ID == id && e.Created && !e.Empty {
				hit = e
			}
		}
		expired := hit == nil || (!hit.ExpireAt.IsZero() && !time.Now().Before(hit.ExpireAt))
		w.mu.Unlock()
		if expired {
			_, _ = rw.Write([]byte(`{"success":false,"status":"The download link has expired.","statusCode":400}`))
			return
		}
		envelope(w.storage.URL + "/obj/" + url.PathEscape(id))
	case "getJobsDasboard":
		w.mu.Lock()
		total := 0
		for _, e := range w.exports {
			if e.Created {
				total++
			}
		}
		for _, rs := range w.restores {
			if rs.Created {
				total++
			}
		}
		w.mu.Unlock()
		envelope(map[string]any{"total": total, "dashboardElements": []any{}})
	case "getSnapshotsCalendar":
		raw, _ := io.ReadAll(r.Body)
		var q struct {
			TaskID    string `json:"taskId"`
			AccountID string `json:"accountId"`
			Year      int    `json:"year"`
			Month     int    `json:"month"`
		}
		_ = json.Unmarshal(raw, &q)
		if q.TaskID == simTask {
			envelope(simMS365Calendar(q.AccountID, q.Year, q.Month))
			return
		}
		var rows []map[string]any
		for d := 1; d <= 31; d++ {
			rows = append(rows, map[string]any{"dateTime": 20251200042000 + int64(d)*1000000, "succeed": true, "status": "SUCCEED",
				"source": "GMAIL", "batchId": fmt.Sprintf("run-%02d", d)})
		}
		envelope(rows)
	default:
		envelope(nil)
	}
}

// simMS365Calendar is a mailbox's backup calendar for the requested month
// and the one before it: one run a day at 04:20 UTC whose sub-source
// snapshots are that day's 04:20 (mail) and 04:10 (calendar) plus the
// mailbox's index. 2026-09-30's run is exactly the task record's
// lastBackupDates, so --on-date 2026-09-30 in UTC and --latest name the same
// archives, while an offset that moves the local day picks another run.
func simMS365Calendar(entity string, year, month int) []map[string]any {
	i := simMailboxIndex(entity)
	var rows []map[string]any
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	for d := first.AddDate(0, -1, 0); d.Before(first.AddDate(0, 1, 0)); d = d.AddDate(0, 0, 1) {
		day := int64(d.Year())*10000 + int64(d.Month())*100 + int64(d.Day())
		subs := []map[string]any{}
		for _, sub := range []struct {
			name string
			hhmm int64
		}{{"O365MAIL", 42000}, {"O365CALENDAR", 41000}} {
			subs = append(subs, map[string]any{"dateTime": day*1000000 + sub.hhmm + i, "succeed": true, "status": "SUCCEED", "source": sub.name})
		}
		rows = append(rows, map[string]any{"dateTime": day*1000000 + 42000 + i, "succeed": true, "status": "SUCCEED", "source": "MS365",
			"batchId": fmt.Sprintf("run-%d-%d", day, i), "subsSourceResponseList": subs})
	}
	return rows
}

func simCompact(t time.Time) int64 {
	v, _ := strconv.ParseInt(t.UTC().Format("20060102150405"), 10, 64)
	return v
}

// listed reports whether a full listing (every page) shows the job at now.
func (w *simWorld) listed(e *simExport, now time.Time) bool {
	if !e.Created || e.Empty || now.Before(e.VisibleAt) {
		return false
	}
	return e.ExpireAt.IsZero() || now.Before(e.ExpireAt)
}

func (e *simExport) statusAt(now time.Time) string {
	if now.Before(e.FinalAt) {
		return "INPROCESS"
	}
	return e.FinalStatus
}

func (w *simWorld) serveExportCreate(rw http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	str := func(k string) string { s, _ := body[k].(string); return s }
	date := int64(0)
	if f, ok := body["date"].(float64); ok {
		date = int64(f)
	}
	format := "zip"
	if b, _ := body["downloadToPst"].(bool); b {
		format = "pst"
		if str("preferSinglePST") == "true" {
			format = "single-pst"
		}
	}
	task := str("taskId")
	sub := str("source")
	identity := fmt.Sprintf("%s|%s|%s|%d|%s|del=%s|hold=%s", strings.ToLower(task), strings.ToLower(str("account")), sub, date,
		format, str("includeDeleted"), str("includeLegalHold"))
	w.mu.Lock()
	k := w.tries["e|"+identity]
	w.tries["e|"+identity]++
	dec := simPick(w.simDraw("export", identity, k), simExportDecisions)
	w.nextID++
	e := &simExport{ID: fmt.Sprintf("job-%d", w.nextID), Identity: identity, Entity: str("account"), Sub: sub, Task: task,
		Label: str("jobLabel"), Decision: dec, Snapshot: date, At: time.Now()}
	w.exports = append(w.exports, e)
	life := w.simDraw("export-life", identity, k)
	switch dec {
	case "accept", "dropCreated", "throttleCreated", "timeoutCreated":
		w.create(e, life, false)
	case "acceptEmpty":
		w.create(e, life, true)
	}
	e.Answered = dec == "accept" || dec == "acceptDrop" || dec == "acceptEmpty"
	var reqID string
	if dec == "deferred" {
		reqID = fmt.Sprintf("req-%d", w.nextID)
		w.pending[reqID] = e
		w.planned[reqID] = simPick(life, []simChoice{{"pollOK", 60}, {"poll401Created", 15}, {"poll401", 10}, {"pollDropCreated", 8}, {"pollDrop", 7}})
	}
	w.mu.Unlock()
	switch dec {
	case "accept", "acceptDrop", "acceptEmpty":
		simJSON(rw, map[string]any{"success": true, "data": map[string]any{"needRedirect": false}})
	case "refuse401":
		rw.WriteHeader(http.StatusUnauthorized)
	case "refuse403":
		rw.WriteHeader(http.StatusForbidden)
	case "refuse400":
		rw.WriteHeader(http.StatusBadRequest)
	case "throttle", "throttleCreated":
		rw.WriteHeader(http.StatusServiceUnavailable)
	case "deferred":
		simJSON(rw, map[string]any{"success": true, "result": "WAIT", "id": reqID})
	case "timeoutCreated", "timeout":
		simHoldPastTimeout(rw, r)
	default:
		simHijackClose(rw)
	}
}

// create makes e a real job. Called with w.mu held.
func (w *simWorld) create(e *simExport, life *rand.Rand, empty bool) {
	e.Created, e.Empty = true, empty
	e.VisibleAt = time.Now().Add(simLag(life))
	e.FinalAt = e.VisibleAt.Add(time.Duration(30+life.Intn(200)) * time.Millisecond)
	e.FinalStatus = simPick(life, []simChoice{{portal.StatusReadyForDownload, 85}, {"FAILED", 7}, {"ABORTED", 4}, {"EXPIRES", 4}})
	if e.FinalStatus == "EXPIRES" {
		e.FinalStatus = portal.StatusReadyForDownload
		e.ExpireAt = e.FinalAt.Add(400 * time.Millisecond)
	}
	e.Body = []byte("archive of " + e.ID + " " + e.Identity)
}

func (w *simWorld) servePoll(rw http.ResponseWriter, reqID string) {
	w.mu.Lock()
	e, plan := w.pending[reqID], w.planned[reqID]
	delete(w.pending, reqID)
	if e != nil {
		e.Decision += "/" + plan
		life := w.simDraw("poll-life", e.Identity, e.ID)
		switch plan {
		case "pollOK", "poll401Created", "pollDropCreated":
			w.create(e, life, false)
		}
		e.Answered = plan == "pollOK"
	}
	w.mu.Unlock()
	switch plan {
	case "pollOK":
		simJSON(rw, map[string]any{"success": true, "data": map[string]any{"needRedirect": false}})
	case "poll401Created", "poll401":
		rw.WriteHeader(http.StatusUnauthorized)
	default:
		simHijackClose(rw)
	}
}

func (w *simWorld) serveStorage(rw http.ResponseWriter, r *http.Request) {
	id, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/obj/"))
	w.mu.Lock()
	var body []byte
	for _, e := range w.exports {
		if e.ID == id && e.Created && !e.Empty {
			body = e.Body
		}
	}
	w.mu.Unlock()
	if body == nil {
		rw.WriteHeader(http.StatusNotFound)
		return
	}
	// Object storage names a type; net/http would sniff text/plain.
	rw.Header().Set("Content-Type", "application/octet-stream")
	_, _ = rw.Write(body)
}

// ---------------------------------------------------------------- running

// simSeams makes a run fast and lets it reach the loopback fakes: no portal
// pacing, a short reconcile wait, and loopback object storage.
func simSeams(storageURL string) func() {
	prevOpen, prevDelay, prevStale := openPortal, reconcileDelay, staleRequestedAge
	prevURL, prevAddr := portal.DownloadURLPolicy, portal.DownloadAddrPolicy
	openPortal = func(ctx context.Context, cfg portal.Options, token string) (*portal.Session, error) {
		cfg.RatePerSec = -1
		cfg.Timeout = simRequestTimeout
		return portal.Open(ctx, cfg, token)
	}
	reconcileDelay = 150 * time.Millisecond
	// The window after which an unlisted accepted create is concluded
	// MISSING (or, with count evidence, EMPTY): longer than the fake's
	// longest listing lag (2.5s) plus the whole second a ledger timestamp
	// rounds away, as an hour is in the field.
	staleRequestedAge = 4 * time.Second
	su, _ := url.Parse(storageURL)
	portal.DownloadURLPolicy = func(u *url.URL) error {
		if su != nil && u.Host == su.Host {
			return nil
		}
		return portal.StrictDownloadURL(u)
	}
	portal.DownloadAddrPolicy = func(a netip.Addr) error {
		if a.IsLoopback() {
			return nil
		}
		return portal.StrictDownloadAddr(a)
	}
	return func() {
		openPortal, reconcileDelay, staleRequestedAge = prevOpen, prevDelay, prevStale
		portal.DownloadURLPolicy, portal.DownloadAddrPolicy = prevURL, prevAddr
	}
}

// simRequestTimeout bounds each API and portal call of a simulated run, so
// a create the fake holds past it times out as it would in the field.
const simRequestTimeout = 1500 * time.Millisecond

type simResult struct {
	stdout, stderr, errText string
	code                    int // -1 killed
	killed                  bool
}

func simRunInProcess(ctx context.Context, args []string) (res simResult) {
	defer func() {
		if p := recover(); p != nil {
			res = simResult{code: -2, errText: fmt.Sprintf("panic: %v", p)}
		}
	}()
	var flags rootFlags
	root := newRootCmd(&flags)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(withNoClientPacing(args))
	err := root.ExecuteContext(ctx)
	res = simResult{stdout: out.String(), stderr: errOut.String()}
	if err != nil {
		res.code, res.errText = ExitCode(err), err.Error()
	}
	return res
}

const (
	simChildFlag = "GUARD_SIM_CHILD"
	simChildArgs = "GUARD_SIM_CHILD_ARGS"
	simChildEnv  = "GUARD_SIM_CHILD_ENV"
	simChildStor = "GUARD_SIM_CHILD_STORAGE"
)

// TestGuardSimChildProcess is not a test: it is one command run in a child
// process for the simulation, which may kill it with SIGKILL.
func TestGuardSimChildProcess(t *testing.T) {
	if os.Getenv(simChildFlag) != "1" {
		t.Skip("simulation child process only")
	}
	var env map[string]string
	_ = json.Unmarshal([]byte(os.Getenv(simChildEnv)), &env)
	for k, v := range env {
		_ = os.Setenv(k, v)
	}
	simSeams(os.Getenv(simChildStor))
	var args []string
	_ = json.Unmarshal([]byte(os.Getenv(simChildArgs)), &args)
	res := simRunInProcess(context.Background(), args)
	fmt.Fprint(os.Stdout, res.stdout)
	fmt.Fprint(os.Stderr, res.stderr)
	if res.code != 0 {
		fmt.Fprintln(os.Stderr, "GUARD_SIM_ERROR:", res.errText)
	}
	os.Exit(max(res.code, 0))
}

// simEnvSnapshot is the sandbox the child restores over TestMain's own.
func simEnvSnapshot() string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "CLOUDALLY_") || strings.HasPrefix(k, "XDG_") || k == "HOME" || k == "USERPROFILE" {
			env[k] = v
		}
	}
	raw, _ := json.Marshal(env)
	return string(raw)
}

// simRunChild runs args in a child process; killAfter > 0 sends SIGKILL then.
func simRunChild(args []string, storageURL string, killAfter time.Duration) simResult {
	raw, _ := json.Marshal(args)
	c := exec.Command(os.Args[0], "-test.run=^TestGuardSimChildProcess$", "-test.count=1")
	c.Env = append(os.Environ(), simChildFlag+"=1", simChildArgs+"="+string(raw), simChildEnv+"="+simEnvSnapshot(), simChildStor+"="+storageURL)
	var out, errOut bytes.Buffer
	c.Stdout, c.Stderr = &out, &errOut
	if err := c.Start(); err != nil {
		return simResult{code: -3, errText: err.Error()}
	}
	var killed bool
	var timer *time.Timer
	if killAfter > 0 {
		timer = time.AfterFunc(killAfter, func() { killed = true; _ = c.Process.Kill() })
	}
	err := c.Wait()
	if timer != nil {
		timer.Stop()
	}
	res := simResult{stdout: out.String(), stderr: errOut.String(), killed: killed}
	if ee, ok := err.(*exec.ExitError); ok {
		res.code = ee.ExitCode()
		if killed {
			res.code = -1
		}
	} else if err != nil {
		res.code = -3
	}
	if i := strings.Index(res.stderr, "GUARD_SIM_ERROR: "); i >= 0 {
		res.errText = strings.TrimSpace(res.stderr[i+len("GUARD_SIM_ERROR: "):])
	}
	return res
}

// ---------------------------------------------------------------- operations

type simOp struct {
	idx            int
	kind, mode     string
	args           []string
	start, end     time.Time
	res            simResult
	newBatch       bool
	retryUnknown   bool
	entities       []string // what the op selects (nil = every mailbox)
	task           string
	restoreBatchID string
	downloadKey    string
	notes          []string
	// watchAt is when the 'restore watch --once' an operator runs before
	// --retry-unknown (as the docs require) read the RESTORE task list.
	watchAt time.Time
}

type simRestoreCmd struct {
	args    []string
	batchID string
	ents    []string
}

type simState struct {
	t          *testing.T
	w          *simWorld
	rng        *rand.Rand
	dest       string
	ops        []*simOp
	rCmds      []simRestoreCmd
	dKeys      []string
	violations []string
	gaps       []string
	seenViol   map[string]bool
	// newBatches is when the --new-batch op that made each batch (restore
	// batch id or download batch key) started: a later create of that
	// batch, its resumes included, carries out that --new-batch.
	newBatches map[string]time.Time
	// labelBatch is every job label any download ledger has carried, by
	// batch: a row that is created once more gets a new label, so the
	// ledger alone forgets which batch sent an earlier create.
	labelBatch map[string]string
}

// noteLabels records the label of every download ledger row, by batch.
func (s *simState) noteLabels() {
	for key, l := range simLedgers(s.t) {
		for _, r := range l.Rows {
			if r.Label != "" {
				s.labelBatch[r.Label] = key
			}
		}
	}
}

// resumeTarget is the batch 'restore submit --resume' acts on for this
// entity filter: the newest batch recorded with the same resume key
// (ledgerFindBatchBy, created_at DESC), which a --new-batch run replaces.
// The date spellings restoreArgs draws all name one date, so the entity
// filter decides.
func (s *simState) resumeTarget(c simRestoreCmd) string {
	want := simEntityKey(c.ents)
	id := c.batchID
	for _, rc := range s.rCmds {
		if rc.batchID != "" && simEntityKey(rc.ents) == want {
			id = rc.batchID
		}
	}
	return id
}

func simEntityKey(ents []string) string {
	k := make([]string, 0, len(ents))
	for _, e := range ents {
		k = append(k, strings.ToLower(e))
	}
	sort.Strings(k)
	return strings.Join(k, ",")
}

func (s *simState) restoreArgs(ents []string) []string {
	date := []string{"2026-10-01", "2026-10-01T23:59:59Z", "20261001235959"}[s.rng.Intn(3)]
	a := []string{"restore", "submit", "--account", simAccount, "--task", simTask, "--date", date, "--resolve", "last-backup",
		"--rate", "1000", "--timeout", simRequestTimeout.String(), "--yes", "--json"}
	if len(ents) > 0 {
		a = append(a, "--entities", strings.Join(ents, ","))
	}
	return a
}

func (s *simState) pickEntities() []string {
	switch s.rng.Intn(4) {
	case 0:
		return []string{simEntities[0]}
	case 1:
		return []string{simEntities[1]}
	default:
		return nil
	}
}

func (s *simState) downloadArgs(ents []string) []string {
	a := []string{"download", "submit", "--account", simAccount, "--task", simTask, "--yes", "--json", "--timeout", simRequestTimeout.String(), "--portal-url", s.w.portal.URL}
	if s.rng.Intn(5) < 2 {
		a = append(a, "--on-date", "2026-09-30")
	} else {
		a = append(a, "--latest")
	}
	if len(ents) > 0 {
		a = append(a, "--entities", strings.Join(ents, ","))
	}
	switch s.rng.Intn(6) {
	case 0:
		a = append(a, "--utc-offset", "-5")
	case 1:
		a = append(a, "--utc-offset", "3")
	}
	return a
}

// scriptOp builds op idx from a GUARD_SIM_SCRIPT step such as "dSubmit=a"
// (kind, then an optional mailbox filter: a, b, or a,b): a fixed sequence
// for replaying a finding with the fewest operations. Resumes, watches, and
// fetches act on the newest batch.
func (s *simState) scriptOp(idx int, step string) *simOp {
	kind, filter, _ := strings.Cut(strings.TrimSpace(step), "=")
	var ents []string
	for _, f := range strings.Split(filter, ",") {
		switch strings.TrimSpace(f) {
		case "a":
			ents = append(ents, simEntities[0])
		case "b":
			ents = append(ents, simEntities[1])
		}
	}
	op := &simOp{idx: idx, task: simTask, kind: kind, entities: ents, mode: "inproc"}
	portalArgs := []string{"--portal-url", s.w.portal.URL}
	lastR := func() simRestoreCmd {
		if len(s.rCmds) == 0 {
			s.t.Fatalf("script step %q needs an earlier restore submit", step)
		}
		return s.rCmds[len(s.rCmds)-1]
	}
	lastD := func() string {
		if len(s.dKeys) == 0 {
			s.t.Fatalf("script step %q needs an earlier download batch", step)
		}
		return s.dKeys[len(s.dKeys)-1]
	}
	switch kind {
	case "rSubmit", "rNewBatch":
		op.args = s.restoreArgs(ents)
		if kind == "rNewBatch" {
			op.args, op.newBatch = append(op.args, "--new-batch"), true
		}
	case "rResume":
		c := lastR()
		op.args, op.entities, op.restoreBatchID = append(append([]string{}, c.args...), "--resume"), c.ents, s.resumeTarget(c)
	case "rRetry":
		c := lastR()
		op.args, op.entities, op.restoreBatchID, op.retryUnknown = append(append([]string{}, c.args...), "--resume", "--retry-unknown"), c.ents, s.resumeTarget(c), true
	case "rWatch":
		c := lastR()
		op.args, op.restoreBatchID = []string{"restore", "watch", "--batch", c.batchID, "--json", "--interval", "40ms", "--max-wait", "300ms"}, c.batchID
	case "dSubmit", "dNewBatch":
		op.args = append([]string{"download", "submit", "--account", simAccount, "--task", simTask, "--latest", "--yes", "--json"}, portalArgs...)
		if len(ents) > 0 {
			op.args = append(op.args, "--entities", strings.Join(ents, ","))
		}
		if kind == "dNewBatch" {
			op.args, op.newBatch = append(op.args, "--new-batch"), true
		}
	case "dResume", "dRetry":
		op.downloadKey = lastD()
		op.args = append([]string{"download", "submit", "--batch", op.downloadKey, "--resume", "--yes", "--json"}, portalArgs...)
		if kind == "dRetry" {
			op.args, op.retryUnknown = append(op.args, "--retry-unknown"), true
		}
	case "dFetch":
		op.downloadKey = lastD()
		op.args = append([]string{"download", "fetch", "--account", simAccount, "--dest", s.dest, "--year", "2022", "--batch", op.downloadKey, "--json"}, portalArgs...)
	case "dStart":
		op.task, op.entities = simGTask, []string{simGEntity}
		op.args = append([]string{"download", "start", "--account", simAccount, "--task", simGTask, "--entity", simGEntity, "--date", "2025-12-31", "--yes", "--json"}, portalArgs...)
	default:
		s.t.Fatalf("unknown script step %q", step)
	}
	return op
}

// nextOp draws one operation.
func (s *simState) nextOp(idx int) *simOp {
	op := &simOp{idx: idx, task: simTask}
	kinds := []simChoice{{"rSubmit", 12}, {"rResume", 7}, {"rRetry", 6}, {"rNewBatch", 4}, {"rWatch", 7},
		{"dSubmit", 12}, {"dResume", 9}, {"dRetry", 4}, {"dNewBatch", 4}, {"dStart", 5}, {"dFetch", 8}}
	for {
		op.kind = simPick(s.rng, kinds)
		switch op.kind {
		case "rSubmit", "rNewBatch":
			op.entities = s.pickEntities()
			op.args = s.restoreArgs(op.entities)
			if op.kind == "rNewBatch" {
				op.args, op.newBatch = append(op.args, "--new-batch"), true
			}
			return op
		case "rResume", "rRetry", "rWatch":
			if len(s.rCmds) == 0 {
				continue
			}
			c := s.rCmds[s.rng.Intn(len(s.rCmds))]
			op.entities, op.restoreBatchID = c.ents, c.batchID
			switch op.kind {
			case "rResume":
				op.args = append(append([]string{}, c.args...), "--resume")
				op.restoreBatchID = s.resumeTarget(c)
			case "rRetry":
				if c.batchID == "" {
					continue
				}
				// The operator watches the batch the resume will act on,
				// as its own UNKNOWN message says, then runs the resume.
				op.restoreBatchID = s.resumeTarget(c)
				op.args, op.retryUnknown = append(append([]string{}, c.args...), "--resume", "--retry-unknown"), true
			case "rWatch":
				if c.batchID == "" {
					continue
				}
				op.args = []string{"restore", "watch", "--batch", c.batchID, "--json", "--interval", "40ms", "--max-wait", "300ms"}
			}
			return op
		case "dSubmit", "dNewBatch":
			op.entities = s.pickEntities()
			op.args = s.downloadArgs(op.entities)
			if op.kind == "dNewBatch" {
				op.args, op.newBatch = append(op.args, "--new-batch"), true
			}
			return op
		case "dResume", "dRetry", "dFetch":
			if len(s.dKeys) == 0 {
				continue
			}
			op.downloadKey = s.dKeys[s.rng.Intn(len(s.dKeys))]
			switch op.kind {
			case "dResume":
				op.args = []string{"download", "submit", "--batch", op.downloadKey, "--resume", "--yes", "--json", "--portal-url", s.w.portal.URL}
			case "dRetry":
				op.args = []string{"download", "submit", "--batch", op.downloadKey, "--resume", "--retry-unknown", "--yes", "--json", "--portal-url", s.w.portal.URL}
				op.retryUnknown = true
			case "dFetch":
				op.args = []string{"download", "fetch", "--account", simAccount, "--dest", s.dest, "--year", "2022", "--batch", op.downloadKey,
					"--json", "--portal-url", s.w.portal.URL}
			}
			return op
		case "dStart":
			op.task, op.entities = simGTask, []string{simGEntity}
			op.args = []string{"download", "start", "--account", simAccount, "--task", simGTask, "--entity", simGEntity,
				"--date", "2025-12-31", "--yes", "--json", "--portal-url", s.w.portal.URL}
			return op
		}
	}
}

// watchFirst runs the 'restore watch --once' the docs require before
// --retry-unknown, so a resend is judged on what the watch could see.
func (s *simState) watchFirst(op *simOp) {
	if op.kind != "rRetry" {
		return
	}
	op.watchAt = time.Now()
	res := simRunInProcess(context.Background(), []string{"restore", "watch", "--batch", op.restoreBatchID, "--once", "--json"})
	op.notes = append(op.notes, fmt.Sprintf("watched first: exit %d", res.code))
}

// run executes op (and maybe a concurrent twin) in the drawn mode.
func (s *simState) run(op *simOp, childOK bool) []*simOp {
	mode := "inproc"
	switch n := s.rng.Intn(20); {
	case n < 2:
		mode = "cancel"
	case n < 4 && childOK:
		mode = "kill"
	case n < 6 && childOK:
		mode = "pair"
	}
	op.mode = mode
	ops := []*simOp{op}
	s.watchFirst(op)
	switch mode {
	case "inproc":
		op.start = time.Now()
		op.res = simRunInProcess(context.Background(), op.args)
		op.end = time.Now()
	case "cancel":
		delay := time.Duration(s.rng.Intn(400)) * time.Millisecond
		op.mode = fmt.Sprintf("cancel@%s", delay)
		ctx, cancel := context.WithCancel(context.Background())
		timer := time.AfterFunc(delay, cancel)
		op.start = time.Now()
		op.res = simRunInProcess(ctx, op.args)
		op.end = time.Now()
		timer.Stop()
		cancel()
	case "kill":
		// A child takes some 20-40ms to start; most ops then finish within
		// a few hundred, so this lands across startup, planning, the
		// creates, and the reconcile wait.
		delay := time.Duration(20+s.rng.Intn(350)) * time.Millisecond
		op.mode = fmt.Sprintf("kill@%s", delay)
		op.start = time.Now()
		op.res = simRunChild(op.args, s.w.storage.URL, delay)
		op.end = time.Now()
	case "pair":
		twin := s.nextOp(op.idx)
		twin.mode, op.mode = "pair", "pair"
		s.watchFirst(twin)
		ops = append(ops, twin)
		var wg sync.WaitGroup
		for _, o := range ops {
			wg.Add(1)
			go func(o *simOp) {
				defer wg.Done()
				o.start = time.Now()
				o.res = simRunChild(o.args, s.w.storage.URL, 0)
				o.end = time.Now()
			}(o)
		}
		wg.Wait()
	}
	return ops
}

// learn records batches an op created, for later resumes, watches, fetches.
func (s *simState) learn(op *simOp) {
	switch {
	case strings.HasPrefix(op.kind, "r") && op.kind != "rWatch":
		var out struct {
			BatchID string `json:"batch_id"`
		}
		_ = json.Unmarshal([]byte(op.res.stdout), &out)
		if out.BatchID == "" && (op.kind == "rSubmit" || op.kind == "rNewBatch") {
			// A run killed or cancelled before it printed: its batch, if
			// it recorded one, is in the ledger.
			out.BatchID = s.unlearnedRestoreBatch(op.start)
		}
		if out.BatchID != "" {
			op.restoreBatchID = out.BatchID
		}
		if op.kind == "rNewBatch" && out.BatchID != "" {
			s.newBatches[out.BatchID] = op.start
		}
		if op.kind == "rSubmit" || op.kind == "rNewBatch" {
			base := op.args[:len(op.args)-btoi(op.newBatch)]
			s.rCmds = append(s.rCmds, simRestoreCmd{args: append([]string{}, base...), batchID: out.BatchID, ents: op.entities})
		}
	case op.kind == "dSubmit" || op.kind == "dNewBatch" || op.kind == "dStart":
		key := ""
		var out map[string]any
		if json.Unmarshal([]byte(op.res.stdout), &out) == nil {
			for _, k := range []string{"batch_key", "batch_id"} {
				if v, ok := out[k].(string); ok && v != "" {
					key = v
					break
				}
			}
		}
		if key == "" {
			if i := strings.Index(op.res.stderr, "batch "); i >= 0 {
				if f := strings.Fields(op.res.stderr[i+len("batch "):]); len(f) > 1 && f[1] == "recorded" {
					key = f[0]
				}
			}
		}
		if key != "" {
			op.downloadKey = key
			if op.kind == "dNewBatch" {
				s.newBatches[key] = op.start
			}
			for _, k := range s.dKeys {
				if k == key {
					return
				}
			}
			s.dKeys = append(s.dKeys, key)
		}
	}
}

// unlearnedRestoreBatch is the newest restore batch the ledger recorded
// since start that no earlier op's output named, or "".
func (s *simState) unlearnedRestoreBatch(start time.Time) string {
	res := simRunInProcess(context.Background(), []string{"restore", "history", "--json"})
	var batches []batchSummary
	if json.Unmarshal([]byte(res.stdout), &batches) != nil {
		return ""
	}
	known := map[string]bool{}
	for _, c := range s.rCmds {
		known[c.batchID] = true
	}
	best := ""
	var bestAt time.Time
	for _, b := range batches {
		if known[b.BatchID] || b.CreatedAt.Before(start.Add(-time.Second)) {
			continue
		}
		if best == "" || b.CreatedAt.After(bestAt) {
			best, bestAt = b.BatchID, b.CreatedAt
		}
	}
	return best
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------- invariants

func (s *simState) violate(kind, msg string) {
	key := kind + ": " + msg
	if s.seenViol[key] {
		return
	}
	s.seenViol[key] = true
	s.violations = append(s.violations, key)
}

// newBatchCovers reports whether a --new-batch op that selects identity's
// mailbox was running when the second creation at t happened.
func (s *simState) newBatchCovers(task, entity string, first, t time.Time, restore bool) bool {
	for _, op := range s.ops {
		if !op.newBatch || op.task != task || (restore != strings.HasPrefix(op.kind, "r")) {
			continue
		}
		if len(op.entities) > 0 && !simContains(op.entities, entity) {
			continue
		}
		if op.end.After(first) && !t.Before(op.start.Add(-50*time.Millisecond)) && !t.After(op.end.Add(50*time.Millisecond)) {
			return true
		}
	}
	return false
}

// restoreLineageCovers reports whether the second create came from a resume
// of a batch a --new-batch op made after the first: the resume carries out
// that --new-batch.
func (s *simState) restoreLineageCovers(first, t time.Time) bool {
	for _, op := range s.opAt(t) {
		if op.kind != "rResume" && op.kind != "rRetry" {
			continue
		}
		if at, ok := s.newBatches[op.restoreBatchID]; ok && at.After(first) {
			return true
		}
	}
	return false
}

// exportLineageCovers reports whether the second create belongs to a batch a
// --new-batch op made (by every label its ledger has carried) whose own first
// create of this archive came after the first: that --new-batch targeted the
// archive after it existed, and its later creates (its resumes) carry it out.
// Called with s.w.mu not held.
func (s *simState) exportLineageCovers(c1, c2 *simExport) bool {
	if c2.Label == "" {
		return false
	}
	s.noteLabels()
	batch := s.labelBatch[c2.Label]
	if _, ok := s.newBatches[batch]; !ok || batch == "" {
		return false
	}
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	var firstOwn time.Time
	for _, e := range s.w.exports {
		if e.Identity == c1.Identity && e.Label != "" && s.labelBatch[e.Label] == batch && (firstOwn.IsZero() || e.At.Before(firstOwn)) {
			firstOwn = e.At
		}
	}
	return !firstOwn.IsZero() && firstOwn.After(c1.At)
}

// opAt names the ops running at t.
func (s *simState) opAt(t time.Time) []*simOp {
	var out []*simOp
	for _, op := range s.ops {
		if !t.Before(op.start.Add(-50*time.Millisecond)) && !t.After(op.end.Add(50*time.Millisecond)) {
			out = append(out, op)
		}
	}
	return out
}

func simContains(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func (s *simState) checkI1() {
	s.w.mu.Lock()
	byID := map[string][]*simRestore{}
	for _, rs := range s.w.restores {
		if rs.Created {
			byID[rs.Identity] = append(byID[rs.Identity], rs)
		}
	}
	s.w.mu.Unlock()
	for id, list := range byID {
		for i := 1; i < len(list); i++ {
			c1, c2 := list[i-1], list[i]
			if s.newBatchCovers(simTask, c2.Entity, c1.At, c2.At, true) || s.restoreLineageCovers(c1.At, c2.At) {
				continue
			}
			var retry *simOp
			for _, op := range s.opAt(c2.At) {
				if op.retryUnknown {
					retry = op
				}
			}
			if retry != nil && retry.watchAt.Before(c1.VisibleAt) {
				s.gap(fmt.Sprintf("I1 restore %s created twice (%s %s, then %s %s): the first task was not listed yet when the watch before --retry-unknown read the task list", id, c1.ID, c1.Decision, c2.ID, c2.Decision))
				continue
			}
			s.violate("I1", fmt.Sprintf("restore %s created twice without --new-batch: %s (%s at %s) then %s (%s at %s); ops running then: %s",
				id, c1.ID, c1.Decision, s.rel(c1.At), c2.ID, c2.Decision, s.rel(c2.At), s.opNames(c2.At)))
		}
	}
}

func (s *simState) checkI2() {
	s.w.mu.Lock()
	byID := map[string][]*simExport{}
	for _, e := range s.w.exports {
		if e.Created && !e.Empty {
			byID[e.Identity] = append(byID[e.Identity], e)
		}
	}
	type pair struct{ c1, c2 *simExport }
	var pairs []pair
	for _, list := range byID {
		for i := 1; i < len(list); i++ {
			pairs = append(pairs, pair{list[i-1], list[i]})
		}
	}
	shownFirstPage := func(e *simExport, t time.Time) bool {
		if s.w.pageCap == 0 {
			return true
		}
		newer := 0
		for _, o := range s.w.exports {
			if s.w.listed(o, t) && o.At.After(e.At) {
				newer++
			}
		}
		return newer < s.w.pageCap
	}
	type verdict struct {
		p      pair
		reason string
	}
	// confirmed is every job a ledger row was ever confirmed to: a
	// confirmed row keeps its job id for good.
	confirmed := s.confirmedJobs()
	var verdicts []verdict
	for _, p := range pairs {
		c1, c2 := p.c1, p.c2
		switch {
		case !c1.FinalAt.After(c2.At) && c1.FinalStatus != portal.StatusReadyForDownload:
			continue // JOB_FAILED: the documented re-create
		case c2.At.Before(c1.VisibleAt):
			verdicts = append(verdicts, verdict{p, "gap"})
		case !c1.ExpireAt.IsZero() && !c2.At.Before(c1.ExpireAt) && !confirmed[c1.ID]:
			// The fake expires an archive within a second of finishing,
			// far inside the hour the CLI waits before concluding a create
			// MISSING; in the field archives last days. A job that left the
			// listing before any batch confirmed it gave the CLI no
			// evidence, the same documented risk as a job listed too late.
			verdicts = append(verdicts, verdict{p, "expired-gap"})
		case s.w.listed(c1, c2.At) && !shownFirstPage(c1, c2.At):
			verdicts = append(verdicts, verdict{p, "hidden past the first page; the portal reported more pages"})
		default:
			verdicts = append(verdicts, verdict{p, "listed"})
		}
	}
	s.w.mu.Unlock()
	for _, v := range verdicts {
		c1, c2 := v.p.c1, v.p.c2
		task := strings.SplitN(c1.Identity, "|", 2)[0]
		if s.newBatchCovers(task, c2.Entity, c1.At, c2.At, false) || s.exportLineageCovers(c1, c2) {
			continue
		}
		if v.reason == "gap" {
			s.gap(fmt.Sprintf("I2 export %s created twice (%s %s, then %s %s): the first job was not listed yet when the second was sent", c1.Identity, c1.ID, c1.Decision, c2.ID, c2.Decision))
			continue
		}
		if v.reason == "expired-gap" {
			s.gap(fmt.Sprintf("I2 export %s created twice (%s %s, then %s %s): the first job expired off the listing before any batch confirmed it", c1.Identity, c1.ID, c1.Decision, c2.ID, c2.Decision))
			continue
		}
		s.violate("I2", fmt.Sprintf("export %s created twice without --new-batch: %s (%s at %s, listed from %s) then %s (%s at %s); first job %s; ops running then: %s",
			c1.Identity, c1.ID, c1.Decision, s.rel(c1.At), s.rel(c1.VisibleAt), c2.ID, c2.Decision, s.rel(c2.At), v.reason, s.opNames(c2.At)))
	}
}

func (s *simState) gap(msg string) {
	for _, g := range s.gaps {
		if g == msg {
			return
		}
	}
	s.gaps = append(s.gaps, msg)
}

// confirmedJobs is every job id a download ledger row names.
func (s *simState) confirmedJobs() map[string]bool {
	out := map[string]bool{}
	for _, l := range simLedgers(s.t) {
		for _, r := range l.Rows {
			if r.JobID != "" {
				out[r.JobID] = true
			}
		}
	}
	return out
}

// simFetchOutcome is the part of a 'download fetch --json' outcome the
// checks read.
type simFetchOutcome struct {
	Entity    string `json:"entity"`
	SubSource string `json:"sub_source"`
	JobID     string `json:"job_id"`
	Path      string `json:"path"`
	Status    string `json:"status"`
}

// simLedgerRow is the part of a download ledger row the checks read.
type simLedgerRow struct {
	Entity      string `json:"entity"`
	SubSource   string `json:"sub_source"`
	SnapshotRaw int64  `json:"snapshot_raw"`
	Status      string `json:"status"`
	JobID       string `json:"job_id"`
	Label       string `json:"label"`
	HeldBy      string `json:"held_by"`
}

type simLedger struct {
	BatchKey string         `json:"batch_key"`
	TaskID   string         `json:"task_id"`
	Rows     []simLedgerRow `json:"rows"`
}

func simLedgers(t *testing.T) map[string]simLedger {
	dir, err := cliutil.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "download-batches", "*.json"))
	out := map[string]simLedger{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var l simLedger
		if json.Unmarshal(raw, &l) == nil && l.BatchKey != "" {
			out[l.BatchKey] = l
		}
	}
	return out
}

// checkI5 reads every download ledger: an EMPTY row must be a job the fake
// created empty.
func (s *simState) checkI5() {
	for key, l := range simLedgers(s.t) {
		for _, r := range l.Rows {
			if r.Status != "EMPTY" {
				continue
			}
			creates := s.exportsByLabel(r.Label)
			emptyOne := false
			// expiredUnseen: every real job with the label expired off the
			// listing and no batch ever confirmed it, the expiry evidence
			// gap checkI2 describes.
			expiredUnseen, real := true, 0
			confirmed := s.confirmedJobs()
			var what []string
			for _, e := range creates {
				emptyOne = emptyOne || (e.Created && e.Empty)
				if e.Created && !e.Empty {
					real++
					if e.ExpireAt.IsZero() || confirmed[e.ID] {
						expiredUnseen = false
					}
				}
				what = append(what, fmt.Sprintf("%s was %s (created=%v empty=%v, listed from %s, final %s)", e.ID, e.Decision, e.Created, e.Empty, s.rel(e.VisibleAt), e.FinalStatus))
			}
			if !emptyOne && real > 0 && expiredUnseen {
				s.gap(fmt.Sprintf("I5 batch %s row %s %s is EMPTY: its job expired off the listing before any batch confirmed it", key, r.Entity, r.SubSource))
				continue
			}
			if !emptyOne {
				if len(what) == 0 {
					what = []string{"no create carried its label"}
				}
				s.violate("I5", fmt.Sprintf("batch %s row %s %s is EMPTY but no create with its label made an empty job: %s", key, r.Entity, r.SubSource, strings.Join(what, "; ")))
			}
		}
	}
}

// exportsByLabel returns every create that carried a row's label: a resend
// of a row reuses its label.
func (s *simState) exportsByLabel(label string) []*simExport {
	if label == "" {
		return nil
	}
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	var out []*simExport
	for _, e := range s.w.exports {
		if e.Label != "" && (e.Label == label || strings.Contains(e.Label, label) || strings.Contains(label, e.Label)) {
			out = append(out, e)
		}
	}
	return out
}

var simNewBatchAllowed = []string{
	"on purpose", "a person can decide", "Do not use --new-batch", "would send", "would instead send", "--new-batch sends",
	"cannot be combined", "refused even with --new-batch",
}

// checkI4 flags a line that names --new-batch outside the documented
// person-decision and warning cases.
func (s *simState) checkI4(op *simOp) {
	text := op.res.stdout + "\n" + op.res.stderr + "\n" + op.res.errText
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "--new-batch") || strings.Contains(line, "GUARD_SIM_ERROR") && strings.Contains(op.res.stderr, line) {
			continue
		}
		ok := false
		for _, a := range simNewBatchAllowed {
			ok = ok || strings.Contains(line, a)
		}
		if !ok {
			s.violate("I4", fmt.Sprintf("op %d (%s) steers to --new-batch: %q", op.idx, op.kind, simTrim(line, 300)))
		}
	}
}

// checkI3 holds an op that exited 0 to what it was asked for.
func (s *simState) checkI3(op *simOp) {
	if op.res.code != 0 || op.res.killed {
		return
	}
	switch op.kind {
	case "rSubmit", "rResume", "rRetry", "rNewBatch":
		var out struct {
			Rows []restoreJob `json:"rows"`
		}
		if err := json.Unmarshal([]byte(op.res.stdout), &out); err != nil {
			s.violate("I3", fmt.Sprintf("op %d (%s) exited 0 without its JSON summary: %v", op.idx, op.kind, err))
			return
		}
		for _, r := range out.Rows {
			if r.submitOutcome() != jobSubmitted {
				s.violate("I3", fmt.Sprintf("op %d (%s) exited 0 with row %s %s", op.idx, op.kind, r.Entity, r.submitOutcome()))
				continue
			}
			if !s.restoreAcceptedOrCreated(r.Entity, r.ResolvedRaw) {
				s.violate("I3", fmt.Sprintf("op %d (%s) exited 0 and counts %s as accepted, but the API neither answered success for it nor created it", op.idx, op.kind, r.Entity))
			}
		}
	case "rWatch":
		var out watchOutput
		if err := json.Unmarshal([]byte(op.res.stdout), &out); err != nil {
			s.violate("I3", fmt.Sprintf("op %d (rWatch) exited 0 without its JSON summary: %v", op.idx, err))
			return
		}
		for _, r := range out.Rows {
			if st := s.restoreTruth(r.Entity, r.ResolvedRaw, op.end); st != "COMPLETED" {
				s.violate("I3", fmt.Sprintf("op %d (rWatch) exited 0 but restore %s is %s in truth", op.idx, r.Entity, st))
			}
		}
	case "dSubmit", "dResume", "dRetry", "dNewBatch":
		var b simLedger
		if err := json.Unmarshal([]byte(op.res.stdout), &b); err != nil {
			s.violate("I3", fmt.Sprintf("op %d (%s) exited 0 without its JSON batch: %v", op.idx, op.kind, err))
			return
		}
		for _, r := range b.Rows {
			s.checkSettledRow(op, b, r)
		}
	case "dStart":
		if !s.exportAnswered(simGTask, simGEntity, op.start, op.end) {
			s.violate("I3", fmt.Sprintf("op %d (dStart) exited 0 but no create of %s was answered with success during it", op.idx, simGEntity))
		}
	case "dFetch":
		s.checkFetch(op)
	}
}

func (s *simState) checkSettledRow(op *simOp, b simLedger, r simLedgerRow) {
	switch r.Status {
	case "CREATED":
		s.w.mu.Lock()
		var job *simExport
		for _, e := range s.w.exports {
			if e.ID == r.JobID {
				job = e
			}
		}
		s.w.mu.Unlock()
		if job == nil || !job.Created || job.Empty || !strings.EqualFold(job.Entity, r.Entity) || job.Sub != r.SubSource || job.Snapshot != r.SnapshotRaw {
			s.violate("I3", fmt.Sprintf("op %d (%s) exited 0 with %s %s CREATED as %s, which is not a real job of that archive", op.idx, op.kind, r.Entity, r.SubSource, r.JobID))
		}
	case "EMPTY":
		// I5 checks it.
	case "SKIPPED":
		if !s.exportCreatedFor(r.Entity, r.SubSource, r.SnapshotRaw) {
			s.violate("I3", fmt.Sprintf("op %d (%s) exited 0 with %s %s SKIPPED (held by %s), but no job of that archive exists", op.idx, op.kind, r.Entity, r.SubSource, r.HeldBy))
		}
	default:
		s.violate("I3", fmt.Sprintf("op %d (%s) exited 0 with %s %s %s", op.idx, op.kind, r.Entity, r.SubSource, r.Status))
	}
}

func (s *simState) checkFetch(op *simOp) {
	var outcomes []simFetchOutcome
	if err := json.Unmarshal([]byte(op.res.stdout), &outcomes); err != nil {
		s.violate("I3", fmt.Sprintf("op %d (dFetch) exited 0 without its JSON outcomes: %v", op.idx, err))
		return
	}
	all := simLedgers(s.t)
	l, ok := all[op.downloadKey]
	if !ok {
		s.violate("I3", fmt.Sprintf("op %d (dFetch) exited 0 for batch %s, whose ledger cannot be read", op.idx, op.downloadKey))
		return
	}
	got := map[string]simFetchOutcome{}
	for _, o := range outcomes {
		got[strings.ToLower(o.Entity)+"|"+o.SubSource] = o
	}
	for _, r := range l.Rows {
		o, has := got[strings.ToLower(r.Entity)+"|"+r.SubSource]
		switch r.Status {
		case "EMPTY":
			continue
		case "CREATED":
		case "SKIPPED":
			// Collected through the batch that holds it: an EMPTY holder
			// row is nothing to download.
			if holder, ok := all[r.HeldBy]; ok && simHolderRowStatus(holder, r) == "EMPTY" {
				continue
			}
		default:
			s.violate("I3", fmt.Sprintf("op %d (dFetch) exited 0 while batch row %s %s is %s", op.idx, r.Entity, r.SubSource, r.Status))
			continue
		}
		if !has || o.Path == "" {
			s.violate("I3", fmt.Sprintf("op %d (dFetch) exited 0 without the archive of %s %s (%s)", op.idx, r.Entity, r.SubSource, r.Status))
			continue
		}
		body, err := os.ReadFile(o.Path)
		if err != nil || !s.isRealBodyOf(body, r) {
			s.violate("I3", fmt.Sprintf("op %d (dFetch) exited 0 but %s holds no real archive of %s %s", op.idx, o.Path, r.Entity, r.SubSource))
		}
	}
}

// simHolderRowStatus is the status of r's archive in the batch holding it.
func simHolderRowStatus(holder simLedger, r simLedgerRow) string {
	for _, h := range holder.Rows {
		if strings.EqualFold(h.Entity, r.Entity) && h.SubSource == r.SubSource && h.SnapshotRaw == r.SnapshotRaw {
			return h.Status
		}
	}
	return ""
}

func (s *simState) isRealBodyOf(body []byte, r simLedgerRow) bool {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	for _, e := range s.w.exports {
		if e.Created && !e.Empty && strings.EqualFold(e.Entity, r.Entity) && e.Sub == r.SubSource && e.Snapshot == r.SnapshotRaw && bytes.Equal(e.Body, body) {
			return true
		}
	}
	return false
}

func (s *simState) exportCreatedFor(entity, sub string, snap int64) bool {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	for _, e := range s.w.exports {
		if e.Created && strings.EqualFold(e.Entity, entity) && e.Sub == sub && e.Snapshot == snap {
			return true
		}
	}
	return false
}

func (s *simState) exportAnswered(task, entity string, from, to time.Time) bool {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	for _, e := range s.w.exports {
		if e.Task == task && strings.EqualFold(e.Entity, entity) && e.Answered && !e.At.Before(from.Add(-time.Second)) && !e.At.After(to) {
			return true
		}
	}
	return false
}

// restoreAcceptedOrCreated reports whether a row a submit counts as
// accepted has evidence behind it: the API answered success for it, or it
// exists (a 'restore watch' settles an UNKNOWN row SUBMITTED when it finds
// the RESTORE task, so a later resume rightly counts it accepted).
func (s *simState) restoreAcceptedOrCreated(entity string, resolved int64) bool {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	for _, rs := range s.w.restores {
		if strings.EqualFold(rs.Entity, entity) && rs.Resolved == resolved && (rs.Answered || rs.Created) {
			return true
		}
	}
	return false
}

// restoreTruth is the true state of a mailbox's restore at t: the newest
// real creation's status, or why there is none.
func (s *simState) restoreTruth(entity string, resolved int64, t time.Time) string {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	state := "never created"
	for _, rs := range s.w.restores {
		if !strings.EqualFold(rs.Entity, entity) || rs.Resolved != resolved || !rs.Created {
			continue
		}
		if t.Before(rs.FinalAt) {
			state = "still running"
		} else {
			state = rs.FinalStatus
		}
		if state == "COMPLETED" {
			return state
		}
	}
	return state
}

func (s *simState) rel(t time.Time) string {
	if len(s.ops) == 0 || t.IsZero() {
		return "-"
	}
	return fmt.Sprintf("+%dms", t.Sub(s.ops[0].start).Milliseconds())
}

func (s *simState) opNames(t time.Time) string {
	var names []string
	for _, op := range s.opAt(t) {
		names = append(names, fmt.Sprintf("#%d %s", op.idx, op.kind))
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

func simTrim(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// trace renders the seed's ops and the fake's creations for a replay.
func (s *simState) trace() string {
	var b strings.Builder
	for _, op := range s.ops {
		args := strings.Join(op.args, " ")
		args = strings.ReplaceAll(args, s.w.portal.URL, "<portal>")
		args = strings.ReplaceAll(args, s.dest, "<dest>")
		fmt.Fprintf(&b, "  #%d %s [%s, %s..%s] exit %d: %s\n", op.idx, op.kind, op.mode, s.rel(op.start), s.rel(op.end), op.res.code, args)
		if op.res.code != 0 && op.res.errText != "" {
			fmt.Fprintf(&b, "      error: %s\n", simTrim(op.res.errText, 220))
		}
		for _, n := range op.notes {
			fmt.Fprintf(&b, "      note: %s\n", n)
		}
	}
	keys := []string{}
	ledgers := simLedgers(s.t)
	for k := range ledgers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, r := range ledgers[k].Rows {
			fmt.Fprintf(&b, "  ledger %s: %s %s %d %s job=%s held_by=%s label=%q\n", k, r.Entity, r.SubSource, r.SnapshotRaw, r.Status, r.JobID, r.HeldBy, simTrim(r.Label, 40))
		}
	}
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	fmt.Fprintf(&b, "  fake (page cap %d):\n", s.w.pageCap)
	for _, rs := range s.w.restores {
		fmt.Fprintf(&b, "    restore %s %s %s at %s created=%v answered=%v final=%s\n", rs.ID, rs.Identity, rs.Decision, s.rel(rs.At), rs.Created, rs.Answered, rs.FinalStatus)
	}
	for _, e := range s.w.exports {
		fmt.Fprintf(&b, "    export %s %s %s at %s created=%v empty=%v answered=%v listed=%s final=%s label=%q\n", e.ID, e.Identity, e.Decision,
			s.rel(e.At), e.Created, e.Empty, e.Answered, s.rel(e.VisibleAt), e.FinalStatus, simTrim(e.Label, 40))
	}
	return b.String()
}

// ---------------------------------------------------------------- driver

// simRegressionSeeds are the seeds that found a violation, one per class
// (4 and 1014 replayed by every default run, all with
// GUARD_SIM_ALL_REGRESSION=1):
//
//   - 4: a create counted by the portal but listed only after the
//     reconcile wait went EMPTY, a terminal state, while its archive
//     existed (TestACountedCreateListedLateIsNeverEmpty);
//   - 1014 (also 1040): a row SKIPPED because another batch held its
//     archive counted as settled while the holder had never created it
//     (TestAHeldRowIsSettledOnlyWhenItsHolderIs);
//   - 10011: a listing stopped at a page the server capped short, so a
//     job on page 2 read as absent (TestDownloadJobsReadsEveryPageThePortalNames,
//     TestRestoreTaskFeedReadsEveryPageTheAPINames);
//   - 10309: restore watch settled a row on a later batch's task
//     (TestRestoreWatchNeverTakesALaterBatchsTask);
//   - 11416 (also 12059, 20185): --retry-unknown resent a row a RESTORE
//     task may already be (TestRetryUnknownNeverResendsARowATaskMayBe);
//   - 20125 (25 ops): an accepted create listed after the reconcile wait
//     went MISSING at once and the next resume created it again
//     (TestAnAcceptedCreateListedLateIsNeverResent).
var simRegressionSeeds = []struct {
	seed int64
	ops  int
}{{4, 10}, {1014, 10}, {10011, 10}, {10309, 10}, {11416, 10}, {20125, 25}}

func simEnvInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

func TestGuardInvariantSimulation(t *testing.T) {
	if os.Getenv(simChildFlag) == "1" {
		t.Skip("child process")
	}
	if testing.Short() {
		t.Skip("simulation runs real command sequences")
	}
	seeds := simEnvInt("GUARD_SIM_SEEDS", 0)
	ops := simEnvInt("GUARD_SIM_OPS", 10)
	first := int64(simEnvInt("GUARD_SIM_FIRST_SEED", 1))
	type run struct {
		seed int64
		ops  int
	}
	var list []run
	if one := os.Getenv("GUARD_SIM_SEED"); one != "" {
		v, _ := strconv.ParseInt(one, 10, 64)
		list = []run{{v, ops}}
	} else {
		for i := 0; i < seeds; i++ {
			list = append(list, run{first + int64(i), ops})
		}
		if os.Getenv("GUARD_SIM_SEEDS") == "" && os.Getenv("GUARD_SIM_FIRST_SEED") == "" {
			// A default run (go test, CI) replays the seeds that found the
			// two settlement bugs; GUARD_SIM_ALL_REGRESSION=1 replays one
			// seed of every class found, and a soak sets GUARD_SIM_SEEDS.
			for _, r := range simRegressionSeeds {
				if r.seed == 4 || r.seed == 1014 || os.Getenv("GUARD_SIM_ALL_REGRESSION") == "1" {
					list = append(list, run{r.seed, r.ops})
				}
			}
		}
	}
	// Windows: a child killed mid-write can leave a file locked for a
	// moment after the kill, which turns timing into failures that are not
	// the CLI's; the in-process modes (cancel included) still run there.
	childOK := os.Getenv("GUARD_SIM_NO_CHILD") != "1" && runtime.GOOS != "windows"
	var failed []int64
	totalGaps := 0
	for _, r := range list {
		seed, ops := r.seed, r.ops
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			s := simSeed(t, seed, ops, childOK)
			totalGaps += len(s.gaps)
			if len(s.gaps) > 0 {
				t.Logf("seed %d: %d documented-risk duplicate(s) (evidence gaps):\n  %s", seed, len(s.gaps), strings.Join(s.gaps, "\n  "))
			}
			if os.Getenv("GUARD_SIM_TRACE") == "1" {
				t.Logf("seed %d trace:\n%s", seed, s.trace())
			}
			if len(s.violations) > 0 {
				failed = append(failed, seed)
				replay := fmt.Sprintf("GUARD_SIM_SEED=%d GUARD_SIM_OPS=%d", seed, ops)
				if sc := os.Getenv("GUARD_SIM_SCRIPT"); sc != "" {
					replay = fmt.Sprintf("GUARD_SIM_SEED=%d GUARD_SIM_SCRIPT=%q", seed, sc)
				}
				t.Errorf("seed %d: %d invariant violation(s); replay with %s:\n  %s\nop trace:\n%s",
					seed, len(s.violations), replay, strings.Join(s.violations, "\n  "), s.trace())
			}
		})
	}
	t.Logf("guard simulation: %d seed(s); failing seeds %v; evidence-gap duplicates %d", len(list), failed, totalGaps)
}

func simSeed(t *testing.T, seed int64, nOps int, childOK bool) *simState {
	isolateAuthState(t)
	w := newSimWorld(t, seed)
	t.Setenv("CLOUDALLY_CLIENT_ID", "cid")
	t.Setenv("CLOUDALLY_CLIENT_SECRET", "csecret")
	t.Setenv("CLOUDALLY_BASE_URL", w.api.URL)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.BaseURL = w.api.URL
	if err := cfg.SaveTokens("cid", "csecret", "sim-token", "", time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	restoreSeams := simSeams(w.storage.URL)
	t.Cleanup(restoreSeams)
	s := &simState{t: t, w: w, rng: rand.New(rand.NewSource(seed)), dest: t.TempDir(), seenViol: map[string]bool{}, newBatches: map[string]time.Time{}, labelBatch: map[string]string{}}
	sawLedger := false
	var script []string
	if v := strings.TrimSpace(os.Getenv("GUARD_SIM_SCRIPT")); v != "" {
		script = strings.Split(v, ";")
		nOps = len(script)
	}
	for i := 1; i <= nOps; i++ {
		var ran []*simOp
		if script != nil {
			op := s.scriptOp(i, script[i-1])
			s.watchFirst(op)
			op.start = time.Now()
			op.res = simRunInProcess(context.Background(), op.args)
			op.end = time.Now()
			ran = []*simOp{op}
		} else {
			ran = s.run(s.nextOp(i), childOK)
		}
		s.noteLabels()
		for _, o := range ran {
			s.ops = append(s.ops, o)
			if o.res.code == -2 || strings.Contains(o.res.stderr, "panic:") {
				s.violate("P0", fmt.Sprintf("op %d (%s) panicked: %s %s", o.idx, o.kind, simTrim(o.res.errText, 300), simTrim(o.res.stderr, 300)))
			}
			s.learn(o)
			s.checkI3(o)
			s.checkI4(o)
		}
		s.checkI1()
		s.checkI2()
		s.checkI5()
		if len(simLedgers(t)) > 0 {
			sawLedger = true
		}
	}
	// Let every lag, final status, and expiry play out, then check again.
	time.Sleep(3 * time.Second)
	s.checkI1()
	s.checkI2()
	s.checkI5()
	if !sawLedger {
		for _, op := range s.ops {
			if strings.HasPrefix(op.kind, "d") && op.kind != "dFetch" && op.res.code == 0 {
				s.violate("SIM", "a download run exited 0 but no ledger was found where the simulation reads them; the I5 check saw nothing")
			}
		}
	}
	return s
}
