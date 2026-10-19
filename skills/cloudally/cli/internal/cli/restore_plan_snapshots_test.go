// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/portal"
)

// stubResolver answers from a fixed table instead of a portal session.
// Entities are resolved concurrently, so its counters are guarded.
type stubResolver struct {
	mu sync.Mutex
	// byEntity maps a lowercase entity name to its backup timestamps.
	byEntity map[string][]int64
	// fail maps a lowercase entity name to an error to return.
	fail map[string]error
	// lookups counts resolution calls, per entity.
	lookups map[string]int
}

func (s *stubResolver) NewestSuccessfulAtOrBefore(_ context.Context, ent portal.Entity, cutoff time.Time) (portal.Snapshot, bool, error) {
	key := strings.ToLower(ent.AccountID)
	s.mu.Lock()
	if s.lookups == nil {
		s.lookups = map[string]int{}
	}
	s.lookups[key]++
	failure, failed := s.fail[key]
	s.mu.Unlock()
	if failed {
		return portal.Snapshot{}, false, failure
	}
	var best int64
	for _, raw := range s.byEntity[key] {
		t, err := portal.Snapshot{DateTime: raw}.Time()
		if err != nil || t.After(cutoff) {
			continue
		}
		if raw > best {
			best = raw
		}
	}
	if best == 0 {
		return portal.Snapshot{}, false, nil
	}
	return portal.Snapshot{
		DateTime: best,
		Succeed:  true,
		Status:   "SUCCEED",
		Source:   "MS365",
		BatchID:  fmt.Sprintf("batch-%d", best),
	}, true, nil
}

// snapshotTask is a three-entity MS365 task covering the entity states a
// plan must keep visible.
func snapshotTask() *taskWithEntities {
	return &taskWithEntities{
		ID:     "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		Source: "MS365",
		Entities: []taskEntity{
			{Entity: "jane.doe@contoso.com", ExtID: "ext-jane", Status: "ACTIVE",
				LastBackupDates: map[string]flexInt{"MAIL": 20261015042000}},
			{Entity: "archived.user@contoso.com", ExtID: "ext-arch", Status: "ARCHIVED",
				LastBackupDates: map[string]flexInt{"MAIL": 20251201042000}},
			{Entity: "onprem.user@contoso.com", ExtID: "ext-onprem", Status: "ON_PREMISE"},
		},
	}
}

func TestBuildPlanWithResolverPicksSnapshotAtOrBeforeDate(t *testing.T) {
	task := snapshotTask()
	res := &stubResolver{byEntity: map[string][]int64{
		"jane.doe@contoso.com": {
			20251230043000, 20251231043522, 20260102041500, 20261015042000,
		},
		"archived.user@contoso.com": {20251201042000},
	}}
	dates, err := parseRequestedDates([]string{"2025-12-31"})
	if err != nil {
		t.Fatalf("parseRequestedDates: %v", err)
	}
	rows, warnings := buildPlanWithResolver(context.Background(), task, dates, nil, "", res)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(rows) != 3 {
		t.Fatalf("want one row per entity, got %d", len(rows))
	}

	jane := findPlanRow(rows, "jane.doe@contoso.com", "2025-12-31")
	if jane == nil {
		t.Fatal("no row for jane")
	}
	// The 2026-01-02 and 2026-10-15 runs are after the cutoff and the
	// 12-30 run is older, so 12-31 wins.
	if jane.ResolvedRaw != 20251231043522 {
		t.Fatalf("jane resolved_raw = %d, want 20251231043522", jane.ResolvedRaw)
	}
	if jane.Resolved != "2025-12-31T04:35:22Z" {
		t.Fatalf("jane resolved = %q", jane.Resolved)
	}
	if jane.DateSource != dateSourceSnapshots {
		t.Fatalf("jane date_source = %q, want %q", jane.DateSource, dateSourceSnapshots)
	}
	if jane.Status != planOK || jane.EntityStatus != "ACTIVE" || jane.BatchID == "" {
		t.Fatalf("jane row = %+v", *jane)
	}

	// An ARCHIVED entity stays in the plan, with its own status, and is
	// still resolvable.
	arch := findPlanRow(rows, "archived.user@contoso.com", "2025-12-31")
	if arch == nil || arch.EntityStatus != "ARCHIVED" || arch.Status != planOK {
		t.Fatalf("archived row = %+v", arch)
	}
	if arch.ResolvedRaw != 20251201042000 {
		t.Fatalf("archived resolved_raw = %d", arch.ResolvedRaw)
	}

	// An ON_PREMISE entity with no backups is visible as NO_BACKUP and
	// carries no date at all.
	onprem := findPlanRow(rows, "onprem.user@contoso.com", "2025-12-31")
	if onprem == nil || onprem.EntityStatus != "ON_PREMISE" {
		t.Fatalf("on-premise row = %+v", onprem)
	}
	if onprem.Status != planNoBackup || onprem.Resolved != "" || onprem.ResolvedRaw != 0 {
		t.Fatalf("on-premise row must carry no date: %+v", *onprem)
	}
	if onprem.Detail == "" {
		t.Fatal("a NO_BACKUP row should explain itself")
	}
}

