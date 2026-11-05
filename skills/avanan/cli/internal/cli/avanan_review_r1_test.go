// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards from the 4.34.0 reprint
// review, round 1.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"avanan-pp-cli/internal/avananmirror"
	"avanan-pp-cli/internal/store"
)

// reviewServer is a scriptable Avanan stand-in. Handlers are keyed by
// "METHOD /path"; the default answer is an empty success envelope.
type reviewServer struct {
	mu       sync.Mutex
	handlers map[string]func(w http.ResponseWriter, r *http.Request)
	calls    []string
	bodies   map[string][]string
}

func (s *reviewServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.Method + " " + r.URL.Path
	buf := new(bytes.Buffer)
	if r.Body != nil {
		_, _ = buf.ReadFrom(r.Body)
	}
	s.mu.Lock()
	s.calls = append(s.calls, key)
	if s.bodies == nil {
		s.bodies = map[string][]string{}
	}
	s.bodies[key] = append(s.bodies[key], buf.String())
	h := s.handlers[key]
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if h != nil {
		h(w, r)
		return
	}
	_, _ = w.Write([]byte(`{"responseEnvelope":{"responseCode":200,"responseText":"OK"},"responseData":[]}`))
}

func (s *reviewServer) count(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if c == key {
			n++
		}
	}
	return n
}

func jsonReply(body string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }
}

// runAgainst executes the root command against srv with an isolated home.
func runAgainst(t *testing.T, srv *httptest.Server, args ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AVANAN_HOME", dir)
	t.Setenv("AVANAN_CONFIG", filepath.Join(dir, "config.toml"))
	t.Setenv("AVANAN_BASE_URL", srv.URL)
	t.Setenv("AVANAN_APP_ID", "")
	t.Setenv("AVANAN_CLIENT_SECRET", "")
	root := RootCmd()
	root.SetArgs(args)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	return out.String(), err
}

// TestAuthLoginNeverPrintsTheApplicationID: the threat model treats the
// application ID as a credential, and `auth login --json` printed it.
func TestAuthLoginNeverPrintsTheApplicationID(t *testing.T) {
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
		"GET /v1.0/auth": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("jwt-token-value")) },
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	out, err := runAgainst(t, srv, "auth", "login", "--base-url", srv.URL,
		"--app-id", "US:myapp29", "--secret", "s3cr3t-value-123", "--json")
	if err != nil {
		t.Fatalf("auth login: %v\n%s", err, out)
	}
	for _, leak := range []string{"US:myapp29", "myapp29", "s3cr3t-value-123", "jwt-token-value"} {
		if strings.Contains(out, leak) {
			t.Errorf("auth login output contains credential material %q:\n%s", leak, out)
		}
	}
}

// TestRemediateRefusesWithoutYes: remediate quarantines or restores mail and
// is mirrored as an MCP tool, which never passes --yes. Without --yes it must
// not reach the action endpoint.
func TestRemediateRefusesWithoutYes(t *testing.T) {
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	out, err := runAgainst(t, srv, "remediate", "quarantine",
		"--entity", "f05b74da3ee859eea41aeac40aaad3c2",
		"--scope", "farm1:tenant-a", "--entity-type", "office365_emails_email", "--json")
	if err == nil {
		t.Fatalf("remediate without --yes succeeded\n%s", out)
	}
	if n := s.count("POST /v1.0/action/entity"); n != 0 {
		t.Fatalf("remediate without --yes sent %d action request(s)", n)
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("refusal does not name --yes: %v", err)
	}
}

// TestActionCommandFailsOnEnvelopeFailure: HTTP 200 with a failing
// responseEnvelope.responseCode (spec components.schemas ResponseEnvelope) is
// a failed action, not a success.
func TestActionCommandFailsOnEnvelopeFailure(t *testing.T) {
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
		"POST /v1.0/action/entity": jsonReply(`{"responseEnvelope":{"responseCode":400,"responseText":"Invalid scope"},"responseData":[]}`),
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	out, err := runAgainst(t, srv, "action", "post-entity",
		"--request-data-entity-action-name", "quarantine",
		"--request-data-entity-ids", "f05b74da3ee859eea41aeac40aaad3c2",
		"--request-data-entity-type", "office365_emails_email", "--json")
	if err == nil {
		t.Fatalf("a rejected action exited 0\n%s", out)
	}
	if strings.Contains(out, `"success":true`) {
		t.Errorf("a rejected action reported success:\n%s", out)
	}
}

