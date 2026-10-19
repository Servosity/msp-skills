// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// calendarFake serves getSnapshotsCalendar from a fixed set of backup days,
// mirroring the real portal: a request for one month returns that month and
// the month before it.
type calendarFake struct {
	t *testing.T
	// days holds one compact timestamp per backup run.
	days []int64
	// requested records each (year, month) the client asked for.
	requested []string
	// failed marks runs that did not succeed.
	failed map[int64]bool
}

func (c *calendarFake) rowsFor(year, month int) []Snapshot {
	// One response covers the requested month and the preceding one.
	prevYear, prevMonth := year, month-1
	if prevMonth == 0 {
		prevYear, prevMonth = year-1, 12
	}
	var out []Snapshot
	for _, d := range c.days {
		y := int(d / 10000000000)
		m := int((d / 100000000) % 100)
		if (y == year && m == month) || (y == prevYear && m == prevMonth) {
			ok := !c.failed[d]
			status := statusSucceeded
			if !ok {
				status = "FAILED"
			}
			out = append(out, Snapshot{
				DateTime: d,
				Succeed:  ok,
				Status:   status,
				Source:   "MS365",
				BatchID:  fmt.Sprintf("batch-%d", d),
			})
		}
	}
	return out
}

func (c *calendarFake) session() (*Session, func()) {
	f := newFakePortal(c.t, "tok")
	f.handler = func(op string, body []byte, _ string) string {
		if op != calendarOperation {
			c.t.Fatalf("unexpected operation %q", op)
		}
		var req calendarRequest
		if err := json.Unmarshal(body, &req); err != nil {
			c.t.Fatalf("decoding request body %q: %v", body, err)
		}
		if req.TaskID == "" || req.AccountID == "" {
			c.t.Fatalf("request missing identifiers: %+v", req)
		}
		c.requested = append(c.requested, fmt.Sprintf("%04d-%02d", req.Year, req.Month))
		rows, err := json.Marshal(c.rowsFor(req.Year, req.Month))
		if err != nil {
			c.t.Fatalf("encoding rows: %v", err)
		}
		return fmt.Sprintf(`{"success":true,"data":%s}`, rows)
	}
	srv := f.server()
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		c.t.Fatalf("Open: %v", err)
	}
	return s, srv.Close
}

// dailyDays builds one backup per day for a month at 04:20:00 UTC.
func dailyDays(year, month, days int) []int64 {
	var out []int64
	for d := 1; d <= days; d++ {
		out = append(out, int64(year)*10000000000+int64(month)*100000000+int64(d)*1000000+42000)
	}
	return out
}

func testEntity() Entity {
	return Entity{
		TaskID:    "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		AccountID: "jane.doe@contoso.com",
		ExtID:     "ext-jane",
	}
}

func TestSnapshotsCalendarDecodesRows(t *testing.T) {
	f := newFakePortal(t, "tok")
	f.handler = func(string, []byte, string) string {
		return `{"success":true,"data":[` +
			`{"dateTime":20251231043522,"succeed":true,"status":"SUCCEED","source":"MS365","batchId":"b-1",` +
			`"subsSourceResponseList":[{"dateTime":20251231043504,"succeed":true,"status":"SUCCEED","source":"O365MAIL"}]}]}`
	}
	srv := f.server()
	defer srv.Close()
	s, err := Open(context.Background(), Config{BaseURL: srv.URL, RatePerSec: -1}, "tok")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	snaps, err := s.SnapshotsCalendar(context.Background(), testEntity(), 2025, time.December)
	if err != nil {
		t.Fatalf("SnapshotsCalendar: %v", err)
	}
	if len(snaps) != 1 || snaps[0].BatchID != "b-1" {
		t.Fatalf("snaps = %+v", snaps)
	}
	got, err := snaps[0].Time()
	if err != nil {
		t.Fatalf("Time: %v", err)
	}
	if want := time.Date(2025, 12, 31, 4, 35, 22, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("Time = %s, want %s", got, want)
	}
	if len(snaps[0].SubSources) != 1 || snaps[0].SubSources[0].Source != "O365MAIL" {
		t.Fatalf("sub-sources = %+v", snaps[0].SubSources)
	}
}

