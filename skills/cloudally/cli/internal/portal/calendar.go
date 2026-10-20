// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"cloudally-pp-cli/internal/cadate"
)

// calendarOperation is the service operation the restore wizard's calendar
// uses. One call returns the requested month and, in every response observed,
// the month before it as well.
const calendarOperation = "getSnapshotsCalendar"

// monthsPerCall is how many months a single calendar response covers: the
// requested month plus the preceding one.
const monthsPerCall = 2

// DefaultMaxLookbackMonths bounds how far back a resolution walks before
// giving up on a mailbox. Two years covers any end-of-year restore.
const DefaultMaxLookbackMonths = 24

// statusSucceeded is the status string on a usable backup. The succeed
// boolean carries the same meaning; both are checked.
const statusSucceeded = "SUCCEED"

// Entity identifies one mailbox (or site, or drive) within a backup task.
// AccountID is the entity name the API calls "account": for MS365 that is
// the mailbox's UPN. ExtID is the entity's external id.
type Entity struct {
	TaskID    string
	AccountID string
	ExtID     string
}

// key is the per-entity cache key component.
func (e Entity) key() string {
	return strings.ToLower(e.TaskID + "\x00" + e.AccountID + "\x00" + e.ExtID)
}

// Snapshot is one backup run as the calendar reports it. DateTime is the
// compact yyyyMMddHHmmss integer the whole API family uses; see cadate.
type Snapshot struct {
	DateTime   int64      `json:"dateTime"`
	Succeed    bool       `json:"succeed"`
	Status     string     `json:"status"`
	Source     string     `json:"source"`
	BatchID    string     `json:"batchId"`
	SubSources []Snapshot `json:"subsSourceResponseList"`
}

// Time decodes DateTime as a UTC time.
func (s Snapshot) Time() (time.Time, error) {
	return cadate.ParseCompact(s.DateTime)
}

// Succeeded reports whether this run is a usable restore point.
func (s Snapshot) Succeeded() bool {
	return s.Succeed || strings.EqualFold(strings.TrimSpace(s.Status), statusSucceeded)
}

// calendarRequest is the service-layer body. month is 1-based.
type calendarRequest struct {
	TaskID    string `json:"taskId"`
	AccountID string `json:"accountId"`
	ExtID     string `json:"extid"`
	Year      int    `json:"year"`
	Month     int    `json:"month"`
}

// SnapshotsCalendar reads one month of backup runs for one entity. month is
// 1-based. The response also carries the preceding month, so rows outside the
// requested month are normal; callers that want a single month filter by
// InMonth.
func (s *Session) SnapshotsCalendar(ctx context.Context, ent Entity, year int, month time.Month) ([]Snapshot, error) {
	if strings.TrimSpace(ent.TaskID) == "" {
		return nil, fmt.Errorf("portal: calendar needs a task id")
	}
	if strings.TrimSpace(ent.AccountID) == "" {
		return nil, fmt.Errorf("portal: calendar needs an entity account id")
	}
	if month < time.January || month > time.December {
		return nil, fmt.Errorf("portal: calendar month %d is out of range", int(month))
	}
	raw, err := s.CallRead(ctx, calendarOperation, calendarRequest{
		TaskID:    ent.TaskID,
		AccountID: ent.AccountID,
		ExtID:     ent.ExtID,
		Year:      year,
		Month:     int(month),
	})
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var snaps []Snapshot
	if err := json.Unmarshal(raw, &snaps); err != nil {
		return nil, fmt.Errorf("portal: decoding %s rows: %w", calendarOperation, err)
	}
	return snaps, nil
}

// InMonth reports whether a snapshot falls in the given year and month.
func InMonth(s Snapshot, year int, month time.Month) bool {
	t, err := s.Time()
	if err != nil {
		return false
	}
	return t.Year() == year && t.Month() == month
}

// monthKey identifies one entity-month in the resolver cache.
type monthKey struct {
	entity string
	year   int
	month  time.Month
}