const documentedEntityActionResponse = `{"responseEnvelope":{"requestId":"r1","responseCode":0,"responseText":"OK","recordsNumber":1},` +
	`"responseData":[{"entityId":"f05b74da3ee859eea41aeac40aaad3c2","taskId":"T-1"}]}`

func taskReply(status string) string {
	return `{"responseEnvelope":{"responseCode":0,"responseText":"OK"},"responseData":[{"taskId":"T-1","status":"` + status + `"}]}`
}

// sequence answers successive calls with successive bodies (the last repeats).
func sequence(bodies ...string) func(w http.ResponseWriter, r *http.Request) {
	var mu sync.Mutex
	n := 0
	return func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		i := n
		if i >= len(bodies) {
			i = len(bodies) - 1
		}
		n++
		mu.Unlock()
		_, _ = w.Write([]byte(bodies[i]))
	}
}

// TestActionWaitPollsTheDocumentedTask: with the documented
// SaasEntityActionResponse, --wait must poll /v1.0/task/{taskId} with the
// action's scope as a query-string parameter and return only once the task is
// terminal. The generic path read no task ID from responseData[], polled
// nothing and exited 0 the moment the quarantine was queued.
func TestActionWaitPollsTheDocumentedTask(t *testing.T) {
	var scopes []string
	var mu sync.Mutex
	next := sequence(taskReply("inprogress"), taskReply("completed"))
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
		"POST /v1.0/action/entity": jsonReply(documentedEntityActionResponse),
		"GET /v1.0/task/T-1": func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			scopes = append(scopes, r.URL.Query().Get("scope"))
			mu.Unlock()
			next(w, r)
		},
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	out, err := runAgainst(t, srv, "action", "post-entity",
		"--request-data-entity-action-name", "quarantine",
		"--request-data-entity-ids", "f05b74da3ee859eea41aeac40aaad3c2",
		"--request-data-entity-type", "office365_emails_email",
		"--request-data-scope", "farm1:tenant-a",
		"--wait", "--wait-timeout", "30s", "--json")
	if err != nil {
		t.Fatalf("--wait on a task that completed returned %v\n%s", err, out)
	}
	if n := s.count("GET /v1.0/task/T-1"); n < 2 {
		t.Fatalf("--wait polled task T-1 %d time(s), want it polled until terminal\n%s", n, out)
	}
	for _, sc := range scopes {
		if sc != "farm1:tenant-a" {
			t.Errorf("task poll sent scope=%q, want the action's scope as a query-string parameter", sc)
		}
	}
	if !strings.Contains(out, "completed") {
		t.Errorf("output does not report the terminal task status:\n%s", out)
	}
}

// TestActionWaitFailsClosedWithoutATaskID: Google Mail returns taskId null
// (live-verified, ledger task-poll-needs-query-string-scope). A 200 is not
// proof the action landed, so --wait must not exit 0.
func TestActionWaitFailsClosedWithoutATaskID(t *testing.T) {
	for _, body := range []string{
		`{"responseEnvelope":{"responseCode":0,"responseText":"OK"},"responseData":[{"entityId":"f05b74da3ee859eea41aeac40aaad3c2","taskId":null}]}`,
		`{"responseEnvelope":{"responseCode":0,"responseText":"OK"},"responseData":[]}`,
	} {
		s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
			"POST /v1.0/action/entity": jsonReply(body),
		}}
		srv := httptest.NewServer(s)
		out, err := runAgainst(t, srv, "action", "post-entity",
			"--request-data-entity-action-name", "quarantine",
			"--request-data-entity-ids", "f05b74da3ee859eea41aeac40aaad3c2",
			"--request-data-entity-type", "office365_emails_email",
			"--wait", "--wait-timeout", "5s", "--json")
		srv.Close()
		if err == nil {
			t.Fatalf("--wait with no task ID exited 0 for %s\n%s", body, out)
		}
		var ce *cliError
		if !errors.As(err, &ce) || ce.code != ExitJobPending {
			t.Errorf("--wait with no task ID returned %v, want exit %d (submitted, outcome unknown)", err, ExitJobPending)
		}
		if !strings.Contains(err.Error(), "Do not resubmit") {
			t.Errorf("error does not tell the operator the action was already submitted: %v", err)
		}
	}
}

