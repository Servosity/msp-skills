// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated). Recorded in handfixes.json (fixed-scripts).

package winsrv

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// Section is one part of an inventory snapshot. Only Fields are kept, so
// volatile values (uptime, free memory, service state) never show up as a
// change.
type Section struct {
	Name   string
	Script string
	Args   []map[string]any // one run per entry; nil means one run without arguments
	Fields []string
}

// Sections are what `inventory snapshot` records and `inventory diff` compares.
var Sections = []Section{
	{Name: "system", Script: "system", Fields: []string{"computer_name", "domain", "os_caption", "os_version", "os_build", "total_memory_bytes"}},
	{Name: "features", Script: "features", Fields: []string{"display_name", "install_state"}},
	{Name: "shares", Script: "shares", Fields: []string{"path", "description", "encrypt_data"}},
	{Name: "local_admins", Script: "local_admins", Fields: []string{"name", "object_class"}},
	{Name: "certificates", Script: "certificates", Args: []map[string]any{{"store": "My"}, {"store": "WebHosting"}, {"store": "Remote Desktop"}}, Fields: []string{"subject", "not_after", "has_private_key"}},
	{Name: "installed_updates", Script: "installed_updates", Fields: []string{"description"}},
	{Name: "services", Script: "services", Fields: []string{"display_name", "start_mode", "account"}},
	{Name: "hyperv_vms", Script: "hyperv_vms", Fields: []string{"name", "generation", "version"}},
}

// Snapshot is one server's inventory: section -> row id -> kept fields.
// Sections that failed are named in Errors; sections that do not apply (the
// role is absent) are named in Absent.
type Snapshot struct {
	Server   string
	TakenAt  time.Time
	Sections map[string]map[string]map[string]any
	Errors   map[string]string
	Absent   []string
	FirstErr error // the first section failure, for the exit code
}

// TakeSnapshot reads every section from one server.
func TakeSnapshot(ctx context.Context, r Runner, p Profile, now time.Time) Snapshot {
	s := Snapshot{Server: p.Name, TakenAt: now, Sections: map[string]map[string]map[string]any{}, Errors: map[string]string{}}
	for i, sec := range Sections {
		runs := sec.Args
		if runs == nil {
			runs = []map[string]any{nil}
		}
		rows := map[string]map[string]any{}
		var failed error
		for _, args := range runs {
			got, err := Rows(ctx, r, p, sec.Script, args, now)
			if err != nil {
				failed = err
				break
			}
			for _, row := range got {
				kept, key := map[string]any{}, Str(row["id"])
				for _, f := range sec.Fields {
					kept[f] = row[f]
				}
				rows[key] = kept
			}
		}
		var se *ScriptError
		switch {
		case failed == nil:
			s.Sections[sec.Name] = rows
		case errors.As(failed, &se) && se.Kind == KindRoleAbsent:
			s.Absent = append(s.Absent, sec.Name)
			s.Sections[sec.Name] = map[string]map[string]any{}
		case i == 0 && serverDown(failed):
			// The server itself is down: every section fails the same way,
			// without one dial timeout per section.
			s.FirstErr = failed
			msg := failed.Error()
			for _, rest := range Sections {
				s.Errors[rest.Name] = msg
			}
			return s
		default:
			if s.FirstErr == nil {
				s.FirstErr = failed
			}
			s.Errors[sec.Name] = failed.Error()
		}
	}
	return s
}

// Change is one difference between two snapshots.
type Change struct {
	Section string                    `json:"section"`
	Change  string                    `json:"change"` // added, removed, changed
	Key     string                    `json:"key"`
	Fields  map[string]map[string]any `json:"fields,omitempty"`
}

// Diff compares two snapshots of one server. Sections missing from either
// side (a read failed) are returned in notCompared rather than reported as
// everything added or removed.
func Diff(old, cur map[string]map[string]map[string]any) (changes []Change, notCompared []string) {
	for _, sec := range Sections {
		a, okA := old[sec.Name]
		b, okB := cur[sec.Name]
		if !okA || !okB {
			notCompared = append(notCompared, sec.Name)
			continue
		}
		for key, row := range b {
			prev, ok := a[key]
			if !ok {
				changes = append(changes, Change{Section: sec.Name, Change: "added", Key: key, Fields: asFrom(nil, row)})
				continue
			}
			if d := fieldDiff(prev, row); len(d) > 0 {
				changes = append(changes, Change{Section: sec.Name, Change: "changed", Key: key, Fields: d})
			}
		}
		for key, row := range a {
			if _, ok := b[key]; !ok {
				changes = append(changes, Change{Section: sec.Name, Change: "removed", Key: key, Fields: asFrom(row, nil)})
			}
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Section != changes[j].Section {
			return changes[i].Section < changes[j].Section
		}
		return changes[i].Key < changes[j].Key
	})
	return changes, notCompared
}

func asFrom(from, to map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for k, v := range from {
		out[k] = map[string]any{"from": v, "to": nil}
	}
	for k, v := range to {
		out[k] = map[string]any{"from": nil, "to": v}
	}
	return out
}

func fieldDiff(a, b map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	for k := range keys {
		x, _ := json.Marshal(a[k])
		y, _ := json.Marshal(b[k])
		if string(x) != string(y) {
			out[k] = map[string]any{"from": a[k], "to": b[k]}
		}
	}
	return out
}

// SectionNames lists the snapshot sections, for help text.
func SectionNames() string {
	names := make([]string, len(Sections))
	for i, s := range Sections {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}
