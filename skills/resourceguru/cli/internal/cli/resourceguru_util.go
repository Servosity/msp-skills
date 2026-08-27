// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Shared engine for the utilization-intelligence commands (utilization,
// overbooked, bench, capacity). It reads synced resources and bookings from the
// local store and crosses each booking's per-day duration breakdown against
// each resource's daily capacity — the join Resource Guru's API exposes the
// pieces for but never aggregates.
//
// Hand-authored; not regenerated.

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"resourceguru-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

const utilDateFmt = "2006-01-02"

// resourceCapacity is one bookable resource and its daily capacity in minutes.
// When the resource declares available_periods, capacity is computed per weekday
// (so non-working days correctly contribute zero capacity); otherwise it falls
// back to the flat minutes_per_day for every day.
type resourceCapacity struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	MinutesPerDay  int    `json:"minutes_per_day"`
	Archived       bool   `json:"archived"`
	weekdayMinutes [7]int // index by time.Weekday (Sunday=0); populated from active available_periods
	hasPeriods     bool
}

// capacityFor returns the resource's available minutes on the given date.
func (rc resourceCapacity) capacityFor(d time.Time) int {
	if rc.hasPeriods {
		return rc.weekdayMinutes[int(d.Weekday())]
	}
	return rc.MinutesPerDay
}

