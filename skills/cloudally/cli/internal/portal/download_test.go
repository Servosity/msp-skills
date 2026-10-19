// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDownloadURLReturnsTheLink(t *testing.T) {
	const link = "https://ca-s3.example.com/export/job-1.zip?X-Amz-Date=20261015T183000Z&X-Amz-Expires=86400&X-Amz-Signature=abc"
	f := newFakePortal(t, "tok")
	f.getHandler = func(op string, q map[string][]string) string {
		if op != downloadURLOperation {
			t.Fatalf("operation = %q", op)
		}
		if got := q["taskId"]; len(got) != 1 || got[0] != "job-1" {
			t.Fatalf("taskId = %v", got)
		}
		return `{"success":true,"data":"` + link + `"}`
	}
	srv := f.server()
	defer srv.Close()
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := s.DownloadURL(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}
	if got != link {
		t.Fatalf("link = %q", got)
	}
}

// TestDownloadURLSurfacesTheExpiredFailure covers the real failure shape:
// the service answers an unknown or expired job with HTTP 200 and
// success:false, so the body is the only signal.
func TestDownloadURLSurfacesTheExpiredFailure(t *testing.T) {
	f := newFakePortal(t, "tok")
	f.getHandler = func(string, map[string][]string) string {
		return `{"success":false,"status":"The download link has expired.","statusCode":400}`
	}
	srv := f.server()
	defer srv.Close()
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, err = s.DownloadURL(context.Background(), "job-gone")
	if err == nil || !strings.Contains(err.Error(), "download link has expired") {
		t.Fatalf("want the expired message, got %v", err)
	}
}

func TestDownloadURLRequiresAJobID(t *testing.T) {
	s := &Session{csrfHeader: "X-CSRF-TOKEN", csrfToken: "x"}
	if _, err := s.DownloadURL(context.Background(), "  "); err == nil {
		t.Fatal("want an error without a job id")
	}
	if _, err := s.DownloadFiles(context.Background(), ""); err == nil {
		t.Fatal("want an error without a job id")
	}
}

func TestDownloadFilesListsEveryPart(t *testing.T) {
	f := newFakePortal(t, "tok")
	f.getHandler = func(op string, q map[string][]string) string {
		if op != downloadInfoOperation {
			t.Fatalf("operation = %q", op)
		}
		// The live shape: pagination fields and success sit beside data,
		// as getRestoreDownloadStatus returns on the real portal.
		return `{"page":1,"totalPages":1,"total":2,"success":true,"data":[` +
			`{"exportDataId":"d1","fileName":"mailbox-part1.zip","size":"100.00 MB","downloadUrl":"https://s3/1.zip"},` +
			`{"exportDataId":"d2","fileName":"mailbox-part2.zip","size":"50.00 MB","downloadUrl":"https://s3/2.zip"}]}`
	}
	srv := f.server()
	defer srv.Close()
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	files, err := s.DownloadFiles(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("DownloadFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %+v", files)
	}
	if files[0].FileName != "mailbox-part1.zip" || files[0].Size != "100.00 MB" || files[0].DownloadURL == "" {
		t.Fatalf("first file = %+v", files[0])
	}
}

func TestLinkExpiryReadsEachPresignedFlavour(t *testing.T) {
	cases := []struct {
		name string
		link string
		want time.Time
	}{{
		name: "aws sigv4",
		link: "https://s3/x.zip?X-Amz-Date=20261015T183000Z&X-Amz-Expires=86400",
		want: time.Date(2026, 10, 16, 18, 30, 0, 0, time.UTC),
	}, {
		name: "aws sigv2 / gcs",
		// 2026-10-16T18:30:00Z
		link: "https://s3/x.zip?Expires=1792175400",
		want: time.Unix(1792175400, 0).UTC(),
	}, {
		name: "azure sas",
		link: "https://blob/x.zip?se=2026-10-16T18%3A30%3A00Z",
		want: time.Date(2026, 10, 16, 18, 30, 0, 0, time.UTC),
	}}
	for _, tc := range cases {
		got, ok := LinkExpiry(tc.link)
		if !ok {
			t.Fatalf("%s: no expiry found", tc.name)
		}
		if !got.Equal(tc.want) {
			t.Fatalf("%s: expiry = %s, want %s", tc.name, got, tc.want)
		}
	}
}

// TestLinkExpiryUnknownIsReportedNotGuessed matters for a support reply: a
// link with no stated expiry must come back unknown so nobody quotes a
// made-up deadline to a customer.
func TestLinkExpiryUnknownIsReportedNotGuessed(t *testing.T) {
	for _, link := range []string{
		"https://s3/x.zip",
		"https://s3/x.zip?X-Amz-Expires=86400", // no signing date
		"https://s3/x.zip?Expires=0",
		"https://s3/x.zip?se=not-a-time",
		"::not a url::",
	} {
		if _, ok := LinkExpiry(link); ok {
			t.Fatalf("%q should report an unknown expiry", link)
		}
	}
}

func TestLinkLifetime(t *testing.T) {
	d, ok := LinkLifetime("https://s3/x.zip?X-Amz-Date=20261015T183000Z&X-Amz-Expires=3600")
	if !ok || d != time.Hour {
		t.Fatalf("lifetime = %s, ok=%v", d, ok)
	}
	if _, ok := LinkLifetime("https://s3/x.zip?Expires=1792175400"); ok {
		t.Fatal("sigv2 states no lifetime, only an instant")
	}
}