// TestActionWaitReportsATerminalFailure: a task that ends failed is a failed
// action, whatever the HTTP status of the submission was.
func TestActionWaitReportsATerminalFailure(t *testing.T) {
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
		"POST /v1.0/action/event": jsonReply(`{"responseEnvelope":{"responseCode":0},"responseData":[{"eventId":"ev1","taskId":"T-1"}]}`),
		"GET /v1.0/task/T-1":      jsonReply(taskReply("failed")),
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	out, err := runAgainst(t, srv, "action", "post-event",
		"--request-data-event-action-name", "quarantine",
		"--request-data-event-ids", "ev1",
		"--wait", "--wait-timeout", "30s", "--json")
	if err == nil {
		t.Fatalf("--wait on a failed task exited 0\n%s", out)
	}
	if s.count("GET /v1.0/task/T-1") == 0 {
		t.Fatalf("post-event --wait never polled the task\n%s", out)
	}
}

// TestActionWaitWaitsForEveryTask: the documented response is an ARRAY of
// tasks; success needs all of them, not the first.
func TestActionWaitWaitsForEveryTask(t *testing.T) {
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
		"POST /v1.0/action/entity": jsonReply(`{"responseEnvelope":{"responseCode":0},"responseData":[` +
			`{"entityId":"a","taskId":"T-1"},{"entityId":"b","taskId":"T-2"}]}`),
		"GET /v1.0/task/T-1": jsonReply(taskReply("completed")),
		"GET /v1.0/task/T-2": jsonReply(`{"responseEnvelope":{"responseCode":0},"responseData":[{"taskId":"T-2","status":"failed"}]}`),
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	out, err := runAgainst(t, srv, "action", "post-entity",
		"--request-data-entity-action-name", "quarantine",
		"--request-data-entity-ids", "a,b",
		"--request-data-entity-type", "office365_emails_email",
		"--wait", "--wait-timeout", "30s", "--json")
	if err == nil {
		t.Fatalf("--wait reported success while task T-2 failed\n%s", out)
	}
	if s.count("GET /v1.0/task/T-2") == 0 {
		t.Fatalf("--wait never polled the second task\n%s", out)
	}
}

// restoreStateClient serves one entity state for every lookup.
type restoreStateClient struct{ payload string }

func (c restoreStateClient) Get(_ context.Context, path string, _ map[string]string) (json.RawMessage, error) {
	id := path[strings.LastIndex(path, "/")+1:]
	return json.RawMessage(`{"responseEnvelope":{"responseCode":200},"responseData":[{"entityInfo":{"entityId":"` + id +
		`"},"entityPayload":` + c.payload + `}]}`), nil
}

// TestRestoreIsNotConfirmedWithoutEvidence: with the live-verified null task,
// an entity that reads {"isQuarantined":false,"isRestored":false} (documented
// EmailEntityPayload fields) was reported restored immediately - although no
// restore is shown and the entity may never have been quarantined.
func TestRestoreIsNotConfirmedWithoutEvidence(t *testing.T) {
	for _, payload := range []string{
		`{"isQuarantined":false,"isRestored":false}`,
		`{}`,
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		applied, _ := pollEntityApplied(ctx, restoreStateClient{payload: payload}, []string{"e1"}, "farm:a", "restore", 2*time.Second, nil)
		cancel()
		if applied {
			t.Errorf("restore confirmed from %s with no restore evidence", payload)
		}
	}
}

func seedMirror(t *testing.T, rows map[string]map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mirror.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for resource, byID := range rows {
		for id, data := range byID {
			if err := db.Upsert(resource, id, json.RawMessage(data)); err != nil {
				t.Fatalf("seed %s/%s: %v", resource, id, err)
			}
		}
		if err := db.SaveSyncState(resource, "", len(byID)); err != nil {
			t.Fatalf("stamp %s: %v", resource, err)
		}
	}
	return path
}

