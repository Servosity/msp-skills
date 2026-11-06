// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared fixtures for the slice-A ("local") novel command tests. Rows are
// shaped from the spec 200 response schemas of GET /phone/users,
// /phone/call_queues, /phone/devices, /phone/numbers, /phone/sites,
// /phone/emergency_addresses, /phone/locations and /phone/common_areas, and
// are written with the same store upsert sync uses.

package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"zoom-phone-pp-cli/internal/cliutil/testenv"
	"zoom-phone-pp-cli/internal/store"
)

const (
	localTestHQ     = `{"id":"site-hq","name":"San Jose HQ"}`
	localTestBoston = `{"id":"site-br","name":"Boston Branch"}`
	localTestAustin = `{"id":"site-au","name":"Austin"}`
)

var localTestUsers = []string{
	`{"id":"u-jane","email":"jane@example.com","name":"Jane Doe","extension_id":"ext-jane","extension_number":1001,"phone_user_id":"pu-jane","status":"activate","activation_status":"activated","department":"Sales","site":` + localTestHQ + `,"phone_numbers":[{"id":"pn-jane","number":"+14155550123"}],"calling_plans":[{"type":200,"name":"US/CA Unlimited Calling Plan"}]}`,
	`{"id":"u-bob","email":"bob@example.com","name":"Bob Smith","extension_id":"ext-bob","extension_number":1002,"status":"activate","activation_status":"activated","site":` + localTestHQ + `,"phone_numbers":[{"id":"pn-bob","number":"+14155550124"}],"calling_plans":[{"type":200,"name":"US/CA Unlimited Calling Plan"}]}`,
	`{"id":"u-carl","email":"carl@example.com","name":"Carl Gone","extension_id":"ext-carl","extension_number":1003,"status":"deactivate","activation_status":"not ready","site":` + localTestBoston + `,"phone_numbers":[{"id":"pn-carl","number":"+16175550100"}],"calling_plans":[]}`,
}

var localTestQueues = []string{
	`{"id":"cq-sales","name":"Sales","extension_id":"ext-cq-sales","extension_number":2001,"status":"active","site":` + localTestHQ + `,"phone_numbers":[{"id":"pn-cq","number":"+14155550199","source":"internal"}]}`,
	`{"id":"cq-support","name":"Support","extension_id":"ext-cq-support","extension_number":2002,"status":"active","site":` + localTestHQ + `}`,
	`{"id":"cq-billing","name":"Billing","extension_id":"ext-cq-billing","extension_number":2003,"status":"active","site":` + localTestHQ + `}`,
}

var localTestSLGs = []string{
	`{"id":"slg-front","display_name":"Front Desk","extension_id":"ext-slg","extension_number":3001,"status":"active","site":` + localTestHQ + `}`,
}

var localTestPickup = []string{
	`{"id":"gcp-1","display_name":"Sales Pickup","extension_id":"ext-gcp-1","extension_number":4001,"member_count":1,"delay":0,"site":` + localTestHQ + `}`,
	`{"id":"gcp-empty","display_name":"Empty Pickup","extension_id":"ext-gcp-2","extension_number":4002,"member_count":0,"delay":5,"site":` + localTestHQ + `}`,
}

var localTestMonitoring = []string{
	`{"id":"mg-1","name":"Sales Floor","type":1,"monitor_members_count":1,"monitored_members_count":0,"prompt":false,"site":` + localTestHQ + `}`,
}

