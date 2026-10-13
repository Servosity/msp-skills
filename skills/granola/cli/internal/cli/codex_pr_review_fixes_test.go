// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Regression tests for the full-PR Codex review (2026-10-13). Each one was
// written to fail against the code it guards before the fix.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"granola-pp-cli/internal/config"
)

// isolateGranola points every path, the API, and the Keychain hook at
// throwaway locations so a test can never reach the real store, the real
// Keychain, or a Granola host.
func isolateGranola(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GRANOLA_HOME", t.TempDir())
	t.Setenv("GRANOLA_NO_AUTO_REFRESH", "1")
	t.Setenv("GRANOLA_API_KEY", "test-key")
	if baseURL != "" {
		t.Setenv("GRANOLA_BASE_URL", baseURL)
	}
	prev := kcKeychainPath
	kcKeychainPath = filepath.Join(t.TempDir(), "absent.keychain-db")
	t.Cleanup(func() { kcKeychainPath = prev })
}

// TestFindDuplicatesDifferentRecordedIDIsNotADuplicate: a same-title,
// same-day file that records a DIFFERENT meeting id is a different meeting
// (two "Weekly Sync" calls on one day) and must not gate.
func TestFindDuplicatesDifferentRecordedIDIsNotADuplicate(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "2026-10-12 Weekly Sync - Acme - Full.md")
	if err := os.WriteFile(other, []byte("# Weekly Sync\n\n**Meeting ID:** not_AAAAAAAAAAAA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dup, collisions := findDuplicates(root, "not_BBBBBBBBBBBB", "Weekly Sync", "2026-10-12")
	if dup != "" {
		t.Fatalf("a file recording another meeting id gated as a duplicate: %s", dup)
	}
	if len(collisions) != 1 {
		t.Fatalf("expected the near-miss reported as a title collision, got %v", collisions)
	}
	// A legacy file with no recorded id still gates on title + day.
	legacy := filepath.Join(root, "2026-10-12 Weekly Sync - Legacy - Full.md")
	if err := os.WriteFile(legacy, []byte("# Weekly Sync\n\nno id here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if dup, _ := findDuplicates(root, "not_BBBBBBBBBBBB", "Weekly Sync", "2026-10-12"); dup != legacy {
		t.Fatalf("legacy same-day file: dup=%q, want %q", dup, legacy)
	}
}

// TestExtractLeavesNoPartialTriple: if a target cannot be written, extract
// must not leave full_<id>.md behind (a retry would refuse it forever).
func TestExtractLeavesNoPartialTriple(t *testing.T) {
	isolateGranola(t, "")
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "not_partial", Title: "T", CreatedAt: "2026-05-01T10:00:00Z"})
	for i := 0; i < 6; i++ {
		insertSegment(t, s, "not_partial", 2*i, "microphone", "m")
		insertSegment(t, s, "not_partial", 2*i+1, "speaker", "s")
	}
	s.Close()
	out := t.TempDir()
	if err := os.Mkdir(filepath.Join(out, "summary_not_partial.md"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := runExtract("not_partial", out, "", false); err == nil {
		t.Fatal("extract succeeded although summary_<id>.md is a directory")
	}
	if _, err := os.Stat(filepath.Join(out, "full_not_partial.md")); !os.IsNotExist(err) {
		t.Fatalf("a partial triple was left behind: full_not_partial.md stat err=%v", err)
	}
}

// TestDuplicatesScanMatchesAMeetingToItsMemoFile: the scan must put a stored
// meeting and its memo file in one group, by id in the file name or by
// title + the day in the file name. The old fingerprint mixed in an attendee
// set the file side never had, so they never met.
func TestDuplicatesScanMatchesAMeetingToItsMemoFile(t *testing.T) {
	isolateGranola(t, "")
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "not_qbrqbrqbr1", Title: "QBR", CreatedAt: "2026-10-01T15:00:00Z"})
	insertMeeting(t, s, seedMeeting{ID: "not_kickoff001", Title: "Kickoff", CreatedAt: "2026-10-02T15:00:00Z"})
	if _, err := s.DB().Exec(`INSERT INTO attendees(meeting_id,email,name) VALUES ('not_qbrqbrqbr1','alice@example.com','Alice')`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	root := t.TempDir()
	for _, name := range []string{"2026-10-01 QBR - Acme - Full.md", "full_not_kickoff001.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stdout, _, err := runRootArgs(t, "duplicates", "scan", "--root", root, "--no-refresh")
	if err != nil {
		t.Fatalf("duplicates scan: %v", err)
	}
	groups := 0
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		var g struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal([]byte(line), &g); err != nil {
			t.Fatalf("bad ndjson line %q: %v", line, err)
		}
		if g.Count == 2 {
			groups++
		}
	}
	if groups != 2 {
		t.Fatalf("want 2 cache+file groups (title/day match and id match), got %d:\n%s", groups, stdout)
	}
}

// TestUpsertKeepsAttendeesWhenOnlyInviteesArrive: a detail response that
// omits `attendees` and carries only calendar invitees must not delete the
// stored attendees, and must not blank a stored name.
func TestUpsertKeepsAttendeesWhenOnlyInviteesArrive(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	ctx := context.Background()
	full := &restNoteDetail{ID: "not_att", Title: "T", Attendees: &[]restAttendee{{Email: "alice@example.com", Name: "Alice"}, {Email: "bob@example.com", Name: "Bob"}}}
	if err := upsertRichMeeting(ctx, s.DB(), full); err != nil {
		t.Fatal(err)
	}
	partial := &restNoteDetail{ID: "not_att", CalendarEvent: json.RawMessage(`{"invitees":[{"email":"alice@example.com"}]}`)}
	if err := upsertRichMeeting(ctx, s.DB(), partial); err != nil {
		t.Fatal(err)
	}
	rows, err := s.DB().Query(`SELECT email, COALESCE(name,'') FROM attendees WHERE meeting_id='not_att' ORDER BY email`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var e, n string
		_ = rows.Scan(&e, &n)
		got[e] = n
	}
	if got["alice@example.com"] != "Alice" || got["bob@example.com"] != "Bob" {
		t.Fatalf("attendees after an invitees-only response: %v, want Alice and Bob with names intact", got)
	}
}

// TestPrivateNotesAttributionSeesAllSameTitleMeetings: with A (09:00) and B
// (10:00) titled alike and only B in the batch, the MCP copy of A must still
// be matched to A, never written under B.
func TestPrivateNotesAttributionSeesAllSameTitleMeetings(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	insertMeeting(t, s, seedMeeting{ID: "not_A", Title: "Weekly Sync", CreatedAt: "2026-10-12T09:00:00Z"})
	insertMeeting(t, s, seedMeeting{ID: "not_B", Title: "Weekly Sync", CreatedAt: "2026-10-12T10:00:00Z"})
	targets := []restTarget{{ID: "not_B", Title: "Weekly Sync", Created: time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)}}
	byTitle := map[string][]mcpMeeting{normTitle("Weekly Sync"): {{UUID: "uuid-A", Title: "Weekly Sync", Date: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}}}
	got := assignMeetingUUIDs(byTitle, withSameTitleStoredMeetings(context.Background(), s.DB(), targets))
	if got["not_B"] == "uuid-A" {
		t.Fatal("meeting A's private notes were attributed to meeting B")
	}
}

