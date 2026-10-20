// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// TestDownloadFetchHasNoWholeRunDeadline streams an archive for about two
// seconds under --timeout 1s: it must arrive whole.
func TestDownloadFetchHasNoWholeRunDeadline(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("PK\x03\x04"+strings.Repeat("m", 4000)))
	s.slow = 250 * time.Millisecond
	dest := t.TempDir()
	start := time.Now()
	out, stderr, err := runDL(t, s, fetchArgs(dest, "--timeout", "1s")...)
	if err != nil {
		t.Fatalf("an archive that streams past --timeout must still arrive (after %s): %v\n%s", time.Since(start), err, stderr)
	}
	if got := decodeOutcomes(t, out); len(got) != 1 || got[0].Status != fetchDownloaded || got[0].Bytes != 4004 {
		t.Fatalf("outcome = %+v", got)
	}
	if took := time.Since(start); took < time.Second {
		t.Fatalf("the stream finished in %s, so it never outlasted --timeout", took)
	}
}

// TestDownloadFetchExitCodes covers what a run reports when not every
// archive it was asked for ends up on disk.
func TestDownloadFetchExitCodes(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("PK\x03\x04mail"))
	s.addFile("j-cal", jane, "O365CALENDAR", "d2", "cal.zip", []byte("PK\x03\x04cal"))
	s.mu.Lock()
	s.jobs[1].Status = "INPROCESS"
	s.mu.Unlock()

	// Still being built: exit 8, run again later.
	_, _, err := runDL(t, s, fetchArgs(t.TempDir())...)
	if err == nil || ExitCode(err) != ExitJobPending {
		t.Fatalf("a NOT_READY job must exit %d, got %v", ExitJobPending, err)
	}
	// A job that finished without an archive is FAILED, not NOT_READY.
	s.mu.Lock()
	s.jobs[1].Status = "FAILED"
	s.mu.Unlock()
	out, _, err := runDL(t, s, fetchArgs(t.TempDir())...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("a failed job must exit 6, got %v", err)
	}
	statuses := map[string]string{}
	for _, o := range decodeOutcomes(t, out) {
		statuses[o.JobID] = o.Status
	}
	if statuses["j-cal"] != fetchFailed {
		t.Fatalf("a job that finished as FAILED should be FAILED: %v", statuses)
	}

	// A requested mailbox with no job at all is MISSING.
	s.mu.Lock()
	s.jobs = s.jobs[:1]
	s.mu.Unlock()
	out, _, err = runDL(t, s, fetchArgs(t.TempDir(), "--entities", jane+",nobody@contoso.com")...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("a requested mailbox with no job must exit 6, got %v", err)
	}
	missing := false
	for _, o := range decodeOutcomes(t, out) {
		missing = missing || (o.Entity == "nobody@contoso.com" && o.Status == fetchMissing)
	}
	if !missing {
		t.Fatalf("nobody@ should be MISSING:\n%s", out)
	}

	// A --batch row nothing on the portal matches is MISSING.
	b := newSubmitBatch(t, "20221231-000000-fetchset", jane, "gone@contoso.com")
	b.Rows[0].SnapshotRaw = 20221231042000
	_ = saveBatch(b)
	out, _, err = runDL(t, s, fetchArgs(t.TempDir(), "--batch", b.BatchKey)...)
	if err == nil || ExitCode(err) != 6 {
		t.Fatalf("an unmatched batch row must exit 6, got %v", err)
	}
	got := decodeOutcomes(t, out)
	byEntity := map[string]string{}
	for _, o := range got {
		byEntity[o.Entity] = o.Status
	}
	if byEntity[jane] != fetchDownloaded || byEntity["gone@contoso.com"] != fetchMissing {
		t.Fatalf("outcomes = %+v", got)
	}

	// Everything present: exit 0.
	if _, _, err := runDL(t, s, fetchArgs(t.TempDir())...); err != nil {
		t.Fatalf("a complete run must exit 0, got %v", err)
	}
}

// TestDownloadFetchFailsAnErrorPageWhoseSizeIsUnreadable covers a portal
// size the check could not use, with an HTML error page as the body.
func TestDownloadFetchFailsAnErrorPageWhoseSizeIsUnreadable(t *testing.T) {
	for _, size := range []string{"1,024.50 MB", "about a gigabyte", ""} {
		t.Run(size, func(t *testing.T) {
			testenv.Isolate(t)
			s := newDLStub(t)
			s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", nil)
			s.mu.Lock()
			s.jobs[0].Files[0].Size = size
			s.bodies["d1"] = []byte(`<html><body>AccessDenied</body></html>`)
			s.mu.Unlock()
			dest := t.TempDir()
			out, _, err := runDL(t, s, fetchArgs(dest)...)
			if err == nil || ExitCode(err) != 6 {
				t.Fatalf("size %q: want exit 6, got %v", size, err)
			}
			if got := decodeOutcomes(t, out); len(got) != 1 || got[0].Status != fetchFailed {
				t.Fatalf("size %q: want FAILED, got %+v", size, got)
			}
			if _, err := os.Stat(filepath.Join(dest, "2022", jane, "O365MAIL", "jane.zip")); !os.IsNotExist(err) {
				t.Fatalf("size %q: the error page was saved as the archive", size)
			}
		})
	}
}

// TestDownloadFetchSweepsItsOwnStalePartials proves a killed run's old
// .partial files are removed, and nothing else is.
func TestDownloadFetchSweepsItsOwnStalePartials(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("j-mail", jane, "O365MAIL", "d1", "jane.zip", []byte("PK\x03\x04mail"))
	dest := t.TempDir()
	dir := filepath.Join(dest, "2022", jane, "O365MAIL")
	mustMkdir(t, dir)
	old := time.Now().Add(-2 * time.Hour)
	plant := func(name string, mtime time.Time) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("leftover"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stale := plant(".cloudally-0123456789abcdef.partial", old)
	fresh := plant(".cloudally-fedcba9876543210.partial", time.Now())
	notOurs := plant(".cloudally-notours.partial", old)
	customer := plant("notes.partial", old)

	if _, stderr, err := runDL(t, s, fetchArgs(dest)...); err != nil {
		t.Fatalf("fetch: %v\n%s", err, stderr)
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatalf("a stale partial of ours should be removed: %v", err)
	}
	for _, p := range []string{fresh, notOurs, customer} {
		if _, err := os.Lstat(p); err != nil {
			t.Fatalf("%s should be left alone: %v", filepath.Base(p), err)
		}
	}
}

// TestDownloadWatchOutlastsTimeout follows a job that becomes ready after
// --timeout has passed: watching must not be cut off.
func TestDownloadWatchOutlastsTimeout(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("job-1", jane, "O365MAIL", "d1", "jane.zip", []byte("PK\x03\x04"))
	s.mu.Lock()
	s.jobs[0].Status = "INPROCESS"
	s.mu.Unlock()
	go func() {
		time.Sleep(1500 * time.Millisecond)
		s.mu.Lock()
		s.jobs[0].Status = "READY_FOR_DOWNLOAD"
		s.mu.Unlock()
	}()
	if _, stderr, err := runDL(t, s, "download", "watch", "--account", "acct-1", "--job", "job-1", "--interval", "300ms", "--timeout", "1s", "--json"); err != nil {
		t.Fatalf("watch must outlast --timeout: %v\n%s", err, stderr)
	}
}
