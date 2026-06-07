// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package ledger

import "sort"

// DriftChange is one account x service whose unit count moved between the two
// most recent push runs.
type DriftChange struct {
	ServiceID string  `json:"service_id"`
	AccountID string  `json:"account_id"`
	Old       float64 `json:"old"`
	New       float64 `json:"new"`
	Delta     float64 `json:"delta"`
}

// DriftEntry is an account x service present in only one of the two runs.
type DriftEntry struct {
	ServiceID string  `json:"service_id"`
	AccountID string  `json:"account_id"`
	UnitCount float64 `json:"unit_count"`
}

// DriftReport compares the two most recent push runs.
type DriftReport struct {
	BaselineRun string        `json:"baseline_run"`
	CurrentRun  string        `json:"current_run"`
	Changes     []DriftChange `json:"changes"`
	Added       []DriftEntry  `json:"added"`
	Removed     []DriftEntry  `json:"removed"`
	Unchanged   int           `json:"unchanged"`
	Note        string        `json:"note,omitempty"`
}

type pairKey struct{ service, account string }

// ComputeDrift joins the two most recent runs (by first-seen file order of
// run_id) per account x service and reports only successful ("sent") rows.
// With fewer than two runs it returns a report with an explanatory Note.
func ComputeDrift(pushes []PushRecord) DriftReport {
	runOrder := []string{}
	seen := map[string]bool{}
	for _, p := range pushes {
		if !seen[p.RunID] {
			seen[p.RunID] = true
			runOrder = append(runOrder, p.RunID)
		}
	}
	report := DriftReport{Changes: []DriftChange{}, Added: []DriftEntry{}, Removed: []DriftEntry{}}
	if len(runOrder) == 0 {
		report.Note = "push ledger is empty; run 'usage push' at least twice to compute drift"
		return report
	}
	if len(runOrder) == 1 {
		report.CurrentRun = runOrder[0]
		report.Note = "only one push run recorded; drift needs two runs to compare"
		return report
	}
	report.BaselineRun = runOrder[len(runOrder)-2]
	report.CurrentRun = runOrder[len(runOrder)-1]

	counts := func(runID string) map[pairKey]float64 {
		m := map[pairKey]float64{}
		for _, p := range pushes {
			if p.RunID == runID && p.Status == "sent" {
				m[pairKey{p.ServiceID, p.AccountID}] = p.UnitCount // last write per pair wins
			}
		}
		return m
	}
	base := counts(report.BaselineRun)
	cur := counts(report.CurrentRun)

	for k, newCount := range cur {
		if oldCount, ok := base[k]; ok {
			if oldCount != newCount {
				report.Changes = append(report.Changes, DriftChange{
					ServiceID: k.service, AccountID: k.account,
					Old: oldCount, New: newCount, Delta: newCount - oldCount,
				})
			} else {
				report.Unchanged++
			}
		} else {
			report.Added = append(report.Added, DriftEntry{ServiceID: k.service, AccountID: k.account, UnitCount: newCount})
		}
	}
	for k, oldCount := range base {
		if _, ok := cur[k]; !ok {
			report.Removed = append(report.Removed, DriftEntry{ServiceID: k.service, AccountID: k.account, UnitCount: oldCount})
		}
	}

	sort.Slice(report.Changes, func(i, j int) bool {
		if report.Changes[i].ServiceID != report.Changes[j].ServiceID {
			return report.Changes[i].ServiceID < report.Changes[j].ServiceID
		}
		return report.Changes[i].AccountID < report.Changes[j].AccountID
	})
	sort.Slice(report.Added, func(i, j int) bool { return lessEntry(report.Added[i], report.Added[j]) })
	sort.Slice(report.Removed, func(i, j int) bool { return lessEntry(report.Removed[i], report.Removed[j]) })
	return report
}

func lessEntry(a, b DriftEntry) bool {
	if a.ServiceID != b.ServiceID {
		return a.ServiceID < b.ServiceID
	}
	return a.AccountID < b.AccountID
}