// Resolver answers "newest successful backup at or before this date" for an
// entity, caching each entity-month it has read. It is safe for concurrent
// use across entities, which is how a whole task is resolved at once.
type Resolver struct {
	sess        *Session
	maxLookback int
	// mu guards months and loaded.
	mu sync.Mutex
	// months holds the snapshots found for each entity-month, including
	// months that arrived as a side effect of another month's request.
	months map[monthKey][]Snapshot
	// loaded marks entity-months known to be complete.
	loaded map[monthKey]bool
}

// NewResolver builds a Resolver over an open session. A maxLookback of zero
// or less uses DefaultMaxLookbackMonths.
func NewResolver(sess *Session, maxLookback int) *Resolver {
	if maxLookback <= 0 {
		maxLookback = DefaultMaxLookbackMonths
	}
	return &Resolver{
		sess:        sess,
		maxLookback: maxLookback,
		months:      map[monthKey][]Snapshot{},
		loaded:      map[monthKey]bool{},
	}
}

// Calls reports how many service requests the underlying session has made.
func (r *Resolver) Calls() int {
	if r.sess == nil {
		return 0
	}
	return r.sess.Calls()
}

// Month returns one entity-month's snapshots, newest first, reading the
// portal only when that month is not already cached.
func (r *Resolver) Month(ctx context.Context, ent Entity, year int, month time.Month) ([]Snapshot, error) {
	key := monthKey{entity: ent.key(), year: year, month: month}
	if cached, ok := r.cachedMonth(key); ok {
		return cached, nil
	}
	// The network call runs outside the lock so concurrent entities do not
	// serialise behind one another. Two callers racing on the same
	// entity-month would both fetch, which is harmless and does not happen
	// in practice: one worker owns each entity.
	snaps, err := r.sess.SnapshotsCalendar(ctx, ent, year, month)
	if err != nil {
		return nil, err
	}
	// Bucket every row by the month it actually falls in: one response
	// covers the requested month and the one before it.
	buckets := map[monthKey][]Snapshot{key: nil}
	for _, s := range snaps {
		t, terr := s.Time()
		if terr != nil {
			continue
		}
		k := monthKey{entity: ent.key(), year: t.Year(), month: t.Month()}
		buckets[k] = append(buckets[k], s)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, rows := range buckets {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].DateTime > rows[j].DateTime })
		r.months[k] = rows
		// The requested month is complete by definition. A neighbouring
		// month is only trusted when rows for it actually came back; an
		// empty neighbour may simply have been outside the response.
		if k == key || len(rows) > 0 {
			r.loaded[k] = true
		}
	}
	return r.months[key], nil
}

// cachedMonth returns a cached entity-month, if it is known to be complete.
func (r *Resolver) cachedMonth(key monthKey) ([]Snapshot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.loaded[key] {
		return nil, false
	}
	return r.months[key], true
}

// NewestSuccessfulAtOrBefore walks back month by month from cutoff and
// returns the newest successful backup at or before it. found is false when
// the entity has no successful backup within the lookback window.
func (r *Resolver) NewestSuccessfulAtOrBefore(ctx context.Context, ent Entity, cutoff time.Time) (snap Snapshot, found bool, err error) {
	cutoff = cutoff.UTC()
	year, month := cutoff.Year(), cutoff.Month()
	// Each call covers monthsPerCall months, so the walk needs at most
	// that fraction of the lookback window in requests; stepping one month
	// at a time and leaning on the cache keeps the arithmetic simple.
	for stepped := 0; stepped < r.maxLookback; stepped++ {
		snaps, merr := r.Month(ctx, ent, year, month)
		if merr != nil {
			return Snapshot{}, false, merr
		}
		// Rows are newest first, so the first usable one wins.
		for _, s := range snaps {
			if !s.Succeeded() {
				continue
			}
			t, terr := s.Time()
			if terr != nil || t.After(cutoff) {
				continue
			}
			return s, true, nil
		}
		year, month = previousMonth(year, month)
	}
	return Snapshot{}, false, nil
}