func TestSnapshotsCalendarValidatesInput(t *testing.T) {
	s := &Session{csrfHeader: "X-CSRF-TOKEN", csrfToken: "x"}
	ent := testEntity()
	if _, err := s.SnapshotsCalendar(context.Background(), Entity{AccountID: "a"}, 2025, time.December); err == nil {
		t.Fatal("want an error without a task id")
	}
	if _, err := s.SnapshotsCalendar(context.Background(), Entity{TaskID: "t"}, 2025, time.December); err == nil {
		t.Fatal("want an error without an entity id")
	}
	if _, err := s.SnapshotsCalendar(context.Background(), ent, 2025, time.Month(13)); err == nil {
		t.Fatal("want an error for month 13")
	}
}

func TestResolverPicksNewestSuccessfulAtOrBefore(t *testing.T) {
	fake := &calendarFake{t: t, days: dailyDays(2025, 12, 31), failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)

	// A cutoff at the last second of 2025-12-31 resolves to that day's run.
	cutoff := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
	snap, found, err := r.NewestSuccessfulAtOrBefore(context.Background(), testEntity(), cutoff)
	if err != nil || !found {
		t.Fatalf("resolve: found=%v err=%v", found, err)
	}
	if snap.DateTime != 20251231042000 {
		t.Fatalf("resolved %d", snap.DateTime)
	}

	// A mid-month cutoff takes the run on that day, not a later one.
	cutoff = time.Date(2025, 12, 15, 23, 59, 59, 0, time.UTC)
	snap, found, err = r.NewestSuccessfulAtOrBefore(context.Background(), testEntity(), cutoff)
	if err != nil || !found {
		t.Fatalf("resolve mid-month: found=%v err=%v", found, err)
	}
	if snap.DateTime != 20251215042000 {
		t.Fatalf("mid-month resolved %d", snap.DateTime)
	}

	// A cutoff before that day's 04:20 run falls back to the day before.
	cutoff = time.Date(2025, 12, 15, 1, 0, 0, 0, time.UTC)
	snap, found, err = r.NewestSuccessfulAtOrBefore(context.Background(), testEntity(), cutoff)
	if err != nil || !found {
		t.Fatalf("resolve pre-dawn: found=%v err=%v", found, err)
	}
	if snap.DateTime != 20251214042000 {
		t.Fatalf("pre-dawn resolved %d", snap.DateTime)
	}
}

func TestResolverSkipsFailedRuns(t *testing.T) {
	days := dailyDays(2025, 12, 31)
	// Fail the last two days of the year.
	failed := map[int64]bool{20251231042000: true, 20251230042000: true}
	fake := &calendarFake{t: t, days: days, failed: failed}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)

	cutoff := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)
	snap, found, err := r.NewestSuccessfulAtOrBefore(context.Background(), testEntity(), cutoff)
	if err != nil || !found {
		t.Fatalf("resolve: found=%v err=%v", found, err)
	}
	if snap.DateTime != 20251229042000 {
		t.Fatalf("resolved %d, want the newest successful run", snap.DateTime)
	}
}

func TestResolverCachesMonthsAcrossCalls(t *testing.T) {
	// November and December both have daily backups.
	days := append(dailyDays(2025, 11, 30), dailyDays(2025, 12, 31)...)
	fake := &calendarFake{t: t, days: days, failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)
	ent := testEntity()

	// Resolving inside December reads December once.
	if _, _, err := r.NewestSuccessfulAtOrBefore(context.Background(), ent,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)); err != nil {
		t.Fatalf("december: %v", err)
	}
	if got := r.Calls(); got != 1 {
		t.Fatalf("calls after december = %d, want 1", got)
	}

	// December's response also carried November in full, so resolving into
	// November must not make another request.
	snap, found, err := r.NewestSuccessfulAtOrBefore(context.Background(), ent,
		time.Date(2025, 11, 20, 23, 59, 59, 0, time.UTC))
	if err != nil || !found {
		t.Fatalf("november: found=%v err=%v", found, err)
	}
	if snap.DateTime != 20251120042000 {
		t.Fatalf("november resolved %d", snap.DateTime)
	}
	if got := r.Calls(); got != 1 {
		t.Fatalf("calls after november = %d, want still 1 (cached)", got)
	}
	if want := []string{"2025-12"}; len(fake.requested) != 1 || fake.requested[0] != want[0] {
		t.Fatalf("requested = %v, want %v", fake.requested, want)
	}

	// Re-resolving December is served entirely from the cache.
	if _, _, err := r.NewestSuccessfulAtOrBefore(context.Background(), ent,
		time.Date(2025, 12, 5, 23, 59, 59, 0, time.UTC)); err != nil {
		t.Fatalf("december again: %v", err)
	}
	if got := r.Calls(); got != 1 {
		t.Fatalf("calls after repeat = %d, want 1", got)
	}
}

