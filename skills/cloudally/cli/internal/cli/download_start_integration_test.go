// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeCloudAlly stands up both halves a download run talks to: the
// documented API that serves the task and accepts the export, and the
// customer portal that serves the backup calendar.
type fakeCloudAlly struct {
	t *testing.T

	mu sync.Mutex
	// exportPosts records every body POSTed to recovery/export, which is
	// the side effect these tests are guarding.
	exportPosts []map[string]any
	// calendarCalls counts portal calendar reads.
	calendarCalls int
	// deadEndpointHits counts calls to the documented export endpoint,
	// which creates nothing on the live API.
	deadEndpointHits int
	// taskSource is the task's source; empty means MS365.
	taskSource string
}

func (f *fakeCloudAlly) source() string {
	if f.taskSource == "" {
		return "MS365"
	}
	return f.taskSource
}

func (f *fakeCloudAlly) deadHits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deadEndpointHits
}

// apiServer serves the task record and the export endpoint.
func (f *fakeCloudAlly) apiServer() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/backup-tasks/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"task-1","account":"acct-1","type":"BACKUP","source":"` + f.source() + `","status":"ACTIVE",
				"numOfEntities":1,
				"entities":[{"id":"e1","taskId":"task-1","entity":"jane.doe@contoso.com",
				             "extId":"ext-jane","status":"ACTIVE",
				             "lastBackupDates":{"MAIL":20261015042000}}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/recovery/export"):
			// The documented endpoint is a no-op on the live API, so the
			// CLI must not use it. Reaching here is a regression.
			f.mu.Lock()
			f.deadEndpointHits++
			f.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"OK"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	f.t.Cleanup(srv.Close)
	return srv
}

// portalServer serves the handoff exchange and the backup calendar.
func (f *fakeCloudAlly) portalServer() *httptest.Server {
	const shell = `<html><head>` +
		`<meta name="_csrf" content="csrf-1"/>` +
		`<meta name="_csrf_header" content="X-CSRF-TOKEN"/></head><body></body></html>`
	mux := http.NewServeMux()
	mux.HandleFunc("/newapp", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("accessToken") != "" {
			http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "authed"})
		}
		_, _ = w.Write([]byte(shell))
	})
	mux.HandleFunc("/application/service/", func(w http.ResponseWriter, r *http.Request) {
		op := strings.TrimPrefix(r.URL.Path, "/application/service/")
		if c, err := r.Cookie("JSESSIONID"); err != nil || c.Value != "authed" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if op == "startRestoreOrDownload" {
			body, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			_ = json.Unmarshal(body, &parsed)
			f.mu.Lock()
			f.exportPosts = append(f.exportPosts, parsed)
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"success":true,"data":{"needRedirect":false}}`))
			return
		}
		if op == "getSnapshotsCalendar" {
			f.mu.Lock()
			f.calendarCalls++
			f.mu.Unlock()
			// A daily 04:20 run through the end of December 2025.
			var rows []string
			for d := 1; d <= 31; d++ {
				rows = append(rows, `{"dateTime":202512`+twoDigit(d)+`042000,"succeed":true,`+
					`"status":"SUCCEED","source":"MS365","batchId":"batch-`+twoDigit(d)+`"}`)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":[` + strings.Join(rows, ",") + `]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":null}`))
	})
	srv := httptest.NewServer(mux)
	f.t.Cleanup(srv.Close)
	return srv
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func (f *fakeCloudAlly) posts() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.exportPosts...)
}

// mintHandoff makes the API also answer the partner-access-token call.
func (f *fakeCloudAlly) apiServerWithHandoff() *httptest.Server {
	base := f.apiServer()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/partner-access-token") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"handoff-token"}`))
			return
		}
		// Delegate everything else to the task/export handler.
		r2, _ := http.NewRequest(r.Method, base.URL+r.URL.Path, r.Body)
		r2.Header = r.Header.Clone()
		resp, err := http.DefaultClient.Do(r2)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	f.t.Cleanup(srv.Close)
	return srv
}

// TestDownloadStartWithoutYesResolvesButPostsNothing is the safety test
// that matters most: the run does all its read work and shows the operator
// the resolved backup, then stops without creating a job.
func TestDownloadStartWithoutYesResolvesButPostsNothing(t *testing.T) {
	f := &fakeCloudAlly{t: t}
	api := f.apiServerWithHandoff()
	portalSrv := f.portalServer()

	_, stderr, err := runSnapshotCLI(t, api.URL,
		"download", "start", "--account", "acct-1", "--task", "task-1",
		"--entity", "jane.doe@contoso.com", "--date", "2025-12-31",
		"--portal-url", portalSrv.URL)
	if err == nil {
		t.Fatal("want an error without --yes")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error should name --yes, got %v", err)
	}
	if posts := f.posts(); len(posts) != 0 {
		t.Fatalf("nothing should have been created, got %+v", posts)
	}
	// The operator is shown the backup the job would actually use.
	if !strings.Contains(stderr, "2025-12-31T04:20:00Z") || !strings.Contains(stderr, "20251231042000") {
		t.Fatalf("confirmation should name the resolved backup: %s", stderr)
	}
	if !strings.Contains(stderr, "snapshots") {
		t.Fatalf("confirmation should say where the date came from: %s", stderr)
	}
}

