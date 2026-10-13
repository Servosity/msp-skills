// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Regression tests for the full-PR Codex review (2026-10-13). Each one was
// written to fail against the code it guards before the fix.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"granola-pp-cli/internal/client"
	"granola-pp-cli/internal/config"
	"granola-pp-cli/internal/granola"
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

// TestPrivateNoteFromGetMeetingsReadsTheLiveFormat: the live get_meetings
// tag carries extra attributes after date (meeting_at, created_at, url, ...);
// the old exact-shape regex matched nothing and every private note was
// dropped. One block resolves; zero or two do not.
func TestPrivateNoteFromGetMeetingsReadsTheLiveFormat(t *testing.T) {
	live := `<meeting id="0b9a63e7-468f-4950-a9f6-dabea60de3e7" title="Acme / Sync" date="Oct 13, 2026 2:00 PM EDT" meeting_at="2026-10-13T14:00:00.000-04:00" created_at="2026-10-13T13:58:00.000-04:00" updated_at="2026-10-13T15:00:00.000-04:00" url="https://notes.granola.ai/d/0b9a63e7-468f-4950-a9f6-dabea60de3e7">
  <private_notes>the human notes &amp; more</private_notes>
  <summary>the ai summary</summary>
</meeting>`
	note, present, ok := privateNoteFromGetMeetings(live)
	if !ok || !present || note != "the human notes & more" {
		t.Fatalf("live format: note=%q resolved=%v", note, ok)
	}
	if _, _, ok := privateNoteFromGetMeetings(""); ok {
		t.Fatal("an empty answer resolved")
	}
	if _, _, ok := privateNoteFromGetMeetings(live + "\n" + live); ok {
		t.Fatal("an ambiguous two-block answer resolved")
	}
	if note, present, ok := privateNoteFromGetMeetings(`<meeting id="x" title="T" date="D"></meeting>`); !ok || present || note != "" {
		t.Fatalf("a meeting without private notes: note=%q resolved=%v", note, ok)
	}
}

// TestFindDuplicatesUsesDeclaredIdentityNotMentions: a memo that declares
// meeting A and merely mentions meeting B's id is A's memo, not B's; and a
// file that declares only a legacy (non-REST) id falls back to title + day.
func TestFindDuplicatesUsesDeclaredIdentityNotMentions(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "2026-10-12 Weekly Sync - Acme - Full.md")
	body := "- **Meeting Link:** https://notes.granola.ai/d/not_AAAAAAAAAAAA\n\nFollow-up from not_BBBBBBBBBBBB discussed.\n"
	if err := os.WriteFile(a, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if dup, _ := findDuplicates(root, "not_BBBBBBBBBBBB", "Weekly Sync", "2026-10-12"); dup != "" {
		t.Fatalf("a mention of B's id in A's memo gated B: %s", dup)
	}
	if dup, _ := findDuplicates(root, "not_AAAAAAAAAAAA", "Weekly Sync", "2026-10-12"); dup != a {
		t.Fatalf("A's own memo did not gate A: dup=%q", dup)
	}
	legacyRoot := t.TempDir()
	legacy := filepath.Join(legacyRoot, "2026-07-01 Board Call - Acme - Full.md")
	if err := os.WriteFile(legacy, []byte("- **Meeting Link:** https://notes.granola.ai/d/0f8fad5b-d9cb-469f-a165-70867728950e\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if dup, _ := findDuplicates(legacyRoot, "not_CCCCCCCCCCCC", "Board Call", "2026-07-01"); dup != legacy {
		t.Fatalf("a legacy-id memo on the same day did not gate: dup=%q", dup)
	}
}

// TestDuplicatesScanKeepsDistinctMeetingsApart: two different "QBR" meetings
// on the same day are two meetings, never one duplicate group.
func TestDuplicatesScanKeepsDistinctMeetingsApart(t *testing.T) {
	isolateGranola(t, "")
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "not_qbr_alice1", Title: "QBR", CreatedAt: "2026-10-12T09:00:00Z"})
	insertMeeting(t, s, seedMeeting{ID: "not_qbr_bob001", Title: "QBR", CreatedAt: "2026-10-12T15:00:00Z"})
	s.Close()
	stdout, _, err := runRootArgs(t, "duplicates", "scan", "--root", t.TempDir(), "--no-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("two distinct meetings were reported as duplicates:\n%s", stdout)
	}
}

