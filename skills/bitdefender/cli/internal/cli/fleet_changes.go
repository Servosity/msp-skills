// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type fleetChange struct {
	Kind        string `json:"kind"`
	CompanyID   string `json:"company_id"`
	CompanyName string `json:"company_name"`
	EndpointID  string `json:"endpoint_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

type fleetChangesView struct {
	From             *snapRun       `json:"from"`
	To               *snapRun       `json:"to"`
	Complete         bool           `json:"complete"`
	SkippedCompanies []string       `json:"skipped_companies"`
	Changes          []fleetChange  `json:"changes"`
	Summary          map[string]int `json:"summary"`
	Note             string         `json:"note,omitempty"`
}

func newNovelFleetChangesCmd(flags *rootFlags) *cobra.Command {
	var since string
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Diff saved fleet health snapshots (needs two): new, removed, newly unmanaged, outdated or isolated endpoints",
		Long: `Diffs the latest 'fleet health' snapshot against the newest snapshot taken at or before
--since ago (or the oldest one, if none is that old). Reports endpoints added or removed,
re-registrations (an endpoint removed and another added with the same MAC address, which
GravityZone does after clones and auto-cleanup), managed/unmanaged flips, newly outdated
agents, policies no longer applied, isolation on or off, and companies added or removed.
Companies whose fetch failed in either snapshot are skipped, never reported as removals.
Reads only the local snapshot store: run 'fleet health' at least twice first.