var localTestAssignedDevices = []string{
	`{"id":"dev-jane","display_name":"Jane Desk","device_type":"Yealink T54W","mac_address":"80-5e-c0-00-00-01","status":"online","firmware_version":"96.86.0.30","provision_template_id":"tpl-1","site":` + localTestHQ + `,"assignee":{"id":"u-jane","name":"Jane Doe","extension_number":1001,"extension_type":"user"},"assignees":[{"id":"u-jane","name":"Jane Doe","extension_number":1001,"extension_type":"user","extension_id":"ext-jane"}]}`,
	`{"id":"dev-bob","display_name":"Bob Desk","device_type":"Yealink T54W","mac_address":"80-5e-c0-00-00-02","status":"online","firmware_version":"96.86.0.30","provision_template_id":"tpl-1","site":` + localTestHQ + `,"assignee":{"id":"u-bob","name":"Bob Smith","extension_number":1002,"extension_type":"user"},"assignees":[{"id":"u-bob","name":"Bob Smith","extension_number":1002,"extension_type":"user","extension_id":"ext-bob"}]}`,
	`{"id":"dev-carl","display_name":"Carl Desk","device_type":"Yealink T54W","mac_address":"80-5e-c0-00-00-03","status":"offline","firmware_version":"96.85.0.5","provision_template_id":"tpl-1","site":` + localTestBoston + `,"assignee":{"id":"u-carl","name":"Carl Gone","extension_number":1003,"extension_type":"user"},"assignees":[{"id":"u-carl","name":"Carl Gone","extension_number":1003,"extension_type":"user","extension_id":"ext-carl"}]}`,
	`{"id":"dev-lobby","display_name":"Lobby Phone","device_type":"Poly VVX 450","mac_address":"64-16-7f-00-00-04","status":"online","firmware_version":"6.4.0.1","provision_template_id":"","site":` + localTestHQ + `,"assignee":{"id":"ca-lobby","name":"Lobby","extension_number":1900,"extension_type":"commonArea"},"assignees":[{"id":"ca-lobby","name":"Lobby","extension_number":1900,"extension_type":"commonArea","extension_id":"ext-ca-lobby"}]}`,
}

var localTestUnassignedDevices = []string{
	`{"id":"dev-spare","display_name":"Spare T54W","device_type":"Yealink T54W","mac_address":"80-5e-c0-00-00-05","status":"offline","firmware_version":"96.86.0.30","site":` + localTestHQ + `}`,
}

var localTestNumbers = []string{
	`{"id":"pn-jane","number":"+14155550123","display_name":"Jane","source":"internal","status":"available","number_type":"toll","location":"San Jose, California, US","site":` + localTestHQ + `,"capability":["incoming","outgoing"],"assignee":{"id":"u-jane","name":"Jane Doe","type":"user","extension_number":1001}}`,
	`{"id":"pn-bob","number":"+14155550124","source":"internal","status":"available","number_type":"toll","site":` + localTestHQ + `,"assignee":{"id":"u-bob","name":"Bob Smith","type":"user","extension_number":1002}}`,
	`{"id":"pn-carl","number":"+16175550100","source":"internal","status":"available","number_type":"toll","site":` + localTestBoston + `,"assignee":{"id":"u-carl","name":"Carl Gone","type":"user","extension_number":1003}}`,
	`{"id":"pn-free1","number":"+14155550150","source":"internal","status":"available","number_type":"toll","site":` + localTestHQ + `}`,
	`{"id":"pn-free2","number":"+16175550150","source":"external","status":"available","number_type":"toll","site":` + localTestBoston + `}`,
	`{"id":"pn-pending","number":"+14155550160","source":"internal","status":"pending","number_type":"toll","site":` + localTestHQ + `}`,
	`{"id":"pn-cq","number":"+14155550199","source":"internal","status":"available","number_type":"tollfree","site":` + localTestHQ + `,"assignee":{"id":"cq-sales","name":"Sales","type":"callQueue","extension_number":2001}}`,
	`{"id":"pn-ghost","number":"+16175550170","source":"external","status":"available","number_type":"toll","site":` + localTestBoston + `,"assignee":{"id":"u-ghost","name":"Former Employee","type":"user","extension_number":1099}}`,
}