func TestBuildPlanWithResolverNoBackupIsNotSubmitted(t *testing.T) {
	task := snapshotTask()
	// Nobody has a backup at or before mid-2025.
	res := &stubResolver{byEntity: map[string][]int64{
		"jane.doe@contoso.com": {20251231043522},
	}}
	dates, _ := parseRequestedDates([]string{"2025-06-30"})
	rows, _ := buildPlanWithResolver(context.Background(), task, dates, nil, "", res)
	for _, r := range rows {
		if r.Status != planNoBackup {
			t.Fatalf("row %s should be NO_BACKUP: %+v", r.Entity, r)
		}
	}
	// filterPlanRows is what submit uses to decide what to send.
	if sendable := filterPlanRows(rows, nil); len(sendable) != 0 {
		t.Fatalf("NO_BACKUP rows must not be submittable, got %+v", sendable)
	}
}

func TestBuildPlanWithResolverRecordsPerEntityFailureWithoutSubstituting(t *testing.T) {
	task := snapshotTask()
	boom := errors.New("portal: getSnapshotsCalendar failed: upstream timeout")
	res := &stubResolver{
		byEntity: map[string][]int64{"archived.user@contoso.com": {20251201042000}},
		fail:     map[string]error{"jane.doe@contoso.com": boom},
	}
	dates, _ := parseRequestedDates([]string{"2025-12-31"})
	rows, warnings := buildPlanWithResolver(context.Background(), task, dates, nil, "", res)
	if len(warnings) != 1 || !errors.Is(warnings[0], boom) {
		t.Fatalf("warnings = %v", warnings)
	}
	jane := findPlanRow(rows, "jane.doe@contoso.com", "2025-12-31")
	if jane.Status != planNoBackup {
		t.Fatalf("a failed lookup must not resolve: %+v", *jane)
	}
	// Jane's task record has a 2026-10-15 latest backup. A failure must
	// not quietly fall through to it.
	if jane.ResolvedRaw != 0 || jane.Resolved != "" || jane.DateSource != "" {
		t.Fatalf("failed lookup leaked a date: %+v", *jane)
	}
	if !strings.Contains(jane.Detail, "could not be read") {
		t.Fatalf("detail = %q", jane.Detail)
	}
	// The other entities still resolve.
	arch := findPlanRow(rows, "archived.user@contoso.com", "2025-12-31")
	if arch.Status != planOK {
		t.Fatalf("one entity's failure must not stop the others: %+v", *arch)
	}
}

