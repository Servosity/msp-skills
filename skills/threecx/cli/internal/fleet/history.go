// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: local fleet snapshot history and the change feed built on it.

package fleet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"threecx-pp-cli/internal/cliutil"
)

// keepRuns bounds the history; the oldest runs are pruned on save.
const keepRuns = 400

// Snapshot kinds.
const (
	KindHealth = "health"
	KindTrunks = "trunks"
)

// Snapshot is one saved fleet run. Scope lists every PBX the run queried
// and Full says whether that was the whole registry, so a later --only run
// is never mistaken for PBXes being removed.
type Snapshot struct {
	RunAt  time.Time   `json:"run_at"`
	Kind   string      `json:"kind"` // KindHealth or KindTrunks
	Scope  []string    `json:"scope"`
	Full   bool        `json:"full"`
	Health []HealthRow `json:"health,omitempty"`
	Trunks []TrunkRow  `json:"trunks,omitempty"`
}

// HistoryDir holds one JSON file per saved run, next to the fleet file.
// A save writes only its own file, so history never has to be rewritten.
func HistoryDir() (string, error) {
	dir, err := BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fleet-history"), nil
}

// LoadHistory reads every saved run, oldest first. A missing directory is
// an empty history; an unreadable run is an error, never silently skipped.
func LoadHistory() ([]Snapshot, error) {
	dir, err := HistoryDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Snapshot{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading fleet history %s: %w", dir, err)
	}
	out := make([]Snapshot, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- app-owned path
		if errors.Is(err, os.ErrNotExist) {
			continue // pruned by a concurrent save after the listing
		}
		if err != nil {
			return nil, fmt.Errorf("reading fleet history %s: %w", path, err)
		}
		var s Snapshot
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("parsing fleet history %s: %w", path, err)
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].RunAt.Before(out[j].RunAt) })
	return out, nil
}

// SaveSnapshot writes one run atomically and prunes the oldest beyond keepRuns.
func SaveSnapshot(s Snapshot) error {
	dir, err := HistoryDir()
	if err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	// Zero-padded nanoseconds sort lexically in time order.
	name := fmt.Sprintf("%020d-%s.json", s.RunAt.UnixNano(), s.Kind)
	if err := cliutil.AtomicWritePrivateFile(filepath.Join(dir, name), data, 0o600, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var runs []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			runs = append(runs, e.Name())
		}
	}
	sort.Strings(runs)
	for len(runs) > keepRuns {
		if err := os.Remove(filepath.Join(dir, runs[0])); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		runs = runs[1:]
	}
	return nil
}

// Change is one transition between two fleet runs.
type Change struct {
	PBX     string `json:"pbx"`
	Subject string `json:"subject"`
	Change  string `json:"change"`
	From    string `json:"from"`
	To      string `json:"to"`
}

// PickRuns returns the latest run of kind and the latest run at or before
// cutoff (or the oldest run when none is that old). When pbx is set, only
// runs that covered that PBX count. ok is false when fewer than two such
// runs exist.
func PickRuns(hist []Snapshot, kind string, cutoff time.Time, pbx string) (from, to Snapshot, ok bool) {
	var runs []Snapshot
	for _, s := range hist {
		if s.Kind == kind && (pbx == "" || scopeSet(s)[pbx]) {
			runs = append(runs, s)
		}
	}
	if len(runs) < 2 {
		return Snapshot{}, Snapshot{}, false
	}
	to = runs[len(runs)-1]
	from = runs[0]
	for _, s := range runs[:len(runs)-1] {
		if !s.RunAt.After(cutoff) {
			from = s
		}
	}
	return from, to, true
}

func scopeSet(s Snapshot) map[string]bool {
	m := make(map[string]bool, len(s.Scope))
	for _, n := range s.Scope {
		m[n] = true
	}
	if len(s.Scope) == 0 { // snapshot written before scope was recorded
		for _, r := range s.Health {
			m[r.PBX] = true
		}
		for _, r := range s.Trunks {
			m[r.PBX] = true
		}
	}
	return m
}