// previousMonth steps one month back.
func previousMonth(year int, month time.Month) (int, time.Month) {
	if month == time.January {
		return year - 1, time.December
	}
	return year, month - 1
}

// SuccessfulSubSources returns each sub-source of a run that succeeded,
// carrying its own timestamp.
//
// A run's own DateTime is the MAXIMUM of its sub-source timestamps, not a
// shared value: a 2025-12-31 run observed O365NOTES at 044157, O365MAIL at
// 044149 and O365CALENDAR at 044145. A download must be asked for with the
// sub-source's own timestamp. The parent value is silently accepted by the
// create call, so getting this wrong produces an archive nobody can tell is
// wrong without opening it.
//
// Rows are returned in a stable order: sub-source name, ascending.
func (s Snapshot) SuccessfulSubSources() []Snapshot {
	var out []Snapshot
	for _, sub := range s.SubSources {
		if !sub.Succeeded() {
			continue
		}
		if sub.DateTime <= 0 || strings.TrimSpace(sub.Source) == "" {
			continue
		}
		out = append(out, sub)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

// RunOnDate returns the successful run that falls on one UTC calendar day.
//
// This is deliberately not "newest at or before": a year-end archive must
// come from that date's own backup. A mailbox with no run that day has no
// year-end snapshot and is excluded, rather than archived from some other
// date. When a day somehow holds more than one successful run, the latest
// wins.
func (r *Resolver) RunOnDate(ctx context.Context, ent Entity, day time.Time) (Snapshot, bool, error) {
	day = day.UTC()
	snaps, err := r.Month(ctx, ent, day.Year(), day.Month())
	if err != nil {
		return Snapshot{}, false, err
	}
	// Month returns rows newest first, so the first match is the latest.
	for _, s := range snaps {
		if !s.Succeeded() {
			continue
		}
		t, terr := s.Time()
		if terr != nil {
			continue
		}
		if t.Year() == day.Year() && t.Month() == day.Month() && t.Day() == day.Day() {
			return s, true, nil
		}
	}
	return Snapshot{}, false, nil
}

// RunOnLocalDate returns the last successful run that falls on one calendar
// day in the account's own timezone.
//
// CloudAlly stores snapshot timestamps in UTC, but an operator and a customer
// both mean their local day. The nightly backup for a US_EST account runs
// around 23:30 local, which is 04:30 UTC the NEXT day, so the run CloudAlly
// stamps 2022-12-31 holds the 30 December end-of-day state and nothing from
// the 31st. Selecting by UTC date therefore delivers the wrong day's mail.
//
// offset is the account's offset from UTC (-5 for US_EST). The UTC window for
// local day D is [D 00:00 - offset, D 24:00 - offset), which can straddle two
// months, so both are read. When a local day holds more than one successful
// run the latest wins, which is the end-of-day state.
func (r *Resolver) RunOnLocalDate(ctx context.Context, ent Entity, day time.Time, offset time.Duration) (Snapshot, bool, error) {
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	start := day.Add(-offset)
	end := day.AddDate(0, 0, 1).Add(-offset)

	// Read every month the window touches.
	seen := map[monthKey]bool{}
	var candidates []Snapshot
	for _, at := range []time.Time{start, end.Add(-time.Nanosecond)} {
		key := monthKey{entity: ent.key(), year: at.Year(), month: at.Month()}
		if seen[key] {
			continue
		}
		seen[key] = true
		snaps, err := r.Month(ctx, ent, at.Year(), at.Month())
		if err != nil {
			return Snapshot{}, false, err
		}
		candidates = append(candidates, snaps...)
	}

	var best Snapshot
	found := false
	for _, s := range candidates {
		if !s.Succeeded() {
			continue
		}
		t, terr := s.Time()
		if terr != nil {
			continue
		}
		// [start, end)
		if t.Before(start) || !t.Before(end) {
			continue
		}
		if !found || s.DateTime > best.DateTime {
			best, found = s, true
		}
	}
	return best, found, nil
}