func TestBuildPlanWithoutResolverLabelsItsSource(t *testing.T) {
	task := snapshotTask()
	// 2026-10-15 is on or after jane's only known backup, so the latest
	// backup date satisfies it.
	dates, _ := parseRequestedDates([]string{"2026-10-16"})
	rows, warnings := buildPlanWithResolver(context.Background(), task, dates, nil, "", nil)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	jane := findPlanRow(rows, "jane.doe@contoso.com", "2026-10-16")
	if jane.Status != planOK || jane.DateSource != dateSourceLastBackup {
		t.Fatalf("jane row = %+v", *jane)
	}
	// A past date cannot be served from the latest-backup dates, and the
	// row says so instead of resolving to the newer backup.
	dates, _ = parseRequestedDates([]string{"2025-12-31"})
	rows, _ = buildPlanWithResolver(context.Background(), task, dates, nil, "", nil)
	jane = findPlanRow(rows, "jane.doe@contoso.com", "2025-12-31")
	if jane.Status != planNoBackup || jane.ResolvedRaw != 0 {
		t.Fatalf("past date must not resolve from latest dates: %+v", *jane)
	}
	if !strings.Contains(jane.Detail, "--resolve snapshots") {
		t.Fatalf("detail should point at the calendar: %q", jane.Detail)
	}
}

func TestBuildPlanWithResolverHonoursEntityFilter(t *testing.T) {
	task := snapshotTask()
	res := &stubResolver{byEntity: map[string][]int64{"jane.doe@contoso.com": {20251231043522}}}
	dates, _ := parseRequestedDates([]string{"2025-12-31"})
	rows, _ := buildPlanWithResolver(context.Background(), task, dates,
		map[string]bool{"jane.doe@contoso.com": true}, "", res)
	if len(rows) != 1 || rows[0].Entity != "jane.doe@contoso.com" {
		t.Fatalf("rows = %+v", rows)
	}
	// Only the kept entity is ever looked up, so a filtered run does not
	// spend portal calls on mailboxes it will not restore.
	if len(res.lookups) != 1 {
		t.Fatalf("lookups = %v", res.lookups)
	}
}

func TestPlanTableShowsBothDateFormsAndProvenance(t *testing.T) {
	rows := []planRow{{
		Entity: "jane.doe@contoso.com", EntityStatus: "ACTIVE", Requested: "2025-12-31",
		Resolved: "2025-12-31T04:35:22Z", ResolvedRaw: 20251231043522,
		DateSource: dateSourceSnapshots, Status: planOK,
	}}
	cells := planRowsAsMaps(rows)
	if len(cells) != 1 || len(cells[0]) != len(planTableHeaders) {
		t.Fatalf("cells = %v (headers %v)", cells, planTableHeaders)
	}
	joined := strings.Join(cells[0], "|")
	for _, want := range []string{
		"jane.doe@contoso.com", "ACTIVE", "2025-12-31",
		"2025-12-31T04:35:22Z", "20251231043522", dateSourceSnapshots, planOK,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("row %q is missing %q", joined, want)
		}
	}
	// A NO_BACKUP row prints an empty raw date, never a zero.
	empty := planRowsAsMaps([]planRow{{Entity: "x", Status: planNoBackup}})
	if strings.Contains(strings.Join(empty[0], "|"), "0") {
		t.Fatalf("unresolved row rendered a zero date: %v", empty[0])
	}
}

func TestValidateResolveMode(t *testing.T) {
	partner := restoreScope{account: "acct-1"}
	accountScoped := restoreScope{}

	for _, tc := range []struct {
		mode  string
		scope restoreScope
		want  string
	}{
		{"", partner, resolveAuto},
		{"auto", accountScoped, resolveAuto},
		{"SNAPSHOTS", partner, resolveSnapshots},
		{" last-backup ", accountScoped, resolveLastBackup},
	} {
		got, err := validateResolveMode(tc.mode, tc.scope)
		if err != nil {
			t.Fatalf("validateResolveMode(%q): %v", tc.mode, err)
		}
		if got != tc.want {
			t.Fatalf("validateResolveMode(%q) = %q, want %q", tc.mode, got, tc.want)
		}
	}
	if _, err := validateResolveMode("portal", partner); err == nil {
		t.Fatal("an unknown mode should be a usage error")
	}
	// Asking for the calendar without a named customer cannot work.
	if _, err := validateResolveMode(resolveSnapshots, accountScoped); err == nil {
		t.Fatal("--resolve snapshots without --account should be a usage error")
	}
}