var localTestAddresses = []string{
	`{"id":"ea-hq","address_line1":"55 Almaden Blvd","city":"San Jose","state_code":"CA","zip":"95113","country":"US","is_default":true,"level":0,"status":4,"site":` + localTestHQ + `}`,
	`{"id":"ea-jane","address_line1":"100 Main St","city":"Campbell","state_code":"CA","zip":"95008","country":"US","is_default":true,"level":1,"status":4,"owner":{"id":"u-jane","name":"Jane Doe","extension_number":1001},"site":` + localTestHQ + `}`,
}

// localTestStore writes resource rows exactly as `sync --full` would
// (UpsertBatch, a completed sync_state checkpoint per resource, and the
// zoom_sync_scope record the sync hook writes for a full unfiltered walk)
// and returns the db path.
func localTestStore(t *testing.T, rows map[string][]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	for rt, items := range rows {
		raws := make([]json.RawMessage, 0, len(items))
		for _, it := range items {
			if !json.Valid([]byte(it)) {
				t.Fatalf("fixture %s is not valid JSON: %s", rt, it)
			}
			raws = append(raws, json.RawMessage(it))
		}
		if len(raws) > 0 {
			if _, _, err := db.UpsertBatch(rt, raws); err != nil {
				t.Fatalf("upsert %s: %v", rt, err)
			}
		}
		if err := db.SaveSyncState(rt, "", len(raws)); err != nil {
			t.Fatalf("sync state %s: %v", rt, err)
		}
		localTestRecordFullScope(t, db, rt)
		if rt == "numbers" {
			localTestRecordByocWalk(t, db, items)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	return path
}

// localTestRecordFullScope writes the scope record a full, unfiltered
// `sync --full` leaves for rt's current checkpoint.
func localTestRecordFullScope(t *testing.T, db *store.Store, rt string) {
	t.Helper()
	var ts sql.NullString
	if err := db.DB().QueryRow(`SELECT CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = ?`, rt).Scan(&ts); err != nil {
		t.Fatalf("%s checkpoint: %v", rt, err)
	}
	if err := localWriteSyncScope(db, rt, 0, "", ts.String, "test", true); err != nil {
		t.Fatalf("%s scope: %v", rt, err)
	}
}

// localTestRecordByocWalk models the additive BYOC walk that follows the
// full numbers sync: numbers rows carrying emergency_address_status are the
// rows a type=byoc walk wrote.
func localTestRecordByocWalk(t *testing.T, db *store.Store, items []string) {
	t.Helper()
	ids := []string{}
	for _, it := range items {
		var n localNumber
		if json.Unmarshal([]byte(it), &n) == nil && n.EmergencyAddressStatus != nil {
			ids = append(ids, n.ID.String())
		}
	}
	if len(ids) == 0 {
		return
	}
	var ts sql.NullString
	if err := db.DB().QueryRow(`SELECT CAST(last_synced_at AS TEXT) FROM sync_state WHERE resource_type = 'numbers'`).Scan(&ts); err != nil {
		t.Fatal(err)
	}
	if err := localWriteWalk(db, "numbers", "type=byoc", true, false, ts.String, "test", ids); err != nil {
		t.Fatal(err)
	}
}

func localTestFootprintRows() map[string][]string {
	return map[string][]string{
		"users":               localTestUsers,
		"call_queues":         localTestQueues,
		"shared_line_groups":  localTestSLGs,
		"group_call_pickup":   localTestPickup,
		"monitoring_groups":   localTestMonitoring,
		"assigned":            localTestAssignedDevices,
		"unassigned":          localTestUnassignedDevices,
		"numbers":             localTestNumbers,
		"emergency_addresses": localTestAddresses,
	}
}

// localTestEnv isolates config/state and clears the harness switches so
// writes reach the test server.
func localTestEnv(t *testing.T) {
	t.Helper()
	testenv.Isolate(t)
	for _, k := range []string{"PRINTING_PRESS_VERIFY", "PRINTING_PRESS_VERIFY_LIVE_HTTP", "PRINTING_PRESS_DOGFOOD",
		"ZOOM_PHONE_ACCOUNT_ID", "ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET", "ZOOM_PHONE_BASE_URL", "ZOOM_PHONE_TOKEN_URL"} {
		t.Setenv(k, "")
	}
}

// localTestServer serves the member endpoints and records every request.
type localTestServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string // "METHOD /path?query"
	deletes  []string // "/path" (without the /v2 prefix)
	// deleteStatus overrides the status for a DELETE path (default 204).
	deleteStatus map[string]int
	// deleteBody overrides the error body sent with deleteStatus.
	deleteBody map[string]string
	// getStatus overrides the status for a GET path (default 200).
	getStatus map[string]int
	// getBody overrides the 200 body for a GET path.
	getBody map[string]string
	// pages makes a GET path paginate forever: every page returns a new
	// next_page_token, so the caller hits its page cap.
	pages map[string]bool
	// firstDelay stalls the first request; firstOverlap records whether any
	// other request arrived while it was still in flight (a token-mint race).
	firstDelay   time.Duration
	seen         int
	firstActive  bool
	firstOverlap bool
}

func localNewTestServer(t *testing.T) *localTestServer {
	t.Helper()
	ts := &localTestServer{deleteStatus: map[string]int{}, deleteBody: map[string]string{}, getStatus: map[string]int{}, getBody: map[string]string{}, pages: map[string]bool{}}
	members := map[string]string{
		"/phone/call_queues":                         `{"call_queues":[` + strings.Join(localTestQueues, ",") + `],"next_page_token":"","page_size":100,"total_records":3}`,
		"/phone/shared_line_groups":                  `{"shared_line_groups":[` + strings.Join(localTestSLGs, ",") + `],"next_page_token":"","total_records":1}`,
		"/phone/group_call_pickup":                   `{"group_call_pickup":[` + strings.Join(localTestPickup, ",") + `],"next_page_token":"","total_records":2}`,
		"/phone/monitoring_groups":                   `{"monitoring_groups":[` + strings.Join(localTestMonitoring, ",") + `],"next_page_token":"","total_records":1}`,
		"/phone/group_call_pickup/gcp-empty/members": `{"group_call_pickup_member":[],"total_records":0}`,
		// GET /phone/users/{userId}: the live re-check offboard --apply runs before any DELETE.
		"/phone/users/u-jane":                    `{"id":"u-jane","email":"jane@example.com","extension_id":"ext-jane","extension_number":1001,"status":"activate","phone_user_id":"pu-jane"}`,
		"/phone/call_queues/cq-sales/members":    `{"call_queue_members":[{"id":"u-jane","level":"user","name":"Jane Doe","receive_call":true,"extension_id":"ext-jane"},{"id":"u-bob","level":"user","name":"Bob Smith","receive_call":true,"extension_id":"ext-bob"}],"page_size":30,"total_records":2,"next_page_token":""}`,
		"/phone/call_queues/cq-support/members":  `{"call_queue_members":[{"id":"u-jane","level":"user","name":"Jane Doe","receive_call":false,"extension_id":"ext-jane"}],"page_size":30,"total_records":1}`,
		"/phone/call_queues/cq-billing/members":  `{"call_queue_members":[{"id":"u-bob","level":"user","name":"Bob Smith","receive_call":true,"extension_id":"ext-bob"}],"page_size":30,"total_records":1}`,
		"/phone/shared_line_groups/slg-front":    `{"id":"slg-front","display_name":"Front Desk","extension_id":"ext-slg","extension_number":3001,"status":"active","members":{"users":[{"id":"u-bob","name":"Bob Smith","extension_id":"ext-bob"}],"common_areas":[{"id":"ca-lobby","name":"Lobby","extension_id":"ext-ca-lobby"}]},"phone_numbers":[]}`,
		"/phone/group_call_pickup/gcp-1/members": `{"group_call_pickup_member":[{"id":"u-bob","display_name":"Bob Smith","extension_id":"ext-bob","extension_type":"user","extension_number":1002}],"page_size":100,"total_records":1}`,
	}
	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v2")
		ts.mu.Lock()
		ts.seen++
		idx := ts.seen
		if ts.firstActive {
			ts.firstOverlap = true
		}
		if idx == 1 {
			ts.firstActive = true
		}
		delay := ts.firstDelay
		ts.mu.Unlock()
		if idx == 1 {
			defer func() {
				ts.mu.Lock()
				ts.firstActive = false
				ts.mu.Unlock()
			}()
			if delay > 0 {
				time.Sleep(delay)
			}
		}
		ts.mu.Lock()
		ts.requests = append(ts.requests, r.Method+" "+path+"?"+r.URL.RawQuery)
		if r.Method == http.MethodDelete {
			ts.deletes = append(ts.deletes, path)
		}
		delStatus, delOverride := ts.deleteStatus[path]
		delBody, hasBody := ts.deleteBody[path]
		getStatus, getOverride := ts.getStatus[path]
		getBody, hasGetBody := ts.getBody[path]
		endless := ts.pages[path]
		ts.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodDelete:
			if delOverride {
				if !hasBody {
					delBody = `{"code":` + map[bool]string{true: "404", false: "400"}[delStatus == 404] + `,"message":"fixture"}`
				}
				w.WriteHeader(delStatus)
				_, _ = w.Write([]byte(delBody))
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		case http.MethodGet:
			if getOverride {
				w.WriteHeader(getStatus)
				_, _ = w.Write([]byte(`{"code":300,"message":"fixture failure"}`))
				return
			}
			if endless {
				page := r.URL.Query().Get("next_page_token")
				_, _ = w.Write([]byte(`{"call_queue_members":[{"id":"u-other-` + page + `","level":"user","extension_id":"ext-other-` + page + `"}],"group_call_pickup_member":[],"members":[],"next_page_token":"p` + page + `x"}`))
				return
			}
			if hasGetBody {
				_, _ = w.Write([]byte(getBody))
				return
			}
			if path == "/phone/monitoring_groups/mg-1/monitor_members" {
				if r.URL.Query().Get("member_type") == "monitor" {
					_, _ = w.Write([]byte(`{"members":[{"display_name":"Jane Doe","extension_id":"ext-jane","extension_number":1001,"extension_type":"user"}],"page_size":100,"total_records":1}`))
				} else {
					_, _ = w.Write([]byte(`{"members":[],"total_records":0}`))
				}
				return
			}
			if body, ok := members[path]; ok {
				_, _ = w.Write([]byte(body))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
	}))
	t.Cleanup(ts.Close)
	t.Setenv("ZOOM_PHONE_BASE_URL", ts.URL+"/v2")
	return ts
}