// TestUpsertKeepsInviteesWhenOnlyAttendeesArrive is the reverse partial: an
// attendees-only response must not delete a stored invitee or a known name.
func TestUpsertKeepsInviteesWhenOnlyAttendeesArrive(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	ctx := context.Background()
	first := &restNoteDetail{ID: "not_rev", Title: "T",
		Attendees:     &[]restAttendee{{Email: "alice@example.com", Name: "Alice"}},
		CalendarEvent: json.RawMessage(`{"invitees":[{"email":"bob@example.com","name":"Bob"}]}`)}
	if err := upsertRichMeeting(ctx, s.DB(), first); err != nil {
		t.Fatal(err)
	}
	if err := upsertRichMeeting(ctx, s.DB(), &restNoteDetail{ID: "not_rev", Attendees: &[]restAttendee{{Email: "alice@example.com"}}}); err != nil {
		t.Fatal(err)
	}
	var n int
	var aliceName string
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM attendees WHERE meeting_id='not_rev'`).Scan(&n)
	_ = s.DB().QueryRow(`SELECT COALESCE(name,'') FROM attendees WHERE meeting_id='not_rev' AND email='alice@example.com'`).Scan(&aliceName)
	if n != 2 || aliceName != "Alice" {
		t.Fatalf("after an attendees-only response: %d rows, alice name %q; want 2 rows and Alice", n, aliceName)
	}
}

// TestStoreMCPTokenNeverAdvancesExpiryWithoutTheAccessToken: if the access
// token was not saved, the stored expiry must make the next run refresh.
func TestStoreMCPTokenNeverAdvancesExpiryWithoutTheAccessToken(t *testing.T) {
	saved := map[string]string{}
	prev := kcSetFn
	kcSetFn = func(account, value string) error {
		if account == kcAcctAccess {
			return errors.New("keychain write failed")
		}
		saved[account] = value
		return nil
	}
	t.Cleanup(func() { kcSetFn = prev })
	_ = storeMCPToken("client", &mcpToken{AccessToken: "a1", RefreshToken: "r1", ExpiresIn: 3600})
	if saved[kcAcctRefresh] != "r1" {
		t.Fatalf("refresh token not saved: %v", saved)
	}
	if saved[kcAcctExpiry] != "1" {
		t.Fatalf("expiry advanced to %q although the access token was not saved", saved[kcAcctExpiry])
	}
	// A lost refresh token is a distinct, detectable failure.
	kcSetFn = func(account, value string) error {
		if account == kcAcctRefresh {
			return errors.New("keychain write failed")
		}
		return nil
	}
	if err := storeMCPToken("client", &mcpToken{AccessToken: "a2", RefreshToken: "r2", ExpiresIn: 3600}); !errors.Is(err, errRefreshNotSaved) {
		t.Fatalf("a lost refresh token was not reported as errRefreshNotSaved: %v", err)
	}
}

// TestEnvAPIKeyWinsOverStoredAuthHeader: an explicit GRANOLA_API_KEY must be
// the credential sent, even when a stored auth_header exists.
func TestEnvAPIKeyWinsOverStoredAuthHeader(t *testing.T) {
	isolateGranola(t, "")
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("auth_header = \"Bearer old\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GRANOLA_API_KEY", "new")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.AuthHeader(); got != "Bearer new" {
		t.Fatalf("AuthHeader() = %q with GRANOLA_API_KEY set, want the env key", got)
	}
}

// TestLeafRESTClientHonorsNoCache: --no-cache reaches the leaf helpers.
func TestLeafRESTClientHonorsNoCache(t *testing.T) {
	prev := granolaActiveFlags
	granolaActiveFlags = &rootFlags{noCache: true}
	t.Cleanup(func() { granolaActiveFlags = prev })
	c := client.New(&config.Config{BaseURL: "http://127.0.0.1:1"}, 0, 0)
	applyCachePolicy(c)
	if !c.NoCache {
		t.Fatal("--no-cache did not reach the leaf REST client")
	}
}

// TestAttendeeBriefReturnsDistinctMeetings: two matching attendees on one
// meeting must not make it appear twice and push an older meeting out.
func TestAttendeeBriefReturnsDistinctMeetings(t *testing.T) {
	isolateGranola(t, "")
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "not_new", Title: "New", CreatedAt: "2026-10-12T09:00:00Z", StartedAt: "2026-10-12T09:00:00Z"})
	insertMeeting(t, s, seedMeeting{ID: "not_old", Title: "Old", CreatedAt: "2026-10-01T09:00:00Z", StartedAt: "2026-10-01T09:00:00Z"})
	for _, q := range []string{
		`INSERT INTO attendees(meeting_id,email,name) VALUES ('not_new','alice@smith.example','Alice Smith')`,
		`INSERT INTO attendees(meeting_id,email,name) VALUES ('not_new','bob@smith.example','Bob Smith')`,
		`INSERT INTO attendees(meeting_id,email,name) VALUES ('not_old','alice@smith.example','Alice Smith')`,
	} {
		if _, err := s.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	stdout, _, err := runRootArgs(t, "attendee", "brief", "Smith", "--last", "2", "--json", "--data-source", "local", "--no-refresh")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var r struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(line), &r) == nil {
			ids[r.ID]++
		}
	}
	if ids["not_new"] != 1 || ids["not_old"] != 1 {
		t.Fatalf("attendee brief rows: %v, want each meeting once", ids)
	}
}

// TestYAMLEscapeBackslashes: a title with backslashes must stay valid YAML.
func TestYAMLEscapeBackslashes(t *testing.T) {
	if got, want := yamlEscape(`QBR: C:\Users\Alice`), `"QBR: C:\\Users\\Alice"`; got != want {
		t.Fatalf("yamlEscape = %s, want %s", got, want)
	}
}

// TestPendingHumanNotesAreTrackedApartFromREST: hydrated meetings MCP never
// answered for stay pending until an MCP pass records them.
func TestPendingHumanNotesAreTrackedApartFromREST(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	insertMeeting(t, s, seedMeeting{ID: "not_p1", Title: "P1", CreatedAt: "2026-10-12T09:00:00Z"})
	ctx := context.Background()
	pending, err := pendingHumanNoteTargets(ctx, s.DB(), 25)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending before any MCP pass: %v, %v", pending, err)
	}
	markHumanNotesChecked(ctx, s.DB(), pending)
	if pending, _ = pendingHumanNoteTargets(ctx, s.DB(), 25); len(pending) != 0 {
		t.Fatalf("still pending after MCP answered: %v", pending)
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

// --- round 3 -----------------------------------------------------------------

// TestRecordHumanNotesIsAtomic: if the notes write fails, the meeting must
// not be marked done.
func TestRecordHumanNotesIsAtomic(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	insertMeeting(t, s, seedMeeting{ID: "not_atomic", Title: "T", CreatedAt: "2026-10-12T09:00:00Z"})
	if _, err := s.DB().Exec(`CREATE TRIGGER fail_notes BEFORE UPDATE OF notes_markdown ON meetings BEGIN SELECT RAISE(ABORT, 'disk full'); END`); err != nil {
		t.Fatal(err)
	}
	if err := recordHumanNotes(context.Background(), s.DB(), "not_atomic", "private note", true, true, "2026-10-13T00:00:00Z"); err == nil {
		t.Fatal("a failed notes write was not reported")
	}
	var n int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM human_notes_state WHERE meeting_id='not_atomic'`).Scan(&n)
	if n != 0 {
		t.Fatal("the meeting was marked done although its notes were not written")
	}
}