func TestWantsSnapshotResolution(t *testing.T) {
	partner := restoreScope{account: "acct-1"}
	accountScoped := restoreScope{}
	if !wantsSnapshotResolution(resolveAuto, partner) {
		t.Fatal("auto with a named account should read the calendar")
	}
	if wantsSnapshotResolution(resolveAuto, accountScoped) {
		t.Fatal("auto without an account cannot read the calendar")
	}
	if wantsSnapshotResolution(resolveLastBackup, partner) {
		t.Fatal("last-backup must never read the calendar")
	}
	if !wantsSnapshotResolution(resolveSnapshots, partner) {
		t.Fatal("snapshots must read the calendar")
	}
}

func TestPartnerAccessTokenPathPerScope(t *testing.T) {
	got, err := restoreScope{account: "acct-1"}.partnerAccessTokenPath()
	if err != nil {
		t.Fatalf("distributor: %v", err)
	}
	if got != "/v2/distributors/accounts/acct-1/partner-access-token" {
		t.Fatalf("distributor path = %q", got)
	}
	got, err = restoreScope{account: "acct-1", reseller: true}.partnerAccessTokenPath()
	if err != nil {
		t.Fatalf("reseller: %v", err)
	}
	if got != "/v2/resellers/accounts/acct-1/partner-access-token" {
		t.Fatalf("reseller path = %q", got)
	}
	if _, err := (restoreScope{}).partnerAccessTokenPath(); err == nil {
		t.Fatal("an account-scoped login has no handoff endpoint")
	}
}

func TestParseMonthFlag(t *testing.T) {
	for _, in := range []string{"2025-12", "202512"} {
		y, m, err := parseMonthFlag(in)
		if err != nil {
			t.Fatalf("parseMonthFlag(%q): %v", in, err)
		}
		if y != 2025 || m != time.December {
			t.Fatalf("parseMonthFlag(%q) = %d-%s", in, y, m)
		}
	}
	for _, bad := range []string{"", "2025", "december", "2025-13"} {
		if _, _, err := parseMonthFlag(bad); err == nil {
			t.Fatalf("parseMonthFlag(%q) should fail", bad)
		}
	}
}

func TestSnapshotRowsFromTrimsToTheMonth(t *testing.T) {
	// One response carries December and November.
	snaps := []portal.Snapshot{
		{DateTime: 20251231043522, Succeed: true, Status: "SUCCEED", Source: "MS365", BatchID: "b-dec"},
		{DateTime: 20251130041930, Succeed: true, Status: "SUCCEED", Source: "MS365", BatchID: "b-nov"},
		{DateTime: 20251201052357, Succeed: false, Status: "FAILED", Source: "MS365"},
	}
	rows := snapshotRowsFrom("jane@contoso.com", snaps, 2025, time.December, false)
	if len(rows) != 2 {
		t.Fatalf("want only December rows, got %+v", rows)
	}
	// Newest first.
	if rows[0].DateRaw != 20251231043522 || rows[1].DateRaw != 20251201052357 {
		t.Fatalf("order = %+v", rows)
	}
	if rows[0].Date != "2025-12-31T04:35:22Z" {
		t.Fatalf("date = %q", rows[0].Date)
	}
	if countSucceededSnapshots(rows) != 1 {
		t.Fatalf("succeeded count = %d", countSucceededSnapshots(rows))
	}
	// --all keeps the neighbouring month too.
	if all := snapshotRowsFrom("jane@contoso.com", snaps, 2025, time.December, true); len(all) != 3 {
		t.Fatalf("--all should keep every row, got %d", len(all))
	}
}

