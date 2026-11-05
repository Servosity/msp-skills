// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
	"synology-pp-cli/internal/store"
)

// TestSyncSucceedsOnRealDSMShapes: SYNO.Core.SyslogClient.Log items (time,
// level, descr, who, logtype) carry no id, so a sync that included the
// system log aborted it as a critical failure and exited 1 on every run (and
// workflow archive with it), leaving sync_state incomplete so every MCP sql
// and search answer said the latest sync was incomplete. The log is a live
// stream: sync covers folder, group, package and user, and the log command
// reads it live.
func TestSyncSucceedsOnRealDSMShapes(t *testing.T) {
	sweepIsolate(t)
	payload := map[string]string{
		"SYNO.Core.Share/list":            `{"total":1,"offset":0,"shares":[{"name":"homes","vol_path":"/volume1"}]}`,
		"SYNO.Core.Group/list":            `{"total":1,"offset":0,"groups":[{"name":"administrators","gid":101}]}`,
		"SYNO.Core.Package/list":          `{"total":1,"offset":0,"packages":[{"id":"ActiveBackup","name":"Active Backup for Business"}]}`,
		"SYNO.Core.User/list":             `{"total":1,"offset":0,"users":[{"name":"alice","uid":1026}]}`,
		"SYNO.Core.SyslogClient.Log/list": `{"total":1,"offset":0,"items":[{"time":"2026/10/20 10:00:00","level":"info","descr":"backup done","who":"SYSTEM","logtype":"system"}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		_ = r.ParseForm()
		key := r.Form.Get("api") + "/" + r.Form.Get("method")
		switch {
		case key == "SYNO.API.Auth/login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`))
		case payload[key] != "":
			_, _ = w.Write([]byte(`{"success":true,"data":` + payload[key] + `}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
		}
	}))
	defer srv.Close()
	t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "s3cretpw")

	if _, errOut, code := sweepExecute(t, "sync", "--json"); code != 0 {
		t.Fatalf("sync exited %d on DSM's own shapes: %s", code, errOut)
	}
	if _, errOut, code := sweepExecute(t, "workflow", "archive", "--json"); code != 0 {
		t.Fatalf("workflow archive exited %d: %s", code, errOut)
	}
	states, err := syncStates(defaultDBPath("synology-cli"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for rt, complete := range states {
		seen[rt] = true
		if !complete {
			t.Errorf("sync_state %s is incomplete after a clean sync; MCP sql/search would report a partial store", rt)
		}
	}
	for _, want := range []string{"folder", "group", "package", "user"} {
		if !seen[want] {
			t.Errorf("sync did not record %s", want)
		}
	}
	if seen["log"] {
		t.Error("sync still treats the live system log as a synced resource")
	}
	out, errOut, code := sweepExecute(t, "log", "--json")
	if code != 0 || !strings.Contains(out, "backup done") || strings.Contains(errOut, "not cached locally") {
		t.Fatalf("log (live): exit %d out=%s err=%s", code, out, errOut)
	}
}

// syncStates reads sync_state (resource -> last attempt complete), closing
// the store before it returns so no handle outlives the read (Windows).
func syncStates(path string) (map[string]bool, error) {
	db, err := store.OpenReadOnlyContext(context.Background(), path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.DB().Query(`SELECT resource_type, last_attempt_complete FROM sync_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var rt string
		var complete int
		if err := rows.Scan(&rt, &complete); err != nil {
			return nil, err
		}
		out[rt] = complete == 1
	}
	return out, rows.Err()
}