func TestResolverCacheIsPerEntity(t *testing.T) {
	fake := &calendarFake{t: t, days: dailyDays(2025, 12, 31), failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)
	cutoff := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)

	a := testEntity()
	b := Entity{TaskID: a.TaskID, AccountID: "john.smith@contoso.com", ExtID: "ext-john"}
	if _, _, err := r.NewestSuccessfulAtOrBefore(context.Background(), a, cutoff); err != nil {
		t.Fatalf("entity a: %v", err)
	}
	if _, _, err := r.NewestSuccessfulAtOrBefore(context.Background(), b, cutoff); err != nil {
		t.Fatalf("entity b: %v", err)
	}
	if got := r.Calls(); got != 2 {
		t.Fatalf("calls = %d, want one per entity", got)
	}
}

func TestResolverReportsNoBackupWithinLookback(t *testing.T) {
	// The mailbox has backups only in 2025-12; a 2025-06 cutoff predates
	// every one of them.
	fake := &calendarFake{t: t, days: dailyDays(2025, 12, 31), failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 3)

	_, found, err := r.NewestSuccessfulAtOrBefore(context.Background(), testEntity(),
		time.Date(2025, 6, 30, 23, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if found {
		t.Fatal("want found=false for a cutoff before every backup")
	}
	// The walk is bounded by maxLookback, not unbounded.
	if got := len(fake.requested); got > 3 {
		t.Fatalf("requested %d months, want at most the 3-month lookback", got)
	}
}

func TestResolverWalksBackAcrossAYearBoundary(t *testing.T) {
	// Backups stop on 2025-12-31; the cutoff is in 2026-02.
	fake := &calendarFake{t: t, days: dailyDays(2025, 12, 31), failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)

	snap, found, err := r.NewestSuccessfulAtOrBefore(context.Background(), testEntity(),
		time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC))
	if err != nil || !found {
		t.Fatalf("resolve: found=%v err=%v", found, err)
	}
	if snap.DateTime != 20251231042000 {
		t.Fatalf("resolved %d", snap.DateTime)
	}
	if !strings.Contains(strings.Join(fake.requested, ","), "2026-02") {
		t.Fatalf("walk should start at the cutoff month: %v", fake.requested)
	}
}

func TestPreviousMonthWrapsJanuary(t *testing.T) {
	y, m := previousMonth(2026, time.January)
	if y != 2025 || m != time.December {
		t.Fatalf("previousMonth(2026, Jan) = %d-%s", y, m)
	}
	if y, m = previousMonth(2026, time.March); y != 2026 || m != time.February {
		t.Fatalf("previousMonth(2026, Mar) = %d-%s", y, m)
	}
}

func TestInMonthAndSucceeded(t *testing.T) {
	s := Snapshot{DateTime: 20251231043522, Status: statusSucceeded}
	if !InMonth(s, 2025, time.December) {
		t.Fatal("want December 2025")
	}
	if InMonth(s, 2025, time.November) {
		t.Fatal("must not match November")
	}
	// The status alone is enough even when the boolean is absent.
	if !s.Succeeded() {
		t.Fatal("SUCCEED status should count as succeeded")
	}
	if (Snapshot{DateTime: 20251231043522, Status: "FAILED"}).Succeeded() {
		t.Fatal("FAILED must not count as succeeded")
	}
	// An undecodable timestamp is never in any month.
	if InMonth(Snapshot{DateTime: 7}, 2025, time.December) {
		t.Fatal("a malformed timestamp must not match a month")
	}
}

// TestSuccessfulSubSourcesUsesEachOwnTimestamp guards the trap that the
// parent run's DateTime is the max of its sub-sources, so a download asked
// for with the parent value is asking for the wrong snapshot.
func TestSuccessfulSubSourcesUsesEachOwnTimestamp(t *testing.T) {
	run := Snapshot{
		DateTime: 20251231044157, Succeed: true, Status: statusSucceeded, Source: "MS365",
		SubSources: []Snapshot{
			{DateTime: 20251231044157, Succeed: true, Status: statusSucceeded, Source: "O365NOTES"},
			{DateTime: 20251231044149, Succeed: true, Status: statusSucceeded, Source: "O365MAIL"},
			{DateTime: 20251231044145, Succeed: true, Status: statusSucceeded, Source: "O365CALENDAR"},
			{DateTime: 20251231044148, Succeed: false, Status: "FAILED", Source: "O365TASKS"},
			{DateTime: 0, Succeed: true, Status: statusSucceeded, Source: "O365CONTACTS"},
		},
	}
	subs := run.SuccessfulSubSources()
	if len(subs) != 3 {
		t.Fatalf("want 3 usable sub-sources, got %d: %+v", len(subs), subs)
	}
	// Stable order by name.
	if subs[0].Source != "O365CALENDAR" || subs[1].Source != "O365MAIL" || subs[2].Source != "O365NOTES" {
		t.Fatalf("order = %s/%s/%s", subs[0].Source, subs[1].Source, subs[2].Source)
	}
	// Each keeps its own timestamp, and mail is NOT the parent's value.
	byName := map[string]int64{}
	for _, s := range subs {
		byName[s.Source] = s.DateTime
	}
	if byName["O365MAIL"] != 20251231044149 {
		t.Fatalf("mail timestamp = %d, want its own 20251231044149", byName["O365MAIL"])
	}
	if byName["O365MAIL"] == run.DateTime {
		t.Fatal("mail must not inherit the parent run's timestamp")
	}
	if byName["O365CALENDAR"] != 20251231044145 {
		t.Fatalf("calendar timestamp = %d", byName["O365CALENDAR"])
	}
	// A failed sub-source is dropped, as is one with no timestamp.
	if _, ok := byName["O365TASKS"]; ok {
		t.Fatal("a failed sub-source must not be archived")
	}
	if _, ok := byName["O365CONTACTS"]; ok {
		t.Fatal("a sub-source with no timestamp must not be archived")
	}
}

func TestRunOnDateRequiresThatExactDay(t *testing.T) {
	// Backups on 2022-12-29 and 2022-12-31 only.
	days := []int64{20221229042000, 20221231042000}
	fake := &calendarFake{t: t, days: days, failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)
	ent := testEntity()

	snap, found, err := r.RunOnDate(context.Background(), ent, time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil || !found {
		t.Fatalf("31st: found=%v err=%v", found, err)
	}
	if snap.DateTime != 20221231042000 {
		t.Fatalf("resolved %d", snap.DateTime)
	}
	// The 30th has no run. An at-or-before search would have returned the
	// 29th; RunOnDate must report nothing.
	if _, found, err = r.RunOnDate(context.Background(), ent, time.Date(2022, 12, 30, 0, 0, 0, 0, time.UTC)); err != nil || found {
		t.Fatalf("30th should have no run, got found=%v err=%v", found, err)
	}
	// A year with no data at all reports nothing rather than reaching back.
	if _, found, err = r.RunOnDate(context.Background(), ent, time.Date(2021, 12, 31, 0, 0, 0, 0, time.UTC)); err != nil || found {
		t.Fatalf("2021 should have no run, got found=%v err=%v", found, err)
	}
}

func TestRunOnDateSkipsFailedRuns(t *testing.T) {
	days := []int64{20221231042000}
	fake := &calendarFake{t: t, days: days, failed: map[int64]bool{20221231042000: true}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)
	if _, found, err := r.RunOnDate(context.Background(), testEntity(),
		time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC)); err != nil || found {
		t.Fatalf("a failed run must not count: found=%v err=%v", found, err)
	}
}

// TestRunOnLocalDatePicksTheCustomersDay is the regression test for the
// mistake that cost a 22 GB download of the wrong day: CloudAlly stamps
// snapshots in UTC, and a US_EST account's 23:30 local backup is stamped
// 04:30 the NEXT day. Selecting on the UTC date delivers the day before.
func TestRunOnLocalDatePicksTheCustomersDay(t *testing.T) {
	const est = -5 * time.Hour
	// Nightly runs at 04:33 UTC, i.e. 23:33 the previous day in EST.
	days := []int64{
		20221230043326, // EST 2022-12-29 23:33
		20221231043347, // EST 2022-12-30 23:33
		20230101042600, // EST 2022-12-31 23:26  <- the real 31 December
		20230102042328, // EST 2023-01-01 23:23
	}
	fake := &calendarFake{t: t, days: days, failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)
	ent := testEntity()
	dec31 := time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC)

	// The UTC-matching selector returns the wrong day: this is the bug.
	utcRun, found, err := r.RunOnDate(context.Background(), ent, dec31)
	if err != nil || !found {
		t.Fatalf("utc selector: found=%v err=%v", found, err)
	}
	if utcRun.DateTime != 20221231043347 {
		t.Fatalf("utc selector picked %d", utcRun.DateTime)
	}

	// The local selector returns the run that is 31 December in EST.
	localRun, found, err := r.RunOnLocalDate(context.Background(), ent, dec31, est)
	if err != nil || !found {
		t.Fatalf("local selector: found=%v err=%v", found, err)
	}
	if localRun.DateTime != 20230101042600 {
		t.Fatalf("local selector picked %d, want 20230101042600 (EST 2022-12-31 23:26)", localRun.DateTime)
	}
	if localRun.DateTime == utcRun.DateTime {
		t.Fatal("the two selectors must disagree here, or the test proves nothing")
	}
}

