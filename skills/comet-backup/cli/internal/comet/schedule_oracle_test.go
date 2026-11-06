// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package comet

import (
	"math/rand"
	"testing"
	"time"
)

// oracleNextRun checks every occurrence in order, with no jumps: the slow,
// obviously-correct reference for NextRun's jump logic.
func oracleNextRun(s Schedule, after, limit time.Time, loc *time.Location) (run time.Time, found, exhausted bool) {
	sp := int(s.SecondsPast)
	days := []bool{s.DaysSelect.Sunday, s.DaysSelect.Monday, s.DaysSelect.Tuesday, s.DaysSelect.Wednesday,
		s.DaysSelect.Thursday, s.DaysSelect.Friday, s.DaysSelect.Saturday}
	from := int(s.FromTime.Hour*60 + s.FromTime.Minutes)
	to := int(s.ToTime.Hour*60 + s.ToTime.Minutes)
	eligible := func(t time.Time) bool {
		lt := t.In(loc)
		if s.RestrictDays && !days[lt.Weekday()] {
			return false
		}
		if s.RestrictRuntime {
			tod := lt.Hour()*60 + lt.Minute()
			if from <= to && (tod < from || tod > to) {
				return false
			}
			if from > to && tod < from && tod > to {
				return false
			}
		}
		return true
	}
	la := after.In(loc)
	y, m, d := la.Date()
	for i := -2; i < 200000; i++ {
		var t time.Time
		switch s.FrequencyType {
		case freqHourly:
			t = time.Date(y, m, d, la.Hour()+i, (sp%3600)/60, sp%60, 0, loc)
		case freqDaily:
			t = time.Date(y, m, d+i, (sp/3600)%24, (sp%3600)/60, sp%60, 0, loc)
		case freqWeekly:
			t = time.Date(y, m, d-int(la.Weekday())+7*i+sp/86400, (sp%86400)/3600, (sp%3600)/60, sp%60, 0, loc)
		case freqMonthly:
			t = time.Date(y, m+time.Month(i), 1+sp/86400, (sp%86400)/3600, (sp%3600)/60, sp%60, 0, loc)
		case freqPeriodic:
			p := int64(s.SecondsPast)
			k := (after.Unix() - s.Offset) / p
			t = time.Unix(s.Offset+(k+int64(i))*p, 0)
		}
		if !t.After(after) {
			continue
		}
		if t.After(limit) {
			return time.Time{}, false, false
		}
		if eligible(t) {
			return t.Add(time.Duration(s.RandomDelaySecs) * time.Second), true, false
		}
	}
	return time.Time{}, false, true // the oracle ran out of budget: no verdict
}

func TestNextRunMatchesOracle(t *testing.T) {
	var zones []*time.Location
	for _, name := range []string{"UTC", "America/New_York", "America/Santiago", "Australia/Lord_Howe",
		"Asia/Kathmandu", "Pacific/Chatham", "Pacific/Auckland", "Asia/Kolkata", "Europe/London"} {
		l, err := time.LoadLocation(name)
		if err != nil {
			t.Skip("tzdata unavailable")
		}
		zones = append(zones, l)
	}
	// Anchor dates near 2026 DST transitions in both hemispheres.
	anchors := []string{"2026-03-08T00:00:00Z", "2026-03-29T00:00:00Z", "2026-04-05T00:00:00Z",
		"2026-09-06T00:00:00Z", "2026-09-27T00:00:00Z", "2026-10-04T00:00:00Z", "2026-10-25T00:00:00Z", "2026-11-01T00:00:00Z",
		// right after short months, including a leap February
		"2026-03-01T00:00:00Z", "2026-05-01T00:00:00Z", "2028-03-01T00:00:00Z", "2026-07-01T00:00:00Z"}
	rng := rand.New(rand.NewSource(20261015))
	freqs := []uint64{freqHourly, freqDaily, freqWeekly, freqMonthly, freqPeriodic}
	mismatches, skipped := 0, 0
	for n := 0; n < 20000; n++ {
		s := Schedule{FrequencyType: freqs[rng.Intn(len(freqs))]}
		switch s.FrequencyType {
		case freqHourly:
			s.SecondsPast = uint64(rng.Intn(3600))
		case freqDaily:
			s.SecondsPast = uint64(rng.Intn(86400))
		case freqWeekly:
			s.SecondsPast = uint64(rng.Intn(7 * 86400))
		case freqMonthly:
			s.SecondsPast = uint64(rng.Intn(31 * 86400))
		case freqPeriodic:
			s.SecondsPast = uint64([]int{1, 30, 60, 900, 1800, 3600, 5400, 7200, 72000, 86400}[rng.Intn(10)])
			s.Offset = int64(rng.Intn(86400))
		}
		if rng.Intn(2) == 0 {
			s.RestrictDays = true
			for !(s.DaysSelect.Sunday || s.DaysSelect.Monday || s.DaysSelect.Tuesday || s.DaysSelect.Wednesday ||
				s.DaysSelect.Thursday || s.DaysSelect.Friday || s.DaysSelect.Saturday) {
				s.DaysSelect.Sunday, s.DaysSelect.Monday = rng.Intn(3) == 0, rng.Intn(3) == 0
				s.DaysSelect.Tuesday, s.DaysSelect.Wednesday = rng.Intn(3) == 0, rng.Intn(3) == 0
				s.DaysSelect.Thursday, s.DaysSelect.Friday, s.DaysSelect.Saturday = rng.Intn(3) == 0, rng.Intn(3) == 0, rng.Intn(3) == 0
			}
		}
		if rng.Intn(2) == 0 {
			s.RestrictRuntime = true
			s.FromTime = HourMinute{Hour: uint64(rng.Intn(24)), Minutes: uint64(rng.Intn(4) * 15)}
			s.ToTime = HourMinute{Hour: uint64(rng.Intn(24)), Minutes: uint64(rng.Intn(4) * 15)}
			if rng.Intn(2) == 0 { // bias toward the hours DST transitions touch
				h := uint64(rng.Intn(4))
				s.FromTime = HourMinute{Hour: h, Minutes: uint64(rng.Intn(4) * 15)}
				s.ToTime = HourMinute{Hour: h + uint64(rng.Intn(2)), Minutes: uint64(rng.Intn(4) * 15)}
			}
		}
		if rng.Intn(4) == 0 {
			s.RandomDelaySecs = uint64(rng.Intn(18000))
		}
		loc := zones[rng.Intn(len(zones))]
		anchor, _ := time.Parse(time.RFC3339, anchors[rng.Intn(len(anchors))])
		after := anchor.Add(time.Duration(rng.Intn(4*86400)-2*86400) * time.Second)
		limit := after.Add(120 * 24 * time.Hour)
		got, ok, _ := NextRun(s, after, limit, loc)
		want, wok, exhausted := oracleNextRun(s, after, limit, loc)
		if exhausted {
			skipped++
			continue
		}
		if ok != wok || (ok && !got.Equal(want)) {
			mismatches++
			if mismatches <= 5 {
				t.Errorf("mismatch %+v zone=%s after=%s: got %v/%v want %v/%v", s, loc, after.Format(time.RFC3339), got.UTC().Format(time.RFC3339), ok, want.UTC().Format(time.RFC3339), wok)
			}
		}
	}
	if skipped > 2000 {
		t.Fatalf("oracle gave no verdict on %d cases; the sample is not testing enough", skipped)
	}
	t.Logf("%d cases compared, %d skipped (oracle budget)", 20000-skipped, skipped)
	if mismatches > 0 {
		t.Fatalf("%d of 20000 randomized schedules disagree with the oracle", mismatches)
	}
}