// TestUnresolvedHumanNotesAreRetriedAfterADay: a meeting MCP listed no match
// for is retried once its check is a day old; a resolved one never is.
func TestUnresolvedHumanNotesAreRetriedAfterADay(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	ctx := context.Background()
	insertMeeting(t, s, seedMeeting{ID: "not_unres", Title: "U", CreatedAt: "2026-10-12T09:00:00Z"})
	insertMeeting(t, s, seedMeeting{ID: "not_res", Title: "R", CreatedAt: "2026-10-12T10:00:00Z"})
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	if err := recordHumanNotes(ctx, s.DB(), "not_unres", "", false, false, old); err != nil {
		t.Fatal(err)
	}
	if err := recordHumanNotes(ctx, s.DB(), "not_res", "", false, true, old); err != nil {
		t.Fatal(err)
	}
	pending, err := pendingHumanNoteTargets(ctx, s.DB(), 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "not_unres" {
		t.Fatalf("pending = %v, want only the unresolved meeting", pending)
	}
}

// TestEffectiveRESTTargetUsesStoredMetadata: a partial detail response that
// omits title/created_at must not blank the private-notes match target.
func TestEffectiveRESTTargetUsesStoredMetadata(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	insertMeeting(t, s, seedMeeting{ID: "not_meta", Title: "Board Call", CreatedAt: "2026-10-12T09:00:00Z"})
	got := effectiveRESTTarget(context.Background(), s.DB(), &restNoteDetail{ID: "not_meta", UpdatedAt: "2026-10-13T20:00:00Z"})
	if got.Title != "Board Call" || got.Created.IsZero() {
		t.Fatalf("target from a partial response = %+v, want the stored title and date", got)
	}
}

// TestExtractRefusesWhileAnotherRunHoldsTheLock: two extracts of one meeting
// into one folder must not interleave. The lock is OS-held (flock /
// LockFileEx), so releasing it - or the holder dying - lets the next run in.
func TestExtractRefusesWhileAnotherRunHoldsTheLock(t *testing.T) {
	isolateGranola(t, "")
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "not_lock", Title: "T", CreatedAt: "2026-05-01T10:00:00Z"})
	for i := 0; i < 6; i++ {
		insertSegment(t, s, "not_lock", 2*i, "microphone", "m")
		insertSegment(t, s, "not_lock", 2*i+1, "speaker", "s")
	}
	s.Close()
	out := t.TempDir()
	unlock, err := lockExtract(out, "not_lock")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runExtract("not_lock", out, "", false); err == nil || !strings.Contains(err.Error(), "another extract") {
		t.Fatalf("extract ran while another held the lock: err=%v", err)
	}
	unlock()
	if _, err := runExtract("not_lock", out, "", false); err != nil {
		t.Fatalf("extract did not run after the lock was released: %v", err)
	}
}