// TestRunOnLocalDateHandlesRunsEitherSideOfMidnightUTC guards the detail
// that this is not a blanket one-day shift: a run after 05:00 UTC is already
// the same local day in EST.
func TestRunOnLocalDateHandlesRunsEitherSideOfMidnightUTC(t *testing.T) {
	const est = -5 * time.Hour
	days := []int64{
		20221229050554, // EST 2022-12-29 00:05  <- same local day
		20221230043326, // EST 2022-12-29 23:33  <- same local day, later
	}
	fake := &calendarFake{t: t, days: days, failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)

	// Both runs fall on 29 December in EST; end of day wins.
	got, found, err := r.RunOnLocalDate(context.Background(), testEntity(),
		time.Date(2022, 12, 29, 0, 0, 0, 0, time.UTC), est)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.DateTime != 20221230043326 {
		t.Fatalf("picked %d, want the later run on that local day", got.DateTime)
	}
}

func TestRunOnLocalDateWithZeroOffsetMatchesUTC(t *testing.T) {
	days := []int64{20221231043347, 20230101042600}
	fake := &calendarFake{t: t, days: days, failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)
	got, found, err := r.RunOnLocalDate(context.Background(), testEntity(),
		time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC), 0)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.DateTime != 20221231043347 {
		t.Fatalf("with no offset this must match the UTC date, got %d", got.DateTime)
	}
}

func TestRunOnLocalDateReportsNothingWhenThatDayHasNoRun(t *testing.T) {
	const est = -5 * time.Hour
	// Only a run that is 29 December in EST.
	fake := &calendarFake{t: t, days: []int64{20221230043326}, failed: map[int64]bool{}}
	sess, done := fake.session()
	defer done()
	r := NewResolver(sess, 0)
	if _, found, err := r.RunOnLocalDate(context.Background(), testEntity(),
		time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC), est); err != nil || found {
		t.Fatalf("31 December should have no run: found=%v err=%v", found, err)
	}
}
