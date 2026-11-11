// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (handfixes.json sync-progress-throttled and
// sync-object-page-size-100): sync walked by HubSpot's own paging shape, which
// the fleet template's fake cannot reach because HubSpot pages by cursor.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"hubspot-pp-cli/internal/store"
)

// hsPagedEmails serves rows as HubSpot pages them, 10 to a page.
type hsPagedEmails struct{ rows int }

func (f hsPagedEmails) RateLimit() float64 { return 0 }

func (f hsPagedEmails) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	after, _ := strconv.Atoi(params["after"])
	limit, _ := strconv.Atoi(params["limit"])
	var rows []string
	for i := after + 1; i <= min(f.rows, after+limit); i++ {
		rows = append(rows, fmt.Sprintf(`{"id":"%d","properties":{"hs_object_id":"%d"},"archived":false}`, i, i))
	}
	paging := ""
	if after+limit < f.rows {
		paging = fmt.Sprintf(`,"paging":{"next":{"after":"%d"}}`, after+limit)
	}
	return json.RawMessage(`{"results":[` + strings.Join(rows, ",") + `]` + paging + `}`), nil
}

func hsCountSyncEvents(t *testing.T, every time.Duration, rows int) (progress int, completeTotal float64) {
	t.Helper()
	old := syncProgressEvery
	syncProgressEvery = every
	t.Cleanup(func() { syncProgressEvery = old })
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var out bytes.Buffer
	res := syncResource(context.Background(), hsPagedEmails{rows: rows}, db, "hubspot-emails-crm", "", false, 0, false, false, &syncUserParams{}, &out, nil)
	if res.Err != nil || res.Warn != nil {
		t.Fatalf("sync: err=%v warn=%v", res.Err, res.Warn)
	}
	if res.Count != rows {
		t.Fatalf("stored %d rows, want %d", res.Count, rows)
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var ev struct {
			Event string  `json:"event"`
			Total float64 `json:"total"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("event %q: %v", line, err)
		}
		switch ev.Event {
		case "sync_progress":
			progress++
		case "sync_complete":
			completeTotal = ev.Total
		}
	}
	return progress, completeTotal
}

// hsEmailPages is how many pages sync reads for rows emails at its page size.
func hsEmailPages(rows int) int {
	limit := determinePaginationDefaults("hubspot-emails-crm").limit
	return (rows + limit - 1) / limit
}

// A page no longer costs a progress line, and the final count still arrives.
func TestHubSpotSyncWritesProgressAtMostOncePerInterval(t *testing.T) {
	progress, total := hsCountSyncEvents(t, time.Hour, 253)
	if progress != 1 {
		t.Errorf("%d pages inside one interval wrote %d progress lines, want 1 (the first page)", hsEmailPages(253), progress)
	}
	if total != 253 {
		t.Errorf("sync_complete total = %v, want 253", total)
	}
}

// With no interval every page reports, so the gate above is what limits them.
func TestHubSpotSyncWritesProgressPerPageWithNoInterval(t *testing.T) {
	if progress, _ := hsCountSyncEvents(t, 0, 253); progress != hsEmailPages(253) {
		t.Errorf("wrote %d progress lines, want one for each of %d pages", progress, hsEmailPages(253))
	}
}

// HubSpot's CRM object list routes return 100 rows a page when asked (a live
// read of all 13 object types, syncfix2 report); their spec's default is 10.
// Ten times fewer requests and progress lines for the same rows.
func TestHubSpotObjectPagesAskForAHundredRows(t *testing.T) {
	objects := 0
	for _, resource := range defaultSyncResources() {
		if path, err := syncResourcePath(resource); err != nil || !strings.HasPrefix(path, "/crm/v3/objects/") {
			continue
		}
		objects++
		if got := determinePaginationDefaults(resource).limit; got != 100 {
			t.Errorf("%s pages %d rows at a time, want 100", resource, got)
		}
	}
	if objects != 13 {
		t.Errorf("found %d CRM object resources in the default sync, want 13", objects)
	}
}
