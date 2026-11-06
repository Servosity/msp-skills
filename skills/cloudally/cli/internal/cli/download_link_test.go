// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"cloudally-pp-cli/internal/cliutil/testenv"
)

// TestDownloadLinkMintsEachFileFromItsExportDataID proves both modes of
// 'download link' return a working link per file. The stub portal answers a
// link asked for by job id the way the live one does ("expired"), and a
// listing row carries no link of its own.
func TestDownloadLinkMintsEachFileFromItsExportDataID(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("job-1", jane, "O365MAIL", "d1", "jane-part1.zip", []byte("ONE"))
	s.addFile("job-1", jane, "O365MAIL", "d2", "jane-part2.zip", []byte("TWO"))
	for _, extra := range [][]string{nil, {"--files"}} {
		args := append([]string{"download", "link", "--account", "acct-1", "--job", "job-1", "--json"}, extra...)
		out, stderr, err := runDL(t, s, args...)
		if err != nil {
			t.Fatalf("link %v: %v\n%s", extra, err, stderr)
		}
		var got downloadLinkOutput
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("link output is not JSON: %v\n%s", err, out)
		}
		if len(got.Files) != 2 {
			t.Fatalf("link %v: want 2 files, got %+v", extra, got.Files)
		}
		for i, id := range []string{"d1", "d2"} {
			f := got.Files[i]
			if f.URL != s.storage.URL+"/obj/"+id {
				t.Fatalf("link %v: file %d url = %q, want the link minted for %s", extra, i, f.URL, id)
			}
			if f.FileName != "jane-part"+string(rune('1'+i))+".zip" {
				t.Fatalf("link %v: file %d name = %q", extra, i, f.FileName)
			}
		}
	}
}

// hostile carries an OSC title change, a bell and a screen clear, plus a
// C1 CSI, the shapes a reviewer planted in tenant data.
const hostile = "evil\x1b]0;OWNED\x07\x1b[2J\u009b31m"

func assertNoTerminalControls(t *testing.T, where, s string) {
	t.Helper()
	for _, bad := range []string{"\x1b", "\x07", "\u009b"} {
		if strings.Contains(s, bad) {
			t.Fatalf("%s carries a raw terminal control %q:\n%q", where, bad, s)
		}
	}
	if !strings.Contains(s, "OWNED") {
		t.Fatalf("%s should still show the text, scrubbed:\n%q", where, s)
	}
}

// TestDownloadPrintersScrubTenantText drives every human-readable download
// printer with a hostile mailbox, file name and job status.
func TestDownloadPrintersScrubTenantText(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	mailbox := hostile + "@contoso.com"
	s.addFile("job-1", mailbox, "O365MAIL", "d1", hostile+".zip", []byte("ARCHIVE"))

	out, _, err := runDL(t, s, "download", "list", "--account", "acct-1", "--human-friendly")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertNoTerminalControls(t, "download list", out)

	out, _, err = runDL(t, s, "download", "watch", "--account", "acct-1", "--job", "job-1", "--once", "--human-friendly")
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	assertNoTerminalControls(t, "download watch", out)

	out, _, err = runDL(t, s, "download", "link", "--account", "acct-1", "--job", "job-1", "--human-friendly")
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	assertNoTerminalControls(t, "download link", out)

	dest := t.TempDir()
	out, stderr, err := runDL(t, s, "download", "fetch", "--account", "acct-1", "--dest", dest, "--year", "2022", "--human-friendly")
	if err != nil {
		t.Fatalf("fetch: %v\n%s", err, stderr)
	}
	assertNoTerminalControls(t, "download fetch progress", stderr)
	if strings.Contains(out, "\x1b") {
		t.Fatalf("fetch summary carries ESC: %q", out)
	}

	// A status the portal made up reaches the watch line too.
	s.mu.Lock()
	s.jobs[0].Status = "INPROCESS" + hostile
	s.mu.Unlock()
	out, _, err = runDL(t, s, "download", "watch", "--account", "acct-1", "--job", "job-1", "--once", "--human-friendly")
	if err == nil || ExitCode(err) != ExitJobPending {
		t.Fatalf("watch --once on a running job: want exit %d, got %v", ExitJobPending, err)
	}
	assertNoTerminalControls(t, "download watch error", err.Error())
	assertNoTerminalControls(t, "download watch status", out)
}

// TestTenantfScrubsOnlyTenantArguments pins the funnel itself: strings and
// errors are scrubbed, the format's own newline and numbers are not.
func TestTenantfScrubsOnlyTenantArguments(t *testing.T) {
	var b strings.Builder
	tenantf(&b, "%s %d %v\n", "a\x1b[2Jb\nc", 42, errString("e\x07rr"))
	if got := b.String(); got != "a[2Jb c 42 err\n" {
		t.Fatalf("tenantf = %q", got)
	}
	cells := tenantCells([][]string{{"x\ty", "\u009bz"}})
	if cells[0][0] != "x y" || cells[0][1] != "z" {
		t.Fatalf("tenantCells = %q", cells)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// TestDownloadErrorsAreScrubbed proves portal text inside a returned error
// is scrubbed before the root prints it, while the error keeps its identity
// and exit code.
func TestDownloadErrorsAreScrubbed(t *testing.T) {
	testenv.Isolate(t)
	s := newDLStub(t)
	s.addFile("job-1", jane, "O365MAIL", "d1", "jane.zip", []byte("ARCHIVE"))
	s.urlFailure = "The download link has expired." + hostile
	_, _, err := runDL(t, s, "download", "link", "--account", "acct-1", "--job", "job-1")
	if err == nil {
		t.Fatal("want the portal failure")
	}
	assertNoTerminalControls(t, "download link error", err.Error())

	sentinel := errors.New("bad\x1b[2Jinput")
	wrapped := tenantErr(usageErr(sentinel))
	if ExitCode(wrapped) != 2 || !errors.Is(wrapped, sentinel) {
		t.Fatalf("scrubbing changed the error: code=%d is=%v", ExitCode(wrapped), errors.Is(wrapped, sentinel))
	}
	if strings.Contains(wrapped.Error(), "\x1b") || tenantErr(nil) != nil {
		t.Fatalf("tenantErr = %q", wrapped.Error())
	}
}