func runLocal(t *testing.T, args ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AVANAN_HOME", dir)
	t.Setenv("AVANAN_CONFIG", filepath.Join(dir, "config.toml"))
	root := RootCmd()
	root.SetArgs(args)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	err := root.Execute()
	return out.String(), err
}

// TestCampaignCountsDocumentedRecipientArrays: EmailEntityPayload.to and
// .recipients are arrays (spec components.schemas EmailEntityPayload), and the
// scalar accessor read none of them, so every campaign reported 0 recipients.
func TestCampaignCountsDocumentedRecipientArrays(t *testing.T) {
	now := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	db := seedMirror(t, map[string]map[string]string{
		avananmirror.ResourceEvents: {
			"ev1": `{"eventId":"ev1","entityId":"e1","eventCreated":"` + now + `","type":"phishing","state":"new"}`,
			"ev2": `{"eventId":"ev2","entityId":"e2","eventCreated":"` + now + `","type":"phishing","state":"new"}`,
		},
		avananmirror.ResourceEntities: {
			"e1": `{"entityInfo":{"entityId":"e1"},"entityPayload":{"fromEmail":"x@evil.example","subject":"Invoice due","to":["a@tenant.example"],"recipients":["a@tenant.example"]}}`,
			"e2": `{"entityInfo":{"entityId":"e2"},"entityPayload":{"fromEmail":"x@evil.example","subject":"Invoice due","to":["b@tenant.example","c@tenant.example"]}}`,
		},
	})
	out, err := runLocal(t, "campaign", "--since", "7d", "--db", db, "--json")
	if err != nil {
		t.Fatalf("campaign: %v\n%s", err, out)
	}
	var report struct {
		Campaigns []struct {
			Messages   int `json:"messages"`
			Recipients int `json:"recipients"`
		} `json:"campaigns"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(report.Campaigns) != 1 {
		t.Fatalf("campaigns = %d, want 1\n%s", len(report.Campaigns), out)
	}
	if got := report.Campaigns[0].Recipients; got != 3 {
		t.Errorf("recipients = %d, want 3 (a, b, c from the documented to/recipients arrays)\n%s", got, out)
	}
}

func exceptionRow(sub, side, scope, match string, extra string) string {
	row := `{"sub_system":"` + sub + `","engine":"e","id":"` + match + `","match_string":"` + match + `","list_side":"` + side + `"`
	row += `,"partition":"` + sub + `/` + scope + `/"`
	if scope != "" {
		row += `,"scope":"` + scope + `"`
	}
	if extra != "" {
		row += "," + extra
	}
	return row + `,"raw":{}}`
}

// TestExceptionsFindMatchesOnLabelBoundaries: substring containment made a
// rule for badexample.com answer a query for example.com.
func TestExceptionsFindMatchesOnLabelBoundaries(t *testing.T) {
	db := seedMirror(t, map[string]map[string]string{
		avananmirror.ResourceExceptions: {
			"w/1": exceptionRow("anti-phishing-whitelist", "allow", "farm:a", "badexample.com", ""),
		},
	})
	out, err := runLocal(t, "exceptions", "find", "example.com", "--db", db, "--json")
	var ce *cliError
	if err == nil || !errors.As(err, &ce) || ce.code != 3 {
		t.Errorf("find example.com against a badexample.com rule = %v, want no match (exit 3)\n%s", err, out)
	}

	// The bidirectional cases the command documents must still hit.
	for _, tc := range []struct{ stored, query string }{
		{"example.com", "user@example.com"},
		{"user@example.com", "example.com"},
		{"mail.example.com", "example.com"},
		{"example.com", "mail.example.com"},
		{"Example.COM", "example.com"},
	} {
		db := seedMirror(t, map[string]map[string]string{
			avananmirror.ResourceExceptions: {"w/1": exceptionRow("anti-phishing-whitelist", "allow", "farm:a", tc.stored, "")},
		})
		if out, err := runLocal(t, "exceptions", "find", tc.query, "--db", db, "--json"); err != nil {
			t.Errorf("find %q against %q = %v, want a hit\n%s", tc.query, tc.stored, err, out)
		}
	}
}

// TestExceptionsAuditComparesWithinOneTenant: the same value allowed in one
// tenant and blocked in another is two tenants' policies, not a contradiction.
func TestExceptionsAuditComparesWithinOneTenant(t *testing.T) {
	db := seedMirror(t, map[string]map[string]string{
		avananmirror.ResourceExceptions: {
			"w/farm:A/1": exceptionRow("anti-phishing-whitelist", "allow", "farm:A", "partner.example", ""),
			"b/farm:B/1": exceptionRow("anti-phishing-blacklist", "block", "farm:B", "partner.example", ""),
		},
	})
	out, err := runLocal(t, "exceptions", "audit", "--skip-unmatched", "--db", db, "--json")
	if err != nil {
		t.Fatalf("audit: %v\n%s", err, out)
	}
	var report struct {
		Conflicts  []json.RawMessage `json:"conflicts"`
		Duplicates []json.RawMessage `json:"duplicates"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(report.Conflicts) != 0 || len(report.Duplicates) != 0 {
		t.Errorf("audit merged two tenants' policies: %s", out)
	}

	// Same tenant: still a conflict.
	db = seedMirror(t, map[string]map[string]string{
		avananmirror.ResourceExceptions: {
			"w/farm:A/1": exceptionRow("anti-phishing-whitelist", "allow", "farm:A", "partner.example", ""),
			"b/farm:A/1": exceptionRow("anti-phishing-blacklist", "block", "farm:A", "partner.example", ""),
		},
	})
	out, err = runLocal(t, "exceptions", "audit", "--skip-unmatched", "--db", db, "--json")
	if err != nil {
		t.Fatalf("audit: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if len(report.Conflicts) != 1 {
		t.Errorf("same-tenant allow+block not reported as a conflict: %s", out)
	}
}

// fakeExceptionPoster serves documented ApException lists per path.
type fakeExceptionPoster struct{ byPath map[string]string }

func (f fakeExceptionPoster) Post(context.Context, string, any) (json.RawMessage, int, error) {
	return nil, 0, errors.New("unused")
}

func (f fakeExceptionPoster) Get(_ context.Context, path string, _ map[string]string) (json.RawMessage, error) {
	if b, ok := f.byPath[path]; ok {
		return json.RawMessage(b), nil
	}
	return json.RawMessage(`{"responseEnvelope":{"responseCode":200},"responseData":[]}`), nil
}

// TestExceptionsKeepEveryDocumentedSelector: an ApException carrying only
// attachmentMd5 was unsearchable, and one carrying senderDomain + recipient
// was shown as a domain-wide rule (spec components.schemas ApException).
func TestExceptionsKeepEveryDocumentedSelector(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mirror.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p := fakeExceptionPoster{byPath: map[string]string{
		"/v1.0/exceptions/whitelist": `{"responseEnvelope":{"responseCode":200},"responseData":[` +
			`{"entityId":"x1","attachmentMd5":"0cc175b9c0f1b6a831c399e269772661"},` +
			`{"entityId":"x2","senderDomain":"partner.example","recipient":"ceo@tenant.example"}]}`,
	}}
	if _, problems := avananmirror.Exceptions(context.Background(), p, db, avananmirror.Options{Scopes: []string{"farm:a"}}); len(problems) != 0 {
		t.Fatalf("ingest problems: %v", problems)
	}
	if err := db.SaveSyncState(avananmirror.ResourceExceptions, "", 2); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if out, err := runLocal(t, "exceptions", "find", "0cc175b9c0f1b6a831c399e269772661", "--db", path, "--json"); err != nil {
		t.Errorf("attachment-hash exception not found: %v\n%s", err, out)
	}
	out, err := runLocal(t, "exceptions", "find", "partner.example", "--db", path, "--json")
	if err != nil {
		t.Fatalf("find partner.example: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ceo@tenant.example") {
		t.Errorf("a sender-domain + recipient rule was reported without its recipient condition:\n%s", out)
	}
}

// TestMirrorFailsWhenAResourceFailed: events failed while entities and
// exceptions succeeded; events was stamped fresh with zero rows and the run
// exited 0, so triage then answered "no detections" from data never received.
func TestMirrorFailsWhenAResourceFailed(t *testing.T) {
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
		"GET /v1.0/scopes":       jsonReply(`{"responseEnvelope":{"responseCode":200},"responseData":["farm:a"]}`),
		"POST /v1.0/event/query": jsonReply(`{"responseEnvelope":{"responseCode":400,"responseText":"bad request"},"responseData":[]}`),
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	dbPath := filepath.Join(t.TempDir(), "m.db")
	out, err := runAgainst(t, srv, "mirror", "--since", "1d", "--db", dbPath, "--json")
	if err == nil {
		t.Errorf("mirror exited 0 with the events query failing\n%s", out)
	}
	db, oerr := store.Open(dbPath)
	if oerr != nil {
		t.Fatal(oerr)
	}
	defer db.Close()
	if _, last, _, _ := db.GetSyncState(avananmirror.ResourceEvents); !last.IsZero() {
		t.Errorf("events stamped fresh at %v after its query failed", last)
	}
}

// TestAvananTaskIDsKeepsTheGenericSignals: the shared task-ID parser must
// still read every shape the generic ExtractJobID read, beside the documented
// responseData[] shape it adds.
func TestAvananTaskIDsKeepsTheGenericSignals(t *testing.T) {
	for body, want := range map[string]string{
		`{"taskId":"top"}`:                  "top",
		`{"data":{"taskId":"in-data"}}`:     "in-data",
		`{"job":{"taskId":"in-job"}}`:       "in-job",
		`{"result":{"taskId":"in-result"}}`: "in-result",
		documentedEntityActionResponse:      "T-1",
	} {
		ids, _, err := avananTaskIDs([]byte(body))
		if err != nil {
			t.Errorf("avananTaskIDs(%s) error = %v", body, err)
			continue
		}
		if len(ids) != 1 || ids[0] != want {
			t.Errorf("avananTaskIDs(%s) = %v, want [%s]", body, ids, want)
		}
	}
}

// TestEntityRecordsCarryATimestamp: a documented SaasEntity dates itself only
// in entityInfo.entityCreated, so timeline sorted every entity as undated.
func TestEntityRecordsCarryATimestamp(t *testing.T) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(`{"entityInfo":{"entityId":"e1","entityCreated":"2026-10-26T12:00:00Z"},"entityPayload":{"subject":"x"}}`), &obj); err != nil {
		t.Fatal(err)
	}
	if got := avananTime(obj); got.IsZero() {
		t.Error("entity with entityInfo.entityCreated parsed as undated")
	}
}

// TestRemediateReportsEachTargetsOwnTask: the documented action response is
// an array of {entityId, taskId}. remediate polled only the first task and
// stamped its status on every target, so a batch whose second task failed was
// reported completed, exit 0.
func TestRemediateReportsEachTargetsOwnTask(t *testing.T) {
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
		"POST /v1.0/action/entity": jsonReply(`{"responseEnvelope":{"responseCode":0},"responseData":[` +
			`{"entityId":"aaa","taskId":"T-1"},{"entityId":"bbb","taskId":"T-2"}]}`),
		"GET /v1.0/task/T-1": jsonReply(taskReply("completed")),
		"GET /v1.0/task/T-2": jsonReply(`{"responseEnvelope":{"responseCode":0},"responseData":[{"taskId":"T-2","status":"failed"}]}`),
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	out, err := runAgainst(t, srv, "remediate", "quarantine", "--entity", "aaa", "--entity", "bbb",
		"--scope", "farm1:tenant-a", "--entity-type", "office365_emails_email", "--wait", "--wait-timeout", "30s", "--yes", "--json")
	if err == nil {
		t.Errorf("remediate exited 0 with task T-2 failed\n%s", out)
	}
	if s.count("GET /v1.0/task/T-2") == 0 {
		t.Fatalf("remediate never polled the second task\n%s", out)
	}
	var report struct {
		Outcomes []struct {
			TargetID string `json:"target_id"`
			Status   string `json:"status"`
		} `json:"outcomes"`
	}
	if jerr := json.Unmarshal([]byte(out[:strings.LastIndex(out, "}")+1]), &report); jerr != nil {
		t.Fatalf("parse: %v\n%s", jerr, out)
	}
	got := map[string]string{}
	for _, o := range report.Outcomes {
		got[o.TargetID] = o.Status
	}
	if got["aaa"] != "completed" || got["bbb"] != "failed" {
		t.Errorf("per-target outcomes = %v, want aaa=completed bbb=failed", got)
	}
}