// TestEnsureSchemaUpgradesHumanNotesState: a store created before the
// resolved column existed gains it, keeping its rows.
func TestEnsureSchemaUpgradesHumanNotesState(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	ctx := context.Background()
	if _, err := s.DB().Exec(`DROP TABLE human_notes_state`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`CREATE TABLE human_notes_state (meeting_id TEXT PRIMARY KEY, checked_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO human_notes_state VALUES ('not_kept', '2026-10-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := granola.EnsureSchema(ctx, s.DB()); err != nil {
		t.Fatalf("EnsureSchema on the old shape: %v", err)
	}
	insertMeeting(t, s, seedMeeting{ID: "not_new", Title: "N", CreatedAt: "2026-10-12T09:00:00Z"})
	if err := recordHumanNotes(ctx, s.DB(), "not_new", "", false, true, "2026-10-13T00:00:00Z"); err != nil {
		t.Fatalf("recordHumanNotes after the upgrade: %v", err)
	}
	var n int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM human_notes_state WHERE meeting_id = 'not_kept'`).Scan(&n)
	if n != 1 {
		t.Fatal("the upgrade lost an existing row")
	}
}

// TestLiveResultsCarryLiveProvenance: data fetched from the API must not be
// labelled meta.source "local" in the --agent envelope.
func TestLiveResultsCarryLiveProvenance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "not_prov1", "title": "Live"})
	}))
	defer srv.Close()
	isolateGranola(t, srv.URL)
	s := newIsolatedStore(t)
	s.Close()
	stdout, _, err := runRootArgs(t, "meetings", "get", "not_prov1", "--data-source", "live", "--agent", "--no-refresh")
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("not an envelope: %v\n%s", err, stdout)
	}
	if env.Meta["source"] != "live" {
		t.Fatalf("meta.source = %v for a live fetch, want live", env.Meta["source"])
	}
}

// TestYAMLEscapeQuotesIndicatorLedTitles: "[QBR]" and "- Planning" must stay
// strings, not a sequence or invalid YAML.
func TestYAMLEscapeQuotesIndicatorLedTitles(t *testing.T) {
	for _, in := range []string{"[QBR]", "- Planning", "{x}", " padded"} {
		if got := yamlEscape(in); !strings.HasPrefix(got, `"`) {
			t.Errorf("yamlEscape(%q) = %s, want a quoted scalar", in, got)
		}
	}
	for _, in := range []string{"true", "null", "2026", "1e3"} {
		if got := yamlEscape(in); !strings.HasPrefix(got, `"`) {
			t.Errorf("yamlEscape(%q) = %s, want a quoted string", in, got)
		}
	}
	for _, in := range []string{"Weekly Sync", "2026-10-13", ""} {
		if got := yamlEscape(in); got != `"`+in+`"` {
			t.Errorf("yamlEscape(%q) = %s, want it quoted as a string", in, got)
		}
	}
}