// TestStoreMCPTokenSavesRefreshTokenFirstAndReportsFailures: the rotated
// refresh token is the one value that cannot be recovered, so it is written
// first, a failure elsewhere does not stop it, and every failure surfaces.
func TestStoreMCPTokenSavesRefreshTokenFirstAndReportsFailures(t *testing.T) {
	var order []string
	prev := kcSetFn
	kcSetFn = func(account, value string) error {
		order = append(order, account)
		if account == kcAcctAccess {
			return errors.New("keychain write failed")
		}
		return nil
	}
	t.Cleanup(func() { kcSetFn = prev })
	err := storeMCPToken("client", &mcpToken{AccessToken: "a", RefreshToken: "r1", ExpiresIn: 60})
	if err == nil {
		t.Fatal("a failed access-token write was not reported")
	}
	if len(order) == 0 || order[0] != kcAcctRefresh {
		t.Fatalf("write order %v: the refresh token must be written first", order)
	}
	saw := map[string]bool{}
	for _, a := range order {
		saw[a] = true
	}
	if !saw[kcAcctExpiry] || !saw[kcAcctClientID] {
		t.Fatalf("a failed write stopped the remaining ones: %v", order)
	}
}

// TestSetTokenReplacesAStoredAPIKey: with api_key = "old" in the config file,
// `auth set-token` piping "new" must leave "new" as the credential in use.
func TestSetTokenReplacesAStoredAPIKey(t *testing.T) {
	isolateGranola(t, "")
	t.Setenv("GRANOLA_API_KEY", "")
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("api_key = \"old\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := RootCmd()
	root.SetIn(strings.NewReader("new"))
	root.SetOut(&strings.Builder{})
	root.SetErr(&strings.Builder{})
	root.SetArgs([]string{"auth", "set-token", "--config", cfgPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("set-token: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuthHeader(); got != "Bearer new" {
		t.Fatalf("after set-token, AuthHeader() = %q, want the new key", got)
	}
}

// TestRESTHelpersHonorConfigFlag: the leaf REST helpers read the --config
// the running command was given, not the default.
func TestRESTHelpersHonorConfigFlag(t *testing.T) {
	isolateGranola(t, "")
	t.Setenv("GRANOLA_BASE_URL", "")
	cfgPath := filepath.Join(t.TempDir(), "alt.toml")
	if err := os.WriteFile(cfgPath, []byte("base_url = \"https://alt.example.invalid\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := granolaActiveFlags
	granolaActiveFlags = &rootFlags{configPath: cfgPath}
	t.Cleanup(func() { granolaActiveFlags = prev })
	cfg, err := granolaRESTConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://alt.example.invalid" {
		t.Fatalf("REST helper config base_url = %q, want the --config value", cfg.BaseURL)
	}
}

// TestLiveTranscriptRejectsAnotherMeetingsAnswer: a 200 whose id is a
// different meeting must never be labelled as the requested one.
func TestLiveTranscriptRejectsAnotherMeetingsAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seg := restSegment{Text: "hi"}
		_ = json.NewEncoder(w).Encode(restNoteDetail{ID: "not_OTHER", Transcript: &[]restSegment{seg}, SummaryMD: "x"})
	}))
	defer srv.Close()
	isolateGranola(t, srv.URL)
	if _, err := fetchTranscriptREST("not_ASKED"); err == nil {
		t.Fatal("transcript from another meeting was accepted")
	}
	if _, err := restSummaryPanel(context.Background(), "not_ASKED"); err == nil {
		t.Fatal("summary from another meeting was accepted")
	}
}

