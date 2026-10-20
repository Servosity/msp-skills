// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// jobPage answers one getRestoreDownloadStatus page out of total jobs.
func jobPage(body []byte, total int) string {
	var q struct{ Page, Rows int }
	_ = json.Unmarshal(body, &q)
	var rows []string
	for i := (q.Page - 1) * q.Rows; i < q.Page*q.Rows && i < total; i++ {
		rows = append(rows, fmt.Sprintf(`{"taskId":"job-%d","type":"EXPORT"}`, i))
	}
	return `{"success":true,"data":[` + strings.Join(rows, ",") + `]}`
}

// A listing is complete only when it reaches a short page; one stopped at
// the page cap says so, with everything it read.
func TestDownloadJobsSaysWhenTheListingIsCut(t *testing.T) {
	for _, tc := range []struct {
		total    int
		complete bool
	}{
		{total: 250, complete: true},
		{total: maxDownloadInfoPages*downloadInfoPageSize - 1, complete: true},
		// Every page read was full: another page may exist.
		{total: maxDownloadInfoPages * downloadInfoPageSize, complete: false},
		{total: maxDownloadInfoPages*downloadInfoPageSize + 1, complete: false},
	} {
		f := newFakePortal(t, "tok")
		f.handler = func(op string, body []byte, _ string) string { return jobPage(body, tc.total) }
		srv := f.server()
		s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
		if err != nil {
			t.Fatal(err)
		}
		jobs, err := s.DownloadJobs(context.Background())
		srv.Close()
		if tc.complete {
			if err != nil || len(jobs) != tc.total {
				t.Fatalf("%d jobs: got %d, %v; want every job and no error", tc.total, len(jobs), err)
			}
			continue
		}
		if !errors.Is(err, ErrListingIncomplete) || len(jobs) != maxDownloadInfoPages*downloadInfoPageSize {
			t.Fatalf("%d jobs: got %d, %v; want the jobs read and ErrListingIncomplete", tc.total, len(jobs), err)
		}
	}
}

// A job's file list cut at the cap is an error, never a short archive set.
func TestDownloadFilesRefusesACutFileList(t *testing.T) {
	f := newFakePortal(t, "tok")
	f.getHandler = func(string, map[string][]string) string {
		var rows []string
		for i := 0; i < downloadInfoPageSize; i++ {
			rows = append(rows, fmt.Sprintf(`{"exportDataId":"d%d","fileName":"p%d.zip"}`, i, i))
		}
		return `{"success":true,"data":[` + strings.Join(rows, ",") + `]}`
	}
	srv := f.server()
	defer srv.Close()
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DownloadFiles(context.Background(), "job-1"); !errors.Is(err, ErrListingIncomplete) {
		t.Fatalf("want ErrListingIncomplete, got %v", err)
	}
}