// TestStoredTranscriptEnvelopeSaysLocal: the --agent envelope speaks
// "live" | "local"; a stored transcript must not surface the internal
// "cache" label.
func TestStoredTranscriptEnvelopeSaysLocal(t *testing.T) {
	isolateGranola(t, "")
	s := newIsolatedStore(t)
	insertMeeting(t, s, seedMeeting{ID: "not_tsrc", Title: "T", CreatedAt: "2026-05-01T10:00:00Z"})
	insertSegment(t, s, "not_tsrc", 0, "microphone", "hello")
	s.Close()
	stdout, _, err := runRootArgs(t, "transcript", "get", "not_tsrc", "--data-source", "local", "--agent", "--no-refresh")
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("not an envelope: %v\n%s", err, stdout)
	}
	if env.Meta["source"] != "local" {
		t.Fatalf("meta.source = %v for a stored transcript, want local", env.Meta["source"])
	}
}

// --- round 6 -----------------------------------------------------------------

// TestExplicitEmptyPrivateNotesClear: a present-but-empty private_notes
// block is authoritative (the user deleted the notes); an answer without the
// block leaves stored notes alone.
func TestExplicitEmptyPrivateNotesClear(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	ctx := context.Background()
	insertMeeting(t, s, seedMeeting{ID: "not_del", Title: "T", CreatedAt: "2026-10-12T09:00:00Z", NotesMarkdown: "old", NotesPlain: "old"})
	notes := func() string {
		var n string
		_ = s.DB().QueryRow(`SELECT COALESCE(notes_markdown,'') FROM meetings WHERE id='not_del'`).Scan(&n)
		return n
	}
	if err := recordHumanNotes(ctx, s.DB(), "not_del", "", false, true, "2026-10-13T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if notes() != "old" {
		t.Fatal("an answer without a private_notes block erased stored notes")
	}
	_, present, ok := privateNoteFromGetMeetings(`<meeting id="u"><private_notes></private_notes></meeting>`)
	if !present || !ok {
		t.Fatal("an explicitly empty private_notes block was not reported as present")
	}
	if err := recordHumanNotes(ctx, s.DB(), "not_del", "", true, true, "2026-10-13T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if notes() != "" {
		t.Fatalf("deleted private notes survived an authoritative empty answer: %q", notes())
	}
}

// TestMeetingTimesAreStoredInUTC: calendar times arrive with an offset;
// stored as text they must be UTC "Z" so --since filters and ordering are
// chronological. Existing offset rows are normalized by EnsureSchema.
func TestMeetingTimesAreStoredInUTC(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	ctx := context.Background()
	d := &restNoteDetail{ID: "not_utc", Title: "T", CreatedAt: "2026-10-13T08:55:00-04:00",
		CalendarEvent: json.RawMessage(`{"scheduled_start_time":"2026-10-13T09:00:00-04:00","scheduled_end_time":"2026-10-13T09:30:00-04:00"}`)}
	if err := upsertRichMeeting(ctx, s.DB(), d); err != nil {
		t.Fatal(err)
	}
	var started, created string
	_ = s.DB().QueryRow(`SELECT started_at, created_at FROM meetings WHERE id='not_utc'`).Scan(&started, &created)
	if started != "2026-10-13T13:00:00Z" || created != "2026-10-13T12:55:00Z" {
		t.Fatalf("stored started_at=%q created_at=%q, want UTC Z form", started, created)
	}
	if _, err := s.DB().Exec(`UPDATE meetings SET started_at='2026-10-13T09:00:00-04:00' WHERE id='not_utc'`); err != nil {
		t.Fatal(err)
	}
	if err := granola.EnsureSchema(ctx, s.DB()); err != nil {
		t.Fatal(err)
	}
	_ = s.DB().QueryRow(`SELECT started_at FROM meetings WHERE id='not_utc'`).Scan(&started)
	if started != "2026-10-13T13:00:00Z" {
		t.Fatalf("an existing offset row was not normalized: %q", started)
	}
}

