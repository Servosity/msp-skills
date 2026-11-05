// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "agent-compaction-keeps-report-fields" entry.

package cli

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"synology-pp-cli/internal/cliutil"
)

// printReportJSON writes a fleet/abb report (a slice of row structs, or one
// report struct with nested rows) through the generated JSON printer, keeping
// every field the report's own Go types declare when --agent/--compact
// compacts it.
//
// The generated compaction keeps a row key only when it is on the gravity
// allow-list or present on at least 80% of rows. The report rows mark their
// reconciliation fields omitempty (error and warnings on a fleet sync row,
// sync_error on a fleet status row, running and first_error on a failure,
// error_code and params on a triage timeline entry), so the one NAS that
// failed among five lost its error under --agent and read as an unexplained
// ok:false. These rows are already curated summaries; a field the report
// declares is never noise.
func printReportJSON(w io.Writer, v any, flags *rootFlags) error {
	if flags != nil && flags.quiet && flags.selectFields == "" {
		// --quiet prints one identity per row (the NAS, task or host the row
		// is about), declared by each report type; the generic quiet printer
		// picked the first sorted scalar and printed "true".
		if ids, ok := reportIdentities(v); ok {
			for _, id := range ids {
				if _, err := fmt.Fprintln(w, cliutil.ScrubTerminal(id)); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return printJSONFilteredKeep(w, v, flags, reportKeepFields(v)...)
}

// reportIdentity is implemented by every fleet/abb report row type: the one
// value --quiet prints for the row.
type reportIdentity interface{ ReportIdentity() string }

// reportIdentities returns each row's identity when v is a report row, or a
// slice of rows, whose type declares one.
func reportIdentities(v any) ([]string, bool) {
	if r, ok := v.(reportIdentity); ok {
		return []string{r.ReportIdentity()}, true
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() || (rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array) {
		return nil, false
	}
	if !rv.Type().Elem().Implements(reflect.TypeOf((*reportIdentity)(nil)).Elem()) {
		return nil, false
	}
	out := make([]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		if id := rv.Index(i).Interface().(reportIdentity).ReportIdentity(); id != "" {
			out = append(out, id)
		}
	}
	return out, true
}

// reportKeepFields returns the JSON names of every field declared by the
// struct types reachable from v (through pointers, slices, arrays and map
// values), sorted for deterministic output.
func reportKeepFields(v any) []string {
	seen := map[reflect.Type]bool{}
	names := map[string]bool{}
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map) {
			t = t.Elem()
		}
		if t == nil || t.Kind() != reflect.Struct || seen[t] {
			return
		}
		seen[t] = true
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			names[name] = true
			walk(f.Type)
		}
	}
	if v != nil {
		walk(reflect.TypeOf(v))
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