// runSnapshotCLI drives the real root command against a stub API, counting
// how many requests each side received.
func runSnapshotCLI(t *testing.T, apiURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	testenv.Isolate(t)
	t.Setenv("CLOUDALLY_BASE_URL", apiURL)
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

// countingServer answers nothing useful but records that it was reached.
func countingServer(t *testing.T, hits *int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*hits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestRestorePlanDryRunMakesNoPortalCall covers the dry-run path: a plan
// under --dry-run must not reach the API or the portal at all, even when it
// was asked for snapshot resolution.
func TestRestorePlanDryRunMakesNoPortalCall(t *testing.T) {
	var apiHits, portalHits int
	api := countingServer(t, &apiHits)
	portalSrv := countingServer(t, &portalHits)

	out, errOut, err := runSnapshotCLI(t, api.URL,
		"restore", "plan", "--account", "acct-1", "--task", "task-1",
		"--date", "2025-12-31", "--resolve", "snapshots",
		"--portal-url", portalSrv.URL, "--dry-run", "--json")
	if err != nil {
		t.Fatalf("restore plan --dry-run: %v\nstderr: %s", err, errOut)
	}
	if apiHits != 0 || portalHits != 0 {
		t.Fatalf("dry-run made requests: api=%d portal=%d", apiHits, portalHits)
	}
	var env map[string]any
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("dry-run output is not JSON: %v (%s)", jerr, out)
	}
	if env["dry_run"] != true {
		t.Fatalf("dry-run envelope = %v", env)
	}
}

// TestBackupSnapshotsDryRunMakesNoPortalCall is the same guarantee for the
// new read-only command.
func TestBackupSnapshotsDryRunMakesNoPortalCall(t *testing.T) {
	var portalHits int
	portalSrv := countingServer(t, &portalHits)

	out, errOut, err := runSnapshotCLI(t, portalSrv.URL,
		"backup", "snapshots", "--account", "acct-1", "--task", "task-1",
		"--entity", "jane.doe@contoso.com", "--month", "2025-12",
		"--portal-url", portalSrv.URL, "--dry-run", "--json")
	if err != nil {
		t.Fatalf("backup snapshots --dry-run: %v\nstderr: %s", err, errOut)
	}
	if portalHits != 0 {
		t.Fatalf("dry-run reached the portal %d time(s)", portalHits)
	}
	if !strings.Contains(out, "dry_run") {
		t.Fatalf("output = %s", out)
	}
}

// TestBackupSnapshotsRequiresAccount proves the command refuses rather than
// guessing which customer to read.
func TestBackupSnapshotsRequiresAccount(t *testing.T) {
	_, _, err := runSnapshotCLI(t, "http://127.0.0.1:9",
		"backup", "snapshots", "--task", "task-1",
		"--entity", "jane.doe@contoso.com", "--month", "2025-12")
	if err == nil {
		t.Fatal("want a usage error without --account")
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

// TestRestorePlanSnapshotsWithoutAccountIsUsageError keeps the explicit mode
// honest: it cannot silently become a latest-backup run.
func TestRestorePlanSnapshotsWithoutAccountIsUsageError(t *testing.T) {
	_, _, err := runSnapshotCLI(t, "http://127.0.0.1:9",
		"restore", "plan", "--task", "task-1", "--date", "2025-12-31",
		"--resolve", "snapshots")
	if err == nil {
		t.Fatal("want a usage error for --resolve snapshots without --account")
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
}

// TestSnapshotResolutionRefusesEntityWithoutExtID covers the silent-empty
// trap: the portal answers a calendar request that omits the entity's
// external id with an empty list rather than an error, so an entity the
// task carries no id for must never be reported as having no backup.
func TestSnapshotResolutionRefusesEntityWithoutExtID(t *testing.T) {
	task := &taskWithEntities{
		ID:     "task-1",
		Source: "MS365",
		Entities: []taskEntity{
			{Entity: "no.extid@contoso.com", ExtID: "", Status: "ARCHIVED"},
			{Entity: "has.extid@contoso.com", ExtID: "ext-ok", Status: "ACTIVE"},
		},
	}
	res := &stubResolver{byEntity: map[string][]int64{
		"no.extid@contoso.com":  {20251231043522},
		"has.extid@contoso.com": {20251231043522},
	}}
	dates, _ := parseRequestedDates([]string{"2025-12-31"})
	rows, warnings := buildPlanWithResolver(context.Background(), task, dates, nil, "", res)

	bad := findPlanRow(rows, "no.extid@contoso.com", "2025-12-31")
	if bad == nil {
		t.Fatal("the entity should still appear in the plan")
	}
	if bad.Status != planNoBackup || bad.ResolvedRaw != 0 {
		t.Fatalf("row = %+v", *bad)
	}
	// It must say the calendar was unreadable, not that no backup exists.
	if !strings.Contains(bad.Detail, "cannot be read") {
		t.Fatalf("detail = %q", bad.Detail)
	}
	if strings.Contains(bad.Detail, "no successful backup at or before") {
		t.Fatalf("an unreadable calendar must not read as a confident NO_BACKUP: %q", bad.Detail)
	}
	// It is surfaced as a warning rather than passing silently.
	var sawWarning bool
	for _, w := range warnings {
		if errors.Is(w, errNoEntityExtID) {
			sawWarning = true
		}
	}
	if !sawWarning {
		t.Fatalf("warnings = %v", warnings)
	}
	// The entity was never looked up, since the lookup could not be valid.
	if res.lookups["no.extid@contoso.com"] != 0 {
		t.Fatalf("lookups = %v", res.lookups)
	}
	// The healthy entity still resolves.
	good := findPlanRow(rows, "has.extid@contoso.com", "2025-12-31")
	if good.Status != planOK {
		t.Fatalf("good row = %+v", *good)
	}
}

func TestFindTaskEntityIsCaseInsensitive(t *testing.T) {
	task := snapshotTask()
	ent, ok := findTaskEntity(task, "  JANE.DOE@CONTOSO.COM ")
	if !ok || ent.ExtID != "ext-jane" {
		t.Fatalf("findTaskEntity = %+v, %v", ent, ok)
	}
	if _, ok := findTaskEntity(task, "ghost@contoso.com"); ok {
		t.Fatal("a missing entity should report not found")
	}
}

// TestBuildPlanRowsStayInEntityOrder guards the concurrent resolution: rows
// must come back sorted by entity regardless of which goroutine finished
// first, because the plan is read and diffed by people.
func TestBuildPlanRowsStayInEntityOrder(t *testing.T) {
	var entities []taskEntity
	byEntity := map[string][]int64{}
	for i := 0; i < 40; i++ {
		name := fmt.Sprintf("user%02d@contoso.com", i)
		entities = append(entities, taskEntity{
			Entity: name, ExtID: flexString("ext-" + name), Status: "ACTIVE",
		})
		byEntity[name] = []int64{20251231040000 + int64(i)}
	}
	task := &taskWithEntities{ID: "task-1", Source: "MS365", Entities: entities}
	res := &stubResolver{byEntity: byEntity}
	dates, _ := parseRequestedDates([]string{"2025-12-31", "2025-12-30"})

	rows, warnings := buildPlanWithResolver(context.Background(), task, dates, nil, "", res)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(rows) != 80 {
		t.Fatalf("want 40 entities x 2 dates, got %d", len(rows))
	}
	for i := 0; i < len(rows); i += 2 {
		want := fmt.Sprintf("user%02d@contoso.com", i/2)
		if rows[i].Entity != want || rows[i+1].Entity != want {
			t.Fatalf("row %d/%d = %s/%s, want %s", i, i+1, rows[i].Entity, rows[i+1].Entity, want)
		}
		// Dates keep their requested order within an entity.
		if rows[i].Requested != "2025-12-31" || rows[i+1].Requested != "2025-12-30" {
			t.Fatalf("dates out of order: %s then %s", rows[i].Requested, rows[i+1].Requested)
		}
	}
}
