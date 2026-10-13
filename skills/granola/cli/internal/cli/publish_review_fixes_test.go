// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKCLineRefusesOverlongLine: security(1) reads interactive input with a
// 4096-byte buffer and runs whatever spills past it as a second command, so a
// rendered line over kcMaxLine must be refused, never executed. A value the
// size of a real OAuth access token (about 1 KB) must still pass.
func TestKCLineRefusesOverlongLine(t *testing.T) {
	if _, err := kcLine("add-generic-password", "-U", "-a", kcAcctAccess, "-s", mcpKCService, "-w", strings.Repeat("A", 4016)+"help help "); err == nil {
		t.Fatal("kcLine accepted a line past the security(1) buffer; want a refusal")
	}
	line, err := kcLine("add-generic-password", "-U", "-a", kcAcctAccess, "-s", mcpKCService, "-w", strings.Repeat("A", 1200))
	if err != nil {
		t.Fatalf("kcLine refused a token-sized value: %v", err)
	}
	if len(line) > kcMaxLine {
		t.Fatalf("accepted line is %d bytes, over kcMaxLine %d", len(line), kcMaxLine)
	}
}

// TestWriteSafeFileRefusesDirectoryForm: filepath.Dir/Base drop a trailing
// separator, so "out/" would otherwise turn into the file "out/out". Both the
// trailing-separator form and an existing directory must fail, and nothing may
// be created inside the directory.
func TestWriteSafeFileRefusesDirectoryForm(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeSafeFile(dir+string(os.PathSeparator), []byte("x")); err == nil {
		t.Error("writeSafeFile accepted a trailing-separator path")
	}
	if err := writeSafeFile(dir, []byte("x")); err == nil {
		t.Error("writeSafeFile accepted an existing directory as the target")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes")); !os.IsNotExist(err) {
		t.Errorf("a file was created inside the directory: stat err=%v", err)
	}
}

// TestFindDuplicatesSingleFileRoot: --root may name one file. The old
// filepath.Walk visited it, so an id in its name was a duplicate; the os.Root
// rewrite must keep that, through a root on the file's parent.
func TestFindDuplicatesSingleFileRoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "full_not_abc.md")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	dup, _ := findDuplicates(file, "not_abc", "Weekly Sync", "")
	if dup != file {
		t.Fatalf("single-file --root: dup=%q, want %q", dup, file)
	}
}

// TestFindDuplicatesFollowsSymlinkedRoot: the transcripts root may itself be a
// symlink. filepath.Walk never descended into a symlinked root, so the gate
// passed every meeting; the os.Root walk follows it, and reports paths under
// the root the caller passed.
func TestFindDuplicatesFollowsSymlinkedRoot(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "2026-01-05 Weekly Sync - Acme - Full.md"
	if err := os.WriteFile(filepath.Join(real, name), []byte("Meeting ID: not_xyz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	dup, _ := findDuplicates(link, "not_xyz", "Weekly Sync", "2026-02-02")
	if want := filepath.Join(link, name); dup != want {
		t.Fatalf("symlinked --root: dup=%q, want %q", dup, want)
	}
}

// TestDoctorFailOnRecognizesErrorPrefix: a verdict that opens with "ERROR"
// (the wrong-base_url state) must trip --fail-on=error even when it carries
// none of the lowercase keywords the gate also matches.
func TestDoctorFailOnRecognizesErrorPrefix(t *testing.T) {
	report := map[string]any{
		"api":    "ERROR base_url is not the Granola public API root: GET /v1/notes returned HTTP 404",
		"config": "ok",
	}
	if err := doctorExitForFailOn("error", report); err == nil {
		t.Fatal("--fail-on=error stayed green on an ERROR verdict")
	}
	healthy := map[string]any{
		"api":         "reachable (GET /v1/notes answered)",
		"credentials": "verified (authenticated GET /v1/notes succeeded)",
		// Metadata is not a verdict: a config file that happens to be named
		// FAIL.toml must not trip the gate.
		"config_path": "FAIL.toml",
	}
	if err := doctorExitForFailOn("error", healthy); err != nil {
		t.Fatalf("--fail-on=error tripped on a healthy report: %v", err)
	}
}

// TestLoadMeetingsQueryMatchesTranscript: `meetings list --query` must find a
// meeting by something said on the call, not only by its title or typed
// notes. The generated query looked at title + notes_plain alone.
func TestLoadMeetingsQueryMatchesTranscript(t *testing.T) {
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "not_said", Title: "Weekly Sync", CreatedAt: "2026-07-01T09:00:00Z", StartedAt: "2026-07-01T09:00:00Z"})
	insertSegment(t, s, "not_said", 0, "speaker", "we need to talk about the backup migration plan")
	insertMeeting(t, s, seedMeeting{ID: "not_other", Title: "Weekly Sync", CreatedAt: "2026-07-02T09:00:00Z", StartedAt: "2026-07-02T09:00:00Z"})
	insertSegment(t, s, "not_other", 0, "speaker", "nothing relevant here")
	// REST hydration writes workspace_id as '' (never NULL); match it.
	if _, err := s.DB().Exec(`UPDATE meetings SET workspace_id = ''`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	rows, err := loadMeetings(context.Background(), time.Time{}, time.Time{}, "", "backup migration", false, 10, 0)
	if err != nil {
		t.Fatalf("loadMeetings: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "not_said" {
		got := []string{}
		for _, r := range rows {
			got = append(got, r.ID)
		}
		t.Fatalf("--query on transcript text: got %v, want [not_said]", got)
	}
}

// TestPreflightHydratesAMeetingSyncHasNotEnriched: a meeting that is in the
// account but not yet in the rich store used to exit 3, the same code as a
// duplicate, so a caller that skips on 3 dropped it. preflight now fetches
// that one meeting from the REST detail endpoint and judges it.
func TestPreflightHydratesAMeetingSyncHasNotEnriched(t *testing.T) {
	s := newIsolatedStore(t)
	s.Close()
	seg := func(i int, src string) restSegment {
		r := restSegment{Text: "segment", StartTime: "2026-07-01T09:00:00Z", EndTime: "2026-07-01T09:00:05Z"}
		r.Speaker.Source = src
		return r
	}
	var segs []restSegment
	for i := 0; i < 6; i++ {
		segs = append(segs, seg(i, "microphone"), seg(i, "speaker"))
	}
	fetched := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/notes/not_late" {
			http.NotFound(w, r)
			return
		}
		fetched++
		_ = json.NewEncoder(w).Encode(restNoteDetail{
			ID: "not_late", Title: "Late Meeting", CreatedAt: "2026-07-01T09:05:00Z", UpdatedAt: "2026-07-01T09:40:00Z",
			Transcript: &segs,
		})
	}))
	defer srv.Close()
	// Full isolation: HOME, GRANOLA_HOME, and the Keychain hook, so the MCP
	// private-notes step can never read the real Keychain or call Granola.
	isolateGranola(t, srv.URL)

	stdout, _, err := runRootArgs(t, "preflight", "not_late", "--root", t.TempDir(), "--json", "--no-refresh")
	if err != nil {
		t.Fatalf("preflight on an unenriched meeting: %v (stdout %s)", err, stdout)
	}
	if fetched != 1 {
		t.Fatalf("detail endpoint fetched %d times, want 1", fetched)
	}
	if !strings.Contains(stdout, `"ok": true`) && !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("preflight did not pass after hydrating: %s", stdout)
	}
}
