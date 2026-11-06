// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json export-walk-integrity).

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

// jamfExportWalk keeps the totalCount every Jamf Pro page reports, so export
// can tell a complete walk from one that stopped short, the check sync, list
// --all and fleet exec already make.
type jamfExportWalk struct {
	total                   int
	pages                   int
	seen, changed, repeated bool
	ids                     map[string]bool
}

// page reads one page: the id of every record (a repeated id means the walk
// read a record twice, so its count can hide one it missed) and the page's
// totalCount. The empty page that ends a walk carries no records, so its total
// changes nothing that was read; the first page's total always counts.
func (w *jamfExportWalk) page(data json.RawMessage, items []json.RawMessage) {
	w.pages++
	for _, item := range items {
		var rec struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(item, &rec) != nil || len(rec.ID) == 0 || string(rec.ID) == "null" {
			continue
		}
		id := strings.Trim(string(rec.ID), `"`)
		if w.ids == nil {
			w.ids = map[string]bool{}
		}
		if w.ids[id] {
			w.repeated = true
		}
		w.ids[id] = true
	}
	if len(items) == 0 && w.pages > 1 {
		return
	}
	var probe struct {
		TotalCount *int `json:"totalCount"`
	}
	if json.Unmarshal(data, &probe) != nil || probe.TotalCount == nil {
		return
	}
	n := *probe.TotalCount
	if w.seen && n != w.total {
		w.changed = true
	}
	if !w.seen || n > w.total {
		w.total = n
	}
	w.seen = true
}

// check fails a walk that read a different number of records than the server
// reported, or saw the total change. A walk stopped by --limit is partial by
// design and passes.
func (w *jamfExportWalk) check(resource string, count int, capped bool) error {
	if capped || (!w.repeated && (!w.seen || (!w.changed && count == w.total))) {
		return nil
	}
	return fmt.Errorf("export %s: incomplete walk: read %d records of a reported %d (total changed: %t, record repeated: %t); the collection changed while it was read, run export again", resource, count, w.total, w.changed, w.repeated)
}