// TestPartialHydrationStaysSearchable: a pass that writes one meeting and then
// fails on the next fetch must still index the written meeting. The rebuild
// used to sit after the loop, so the early error return skipped it and
// `search` missed rows that were already stored.
func TestPartialHydrationStaysSearchable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/notes/not_ok" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(restNoteDetail{ID: "not_ok", Title: "Quarterly Pipeline Review", CreatedAt: "2026-07-01T09:00:00Z"})
	}))
	defer srv.Close()
	isolateGranola(t, srv.URL)
	s := newIsolatedStore(t)
	defer s.Close()
	c := client.New(&config.Config{BaseURL: srv.URL, GranolaApiKey: "test-key"}, 0, 0)
	c.NoCache = true
	if _, _, err := hydrateRichFromREST(context.Background(), s.DB(), c, []string{"not_ok", "not_bad"}); err == nil {
		t.Fatal("want the not_bad fetch error")
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM meetings_fts WHERE meetings_fts MATCH 'pipeline'`); got != 1 {
		t.Fatalf("written meeting is not searchable after a failed pass: %d FTS hits, want 1", got)
	}
}

// ftsConsistent runs FTS5's own integrity check against the content table:
// it fails when the index holds text a row no longer has, or misses a row.
func ftsConsistent(t *testing.T, db interface {
	Exec(string, ...any) (sql.Result, error)
}) {
	t.Helper()
	for _, tbl := range []string{"meetings_fts", "transcript_fts"} {
		if _, err := db.Exec(`INSERT INTO ` + tbl + `(` + tbl + `, rank) VALUES ('integrity-check', 1)`); err != nil {
			t.Fatalf("%s out of step with its content table: %v", tbl, err)
		}
	}
}

// TestPrivateNoteChangesReindexInTheSameWrite: clearing or changing private
// notes must change search in the same transaction. The index used to be
// rebuilt only after the whole pass, so a cancelled pass left deleted words
// searchable.
func TestPrivateNoteChangesReindexInTheSameWrite(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	ctx := context.Background()
	insertMeeting(t, s, seedMeeting{ID: "not_fts", Title: "T", CreatedAt: "2026-10-12T09:00:00Z", NotesMarkdown: "zebra", NotesPlain: "zebra"})
	hits := func(word string) int {
		return countRows(t, s, `SELECT COUNT(*) FROM meetings_fts WHERE meetings_fts MATCH '`+word+`'`)
	}
	if hits("zebra") != 1 {
		t.Fatal("a newly written meeting is not searchable")
	}
	if err := recordHumanNotes(ctx, s.DB(), "not_fts", "", true, true, "2026-10-13T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if hits("zebra") != 0 {
		t.Fatal("cleared private notes are still searchable")
	}
	if err := recordHumanNotes(ctx, s.DB(), "not_fts", "giraffe", true, true, "2026-10-13T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if hits("giraffe") != 1 {
		t.Fatal("new private notes are not searchable")
	}
	ftsConsistent(t, s.DB())
}

// TestEnsureSchemaReindexesALaggingStore: a store from an earlier build has no
// triggers and may hold an index that lags its rows. Installing the triggers
// must rebuild it in the same step, or the first 'delete' trigger would remove
// text the index never had.
func TestEnsureSchemaReindexesALaggingStore(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	for _, tr := range []string{"meetings_fts_ai", "meetings_fts_ad", "meetings_fts_au", "transcript_fts_ai", "transcript_fts_ad", "transcript_fts_au"} {
		if _, err := s.DB().Exec(`DROP TRIGGER ` + tr); err != nil {
			t.Fatal(err)
		}
	}
	insertMeeting(t, s, seedMeeting{ID: "not_lag", Title: "Lagging Review", CreatedAt: "2026-10-12T09:00:00Z"})
	if err := granola.EnsureSchema(context.Background(), s.DB()); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM meetings_fts WHERE meetings_fts MATCH 'lagging'`); got != 1 {
		t.Fatalf("lagging row not indexed after the upgrade: %d hits", got)
	}
	if _, err := s.DB().Exec(`UPDATE meetings SET title = 'Renamed' WHERE id = 'not_lag'`); err != nil {
		t.Fatal(err)
	}
	ftsConsistent(t, s.DB())
}