// DiffHealth lists per-PBX transitions between two health runs. Only PBXes
// queried by both runs are compared; added/removed is reported only when
// both runs covered the whole registry.
func DiffHealth(from, to Snapshot) []Change {
	out := []Change{}
	inFrom, inTo := scopeSet(from), scopeSet(to)
	full := from.Full && to.Full
	prev := map[string]HealthRow{}
	for _, r := range from.Health {
		prev[r.PBX] = r
	}
	for _, cur := range to.Health {
		old, ok := prev[cur.PBX]
		if !ok || !inFrom[cur.PBX] {
			if full && !inFrom[cur.PBX] {
				out = append(out, Change{PBX: cur.PBX, Subject: "pbx", Change: "added", To: string(cur.Status)})
			}
			continue
		}
		if old.Status != Error && cur.Status == Error {
			out = append(out, Change{PBX: cur.PBX, Subject: "pbx", Change: "unreachable", From: string(old.Status), To: cur.Error})
			continue
		}
		if old.Status == Error && cur.Status != Error {
			out = append(out, Change{PBX: cur.PBX, Subject: "pbx", Change: "reachable_again", From: "error", To: string(cur.Status)})
		}
		if old.Status != Error && cur.Status != Error {
			if old.Version != cur.Version && cur.Version != "" && old.Version != "" {
				out = append(out, Change{PBX: cur.PBX, Subject: "version", Change: "version_changed", From: old.Version, To: cur.Version})
			}
			if later(old.LicenseExpires, cur.LicenseExpires) {
				out = append(out, Change{PBX: cur.PBX, Subject: "license", Change: "license_renewed", From: old.LicenseExpires, To: cur.LicenseExpires})
			}
			if later(old.MaintenanceExpires, cur.MaintenanceExpires) {
				out = append(out, Change{PBX: cur.PBX, Subject: "maintenance", Change: "maintenance_renewed", From: old.MaintenanceExpires, To: cur.MaintenanceExpires})
			}
			out = append(out, checkChanges(cur.PBX, old.Checks, cur.Checks)...)
		}
	}
	if full {
		for _, old := range from.Health {
			if !inTo[old.PBX] {
				out = append(out, Change{PBX: old.PBX, Subject: "pbx", Change: "removed", From: string(old.Status)})
			}
		}
	}
	return out
}

// checkChanges reports each check whose level moved (backup went stale,
// trunks recovered, disk filled, ...). A check missing on either side (an
// older snapshot) is not compared. A move into unknown (the PBX stopped
// reporting what the check needs) is <check>_unknown and a move out of it
// <check>_reported: an unreported check is never a recovery.
func checkChanges(pbx string, from, to []Check) []Change {
	prev := map[string]Check{}
	for _, c := range from {
		prev[c.Name] = c
	}
	var out []Change
	for _, now := range to {
		old, ok := prev[now.Name]
		if !ok || old.Level == now.Level {
			continue
		}
		verb := "worsened"
		switch {
		case now.Level == Unknown:
			verb = "unknown"
		case old.Level == Unknown:
			verb = "reported"
		case Worse(old.Level, now.Level):
			verb = "improved"
		}
		out = append(out, Change{PBX: pbx, Subject: now.Name, Change: now.Name + "_" + verb,
			From: string(old.Level) + ": " + old.Detail, To: string(now.Level) + ": " + now.Detail})
	}
	return out
}

// DiffTrunks lists trunks that went offline, came back, appeared, or
// vanished, for PBXes read successfully by both runs.
func DiffTrunks(from, to Snapshot) []Change {
	out := []Change{}
	readable := func(s Snapshot) map[string]bool {
		ok := scopeSet(s)
		for _, r := range s.Trunks {
			if r.Error != "" {
				delete(ok, r.PBX)
			}
		}
		return ok
	}
	okFrom, okTo := readable(from), readable(to)
	both := func(pbx string) bool { return okFrom[pbx] && okTo[pbx] }
	key := func(r TrunkRow) string { return r.PBX + "\x00" + r.Trunk }
	prev := map[string]TrunkRow{}
	for _, r := range from.Trunks {
		if r.Error == "" {
			prev[key(r)] = r
		}
	}
	seen := map[string]bool{}
	for _, r := range to.Trunks {
		if r.Error != "" || !both(r.PBX) {
			continue
		}
		seen[key(r)] = true
		old, ok := prev[key(r)]
		subject := "trunk " + r.Trunk
		if !ok {
			out = append(out, Change{PBX: r.PBX, Subject: subject, Change: "trunk_added", To: onlineWord(r)})
			continue
		}
		// Registration and configuration issues move independently.
		if reg, oldReg := registration(r), registration(old); reg != oldReg {
			out = append(out, Change{PBX: r.PBX, Subject: subject, Change: "trunk_" + reg, From: oldReg, To: reg})
		}
		// An issue the phone system stopped reporting is not an issue cleared.
		if old.ConfigurationIssue != r.ConfigurationIssue && !r.IssueUnknown {
			change := "trunk_issue"
			if r.ConfigurationIssue == "" {
				change = "trunk_issue_cleared"
			}
			out = append(out, Change{PBX: r.PBX, Subject: subject, Change: change, From: old.ConfigurationIssue, To: r.ConfigurationIssue})
		}
	}
	for k, old := range prev {
		if !seen[k] && both(old.PBX) {
			out = append(out, Change{PBX: old.PBX, Subject: "trunk " + old.Trunk, Change: "trunk_removed", From: onlineWord(old)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PBX != out[j].PBX {
			return out[i].PBX < out[j].PBX
		}
		return out[i].Subject < out[j].Subject
	})
	return out
}

func onlineWord(r TrunkRow) string {
	if r.ConfigurationIssue != "" {
		return "issue: " + r.ConfigurationIssue
	}
	return registration(r)
}

// later reports a renewal: the absolute expiry date moved later.
func later(from, to string) bool {
	a, okA := ParseTime(from)
	b, okB := ParseTime(to)
	return okA && okB && b.After(a)
}