// TestDownloadStartWithYesPostsTheResolvedDate proves the created job uses
// the date resolved from the calendar, not the latest backup.
func TestDownloadStartWithYesPostsTheResolvedDate(t *testing.T) {
	// A single-archive source; MS365 is refused (see the next test).
	f := &fakeCloudAlly{t: t, taskSource: "GMAIL"}
	api := f.apiServerWithHandoff()
	portalSrv := f.portalServer()

	_, _, err := runSnapshotCLI(t, api.URL,
		"download", "start", "--account", "acct-1", "--task", "task-1",
		"--entity", "jane.doe@contoso.com", "--date", "2025-12-31",
		"--portal-url", portalSrv.URL, "--yes", "--json")
	if err != nil {
		t.Fatalf("download start: %v", err)
	}
	posts := f.posts()
	if len(posts) != 1 {
		t.Fatalf("want exactly one export post, got %d: %+v", len(posts), posts)
	}
	body := posts[0]
	// The one field that decides restore versus download. RESTORE on this
	// endpoint writes back into the live mailbox, so this assertion is the
	// most important one in the file.
	if body["type"] != "EXPORT" {
		t.Fatalf("type = %v, want EXPORT: %+v", body["type"], body)
	}
	// The task record's only known backup is 2026-10-15; the calendar's
	// 2025-12-31 run is the correct answer and must be what is sent.
	if got, ok := body["date"].(float64); !ok || int64(got) != 20251231042000 {
		t.Fatalf("date = %v, want 20251231042000", body["date"])
	}
	if body["account"] != "jane.doe@contoso.com" || body["taskId"] != "task-1" || body["source"] != "GMAIL" {
		t.Fatalf("body = %+v", body)
	}
	if body["extid"] != "ext-jane" {
		t.Fatalf("extid = %v", body["extid"])
	}
	// The archive must stay in CloudAlly storage, which is what makes it
	// collectable from the portal's Jobs page.
	if body["storageType"] != "CLOUDALLY_S3" {
		t.Fatalf("storageType = %v", body["storageType"])
	}
	if body["downloadToPst"] != false {
		t.Fatalf("zip must not request PST: %+v", body)
	}
	// The dead documented endpoint must never be touched.
	if f.deadHits() != 0 {
		t.Fatalf("the CLI called the no-op recovery/export endpoint %d time(s)", f.deadHits())
	}
}

// TestDownloadStartRefusesMS365WithYes proves an MS365 mailbox is never sent
// as one parent-source job, which the portal accepts and silently drops; the
// operator is pointed at 'download submit', which asks per sub-source.
func TestDownloadStartRefusesMS365WithYes(t *testing.T) {
	f := &fakeCloudAlly{t: t}
	api := f.apiServerWithHandoff()
	portalSrv := f.portalServer()

	for _, extra := range [][]string{nil, {"--source", "O365MAIL"}} {
		args := []string{"download", "start", "--account", "acct-1", "--task", "task-1",
			"--entity", "jane.doe@contoso.com", "--date", "2025-12-31",
			"--portal-url", portalSrv.URL, "--yes", "--json"}
		_, _, err := runSnapshotCLI(t, api.URL, append(args, extra...)...)
		if err == nil {
			t.Fatalf("want an MS365 refusal (extra args %v)", extra)
		}
		if !strings.Contains(err.Error(), "download submit") {
			t.Fatalf("refusal should name download submit, got %v", err)
		}
	}
	if posts := f.posts(); len(posts) != 0 {
		t.Fatalf("nothing should have been created, got %+v", posts)
	}
}

// TestDownloadStartRefusesMailboxWithNoBackupByThatDate proves a download
// is never built from a different date than the one asked for.
func TestDownloadStartRefusesMailboxWithNoBackupByThatDate(t *testing.T) {
	f := &fakeCloudAlly{t: t}
	api := f.apiServerWithHandoff()
	portalSrv := f.portalServer()

	// The calendar only has December 2025; ask for mid-2025.
	_, _, err := runSnapshotCLI(t, api.URL,
		"download", "start", "--account", "acct-1", "--task", "task-1",
		"--entity", "jane.doe@contoso.com", "--date", "2025-06-30",
		"--portal-url", portalSrv.URL, "--lookback-months", "2", "--yes")
	if err == nil {
		t.Fatal("want a refusal when no backup exists by that date")
	}
	if !strings.Contains(err.Error(), "no usable backup") {
		t.Fatalf("error = %v", err)
	}
	if posts := f.posts(); len(posts) != 0 {
		t.Fatalf("nothing should have been created, got %+v", posts)
	}
}