// TestTimestampMigrationCanonicalizesFractionalZ: "...13:00:00.100Z" ends in Z
// but is not the canonical form, and lexically sorts BEFORE nothing it should.
// Every stored meeting time must end up as whole-second UTC "Z".
func TestTimestampMigrationCanonicalizesFractionalZ(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	insertMeeting(t, s, seedMeeting{ID: "not_frac", Title: "T", CreatedAt: "2026-10-13T13:00:00.100Z", StartedAt: "2026-10-13T13:00:00.100Z"})
	if err := granola.EnsureSchema(context.Background(), s.DB()); err != nil {
		t.Fatal(err)
	}
	var started string
	_ = s.DB().QueryRow(`SELECT started_at FROM meetings WHERE id='not_frac'`).Scan(&started)
	if started != "2026-10-13T13:00:00Z" {
		t.Fatalf("fractional Z value not canonicalized: %q", started)
	}
}

// TestIncrementalRefreshComparesInstants: the thin notes table keeps the
// API's offset text while meetings is UTC "Z". A note revised at 13:01 UTC,
// stored as "09:01:00-04:00", must count as newer than a meeting hydrated at
// 13:00:00Z; as strings it compared smaller and the revision was skipped.
func TestIncrementalRefreshComparesInstants(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	insertMeeting(t, s, seedMeeting{ID: "not_rev", Title: "T", CreatedAt: "2026-10-13T12:00:00Z", UpdatedAt: "2026-10-13T13:00:00Z"})
	if _, err := s.DB().Exec(`INSERT INTO notes(id, data, updated_at) VALUES ('not_rev', '{}', '2026-10-13T09:01:00-04:00')`); err != nil {
		t.Fatal(err)
	}
	ids, err := unhydratedRecentNoteIDs(context.Background(), s.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "not_rev" {
		t.Fatalf("a newer revision with an offset timestamp was skipped: %v", ids)
	}
	// The same instant, stored with milliseconds in the thin table and as
	// whole seconds in meetings (what a live account returns), is NOT newer:
	// treating it as newer re-fetched every hydrated note on every command.
	insertMeeting(t, s, seedMeeting{ID: "not_same", Title: "T", CreatedAt: "2026-10-13T12:00:00Z", UpdatedAt: "2026-10-13T18:32:58Z"})
	if _, err := s.DB().Exec(`INSERT INTO notes(id, data, updated_at) VALUES ('not_same', '{}', '2026-10-13T18:32:58.382Z')`); err != nil {
		t.Fatal(err)
	}
	ids, err = unhydratedRecentNoteIDs(context.Background(), s.DB(), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if id == "not_same" {
			t.Fatalf("an already-hydrated note is re-fetched because of sub-second precision: %v", ids)
		}
	}
}

// TestBackfillAsksNeverAskedMeetingsFirst: meetings whose get_meetings keeps
// failing are retried after a day, but they must not crowd out meetings that
// were never asked, nor each other: never-asked first, then oldest attempt.
func TestBackfillAsksNeverAskedMeetingsFirst(t *testing.T) {
	s := newIsolatedStore(t)
	defer s.Close()
	ctx := context.Background()
	insertMeeting(t, s, seedMeeting{ID: "not_failA", Title: "A", CreatedAt: "2026-10-10T09:00:00Z"})
	insertMeeting(t, s, seedMeeting{ID: "not_failB", Title: "B", CreatedAt: "2026-10-11T09:00:00Z"})
	insertMeeting(t, s, seedMeeting{ID: "not_never", Title: "C", CreatedAt: "2026-10-01T09:00:00Z"})
	old := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	older := time.Now().UTC().Add(-96 * time.Hour).Format(time.RFC3339)
	_ = recordHumanNotes(ctx, s.DB(), "not_failB", "", false, false, old)
	_ = recordHumanNotes(ctx, s.DB(), "not_failA", "", false, false, older)
	got, err := pendingHumanNoteTargets(ctx, s.DB(), 3)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, g := range got {
		order = append(order, g.ID)
	}
	if strings.Join(order, ",") != "not_never,not_failA,not_failB" {
		t.Fatalf("backfill order = %v, want never-asked first, then oldest attempt", order)
	}
}

// TestTokenEndpointHonorsTheCallerDeadline: a stalled token endpoint must not
// hold a command past its own deadline (refresh used to run on no context and
// wait the full 45s client timeout).
func TestTokenEndpointHonorsTheCallerDeadline(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := mcpForm(ctx, srv.URL, url.Values{"grant_type": {"refresh_token"}}); err == nil {
		t.Fatal("a stalled token endpoint returned no error")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("token call ignored the deadline: took %s", d)
	}
}