func (ts *localTestServer) snapshot() ([]string, []string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return append([]string(nil), ts.requests...), append([]string(nil), ts.deletes...)
}

// localRun executes a command with --json semantics and returns stdout,
// stderr and the RunE error.
func localRun(t *testing.T, newCmd func(*rootFlags) *cobra.Command, args ...string) (string, string, error) {
	t.Helper()
	return localRunWith(t, &rootFlags{asJSON: true}, newCmd, args...)
}

// localRunWith is localRun with caller-built root flags (e.g. --agent).
func localRunWith(t *testing.T, flags *rootFlags, newCmd func(*rootFlags) *cobra.Command, args ...string) (string, string, error) {
	t.Helper()
	cmd := newCmd(flags)
	// The root command silences cobra's usage/error printing; mirror it.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// localDecode parses command JSON, unwrapping a {"results":...,"meta":...}
// envelope if the output pipeline added one.
func localDecode(t *testing.T, stdout string, v any) {
	t.Helper()
	raw := []byte(strings.TrimSpace(stdout))
	var env map[string]json.RawMessage
	if json.Unmarshal(raw, &env) == nil {
		if res, ok := env["results"]; ok {
			if _, hasMeta := env["meta"]; hasMeta {
				raw = res
			}
		}
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout)
	}
}

func localExitCode(err error) int {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	if err != nil {
		return 1
	}
	return 0
}
