// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// keeper handfix posture-report (local reads): the cross-company user views
// read each company's newest good posture run, never a failed one, and say
// which run each row came from.

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"keeper-pp-cli/internal/keeper"
)

type companySnapshot struct {
	CompanyID   string
	CompanyName string
	RunID       int64
	TakenAt     string
	Users       map[string]keeper.UserFacts
}

// latestSnapshots loads each company's newest good run (only mc when set).
// A store with no posture run yet is an error that says how to make one.
func latestSnapshots(ctx context.Context, mc string) ([]companySnapshot, error) {
	db, err := openStoreForRead(ctx, "keeper-cli")
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, notFoundErr(fmt.Errorf("no posture run stored yet: run 'keeper-cli posture report' first"))
	}
	defer db.Close()
	rows, taken, err := db.LatestGoodPerCompany(ctx)
	if err != nil {
		return nil, err
	}
	var out []companySnapshot
	for _, r := range rows {
		if mc != "" && r.CompanyID != mc {
			continue
		}
		users, err := loadUserFacts(ctx, db, r.RunID, r.CompanyID)
		if err != nil {
			return nil, err
		}
		out = append(out, companySnapshot{CompanyID: r.CompanyID, CompanyName: r.CompanyName, RunID: r.RunID, TakenAt: taken[r.RunID], Users: users})
	}
	if len(out) == 0 {
		if mc != "" {
			return nil, notFoundErr(fmt.Errorf("no good posture run stored for managed company %s: run 'keeper-cli posture report --mc %s'", mc, mc))
		}
		return nil, notFoundErr(fmt.Errorf("no good posture run stored yet: run 'keeper-cli posture report' first"))
	}
	return out, nil
}

// firstSeenFor loads first-seen times for one company (nil on a missing store).
func firstSeenFor(ctx context.Context, companyID string) (map[string]string, error) {
	db, err := openStoreForRead(ctx, "keeper-cli")
	if err != nil || db == nil {
		return map[string]string{}, err
	}
	defer db.Close()
	return db.FirstSeen(ctx, companyID)
}

func sortedEmails(m map[string]keeper.UserFacts) []string {
	out := make([]string, 0, len(m))
	for e := range m {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

func intOrNil(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// hintStalePosture tells stderr when a company's newest good run is older
// than --max-age (0 turns the hint off); stdout stays machine-clean.
func hintStalePosture(w io.Writer, flags *rootFlags, snaps []companySnapshot) {
	if flags == nil || flags.maxAge <= 0 {
		return
	}
	old := 0
	for _, s := range snaps {
		if t, err := time.Parse(time.RFC3339Nano, s.TakenAt); err == nil && time.Since(t) > flags.maxAge {
			old++
		}
	}
	if old > 0 {
		fmt.Fprintf(w, "note: posture data for %d compan%s is older than %s; run 'keeper-cli posture report' to refresh\n", old, map[bool]string{true: "y", false: "ies"}[old == 1], flags.maxAge)
	}
}