// TestMCPAuthSkipsAutoRefresh: auto-refresh must not run in front of
// mcp-auth, which manages (and may rotate) the MCP tokens itself.
func TestMCPAuthSkipsAutoRefresh(t *testing.T) {
	root := RootCmd()
	cmd, _, err := root.Find([]string{"mcp-auth", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !shouldSkipAutoRefresh(cmd) {
		t.Fatal("auto-refresh would run before mcp-auth status")
	}
}

// TestMeetingsGetLiveSkipsStoredCopy: an explicit --data-source live asks the
// API even when a stored copy exists.
func TestMeetingsGetLiveSkipsStoredCopy(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "not_live1", "title": "Fresh Title"})
	}))
	defer srv.Close()
	isolateGranola(t, srv.URL)
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "not_live1", Title: "Stale Title", CreatedAt: "2026-05-01T10:00:00Z"})
	s.Close()
	stdout, _, err := runRootArgs(t, "meetings", "get", "not_live1", "--data-source", "live", "--json", "--no-refresh")
	if err != nil {
		t.Fatalf("meetings get live: %v", err)
	}
	if hits == 0 || !strings.Contains(stdout, "Fresh Title") {
		t.Fatalf("live get returned the stored copy (hits=%d): %s", hits, stdout)
	}
}

// TestExportUsesStoredSummaryAndReportsFetchFailure: hydration stores the
// AI summary, so a local export carries it; a failed live fetch is reported
// as a failure, never as "no AI panel".
func TestExportUsesStoredSummaryAndReportsFetchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	isolateGranola(t, srv.URL)
	s := newIsolatedStore(t)
	ctx := context.Background()
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{ID: "not_sum1", Title: "With Summary", SummaryMD: "## Summary\nAgreed on the plan."}); err != nil {
		t.Fatal(err)
	}
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{ID: "not_sum2", Title: "No Stored Summary"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	a, err := buildArtifacts("not_sum1", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.PanelSummary, "Agreed on the plan.") {
		t.Fatalf("local export lost the stored summary: %q", a.PanelSummary)
	}
	b, err := buildArtifacts("not_sum2", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if b.PanelFetchErr == "" || !strings.Contains(noSummaryLine(b), "could not be fetched") {
		t.Fatalf("a failed live summary fetch was rendered as absent: %q", noSummaryLine(b))
	}
}

// TestPreflightLookupFailureIsNotNotFound: a refused key or an outage says
// nothing about the meeting, so preflight exits 5, never 3.
func TestPreflightLookupFailureIsNotNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	isolateGranola(t, srv.URL)
	s := newIsolatedStore(t)
	s.Close()
	_, _, err := runRootArgs(t, "preflight", "not_unenriched", "--root", t.TempDir(), "--json", "--no-refresh")
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 5 {
		t.Fatalf("preflight on a 401 lookup: err=%v, want exit code 5", err)
	}
}