// loadResourceCapacities reads synced resources into id->capacity, preserving
// store order for stable output.
func loadResourceCapacities(s *store.Store) (map[string]resourceCapacity, []string, error) {
	rows, err := s.List("resources", 0)
	if err != nil {
		return nil, nil, err
	}
	caps := make(map[string]resourceCapacity, len(rows))
	order := make([]string, 0, len(rows))
	for _, raw := range rows {
		var r struct {
			ID               json.Number `json:"id"`
			Name             string      `json:"name"`
			MinutesPerDay    int         `json:"minutes_per_day"`
			Archived         bool        `json:"archived"`
			AvailablePeriods []struct {
				StartTime  *int    `json:"start_time"`
				EndTime    *int    `json:"end_time"`
				ValidUntil *string `json:"valid_until"`
				WeekDay    *int    `json:"week_day"`
			} `json:"available_periods"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		id := r.ID.String()
		if id == "" || id == "0" {
			continue
		}
		rc := resourceCapacity{ID: id, Name: r.Name, MinutesPerDay: r.MinutesPerDay, Archived: r.Archived}
		for _, p := range r.AvailablePeriods {
			// Active set = valid_until null/empty (per the API contract).
			if p.ValidUntil != nil && *p.ValidUntil != "" {
				continue
			}
			if p.WeekDay == nil || *p.WeekDay < 0 || *p.WeekDay > 6 || p.StartTime == nil || p.EndTime == nil {
				continue
			}
			if mins := *p.EndTime - *p.StartTime; mins > 0 {
				rc.weekdayMinutes[*p.WeekDay] += mins
				rc.hasPeriods = true
			}
		}
		if _, seen := caps[id]; !seen {
			order = append(order, id)
		}
		caps[id] = rc
	}
	return caps, order, nil
}

// normalizeDate reduces a Resource Guru date (bare "2026-06-01" or an ISO
// datetime like "2026-06-01T00:00:00Z") to the YYYY-MM-DD key the grid uses,
// so per-day booked minutes can't silently fail to match the day range.
func normalizeDate(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// loadBookedMinutes reads synced bookings into resource_id -> date -> booked
// minutes. A booking with resource_ids attributes its per-day durations to each
// listed resource; otherwise it falls back to the single resource_id.
func loadBookedMinutes(s *store.Store) (map[string]map[string]int, error) {
	rows, err := s.List("bookings", 0)
	if err != nil {
		return nil, err
	}
	booked := make(map[string]map[string]int)
	for _, raw := range rows {
		var b struct {
			ResourceID  json.Number   `json:"resource_id"`
			ResourceIDs []json.Number `json:"resource_ids"`
			Durations   []struct {
				Date     string `json:"date"`
				Duration int    `json:"duration"`
			} `json:"durations"`
		}
		if err := json.Unmarshal(raw, &b); err != nil {
			continue
		}
		// Dedup resource ids so a booking that repeats an id (or lists it in
		// both resource_id and resource_ids) can't double-count its minutes.
		ids := make([]string, 0, len(b.ResourceIDs)+1)
		seen := make(map[string]bool, len(b.ResourceIDs)+1)
		add := func(id string) {
			if id != "" && id != "0" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		for _, x := range b.ResourceIDs {
			add(x.String())
		}
		if len(ids) == 0 {
			add(b.ResourceID.String())
		}
		for _, id := range ids {
			day := booked[id]
			if day == nil {
				day = make(map[string]int)
				booked[id] = day
			}
			for _, d := range b.Durations {
				if d.Date != "" {
					day[normalizeDate(d.Date)] += d.Duration
				}
			}
		}
	}
	return booked, nil
}

// dayCell is one resource-day: minutes booked, daily capacity, and utilization
// (nil when the resource has no declared capacity, so it can't be divided).
type dayCell struct {
	Date            string   `json:"date"`
	BookedMinutes   int      `json:"booked_minutes"`
	CapacityMinutes int      `json:"capacity_minutes"`
	Utilization     *float64 `json:"utilization"`
}

// resourceUtil is one resource's per-day grid plus window rollups.
type resourceUtil struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	MinutesPerDay   int       `json:"minutes_per_day"`
	BookedMinutes   int       `json:"booked_minutes"`
	CapacityMinutes int       `json:"capacity_minutes"`
	AvgUtilization  *float64  `json:"avg_utilization"`
	OverbookedDays  int       `json:"overbooked_days"`
	Days            []dayCell `json:"days"`
}

// dayRange returns the inclusive list of YYYY-MM-DD dates in [start, end].
func dayRange(start, end time.Time) []string {
	out := make([]string, 0)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format(utilDateFmt))
	}
	return out
}

// resolveWindow parses --start/--end, defaulting to today..today+27d (4 weeks)
// and capping the span so a grid can't run away.
func resolveWindow(startStr, endStr string) (time.Time, time.Time, error) {
	var start, end time.Time
	var err error
	if startStr == "" {
		start = time.Now().UTC().Truncate(24 * time.Hour)
	} else if start, err = time.Parse(utilDateFmt, startStr); err != nil {
		return start, end, fmt.Errorf("invalid --start %q: use YYYY-MM-DD", startStr)
	}
	if endStr == "" {
		end = start.AddDate(0, 0, 27)
	} else if end, err = time.Parse(utilDateFmt, endStr); err != nil {
		return start, end, fmt.Errorf("invalid --end %q: use YYYY-MM-DD", endStr)
	}
	if end.Before(start) {
		return start, end, fmt.Errorf("--end %s is before --start %s", end.Format(utilDateFmt), start.Format(utilDateFmt))
	}
	if days := int(end.Sub(start).Hours()/24) + 1; days > 366 {
		return start, end, fmt.Errorf("window too large (%d days); max 366", days)
	}
	return start, end, nil
}

// buildUtilization computes the per-resource per-day grid over [start, end].
// resourceFilter, when non-empty, limits output to that single resource id.
func buildUtilization(s *store.Store, start, end time.Time, resourceFilter string) ([]resourceUtil, error) {
	caps, order, err := loadResourceCapacities(s)
	if err != nil {
		return nil, err
	}
	booked, err := loadBookedMinutes(s)
	if err != nil {
		return nil, err
	}
	nDays := len(dayRange(start, end))
	out := make([]resourceUtil, 0, len(order))
	for _, id := range order {
		if resourceFilter != "" && id != resourceFilter {
			continue
		}
		rc := caps[id]
		ru := resourceUtil{ID: id, Name: rc.Name, MinutesPerDay: rc.MinutesPerDay, Days: make([]dayCell, 0, nDays)}
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			dateStr := d.Format(utilDateFmt)
			capMin := rc.capacityFor(d)
			b := booked[id][dateStr]
			cell := dayCell{Date: dateStr, BookedMinutes: b, CapacityMinutes: capMin}
			if capMin > 0 {
				u := float64(b) / float64(capMin)
				cell.Utilization = &u
				if b > capMin {
					ru.OverbookedDays++
				}
			}
			ru.BookedMinutes += b
			ru.CapacityMinutes += capMin
			ru.Days = append(ru.Days, cell)
		}
		if ru.CapacityMinutes > 0 {
			avg := float64(ru.BookedMinutes) / float64(ru.CapacityMinutes)
			ru.AvgUtilization = &avg
		}
		out = append(out, ru)
	}
	return out, nil
}

// openUtilStore opens the local store (creating+migrating if absent, so an
// un-synced run degrades to empty results instead of erroring).
func openUtilStore(cmd *cobra.Command, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("resourceguru-cli")
	}
	return store.OpenWithContext(cmd.Context(), dbPath)
}

// pct renders a utilization ratio as a percent string, or "-" when nil.
func pct(u *float64) string {
	if u == nil {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", *u*100)
}

// emptyHint warns once on stderr when the store has no resources, so a zeroed
// grid reads as "run sync" rather than "everyone is idle".
func emptyHint(cmd *cobra.Command, resourceCount int) {
	if resourceCount == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "note: no resources in the local store — run `resourceguru-cli sync` first")
	}
}
