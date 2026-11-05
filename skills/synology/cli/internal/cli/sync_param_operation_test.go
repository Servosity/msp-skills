// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestSyncParamsCannotReselectTheDSMOperation: `sync --param api=...
// --global-param method=backup --resource-param folder:task_ids=[8]` started
// a backup on the NAS. A parameter may add filters; it may never change the
// operation the spec path selects (api, method, version), the session, or a
// key the path fixes. Plain syncs still run.
func TestSyncParamsCannotReselectTheDSMOperation(t *testing.T) {
	sweepIsolate(t)
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		_ = r.ParseForm()
		key := r.Form.Get("api") + "/" + r.Form.Get("method")
		mu.Lock()
		calls = append(calls, key)
		mu.Unlock()
		switch key {
		case "SYNO.API.Auth/login":
			_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`))
		case "SYNO.Core.Share/list":
			_, _ = w.Write([]byte(`{"success":true,"data":{"total":1,"shares":[{"name":"homes"}]}}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
		}
	}))
	defer srv.Close()
	t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "s3cretpw")

	for _, args := range [][]string{
		{"--param", "api=SYNO.ActiveBackup.Task", "--global-param", "method=backup", "--resource-param", "folder:task_ids=[8]"},
		{"--param", "method=backup"},
		{"--global-param", "Version=9"},
	} {
		_, errOut, code := sweepExecute(t, append([]string{"sync", "--resources", "folder", "--json"}, args...)...)
		if code == 0 || !strings.Contains(errOut, "selects the DSM operation") {
			t.Fatalf("sync %v: exit %d, want a refusal naming the operation key: %s", args, code, errOut)
		}
	}
	mu.Lock()
	for _, c := range calls {
		if c != "SYNO.API.Auth/login" {
			mu.Unlock()
			t.Fatalf("an operation override reached the NAS: %q", calls)
		}
	}
	calls = nil
	mu.Unlock()
	if _, errOut, code := sweepExecute(t, "sync", "--resources", "folder", "--json"); code != 0 {
		t.Fatalf("plain sync exited %d: %s", code, errOut)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(calls, " "), "SYNO.Core.Share/list") {
		t.Fatalf("plain sync never listed the shares: %q", calls)
	}
}
