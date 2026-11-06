// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/config"
)

// The task feed pages the same way the portal's job listing does (guard
// simulation, seed 10011): an API that caps its page size below the
// pageSize asked for answers with fewer rows and names its page count. The
// reader stopped at that short page, so a RESTORE task on page 2 read as
// absent. Where the API names its page count, that decides; and a feed still
// going when the page cap stops the reader is an error, never a quiet cut.
func taskFeedServer(t *testing.T, n, pageCap, totalPages int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		var rows []map[string]any
		for i := (page - 1) * pageCap; i < page*pageCap && i < n; i++ {
			rows = append(rows, map[string]any{"id": fmt.Sprintf("rt-%d", i), "account": "a@contoso.com", "type": "RESTORE"})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"page": page, "totalPages": totalPages, "total": n, "data": rows})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRestoreTaskFeedReadsEveryPageTheAPINames(t *testing.T) {
	isolateAuthState(t)
	const n = 8
	srv := taskFeedServer(t, n, 3, (n+2)/3)
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "x-token-1", ClientID: "cid", ClientSecret: "csecret"}, 5*time.Second, 0)
	tasks, err := fetchRestoreTasks(context.Background(), c, restoreScope{account: "acct-1"})
	if err != nil || len(tasks) != n {
		t.Fatalf("got %d tasks, %v; want all %d across the API's capped pages", len(tasks), err, n)
	}
}

func TestRestoreTaskFeedCutAtThePageCapIsAnError(t *testing.T) {
	isolateAuthState(t)
	// Every page full and the API says there are more pages than the
	// reader will read.
	n := (restoreTaskMaxPages + 5) * restoreTaskPageSize
	srv := taskFeedServer(t, n, restoreTaskPageSize, restoreTaskMaxPages+5)
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "x-token-1", ClientID: "cid", ClientSecret: "csecret"}, 5*time.Second, 0)
	if _, err := fetchRestoreTasks(context.Background(), c, restoreScope{account: "acct-1"}); err == nil {
		t.Fatal("a task feed cut at the page cap must be an error: absence in it proves nothing")
	}
}

// Codex round 1, same class: an empty page ended the feed while the API
// still named a page after it, so a RESTORE task there read as absent.
func TestRestoreTaskFeedAnEmptyPageDoesNotEndIt(t *testing.T) {
	isolateAuthState(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		rows := []map[string]any{}
		if page == 2 {
			rows = append(rows, map[string]any{"id": "rt-1", "account": "a@contoso.com", "type": "RESTORE"})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"page": page, "totalPages": 2, "total": 1, "data": rows})
	}))
	t.Cleanup(srv.Close)
	c := client.New(&config.Config{BaseURL: srv.URL, AccessToken: "x-token-1", ClientID: "cid", ClientSecret: "csecret"}, 5*time.Second, 0)
	tasks, err := fetchRestoreTasks(context.Background(), c, restoreScope{account: "acct-1"})
	if err != nil || len(tasks) != 1 {
		t.Fatalf("got %d tasks, %v; want the task on page 2", len(tasks), err)
	}
}