Use this command to see what changed in endpoint protection state between saved fleet
snapshots (new, removed, newly unmanaged, outdated or isolated). Do NOT use this command for
current status; use 'fleet health' instead.`,
		Example:     "  bitdefender-cli fleet changes --since 7d --agent\n  bitdefender-cli fleet changes --since 24h --select changes.kind,changes.company_name,changes.name",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet changes")
			}
			span, err := parseSpan(since)
			if err != nil {
				return fleetUsageErr(cmd, flags, err)
			}
			view := fleetChangesView{Changes: make([]fleetChange, 0), Summary: map[string]int{}, SkippedCompanies: make([]string, 0)}
			if _, statErr := os.Stat(defaultDBPath("bitdefender-cli")); os.IsNotExist(statErr) {
				view.Note = "no fleet snapshots yet; run 'bitdefender-cli fleet health' twice"
				return emitChanges(cmd, flags, view)
			}
			ctx := cmd.Context()
			s, err := openGZStore(ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			runs, err := listRuns(ctx, s.DB())
			if err != nil {
				return err
			}
			if len(runs) < 2 {
				view.Note = "no fleet snapshots yet; run 'bitdefender-cli fleet health' twice (for example a week apart)"
				if len(runs) == 1 {
					view.Note = "1 fleet snapshot saved; run 'bitdefender-cli fleet health' once more to have two to compare"
				}
				return emitChanges(cmd, flags, view)
			}
			to := runs[0]
			from := runs[len(runs)-1]
			cutoff := to.TakenAt.Add(-span)
			for _, r := range runs[1:] {
				if !r.TakenAt.After(cutoff) {
					from = r
					break
				}
			}
			if from.TakenAt.After(cutoff) {
				view.Note = fmt.Sprintf("no snapshot is %s older than the latest; compared with the oldest one (%s)", since, from.TakenAt.Format(time.RFC3339))
			}
			// Drain each result set before the next query (single SQLite connection).
			oldEPs, err := loadRunEndpoints(ctx, s.DB(), from.RunID)
			if err != nil {
				return err
			}
			newEPs, err := loadRunEndpoints(ctx, s.DB(), to.RunID)
			if err != nil {
				return err
			}
			oldCos, oldOK, err := loadRunCompanies(ctx, s.DB(), from.RunID)
			if err != nil {
				return err
			}
			newCos, newOK, err := loadRunCompanies(ctx, s.DB(), to.RunID)
			if err != nil {
				return err
			}
			view.From, view.To = &from, &to
			view.Changes = diffFleet(oldEPs, newEPs, oldCos, oldOK, newCos, newOK, from.TreeComplete && to.TreeComplete)
			if !(from.TreeComplete && to.TreeComplete) {
				view.Note = strings.TrimSpace(view.Note + " company additions/removals not reported: a compared snapshot had an incomplete company list.")
			}
			// Endpoint changes are only compared for companies read in both
			// snapshots; one whose fetch failed in either is skipped.
			skipped := map[string]string{}
			for id, co := range oldCos {
				if !oldOK[id] {
					skipped[id] = co.Name
				}
			}
			for id, co := range newCos {
				if !newOK[id] {
					skipped[id] = co.Name
				}
			}
			for _, name := range skipped {
				view.SkippedCompanies = append(view.SkippedCompanies, name)
			}
			sort.Strings(view.SkippedCompanies)
			view.Complete = from.TreeComplete && to.TreeComplete && len(view.SkippedCompanies) == 0
			if len(view.SkippedCompanies) > 0 {
				view.Note = strings.TrimSpace(view.Note + fmt.Sprintf(" %d %s not compared (their endpoint fetch failed in a snapshot): %s.", len(view.SkippedCompanies), plural(len(view.SkippedCompanies), "company", "companies"), strings.Join(view.SkippedCompanies, ", ")))
			}
			for _, ch := range view.Changes {
				view.Summary[ch.Kind]++
			}
			return emitChanges(cmd, flags, view)
		},
	}
	cmd.Flags().StringVar(&since, "since", "7d", "Compare against the snapshot at least this old (e.g. 24h, 7d, 2w)")
	return cmd
}

// diffFleet compares two snapshots. Endpoint-level changes are computed only for
// companies fetched successfully in both runs.
// Company additions/removals are only inferred when both snapshots walked the
// complete company tree.
func diffFleet(oldEPs, newEPs []snapEndpoint, oldCos map[string]idName, oldOK map[string]bool, newCos map[string]idName, newOK map[string]bool, treesComplete bool) []fleetChange {
	changes := make([]fleetChange, 0)
	if treesComplete {
		for id, co := range newCos {
			if _, ok := oldCos[id]; !ok {
				changes = append(changes, fleetChange{Kind: "company_added", CompanyID: id, CompanyName: co.Name})
			}
		}
		for id, co := range oldCos {
			if _, ok := newCos[id]; !ok {
				changes = append(changes, fleetChange{Kind: "company_removed", CompanyID: id, CompanyName: co.Name})
			}
		}
	}
	comparable := func(companyID string) bool { return oldOK[companyID] && newOK[companyID] }
	key := func(e snapEndpoint) string { return e.CompanyID + "/" + e.EndpointID }
	oldBy := map[string]snapEndpoint{}
	for _, e := range oldEPs {
		if comparable(e.CompanyID) {
			oldBy[key(e)] = e
		}
	}
	newBy := map[string]snapEndpoint{}
	for _, e := range newEPs {
		if comparable(e.CompanyID) {
			newBy[key(e)] = e
		}
	}
	// The deepest successfully read company owns a shared endpoint, so when a
	// child company is skipped its endpoints surface under the parent (and move
	// back on recovery). An id held by a skipped company on the other side moved
	// attribution; it did not join or leave the fleet.
	skippedIDs := func(eps []snapEndpoint) map[string]bool {
		out := map[string]bool{}
		for _, e := range eps {
			if !comparable(e.CompanyID) {
				out[e.EndpointID] = true
			}
		}
		return out
	}
	oldSkipped, newSkipped := skippedIDs(oldEPs), skippedIDs(newEPs)
	var added, removed []snapEndpoint
	for k, n := range newBy {
		o, ok := oldBy[k]
		if !ok {
			if !oldSkipped[n.EndpointID] {
				added = append(added, n)
			}
			continue
		}
		ch := func(kind, detail string) {
			changes = append(changes, fleetChange{Kind: kind, CompanyID: n.CompanyID, CompanyName: n.CompanyName, EndpointID: n.EndpointID, Name: n.Name, Detail: detail})
		}
		if o.IsManaged && !n.IsManaged {
			ch("became_unmanaged", "")
		}
		if !o.IsManaged && n.IsManaged {
			ch("became_managed", "")
		}
		if !o.Outdated && n.Outdated {
			ch("became_outdated", "")
		}
		if (o.PolicyApplied == nil || *o.PolicyApplied) && n.PolicyApplied != nil && !*n.PolicyApplied {
			ch("policy_not_applied", n.PolicyName)
		}
		if o.PolicyName != "" && n.PolicyName != "" && o.PolicyName != n.PolicyName {
			ch("policy_changed", o.PolicyName+" -> "+n.PolicyName)
		}
		if !o.IsIsolated && n.IsIsolated {
			ch("isolated", "")
		}
		if o.IsIsolated && !n.IsIsolated {
			ch("released_from_isolation", "")
		}
	}
	for k, o := range oldBy {
		if _, ok := newBy[k]; !ok && !newSkipped[o.EndpointID] {
			removed = append(removed, o)
		}
	}
	// A removed record and an added record sharing an identifying MAC are one
	// machine re-registered only when exactly one endpoint carries that MAC in
	// each whole snapshot (an unchanged endpoint with it proves it is shared).
	macOwners := func(eps []snapEndpoint) map[string]int {
		count := map[string]int{}
		for _, e := range eps {
			own := map[string]bool{}
			for _, m := range e.MACs {
				if nm := normalizeMAC(m); identifyingMAC(nm) && !own[nm] {
					own[nm] = true
					count[nm]++
				}
			}
		}
		return count
	}
	macIndex := func(eps []snapEndpoint, owners map[string]int) map[string]int {
		idx := map[string]int{}
		for i, e := range eps {
			for _, m := range e.MACs {
				if nm := normalizeMAC(m); identifyingMAC(nm) && owners[nm] == 1 {
					idx[nm] = i
				}
			}
		}
		return idx
	}
	removedByMAC, addedByMAC := macIndex(removed, macOwners(oldEPs)), macIndex(added, macOwners(newEPs))
	// How many distinct added records each removed record's MACs reach.
	reach := map[int]map[int]bool{}
	for m, ri := range removedByMAC {
		if ai, ok := addedByMAC[m]; ok {
			if reach[ri] == nil {
				reach[ri] = map[int]bool{}
			}
			reach[ri][ai] = true
		}
	}
	paired := map[int]bool{}
	for ai, a := range added {
		match, conflict := -1, false
		for _, m := range a.MACs {
			nm := normalizeMAC(m)
			j, inAdded := addedByMAC[nm]
			i, ok := removedByMAC[nm]
			if !ok || !inAdded || j != ai {
				continue
			}
			if match >= 0 && match != i {
				conflict = true // NICs point at different removed records: do not guess
			}
			match = i
		}
		if conflict || (match >= 0 && (paired[match] || len(reach[match]) != 1)) {
			match = -1
		}
		if match >= 0 {
			paired[match] = true
			r := removed[match]
			changes = append(changes, fleetChange{Kind: "re_registered", CompanyID: a.CompanyID, CompanyName: a.CompanyName, EndpointID: a.EndpointID, Name: a.Name,
				Detail: "was " + r.EndpointID + " in " + r.CompanyName})
			continue
		}
		changes = append(changes, fleetChange{Kind: "endpoint_added", CompanyID: a.CompanyID, CompanyName: a.CompanyName, EndpointID: a.EndpointID, Name: a.Name})
	}
	for i, r := range removed {
		if paired[i] {
			continue
		}
		changes = append(changes, fleetChange{Kind: "endpoint_removed", CompanyID: r.CompanyID, CompanyName: r.CompanyName, EndpointID: r.EndpointID, Name: r.Name})
	}
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].CompanyName != changes[j].CompanyName {
			return changes[i].CompanyName < changes[j].CompanyName
		}
		if changes[i].Kind != changes[j].Kind {
			return changes[i].Kind < changes[j].Kind
		}
		return changes[i].Name < changes[j].Name
	})
	return changes
}

func emitChanges(cmd *cobra.Command, flags *rootFlags, view fleetChangesView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), view, flags)
	}
	humanNote(cmd, view.Note)
	if view.From == nil {
		return nil // fewer than two snapshots: the note says what to run
	}
	if len(view.Changes) == 0 {
		if view.Complete {
			fmt.Fprintln(cmd.OutOrStdout(), "No changes between the compared snapshots.")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "No changes found in the parts both snapshots cover (see the note above).")
		}
		return nil
	}
	rows := make([]map[string]any, 0, len(view.Changes))
	for _, ch := range view.Changes {
		rows = append(rows, map[string]any{"company": ch.CompanyName, "kind": ch.Kind, "endpoint": ch.Name, "detail": ch.Detail})
	}
	return printAutoTable(cmd.OutOrStdout(), rows)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
