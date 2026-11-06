// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// Found by the guard simulation (seed 10011): a portal that caps its page
// size below the rows asked for answers page 1 with fewer rows and says how
// many pages there are. The listings stopped at that short page, so a job on
// page 2 read as absent: a confirmed archive was concluded EMPTY, and an
// archive already created was created again. Where the portal names its page
// count (or its total), that decides; only a listing with neither ends on a
// short page.

const serverPageCap = 3

func cappedPage(page, n int, row func(i int) string) string {
	var rows []string
	for i := (page - 1) * serverPageCap; i < page*serverPageCap && i < n; i++ {
		rows = append(rows, row(i))
	}
	pages := (n + serverPageCap - 1) / serverPageCap
	return fmt.Sprintf(`{"success":true,"data":{"page":%d,"totalPages":%d,"total":%d,"data":[%s]}}`, page, pages, n, strings.Join(rows, ","))
}

func TestDownloadJobsReadsEveryPageThePortalNames(t *testing.T) {
	const n = 7
	f := newFakePortal(t, "tok")
	f.handler = func(op string, body []byte, _ string) string {
		var q struct{ Page int }
		_ = json.Unmarshal(body, &q)
		return cappedPage(q.Page, n, func(i int) string { return fmt.Sprintf(`{"taskId":"job-%d","type":"EXPORT"}`, i) })
	}
	srv := f.server()
	defer srv.Close()
	s, err := Open(context.Background(), Options{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := s.DownloadJobs(context.Background())
	if err != nil || len(jobs) != n {
		t.Fatalf("got %d jobs, %v; want all %d across the portal's capped pages", len(jobs), err, n)
	}
}

func TestDownloadFilesReadsEveryPageThePortalNames(t *testing.T) {
	const n = 5
	f := newFakePortal(t, "tok")
	f.getHandler = func(_ string, q map[string][]string) string {
		page, _ := strconv.Atoi(q["page"][0])
		return cappedPage(page, n, func(i int) string { return fmt.Sprintf(`{"exportDataId":"d%d","fileName":"p%d.zip"}`, i, i) })
	}
	srv := f.server()
	defer srv.Close()
	s, err := Open(context.Background(), Options{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatal(err)
	}
	files, err := s.DownloadFiles(context.Background(), "job-1")
	if err != nil || len(files) != n {
		t.Fatalf("got %d files, %v; want all %d: a cut file list downloads as a short archive set", len(files), err, n)
	}
}

// MorePages both ways: the server's page count or total decides over a short
// page, empty included; with neither, a short or empty page ends the listing.
func TestMorePages(t *testing.T) {
	for _, tc := range []struct {
		name                                      string
		page, got, asked, totalPages, total, read int
		want                                      bool
	}{
		{"short page, server names more pages", 1, 3, 100, 3, 0, 3, true},
		{"last page the server names", 3, 1, 100, 3, 0, 7, false},
		{"short page, total not reached", 1, 3, 100, 0, 7, 3, true},
		{"total reached", 3, 1, 100, 0, 7, 7, false},
		{"no paging info, full page", 1, 100, 100, 0, 0, 100, true},
		{"no paging info, short page", 2, 40, 100, 0, 0, 140, false},
		{"empty page, server names more pages", 2, 0, 100, 9, 0, 3, true},
		{"empty page, total not reached", 1, 0, 100, 0, 1, 0, true},
		{"empty page, no paging info", 2, 0, 100, 0, 0, 100, false},
	} {
		if got := MorePages(tc.page, tc.got, tc.asked, tc.totalPages, tc.total, tc.read); got != tc.want {
			t.Errorf("%s: MorePages = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Codex round 1: an empty page ended the listing before the page count was
// read, so a server answering page 1 with no rows while naming two pages
// hid the archive on page 2 and fetch could conclude EMPTY.
func TestAnEmptyPageDoesNotEndAListingThatNamesMorePages(t *testing.T) {
	f := newFakePortal(t, "tok")
	f.getHandler = func(_ string, q map[string][]string) string {
		if q["page"][0] == "1" {
			return `{"success":true,"data":{"page":1,"totalPages":2,"total":1,"data":[]}}`
		}
		return `{"success":true,"data":{"page":2,"totalPages":2,"total":1,"data":[{"exportDataId":"d1","fileName":"p1.zip"}]}}`
	}
	f.handler = func(_ string, body []byte, _ string) string {
		var q struct{ Page int }
		_ = json.Unmarshal(body, &q)
		if q.Page == 1 {
			return `{"success":true,"data":{"page":1,"totalPages":2,"total":1,"data":[]}}`
		}
		return `{"success":true,"data":{"page":2,"totalPages":2,"total":1,"data":[{"taskId":"job-1","type":"EXPORT"}]}}`
	}
	srv := f.server()
	defer srv.Close()
	s, err := Open(context.Background(), Options{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if files, err := s.DownloadFiles(context.Background(), "job-1"); err != nil || len(files) != 1 {
		t.Errorf("DownloadFiles: got %d files, %v; want the file on page 2", len(files), err)
	}
	if jobs, err := s.DownloadJobs(context.Background()); err != nil || len(jobs) != 1 {
		t.Errorf("DownloadJobs: got %d jobs, %v; want the job on page 2", len(jobs), err)
	}
}
