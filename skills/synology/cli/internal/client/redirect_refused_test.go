// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-tls): no DSM request follows a redirect.

package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/dsm/dsmtest"
)

const tasksPath = "/webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=list&version=1"

// TestNoDSMRequestFollowsARedirect: DSM's WebAPI never redirects, so the
// generated client follows no redirect, same origin included. A NAS (or
// anything on the path) answering an authenticated read with
// 302 Location: /webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=backup
// used to have the client carry the sid and SynoToken into a backup start.
// The error names the status, never the Location, and is not retried; a
// 3xx net/http returns unfollowed (no Location) is not a success either; a
// redirect of the login POST never re-sends the password.
func TestNoDSMRequestFollowsARedirect(t *testing.T) {
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusMovedPermanently} {
		testenv.Isolate(t)
		nas := dsmtest.NewRedirectingNAS(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("method") == "backup" {
				_, _ = io.WriteString(w, `{"success":true,"data":{}}`)
				return
			}
			http.Redirect(w, r, dsmtest.BackupLocation, status)
		})
		c := New(&config.Config{BaseURL: nas.URL, SynologyAccount: "admin", SynologyPassword: "pw-redirect-1"}, 5*time.Second, 0)
		c.NoCache = true
		data, err := c.Get(context.Background(), tasksPath, nil)
		if err == nil {
			t.Fatalf("status %d: a redirected read was accepted: %s", status, data)
		}
		if n := nas.Count("method=backup"); n != 0 {
			t.Fatalf("status %d: the client followed the redirect into a backup start (%d request(s)): %v", status, n, nas.Requests())
		}
		if !errors.Is(err, dsm.ErrRedirectRefused) || !strings.Contains(err.Error(), "status "+strconv.Itoa(status)) {
			t.Fatalf("status %d: err = %v, want a refused redirect naming the status", status, err)
		}
		for _, leak := range []string{"backup", "task_ids", "SID1", "TOK1"} {
			if strings.Contains(err.Error(), leak) {
				t.Fatalf("status %d: the error quoted the Location (%q): %v", status, leak, err)
			}
		}
		if n := nas.Count("method=list"); n != 1 {
			t.Fatalf("status %d: a refused redirect was retried (%d list requests)", status, n)
		}
	}

	// A 3xx that net/http hands back unfollowed is a redirect, not a success.
	testenv.Isolate(t)
	nas := dsmtest.NewRedirectingNAS(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusFound) // no Location header
		_, _ = io.WriteString(w, `{"success":true,"data":{"tasks":[{"task_id":5}]}}`)
	})
	c := New(&config.Config{BaseURL: nas.URL, SynologyAccount: "admin", SynologyPassword: "pw-redirect-1"}, 5*time.Second, 0)
	c.NoCache = true
	if data, err := c.Get(context.Background(), tasksPath, nil); err == nil || !strings.Contains(err.Error(), "status 302") {
		t.Fatalf("a 302 without a Location read as success: data=%s err=%v", data, err)
	}

	// The login POST answered with a same-origin 307: the form (password
	// included) is not re-sent.
	testenv.Isolate(t)
	var posts atomic.Int32
	login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dsmtest.ServeAPIInfo(w, r) {
			return
		}
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
		http.Redirect(w, r, "/webapi/entry.cgi?again=1", http.StatusTemporaryRedirect)
	}))
	defer login.Close()
	c = New(&config.Config{BaseURL: login.URL, SynologyAccount: "admin", SynologyPassword: "pw-redirect-1"}, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), tasksPath, nil); err == nil || !strings.Contains(err.Error(), "status 307") || strings.Contains(err.Error(), "again") {
		t.Fatalf("login redirect: err = %v, want a refused redirect naming status 307", err)
	}
	if n := posts.Load(); n != 1 {
		t.Fatalf("the login form was sent %d times; a redirect must never re-send the password", n)
	}
}
